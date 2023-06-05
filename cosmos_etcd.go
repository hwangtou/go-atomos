package go_atomos

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	"go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"strconv"
	"strings"
	"time"
)

const (
	etcdKeepaliveTime = 10
)

const (
	etcdClusterServerCertKey = "cluster_server_cert"
	etcdClusterServerKeyKey  = "cluster_server_key"

	etcdClusterClientCertKey = "cluster_client_cert"
	etcdClusterClientKeyKey  = "cluster_client_key"
)

const (
	etcdURICosmosPrefix            = "/cosmos/"
	etcdURICosmosNodePrefix        = "/node/"
	etcdURICosmosNodeVersionPrefix = "/version/"
	etcdURICosmosNodeCurrentSuffix = "current"

	etcdURICosmosFormat = etcdURICosmosPrefix + "%s"

	// Resolved to /cosmos/{cosmosName}/node/{nodeName}
	etcdURICosmosNodeFormat = etcdURICosmosFormat + etcdURICosmosNodePrefix + "%s"

	// Resolved to /cosmos/{cosmosName}/node/{nodeName}/version/{version}
	etcdURICosmosNodeVersionFormat = etcdURICosmosNodeFormat + etcdURICosmosNodeVersionPrefix + "%d"

	// Resolved to /cosmos/{cosmosName}/node/{nodeName}/version/current
	etcdURICosmosNodeCurrentVersionFormat = etcdURICosmosNodeFormat + etcdURICosmosNodeVersionPrefix + etcdURICosmosNodeCurrentSuffix
)

// Resolved to /cosmos/{cosmosName}/node/
// Split to {nodeName}/version/{version|current}
func etcdGetCosmosNodeWatchURI(cosmosName string) string {
	return fmt.Sprintf(etcdURICosmosFormat+etcdURICosmosNodePrefix, cosmosName)
}

func etcdGetCosmosNodeVersionURI(cosmosName, nodeName string, version int64) string {
	return fmt.Sprintf(etcdURICosmosNodeVersionFormat, cosmosName, nodeName, version)
}

func etcdGetCosmosNodeCurrentVersionURI(cosmosName, nodeName string) string {
	return fmt.Sprintf(etcdURICosmosNodeCurrentVersionFormat, cosmosName, nodeName)
}

// etcdGetClusterTLSConfig
// 获取集群的TLS配置
func (p *CosmosProcess) etcdGetClusterTLSConfig(cli *clientv3.Client) (bool, *grpc.ServerOption, *grpc.DialOption, *Error) {
	// Server-side
	serverCertPem, err := etcdGet(cli, etcdClusterServerCertKey)
	if err != nil {
		return false, nil, nil, err.AddStack(nil)
	}
	serverKeyPem, err := etcdGet(cli, etcdClusterServerKeyKey)
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
	clientCertPem, err := etcdGet(cli, etcdClusterClientCertKey)
	if err != nil {
		return false, nil, nil, err.AddStack(nil)
	}
	clientKeyPem, err := etcdGet(cli, etcdClusterClientKeyKey)
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

// etcdKeepClusterAlive
// 保持当前节点的活跃
func (p *CosmosProcess) etcdKeepClusterAlive(cli *clientv3.Client, nodeName, listenAddress string, version int64) *Error {
	// Register a service
	info := &CosmosProcessInfo{
		Node:    nodeName,
		Address: listenAddress,
	}
	infoBuf, er := json.Marshal(info)
	if er != nil {
		return NewErrorf(ErrCosmosEtcdKeepaliveFailed, "etcd: Failed to marshal process info. err=(%s)", er).AddStack(nil)
	}
	key := etcdGetCosmosNodeVersionURI(p.local.runnable.config.Cosmos, nodeName, version)

	// Keep alive
	lease, err := etcdKeepalive(cli, key, infoBuf, etcdKeepaliveTime)
	if err != nil {
		return err.AddStack(nil)
	}

	// start a separate goroutine to keep the service registration alive
	go func(firstLease *clientv3.LeaseGrantResponse) {
		var err *Error
		var lease = firstLease
		for {
			select {
			case <-time.After(etcdKeepaliveTime * time.Second / 3):
				p.mutex.Lock()
				state := p.state
				p.mutex.Unlock()
				if state != CosmosProcessStateRunning {
					return
				}
				_, er := cli.KeepAliveOnce(context.Background(), lease.ID)
				if er != nil {
					// TODO: Logging
					if er == rpctypes.ErrLeaseNotFound {
						lease, err = etcdKeepalive(cli, key, infoBuf, etcdKeepaliveTime)
						if err != nil {
							p.etcdErrorHandler(err)
							// TODO: Logging
						}
					}
				}
			case infoBuf, more := <-p.cluster.etcdInfoCh:
				if !more {
					// TODO: Logging
					_, er = cli.Delete(context.Background(), key)
					if er != nil {
						// TODO: Logging
						p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdKeepaliveFailed, "etcd: Failed to delete process info. err=(%s)", er).AddStack(nil))
					}
					return
				}
				_, er = cli.Put(context.Background(), key, string(infoBuf), clientv3.WithLease(lease.ID))
				if er != nil {
					// TODO: Logging
					p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdKeepaliveFailed, "etcd: Failed to update process info. err=(%s)", er).AddStack(nil))
				}
			}
		}
	}(lease)

	return nil
}

// etcdKeepalive
// 保持当前节点的活跃
func (p *CosmosProcess) etcdWatchCluster(cli *clientv3.Client) *Error {
	keyPrefix := etcdGetCosmosNodeWatchURI(p.local.runnable.config.Cosmos)
	keyPrefixBytes := []byte(keyPrefix)

	// Fetch the required value
	getResp, er := cli.Get(context.Background(), keyPrefix)
	if er != nil {
		return NewErrorf(ErrCosmosEtcdGetFailed, "etcd: Failed to fetch cluster info. err=(%s)", er).AddStack(nil)
	}
	for _, ev := range getResp.Kvs {
		if !bytes.HasPrefix(ev.Key, keyPrefixBytes) {
			p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdInvalidKey, "etcd: Invalid key. key=(%s)", string(ev.Key)).AddStack(nil))
			continue
		}
		key := strings.TrimLeft(string(ev.Key), keyPrefix)
		p.handleKey(key, ev.Value, true)
	}

	// Watch for changes
	go func() {
		watchCh := cli.Watch(context.Background(), keyPrefix, clientv3.WithPrefix())
		for watchResp := range watchCh {
			for _, event := range watchResp.Events {
				if !bytes.HasPrefix(event.Kv.Key, keyPrefixBytes) {
					p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdInvalidKey, "etcd: Invalid key. key=(%s)", string(event.Kv.Key)).AddStack(nil))
					continue
				}
				key := strings.TrimLeft(string(event.Kv.Key), keyPrefix)
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

// handleKey
// 处理etcd的key
func (p *CosmosProcess) handleKey(key string, value []byte, updateOrDelete bool) {
	keys := strings.Split(key, "/")
	if len(keys) != 3 {
		p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdInvalidKey, "etcd: Invalid key. key=(%s)", key).AddStack(nil))
		return
	}
	if keys[1] != "version" {
		p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdInvalidKey, "etcd: Invalid key. key=(%s)", key).AddStack(nil))
		return
	}
	nodeName := keys[0]
	version := keys[2]
	if version == etcdURICosmosNodeCurrentSuffix {
		if updateOrDelete {
			p.etcdUpdateClusterCosmosNodeCurrent(nodeName, value)
		} else {
			p.etcdDeleteClusterCosmosNodeCurrent(nodeName)
		}
	} else {
		if updateOrDelete {
			p.etcdUpdateClusterCosmosNode(nodeName, version, value)
		} else {
			p.etcdDeleteClusterCosmosNode(nodeName, version, value)
		}
	}
}

// etcdSetClusterCosmosNodeInfo
// 设定当前节点的信息
func (p *CosmosProcess) etcdSetClusterCosmosNodeInfo() *Error {
	info := &CosmosProcessInfo{
		Node:     p.local.runnable.config.Node,
		Address:  p.cluster.grpcAddress,
		Id:       p.local.GetIDInfo(),
		Elements: p.local.getClusterElementsInfo(),
	}
	infoBuf, er := json.Marshal(info)
	if er != nil {
		return NewErrorf(ErrCosmosEtcdUpdateFailed, "etcd: Failed to marshal process info. err=(%s)", er).AddStack(nil)
	}
	p.cluster.etcdInfoCh <- infoBuf
	return nil
}

// etcdUnsetClusterCosmosNodeInfo
// 取消当前节点的信息
func (p *CosmosProcess) etcdUnsetClusterCosmosNodeInfo() *Error {
	if p.cluster.etcdInfoCh != nil {
		close(p.cluster.etcdInfoCh)
		p.cluster.etcdInfoCh = nil
	}
	return nil
}

// etcdClusterSetCurrent
// 设定当前节点的版本
func (p *CosmosProcess) etcdClusterSetCurrent(cli *clientv3.Client, nodeName string, version int64) *Error {
	key := etcdGetCosmosNodeCurrentVersionURI(p.local.runnable.config.Cosmos, nodeName)
	versionStr := strconv.FormatInt(version, 10)
	if err := etcdPut(cli, key, []byte(versionStr)); err != nil {
		return err.AddStack(nil)
	}
	return nil
}

// etcdClusterUnsetCurrent
// 取消当前节点的版本
func (p *CosmosProcess) etcdClusterUnsetCurrent(cli *clientv3.Client, nodeName string) *Error {
	key := etcdGetCosmosNodeCurrentVersionURI(p.local.runnable.config.Cosmos, nodeName)
	if err := etcdDelete(cli, key); err != nil {
		return err.AddStack(nil)
	}
	return nil
}

// etcdUpdateClusterCosmosNodeCurrent - watcher
// 设定当前节点的版本
func (p *CosmosProcess) etcdUpdateClusterCosmosNodeCurrent(nodeName string, value []byte) {
	p.cluster.remoteMutex.Lock()
	defer p.cluster.remoteMutex.Unlock()

	version := string(value)
	remote, has := p.cluster.remoteCosmos[nodeName]
	if !has {
		// TODO: error
		return
	}
	remote.setCurrent(version)
}

// etcdDeleteClusterCosmosNodeCurrent - watcher
// 取消当前节点的版本
func (p *CosmosProcess) etcdDeleteClusterCosmosNodeCurrent(nodeName string) {
	p.cluster.remoteMutex.Lock()
	defer p.cluster.remoteMutex.Unlock()

	remote, has := p.cluster.remoteCosmos[nodeName]
	if !has {
		// TODO: error
		return
	}
	remote.unsetCurrent()
}

// etcdUpdateClusterCosmosNode - watcher
// 设定节点的版本
func (p *CosmosProcess) etcdUpdateClusterCosmosNode(nodeName string, versionKey string, value []byte) {
	info := &CosmosProcessInfo{}
	if er := json.Unmarshal(value, info); er != nil {
		p.etcdErrorHandler(NewErrorf(ErrCosmosEtcdUpdateFailed, "etcd: Invalid value. value=(%s)", string(value)).AddStack(nil))
		return
	}

	hasOld, remote := func(versionKey string, info *CosmosProcessInfo) (hasOld bool, r *CosmosRemote) {
		p.cluster.remoteMutex.Lock()
		defer p.cluster.remoteMutex.Unlock()

		remote, has := p.cluster.remoteCosmos[nodeName]
		if !has {
			remote = newCosmosRemote(p, info, versionKey)
			p.cluster.remoteCosmos[nodeName] = remote
		}
		return has, remote
	}(versionKey, info)

	if hasOld {
		remote.updateVersion(info, versionKey)
	}
}

// etcdDeleteClusterCosmosNode - watcher
// 取消节点的版本
func (p *CosmosProcess) etcdDeleteClusterCosmosNode(nodeName string, versionKey string, value []byte) {
	p.cluster.remoteMutex.Lock()
	defer p.cluster.remoteMutex.Unlock()

	remote, has := p.cluster.remoteCosmos[nodeName]
	if has {
		remote.setDeleted()
	}
}

//// etcdCheckLinkCurrent - watcher
//// 检查当前节点的版本
//func (p *CosmosProcess) etcdCheckLinkCurrent() {
//}

// etcdErrorHandler
// etcd的错误处理
func (p *CosmosProcess) etcdErrorHandler(err *Error) {
	p.local.Log().Fatal("etcd critical error: %s", err)
	// TODO
}
