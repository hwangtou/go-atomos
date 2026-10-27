package atomos

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
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	etcdDialTime      = 10
	etcdKeepaliveTime = 20
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
func (p *CosmosProcess) getClusterTLSConfig(cli EtcdClient) (bool, *grpc.ServerOption, *grpc.DialOption, *Error) {
	// Server-side
	serverCertPem, _, err := etcdGet(cli, etcdClusterServerCertKey)
	if err != nil {
		return false, nil, nil, err.AddStack(nil)
	}
	serverKeyPem, _, err := etcdGet(cli, etcdClusterServerKeyKey)
	if err != nil {
		return false, nil, nil, err.AddStack(nil)
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
	// No TLS material at all.
	if serverCertPem == nil && serverKeyPem == nil && clientCertPem == nil && clientKeyPem == nil {
		return false, nil, nil, nil
	}
	// Incomplete TLS material must fail loudly instead of silently degrading:
	// a half-written server pair (or client material without the server pair)
	// used to return "no TLS" — a plaintext cluster hiding the operator's
	// mistake — and a half-written client pair used to produce a mutual-TLS
	// config whose handshake can never succeed.
	if serverCertPem == nil || serverKeyPem == nil {
		return false, nil, nil, NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid,
			"etcd: incomplete cluster TLS config: the server cert and key must be set together. serverCert=(%v),serverKey=(%v),clientCert=(%v),clientKey=(%v)",
			serverCertPem != nil, serverKeyPem != nil, clientCertPem != nil, clientKeyPem != nil).AddStack(nil)
	}
	if (clientCertPem == nil) != (clientKeyPem == nil) {
		return false, nil, nil, NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid,
			"etcd: incomplete cluster TLS config: the client cert and key must be set together. clientCert=(%v),clientKey=(%v)",
			clientCertPem != nil, clientKeyPem != nil).AddStack(nil)
	}
	// Parse the key pair to create tls config of gRPC server.
	serverCert, er := tls.X509KeyPair(serverCertPem, serverKeyPem)
	if er != nil {
		return false, nil, nil, NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid, "etcd: failed to parse key pair. err=(%s)", er).AddStack(nil)
	}

	var certPool *x509.CertPool
	var clientCert tls.Certificate
	if clientCertPem != nil && clientKeyPem != nil {
		certPool = x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM(clientCertPem); !ok {
			return false, nil, nil, NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid, "etcd: failed to parse client certificate").AddStack(nil)
		}
		// Load client's certificate and private key
		clientCert, er = tls.X509KeyPair(clientCertPem, clientKeyPem)
		if er != nil {
			return false, nil, nil, NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid, "etcd: failed to parse key pair. err=(%s)", er).AddStack(nil)
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

		// A server-only configuration still verifies the server certificate and
		// its SAN. The certificate PEM may contain a self-signed certificate or a
		// chain ending in the cluster CA.
		serverPool := x509.NewCertPool()
		if ok := serverPool.AppendCertsFromPEM(serverCertPem); !ok {
			return false, nil, nil, NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid, "etcd: failed to parse server certificate").AddStack(nil)
		}
		dialOption = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			RootCAs:    serverPool,
			MinVersion: tls.VersionTLS12,
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

// getClusterTLSConfigFromFiles builds the server-only TLS options configured by
// YAML. Verification (including SAN) is enabled unless explicitly overridden.
func getClusterTLSConfigFromFiles(config *CertConfig) (bool, *grpc.ServerOption, *grpc.DialOption, *Error) {
	if config == nil {
		return false, nil, nil, nil
	}
	certPEM, er := os.ReadFile(config.CertPath)
	if er != nil {
		return false, nil, nil, NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid, "cluster: failed to read certificate. err=(%v)", er).AddStack(nil)
	}
	keyPEM, er := os.ReadFile(config.KeyPath)
	if er != nil {
		return false, nil, nil, NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid, "cluster: failed to read private key. err=(%v)", er).AddStack(nil)
	}
	certificate, er := tls.X509KeyPair(certPEM, keyPEM)
	if er != nil {
		return false, nil, nil, NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid, "cluster: failed to parse key pair. err=(%v)", er).AddStack(nil)
	}
	roots := x509.NewCertPool()
	if ok := roots.AppendCertsFromPEM(certPEM); !ok {
		return false, nil, nil, NewError(ErrCodeCosmosEtcdClusterTLSInvalid, "cluster: failed to parse certificate").AddStack(nil)
	}
	serverOption := grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS12,
	}))
	dialOption := grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		RootCAs:            roots,
		InsecureSkipVerify: config.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}))
	return true, &serverOption, &dialOption, nil
}

var (
	etcdRegistrationAttempts = 10
	etcdRegistrationBackoff  = 3 * time.Second
	etcdWatchBackoff         = 200 * time.Millisecond
	etcdInfoPublishBackoff   = time.Second
)

type etcdInfoUpdate struct {
	seq  uint64
	info string
}

// Process-scoped etcd health is safe for concurrent readers and is removed on
// an orderly registration shutdown.
type etcdRuntimeState struct {
	registrationHealthy atomic.Bool
	watchHealthy        atomic.Bool
	resyncWatch         chan struct{}
}

var etcdRuntimeStates sync.Map

func (p *CosmosProcess) etcdRuntime() *etcdRuntimeState {
	runtime, _ := etcdRuntimeStates.LoadOrStore(p, &etcdRuntimeState{resyncWatch: make(chan struct{}, 1)})
	return runtime.(*etcdRuntimeState)
}

// EtcdClusterHealthy reports whether registration and cluster watching are both
// live. A standalone process has no etcd dependency and is therefore healthy.
func (p *CosmosProcess) EtcdClusterHealthy() bool {
	if p == nil {
		return false
	}
	if !p.cluster.enable {
		return true
	}
	runtime, ok := etcdRuntimeStates.Load(p)
	if !ok {
		return false
	}
	state := runtime.(*etcdRuntimeState)
	return state.registrationHealthy.Load() && state.watchHealthy.Load()
}

func (p *CosmosProcess) etcdRegisterWithRetry(key string, info *etcdInfoUpdate, infoCh <-chan etcdInfoUpdate) (*clientv3.LeaseGrantResponse, <-chan *clientv3.LeaseKeepAliveResponse, *Error) {
	var lastErr *Error
	for attempt := 0; attempt < etcdRegistrationAttempts; attempt++ {
		if attempt > 0 {
			timer := p.getClock().NewTimer(etcdRegistrationBackoff)
			select {
			case <-timer.C():
			case updated, more := <-infoCh:
				if !timer.Stop() {
					select {
					case <-timer.C():
					default:
					}
				}
				if !more {
					return nil, nil, NewError(ErrCodeCosmosEtcdKeepaliveFailed, "etcd: registration stopped.").AddStack(nil)
				}
				if updated.seq > info.seq {
					*info = updated
				}
			}
		}
		lease, keepAliveCh, err := etcdKeepalive(p.cluster.etcdClient, key, info.info, etcdKeepaliveTime)
		if err == nil {
			return lease, keepAliveCh, nil
		}
		lastErr = err
	}
	return nil, nil, lastErr.AddStack(nil)
}

// trySettingClusterToCurrentAndKeepalive registers the node and maintains its
// lease. A closed keepalive stream starts a bounded re-registration cycle.
func (p *CosmosProcess) trySettingClusterToCurrentAndKeepalive() *Error {
	if !p.cluster.enable {
		return nil
	}

	p.cluster.remoteMutex.Lock()
	cosmosRemote, has := p.cluster.remoteCosmos[p.local.runnable.config.Node]
	p.cluster.remoteMutex.Unlock()
	if has {
		p.local.coreLog().coreInfo("CosmosProcess: trySettingClusterToCurrentAndKeepalive has existed.")
		_ = cosmosRemote
	}

	key, infoBuf, err := p.etcdNodeVersion(p.local.runnable.config.Node, p.cluster.etcdVersion, &CosmosNodeVersionInfo{
		Node:      p.local.runnable.config.Node,
		Address:   p.cluster.grpcAddress,
		Id:        p.local.GetIDInfo(),
		State:     ClusterNodeState_Started,
		Elements:  p.local.getClusterElementsInfo(),
		StartupId: p.startupID,
	})
	if err != nil {
		return err.AddStack(nil)
	}

	info := etcdInfoUpdate{info: infoBuf}
	infoCh := p.cluster.etcdInfoCh
	lease, keepAliveCh, err := p.etcdRegisterWithRetry(key, &info, infoCh)
	if err != nil {
		return err.AddStack(nil)
	}
	runtime := p.etcdRuntime()
	p.lifecycle.publishLease(lease.ID, true)
	exitCh := make(chan struct{})
	p.cluster.etcdExitCh = exitCh

	go func(lease *clientv3.LeaseGrantResponse, keepAliveCh <-chan *clientv3.LeaseKeepAliveResponse) {
		orderly := false
		publishedSeq := info.seq
		var retryTimer Timer
		var retryCh <-chan time.Time
		stopRetry := func() {
			if retryTimer == nil {
				return
			}
			if !retryTimer.Stop() {
				select {
				case <-retryTimer.C():
				default:
				}
			}
			retryTimer = nil
			retryCh = nil
		}
		publish := func() {
			if info.seq <= publishedSeq {
				return
			}
			p.local.coreLog().coreInfo("etcd: Watcher updates process info. info=(%s)", info.info)
			if err := etcdPutLease(p.cluster.etcdClient, key, info.info, lease.ID); err != nil {
				runtime.registrationHealthy.Store(false)
				p.lifecycle.publishLease(0, false)
				p.etcdErrorHandler(err)
				stopRetry()
				retryTimer = p.getClock().NewTimer(etcdInfoPublishBackoff)
				retryCh = retryTimer.C()
				return
			}
			publishedSeq = info.seq
			runtime.registrationHealthy.Store(true)
			p.lifecycle.publishLease(lease.ID, true)
			stopRetry()
		}
		defer func() {
			stopRetry()
			if r := recover(); r != nil {
				p.etcdErrorHandler(NewErrorf(ErrCodeFrameworkInternalError, "etcd: keepalive stopped after panic. reason=(%v)", r).AddStack(nil))
			}
			runtime.registrationHealthy.Store(false)
			p.lifecycle.publishLease(0, false)
			p.local.coreLog().coreInfo("etcd: registration loop will stop.")
			if err := etcdDelete(p.cluster.etcdClient, key); err != nil {
				p.etcdErrorHandler(err)
			}
			if cancel := p.cluster.etcdCancelWatch; cancel != nil {
				cancel()
			}
			if orderly {
				etcdRuntimeStates.Delete(p)
			}
			close(exitCh)
		}()

		for {
			select {
			case keepAlive, more := <-keepAliveCh:
				if !more || keepAlive == nil {
					runtime.registrationHealthy.Store(false)
					p.lifecycle.publishLease(0, false)
					p.local.coreLog().coreError("etcd: keepalive stream closed; re-registering.")
					stopRetry()
					lease, keepAliveCh, err = p.etcdRegisterWithRetry(key, &info, infoCh)
					if err != nil {
						p.local.coreLog().coreError("etcd: bounded re-registration failed. err=(%v)", err)
						return
					}
					publishedSeq = info.seq
					runtime.registrationHealthy.Store(true)
					p.lifecycle.publishLease(lease.ID, true)
					select {
					case runtime.resyncWatch <- struct{}{}:
					default:
					}
					continue
				}
				runtime.registrationHealthy.Store(info.seq == publishedSeq)
				if !p.muteKeepaliveLog {
					p.local.coreLog().coreInfo("etcd: Watcher keepalive.")
				}
				p.mutex.RLock()
				state := p.state
				p.mutex.RUnlock()
				if state >= CosmosProcessStateShutdown {
					orderly = true
					return
				}
			case updated, more := <-infoCh:
				if !more {
					orderly = true
					return
				}
				if updated.seq <= info.seq {
					continue
				}
				info = updated
				stopRetry()
				publish()
			case <-retryCh:
				retryCh = nil
				publish()
			}
		}
	}(lease, keepAliveCh)

	if err := p.etcdStartedVersionNodeLock(); err != nil {
		runtime.registrationHealthy.Store(false)
		p.lifecycle.publishLease(0, false)
		return err.AddStack(p.local)
	}
	runtime.registrationHealthy.Store(true)
	return nil
}

func (p *CosmosProcess) applyClusterSnapshot(kvs []*mvccpb.KeyValue, keyPrefix string) {
	keyPrefixBytes := []byte(keyPrefix)
	present := make(map[string]bool, len(kvs))
	for _, kv := range kvs {
		if !bytes.HasPrefix(kv.Key, keyPrefixBytes) {
			p.etcdErrorHandler(NewErrorf(ErrCodeCosmosEtcdInvalidKey, "etcd: Invalid key. key=(%s)", string(kv.Key)).AddStack(nil))
			continue
		}
		key := strings.TrimPrefix(string(kv.Key), keyPrefix)
		present[key] = true
		p.handleKey(key, kv.Value, true)
	}

	// Applying additions alone leaves phantom nodes after a disconnected watch.
	// Remove every cached lock/version absent from the authoritative snapshot.
	p.cluster.remoteMutex.Lock()
	defer p.cluster.remoteMutex.Unlock()
	for nodeName, remote := range p.cluster.remoteCosmos {
		remote.mutex.RLock()
		hasLock := remote.lock != nil
		versions := make([]string, 0, len(remote.version))
		for version := range remote.version {
			versions = append(versions, version)
		}
		remote.mutex.RUnlock()
		if hasLock && !present[nodeName+etcdVerName+"/"] {
			remote.etcdDeleteLock()
		}
		for _, version := range versions {
			if !present[nodeName+etcdVerName+"/"+version] {
				remote.etcdDeleteVersion(version)
			}
		}
	}
}

func (p *CosmosProcess) handleWatchEvent(event *clientv3.Event, keyPrefix string) {
	p.local.coreLog().coreInfo("etcd: Watch event. type=(%s),key=(%s),value=(%s)", event.Type, string(event.Kv.Key), string(event.Kv.Value))
	if !bytes.HasPrefix(event.Kv.Key, []byte(keyPrefix)) {
		p.etcdErrorHandler(NewErrorf(ErrCodeCosmosEtcdInvalidKey, "etcd: Watcher handle key, got invalid key. key=(%s)", string(event.Kv.Key)).AddStack(nil))
		return
	}
	key := strings.TrimPrefix(string(event.Kv.Key), keyPrefix)
	switch event.Type {
	case mvccpb.PUT:
		p.handleKey(key, event.Kv.Value, true)
	case mvccpb.DELETE:
		p.handleKey(key, event.Kv.Value, false)
	}
}

// watchCluster loads an authoritative snapshot, then watches from its revision
// plus one. Any terminated/canceled stream causes a fresh snapshot and revision
// reconciliation, with cancellation-aware backoff between attempts.
func (p *CosmosProcess) watchCluster(cli EtcdClient) *Error {
	keyPrefix := etcdCosmosNodeWatchAllURI(p.local.runnable.config.Cosmos)
	getResp, err := etcdGetPrefix(cli, keyPrefix)
	if err != nil {
		return err.AddStack(nil)
	}
	p.applyClusterSnapshot(getResp.Kvs, keyPrefix)

	ctx, cancel := context.WithCancel(context.Background())
	p.cluster.etcdCancelWatch = cancel
	runtime := p.etcdRuntime()
	runtime.watchHealthy.Store(true)
	go func() {
		defer func() {
			runtime.watchHealthy.Store(false)
			if r := recover(); r != nil {
				p.local.coreLog().coreError("etcd: Watcher recovered from panic. reason=(%v),stack=(%s)", r, string(debug.Stack()))
			}
			p.local.coreLog().coreInfo("etcd: Watcher stopped.")
		}()

		backoff := etcdWatchBackoff
		revision := getResp.Header.Revision
		for {
			watchCtx, watchCancel := context.WithCancel(ctx)
			watchCh := cli.Watch(watchCtx, keyPrefix, clientv3.WithPrefix(), clientv3.WithRev(revision+1))
			rebuild := false
			for !rebuild {
				select {
				case <-ctx.Done():
					watchCancel()
					return
				case <-runtime.resyncWatch:
					rebuild = true
				case watchResp, more := <-watchCh:
					if !more || watchResp.Canceled || watchResp.Err() != nil {
						if more && watchResp.Err() != nil && ctx.Err() == nil {
							p.local.coreLog().coreError("etcd: watch terminated; rebuilding from a snapshot. err=(%v)", watchResp.Err())
						}
						rebuild = true
						continue
					}
					for _, event := range watchResp.Events {
						p.handleWatchEvent(event, keyPrefix)
					}
					backoff = etcdWatchBackoff
				}
			}
			watchCancel()
			runtime.watchHealthy.Store(false)

			for {
				timer := p.getClock().NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C():
				}
				snapshot, err := etcdGetPrefix(cli, keyPrefix)
				if err != nil {
					p.local.coreLog().coreError("etcd: snapshot rebuild failed. err=(%v)", err)
					if backoff < 5*time.Second {
						backoff *= 2
						if backoff > 5*time.Second {
							backoff = 5 * time.Second
						}
					}
					continue
				}
				if ctx.Err() != nil {
					return
				}
				p.applyClusterSnapshot(snapshot.Kvs, keyPrefix)
				revision = snapshot.Header.Revision
				runtime.watchHealthy.Store(true)
				backoff = etcdWatchBackoff
				break
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
		p.local.coreLog().coreFatal("etcd: Failed to unset cluster current. err=(%v)", err)
	}
	// 更新节点信息
	// Update node information
	_, infoBuf, err := p.etcdNodeVersion(p.local.runnable.config.Node, p.cluster.etcdVersion, &CosmosNodeVersionInfo{
		Node:      p.local.runnable.config.Node,
		Address:   p.cluster.grpcAddress,
		Id:        p.local.GetIDInfo(),
		State:     ClusterNodeState_Stopping,
		Elements:  p.local.getClusterElementsInfo(),
		StartupId: p.startupID,
	})
	if err != nil {
		return err.AddStack(nil)
	}
	p.queueEtcdInfo(infoBuf)
	return nil
}

// handleKey
// 处理etcd的key
func (p *CosmosProcess) handleKey(key string, value []byte, updateOrDelete bool) {
	// key: {nodeName}/version/{version}
	keys := strings.Split(key, "/")
	if len(keys) != 3 {
		p.etcdErrorHandler(NewErrorf(ErrCodeCosmosEtcdInvalidKey, "etcd: Watcher handle key, got invalid key format. key=(%s)", key).AddStack(nil))
		return
	}
	if keys[1] != "version" {
		p.etcdErrorHandler(NewErrorf(ErrCodeCosmosEtcdInvalidKey, "etcd: Watcher handle key, got invalid key version. key=(%s)", key).AddStack(nil))
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
	p.local.coreLog().coreInfo("etcd: Update current version lock. node=(%s),lock=(%s)", nodeName, string(buf))
	// Unmarshal the lock info
	lockInfo := &CosmosNodeVersionLock{}
	if er := json.Unmarshal(buf, lockInfo); er != nil {
		p.etcdErrorHandler(NewErrorf(ErrCodeCosmosEtcdUpdateFailed, "etcd: Update current version lock, got invalid value. node=(%s),err=(%s)", nodeName, er).AddStack(nil))
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
	p.local.coreLog().coreInfo("etcd: Delete current version lock. node=(%s)", nodeName)
	p.cluster.remoteMutex.Lock()
	defer p.cluster.remoteMutex.Unlock()

	remote, has := p.cluster.remoteCosmos[nodeName]
	if !has {
		p.etcdErrorHandler(NewErrorf(ErrCodeCosmosEtcdUpdateFailed, "etcd: Delete current version lock, got invalid node. node=(%s)", nodeName).AddStack(nil))
		return
	}
	remote.etcdDeleteLock()
}

// etcdUpdateClusterVersionNodeInfo - watcher
// 设定节点的版本
func (p *CosmosProcess) etcdUpdateClusterVersionNodeInfo(nodeName string, versionKey string, value []byte) {
	p.local.coreLog().coreInfo("etcd: Update cluster version info. node=(%s),version=(%s)", nodeName, versionKey)
	info := &CosmosNodeVersionInfo{}
	if er := json.Unmarshal(value, info); er != nil {
		p.etcdErrorHandler(NewErrorf(ErrCodeCosmosEtcdUpdateFailed, "etcd: Update cluster version info, got invalid cluster info value. node=(%s),version=(%s),err=(%v)", nodeName, versionKey, er).AddStack(nil))
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
	p.local.coreLog().coreInfo("etcd: Delete cluster version info. node=(%s),version=(%s)", nodeName, versionKey)
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
		return "", "", NewErrorf(ErrCodeCosmosEtcdKeepaliveFailed, "etcd: Failed to marshal cluster version info. err=(%s)", er).AddStack(nil)
	}
	return key, string(infoBuf), nil
}

// etcdErrorHandler
// etcd的错误处理
func (p *CosmosProcess) etcdErrorHandler(err *Error) {
	p.local.Log().Fatal("etcd: CRITICAL ERROR. err=(%s)", err)
}
