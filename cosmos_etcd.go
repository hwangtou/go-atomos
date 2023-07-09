package go_atomos

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"strings"
)

const (
	etcdDialTime      = 5
	etcdKeepaliveTime = 10
)

const (
	etcdClusterServerCertKey = "cluster_server_cert"
	etcdClusterServerKeyKey  = "cluster_server_key"

	etcdClusterClientCertKey = "cluster_client_cert"
	etcdClusterClientKeyKey  = "cluster_client_key"
)

const (
	etcdCosmosName = "/cosmos"
	etcdNodeName   = "/node"
	etcdVerName    = "/version"
)

// Resolved to /cosmos/{cosmosName}/node/
// Use to trim prefix for suffix {nodeName}/version/{version}
func etcdCosmosNodeWatchAllURI(cosmosName string) string {
	s := etcdCosmosName + "/%s" + etcdNodeName + "/"
	return fmt.Sprintf(s, cosmosName)
}

// Resolved to /cosmos/{cosmosName}/node/{nodeName}/version/
// Use to set or unset a current version of a node
func etcdCosmosNodeLockURI(cosmosName, nodeName string) string {
	// Resolved to /cosmos/{cosmosName}/node/{nodeName}/version
	s := etcdCosmosName + "/%s" + etcdNodeName + "/%s" + etcdVerName + "/"
	return fmt.Sprintf(s, cosmosName, nodeName)
}

// Resolved to /cosmos/{cosmosName}/node/{nodeName}/version/{version}
func etcdCosmosNodeVersionURI(cosmosName, nodeName string, version int64) string {
	// Resolved to /cosmos/{cosmosName}/node/{nodeName}/version/{version}
	s := etcdCosmosName + "/%s" + etcdNodeName + "/%s" + etcdVerName + "/%d"
	return fmt.Sprintf(s, cosmosName, nodeName, version)
}

//// Resolved to /cosmos/{cosmosName}/node/{nodeName}/version
//// Use to set or unset a current version of a node
//func etcdGetCosmosNodeCurrentVersionURI(cosmosName, nodeName string) string {
//	// Resolved to /cosmos/{cosmosName}/node/{nodeName}/version
//	s := etcdCosmosName + "/%s" + etcdNodeName + "/%s" + etcdVerName
//	return fmt.Sprintf(s, cosmosName, nodeName)
//}

// getClusterTLSConfig
// 获取集群的TLS配置
func (p *CosmosProcess) getClusterTLSConfig(cli *clientv3.Client) (bool, *grpc.ServerOption, *grpc.DialOption, *Error) {
	// Server-side
	serverCertPem, _, err := etcdGet(cli, etcdClusterServerCertKey)
	if err != nil {
		return false, nil, nil, err.AddStack(nil)
	}
	serverKeyPem, _, err := etcdGet(cli, etcdClusterServerKeyKey)
	if err != nil {
		return false, nil, nil, err.AddStack(nil)
	}
	// If no TLS, return
	if serverCertPem == nil || serverKeyPem == nil {
		return false, nil, nil, nil
	}
	// Parse the key pair to create tls config of gRPC server.
	serverCert, er := tls.X509KeyPair(serverCertPem, serverKeyPem)
	if er != nil {
		return false, nil, nil, NewErrorf(ErrCosmosEtcdClusterTLSInvalid, "etcd: failed to parse key pair. err=(%s)", er).AddStack(nil)
	}

	// Client-side
	clientCertPem, _, err := etcdGet(cli, etcdClusterClientCertKey)
	if err != nil {
		return false, nil, nil, err.AddStack(nil)
	}
	clientKeyPem, _, err := etcdGet(cli, etcdClusterClientKeyKey)
	if err != nil {
		return false, nil, nil, err.AddStack(nil)
	}
	var certPool *x509.CertPool
	var clientCert tls.Certificate
	if clientCertPem != nil && clientKeyPem != nil {
		certPool = x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM(clientCertPem); !ok {
			return false, nil, nil, NewErrorf(ErrCosmosEtcdClusterTLSInvalid, "etcd: failed to parse client certificate").AddStack(nil)
		}
		// Load client's certificate and private key
		clientCert, er = tls.X509KeyPair(serverCertPem, serverKeyPem)
		if err != nil {
			return false, nil, nil, NewErrorf(ErrCosmosEtcdClusterTLSInvalid, "etcd: failed to parse key pair. err=(%s)", er).AddStack(nil)
		}
	}

	// Create TLS config for gRPC server and client.
	var serverOption grpc.ServerOption
	var dialOption grpc.DialOption
	if clientCertPem == nil {
		// For server-side
		// Create TLS config for gRPC server
		serverOption = grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientAuth:   tls.NoClientCert,
		}))

		// For client-side
		// Create TLS config for gRPC client
		dialOption = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true, // only for testing purposes, don't use in production
		}))

		return true, &serverOption, &dialOption, nil
	} else {
		// For server-side
		// Create TLS config for gRPC server
		serverOption = grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    certPool,
		}))

		// For client-side
		// Create TLS config for gRPC client
		dialOption = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      certPool,
		}))

		return true, &serverOption, &dialOption, nil
	}
}

// trySettingClusterToCurrentAndKeepalive
// 尝试设置当前节点为活跃， 并保持当前节点的活跃。如果节点挂了，etcd会在失效后删除节点；如果节点正常退出，会在退出前删除节点。
// Try to set the current node to active and keep the current node active. If the node hangs, etcd will delete the node after it fails;
// if the node exits normally, it will delete the node before exiting.
func (p *CosmosProcess) trySettingClusterToCurrentAndKeepalive() *Error {
	if !p.cluster.enable {
		return nil
	}

	// If there is, notify the remote node to exit.
	// 如果有，通知远程节点退出。
	p.cluster.remoteMutex.Lock()
	cosmosRemote, has := p.cluster.remoteCosmos[p.local.runnable.config.Node]
	p.cluster.remoteMutex.Unlock()
	if has {
		// 如果节点已经是活跃的，直接退出并等待响应。
		// If the node is already active, exit directly and wait response.
		if err := cosmosRemote.tryKillingRemote(); err != nil {
			return err.AddStack(nil)
		}
	}

	key, infoBuf, err := p.etcdNodeVersion(p.local.runnable.config.Node, p.cluster.etcdVersion, &CosmosNodeVersionInfo{
		Node:     p.local.runnable.config.Node,
		Address:  p.cluster.grpcAddress,
		Id:       p.local.GetIDInfo(),
		State:    ClusterNodeState_Started,
		Elements: p.local.getClusterElementsInfo(),
	})
	if err != nil {
		return err.AddStack(nil)
	}

	// Keep the cosmos remote alive.
	lease, keepAliveCh, err := etcdKeepalive(p.cluster.etcdClient, key, infoBuf, etcdKeepaliveTime)
	if err != nil {
		return err.AddStack(nil)
	}

	// start a separate goroutine to keep the service registration alive, and to handle the case where the service is not able to renew the lease
	go func(firstLease *clientv3.LeaseGrantResponse) {
		defer func() {
			if r := recover(); r != nil {
				p.etcdErrorHandler(NewErrorf(ErrFrameworkInternalError, "etcd: Watcher keepalive stopped.").AddStack(nil))
			}
		}()

		//var err *Error
		var lease = firstLease
		id := p.local.atomos.id

		defer func() {
			p.logging.PushLogging(id, LogLevel_Info, "etcd: Watcher will stop.")
			_, er := p.cluster.etcdClient.Delete(context.Background(), key)
			if er != nil {
				p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdKeepaliveFailed, "etcd: Watcher, failed to delete cluster version info. err=(%s)", er).AddStack(nil))
			}
			if cancel := p.cluster.etcdCancelWatch; cancel != nil {
				cancel()
			}
			if p.cluster.etcdExitCh != nil {
				p.cluster.etcdExitCh <- struct{}{}
			}
		}()

		p.cluster.etcdExitCh = make(chan struct{})
		for {
			select {
			// 保持活跃
			// Keepalive
			//case <-time.After(etcdKeepaliveTime * time.Second * 2 / 3):
			case keepAlive := <-keepAliveCh:
				p.logging.PushLogging(id, LogLevel_Debug, "etcd: Watcher keepalive.")
				p.mutex.Lock()
				state := p.state
				p.mutex.Unlock()
				if state >= CosmosProcessStateShutdown {
					p.logging.PushLogging(id, LogLevel_Info, fmt.Sprintf("etcd: Watcher keepalive stopped. state=(%v)", state))
					return
				}
				if keepAlive == nil {
					p.logging.PushLogging(id, LogLevel_Info, "etcd: Watcher keepalive stopped, lease is not renewed.")
					return
				}
			// 更新节点信息
			// Update node information
			case infoBuf, more := <-p.cluster.etcdInfoCh:
				p.logging.PushLogging(id, LogLevel_Debug, fmt.Sprintf("etcd: Watcher updates process info. info=(%s),more=(%v)", infoBuf, more))
				if !more {
					return
				}
				_, er := p.cluster.etcdClient.Put(context.Background(), key, string(infoBuf), clientv3.WithLease(lease.ID))
				if er != nil {
					p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdKeepaliveFailed, "etcd: Watcher, failed to update cluster version info. err=(%s)", er).AddStack(nil))
				}
			}
		}
	}(lease)

	// 设置当前节点为活跃
	// Set the current node to active
	if err := p.etcdStartedVersionNodeLock(); err != nil {
		return err.AddStack(p.local)
	}

	return nil
}

// watchCluster
// 保持当前节点的活跃
func (p *CosmosProcess) watchCluster(cli *clientv3.Client) *Error {
	keyPrefix := etcdCosmosNodeWatchAllURI(p.local.runnable.config.Cosmos)
	keyPrefixBytes := []byte(keyPrefix)

	// 会先把当前的节点信息全部获取一次先。
	// First, get all the node information.
	getResp, er := cli.Get(context.Background(), keyPrefix, clientv3.WithPrefix())
	if er != nil {
		return NewErrorf(ErrCosmosEtcdGetFailed, "etcd: Failed to fetch cluster info. err=(%s)", er).AddStack(nil)
	}
	for _, ev := range getResp.Kvs {
		if !bytes.HasPrefix(ev.Key, keyPrefixBytes) {
			p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdInvalidKey, "etcd: Invalid key. key=(%s)", string(ev.Key)).AddStack(nil))
			continue
		}
		key := strings.TrimPrefix(string(ev.Key), keyPrefix)
		p.handleKey(key, ev.Value, true)
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cluster.etcdCancelWatch = cancel
	// Watch for changes
	go func() {
		defer p.local.Log().Info("etcd: Watcher stopped.")
		watchCh := cli.Watch(ctx, keyPrefix, clientv3.WithPrefix())
		for watchResp := range watchCh {
			for _, event := range watchResp.Events {
				p.logging.PushLogging(p.local.atomos.id, LogLevel_Debug, fmt.Sprintf("etcd: Watch event. type=(%s),key=(%s),value=(%s)", event.Type, string(event.Kv.Key), string(event.Kv.Value)))
				if !bytes.HasPrefix(event.Kv.Key, keyPrefixBytes) {
					p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdInvalidKey, "etcd: Watcher handle key, got invalid key. key=(%s)", string(event.Kv.Key)).AddStack(nil))
					continue
				}
				key := strings.TrimPrefix(string(event.Kv.Key), keyPrefix)
				switch event.Type {
				case mvccpb.PUT:
					p.handleKey(key, event.Kv.Value, true)
				case mvccpb.DELETE:
					p.handleKey(key, event.Kv.Value, false)
				}
			}
		}
	}()

	return nil
}

// tryUnsettingCurrentAndUpdateNodeInfo
// 尝试取消当前节点的活跃状态并更新节点信息
// Try to cancel the active status of the current node and update the node information
func (p *CosmosProcess) tryUnsettingCurrentAndUpdateNodeInfo() *Error {
	if !p.cluster.enable {
		return nil
	}
	// 取消当前节点的活跃状态
	// Cancel the active status of the current node
	if err := p.etcdStoppingNodeVersionUnlock(); err != nil {
		p.logging.PushLogging(p.local.atomos.id, LogLevel_Err, fmt.Sprintf("etcd: Failed to unset cluster current. err=(%v)", err))
	}
	// 更新节点信息
	// Update node information
	_, infoBuf, err := p.etcdNodeVersion(p.local.runnable.config.Node, p.cluster.etcdVersion, &CosmosNodeVersionInfo{
		Node:     p.local.runnable.config.Node,
		Address:  p.cluster.grpcAddress,
		Id:       p.local.GetIDInfo(),
		State:    ClusterNodeState_Stopping,
		Elements: p.local.getClusterElementsInfo(),
	})
	if err != nil {
		return err.AddStack(nil)
	}
	select {
	case p.cluster.etcdInfoCh <- infoBuf:
	default:
	}
	return nil
}

// handleKey
// 处理etcd的key
func (p *CosmosProcess) handleKey(key string, value []byte, updateOrDelete bool) {
	// key: {nodeName}/version/{version}
	keys := strings.Split(key, "/")
	if len(keys) != 3 {
		p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdInvalidKey, "etcd: Watcher handle key, got invalid key format. key=(%s)", key).AddStack(nil))
		return
	}
	if keys[1] != "version" {
		p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdInvalidKey, "etcd: Watcher handle key, got invalid key version. key=(%s)", key).AddStack(nil))
		return
	}

	nodeName := keys[0]
	version := keys[2]
	if version == "" {
		if updateOrDelete {
			p.etcdUpdateClusterVersionLockInfo(nodeName, value)
		} else {
			p.etcdDeleteClusterVersionLockInfo(nodeName)
		}
	} else {
		if updateOrDelete {
			p.etcdUpdateClusterVersionNodeInfo(nodeName, version, value)
		} else {
			p.etcdDeleteClusterVersionNodeInfo(nodeName, version)
		}
	}
}

// etcdUpdateClusterVersionLockInfo - watcher
// 设定当前节点的版本
func (p *CosmosProcess) etcdUpdateClusterVersionLockInfo(nodeName string, buf []byte) {
	p.logging.PushLogging(p.local.atomos.id, LogLevel_Debug, fmt.Sprintf("etcd: Update current version lock. node=(%s),lock=(%s)", nodeName, string(buf)))
	// Unmarshal the lock info
	lockInfo := &CosmosNodeVersionLock{}
	if er := json.Unmarshal(buf, lockInfo); er != nil {
		p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdUpdateFailed, "etcd: Update current version lock, got invalid value. node=(%s),err=(%s)", nodeName, er).AddStack(nil))
		return
	}

	// Update the remote
	hasOld, remote := func(nodeName string, info *CosmosNodeVersionLock) (hasOld bool, r *CosmosRemote) {
		p.cluster.remoteMutex.Lock()
		defer p.cluster.remoteMutex.Unlock()

		remote, has := p.cluster.remoteCosmos[nodeName]
		if !has {
			remote = newCosmosRemoteFromLockInfo(p, info)
			p.cluster.remoteCosmos[nodeName] = remote
		}

		return has, remote
	}(nodeName, lockInfo)

	if hasOld {
		remote.etcdUpdateLock(lockInfo)
	} else {
		remote.etcdUpdateLock(lockInfo)
	}
}

// etcdDeleteClusterVersionLockInfo - watcher
// 取消当前节点的版本
func (p *CosmosProcess) etcdDeleteClusterVersionLockInfo(nodeName string) {
	p.logging.PushLogging(p.local.atomos.id, LogLevel_Debug, fmt.Sprintf("etcd: Delete current version lock. node=(%s)", nodeName))
	p.cluster.remoteMutex.Lock()
	defer p.cluster.remoteMutex.Unlock()

	remote, has := p.cluster.remoteCosmos[nodeName]
	if !has {
		p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdUpdateFailed, "etcd: Delete current version lock, got invalid node. node=(%s)", nodeName).AddStack(nil))
		return
	}
	remote.etcdDeleteLock()
}

// etcdUpdateClusterVersionNodeInfo - watcher
// 设定节点的版本
func (p *CosmosProcess) etcdUpdateClusterVersionNodeInfo(nodeName string, versionKey string, value []byte) {
	p.logging.PushLogging(p.local.atomos.id, LogLevel_Debug, fmt.Sprintf("etcd: Update cluster version info. node=(%s),version=(%s)", nodeName, versionKey))
	info := &CosmosNodeVersionInfo{}
	if er := json.Unmarshal(value, info); er != nil {
		p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdUpdateFailed, "etcd: Update cluster version info, got invalid cluster info value. node=(%s),version=(%s),err=(%v)", nodeName, versionKey, er).AddStack(nil))
		return
	}

	// Update the remote
	hasOld, remote := func(info *CosmosNodeVersionInfo) (hasOld bool, r *CosmosRemote) {
		p.cluster.remoteMutex.Lock()
		defer p.cluster.remoteMutex.Unlock()

		remote, has := p.cluster.remoteCosmos[nodeName]
		if !has {
			remote = newCosmosRemoteFromNodeInfo(p, info)
			p.cluster.remoteCosmos[nodeName] = remote
		}

		return has, remote
	}(info)

	if hasOld {
		remote.etcdUpdateVersion(info, versionKey)
	} else {
		remote.etcdCreateVersion(info, versionKey)
	}
}

// etcdDeleteClusterVersionNodeInfo - watcher
// 取消节点的版本
func (p *CosmosProcess) etcdDeleteClusterVersionNodeInfo(nodeName, versionKey string) {
	p.logging.PushLogging(p.local.atomos.id, LogLevel_Debug, fmt.Sprintf("etcd: Delete cluster version info. node=(%s),version=(%s)", nodeName, versionKey))
	p.cluster.remoteMutex.Lock()
	defer p.cluster.remoteMutex.Unlock()

	remote, has := p.cluster.remoteCosmos[nodeName]
	if has {
		remote.etcdDeleteVersion(versionKey)
	}
}

// etcdNodeVersion etcd节点版本信息
func (p *CosmosProcess) etcdNodeVersion(nodeName string, version int64, info *CosmosNodeVersionInfo) (string, string, *Error) {
	key := etcdCosmosNodeVersionURI(p.local.runnable.config.Cosmos, nodeName, version)
	infoBuf, er := json.Marshal(info)
	if er != nil {
		return "", "", NewErrorf(ErrCosmosEtcdKeepaliveFailed, "etcd: Failed to marshal cluster version info. err=(%s)", er).AddStack(nil)
	}
	return key, string(infoBuf), nil
}

// etcdErrorHandler
// etcd的错误处理
func (p *CosmosProcess) etcdErrorHandler(err *Error) {
	p.local.Log().Fatal("etcd critical error: %s", err)
	p.logging.PushLogging(p.local.atomos.id, LogLevel_Fatal, fmt.Sprintf("etcd critical error: %s", err))
}
