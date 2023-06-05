package go_atomos

import (
	"fmt"
	"go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"net"
	"sync"
	"time"
)

// CosmosProcess
// 这个才是进程的主循环。

type CosmosProcess struct {
	mutex sync.RWMutex
	state CosmosProcessState

	// 本地Cosmos节点
	// Local Cosmos Node
	local *CosmosLocal

	// 全局的Cosmos节点
	// Global Cosmos Node
	cluster struct {
		enable bool
		// ETCD
		etcdClient  *clientv3.Client
		etcdVersion int64
		etcdInfoCh  chan []byte
		// GRPC
		serverOption *grpc.ServerOption
		dialOption   *grpc.DialOption
		grpcAddress  string
		grpcServer   *grpc.Server
		grpcListener net.Listener
		// GRPC Implementation
		grpcImpl *atomosRemoteService

		// Cluster Info
		remoteMutex  sync.RWMutex
		remoteCosmos map[string]*CosmosRemote

		remoteTrackCurID uint64
		remoteTrackIDMap map[uint64]*IDTracker
	}
}

type CosmosMainScript interface {
	OnStartup(cosmosMain *CosmosLocal) *Error
	OnShutdown() *Error
}

type CosmosMainGlobalRouter interface {
	GetCosmosNodeName(element, atom string) (string, bool)
	GetCosmosNodeAddress(node string) string
}

type CosmosProcessState int

const (
	CosmosProcessStatePrepare  CosmosProcessState = 0
	CosmosProcessStateStartup  CosmosProcessState = 1
	CosmosProcessStateRunning  CosmosProcessState = 2
	CosmosProcessStateShutdown CosmosProcessState = 3
	CosmosProcessStateOff      CosmosProcessState = 4
)

var sharedCosmosProcess *CosmosProcess
var onceInitSharedCosmosProcess sync.Once

func SharedCosmosProcess() *CosmosProcess {
	return sharedCosmosProcess
}

// InitCosmosProcess 初始化进程
// 该函数只能被调用一次，且必须在进程启动时调用。
func InitCosmosProcess(accessLogFn, errLogFn LoggingFn) {
	onceInitSharedCosmosProcess.Do(func() {
		if IsParentProcess() {
			processIDType = IDType_AppLoader
		} else {
			processIDType = IDType_App
		}
		sharedCosmosProcess = &CosmosProcess{}
		initSharedLoggingAtomos(accessLogFn, errLogFn)
		initCosmosMain(sharedCosmosProcess)
	})
}

// Start 启动进程
// 检查runnable是否合法，再根据配置获取网络监听信息，并尝试监听。
func (p *CosmosProcess) Start(runnable *CosmosRunnable) *Error {
	// Check if in prepare state.
	if err := func() *Error {
		if p == nil {
			return NewError(ErrCosmosProcessHasNotInitialized, "CosmosProcess: Process has not initialized.").
				AddStack(nil)
		}
		p.mutex.Lock()
		defer p.mutex.Unlock()

		if p.state != CosmosProcessStatePrepare {
			return NewError(ErrCosmosProcessHasBeenStarted, "CosmosProcess: Process can only start once.").
				AddStack(nil)
		}
		p.state = CosmosProcessStateStartup
		return nil
	}(); err != nil {
		return err
	}

	// Starting Up.
	if err := func() (err *Error) {
		defer func() {
			if r := recover(); r != nil {
				err = NewError(ErrCosmosProcessOnStartupPanic, "CosmosProcess: Process startups panic.").
					AddPanicStack(p.local, 1, r)
			}
		}()

		// Check if runnable is valid.
		if err = runnable.Check(); err != nil {
			return err.AddStack(nil)
		}

		// If it is a cluster process, try to load networking configuration via etcd, then try to listen.
		if err = p.prepareCluster(runnable); err != nil {
			return err.AddStack(nil)
		}

		// Load runnable.
		if err = p.local.loadOnce(runnable); err != nil {
			return err.AddStack(nil)
		}
		if err = p.local.runnable.mainScript.OnStartup(p.local); err != nil {
			p.mutex.Lock()
			p.state = CosmosProcessStateOff
			p.mutex.Unlock()
			return err
		}

		if err = p.loadedCluster(); err != nil {
			p.mutex.Lock()
			p.state = CosmosProcessStateOff
			p.mutex.Unlock()
			return err
		}

		return nil
	}(); err != nil {
		return err
	}

	p.mutex.Lock()
	p.state = CosmosProcessStateRunning
	p.mutex.Unlock()

	return nil
}

// Stop 停止进程
// 检查进程状态，如果是运行中，则调用OnShutdown，然后关闭网络监听。
func (p *CosmosProcess) Stop() *Error {
	if err := func() *Error {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		switch p.state {
		case CosmosProcessStatePrepare:
			return NewError(ErrCosmosProcessCannotStopPrepareState, "CosmosProcess: Stopping app is preparing.").AddStack(nil)
		case CosmosProcessStateStartup:
			return NewError(ErrCosmosProcessCannotStopStartupState, "CosmosProcess: Stopping app is starting up.").AddStack(nil)
		case CosmosProcessStateRunning:
			p.state = CosmosProcessStateShutdown
			return nil
		case CosmosProcessStateShutdown:
			return NewError(ErrCosmosProcessCannotStopShutdownState, "CosmosProcess: Stopping app is shutting down.").AddStack(nil)
		case CosmosProcessStateOff:
			return NewError(ErrCosmosProcessCannotStopOffState, "CosmosProcess: Stopping app is halt.").AddStack(nil)
		}
		return NewError(ErrCosmosProcessInvalidState, "CosmosProcess: Stopping app is in invalid app state.").AddStack(nil)
	}(); err != nil {
		return err
	}

	err := func() (err *Error) {
		defer func() {
			if r := recover(); r != nil {
				err = NewError(ErrCosmosProcessOnShutdownPanic, "CosmosProcess: Process shutdowns panic.").
					AddPanicStack(p.local, 1, r)
			}
		}()
		if err = p.local.runnable.mainScript.OnShutdown(); err != nil {
			return err
		}
		return p.local.pushKillMail(p.local, true, 0)
	}()

	p.mutex.Lock()
	p.state = CosmosProcessStateOff
	p.mutex.Unlock()

	if err != nil {
		return err
	}
	return nil
}

func (p *CosmosProcess) Self() *CosmosLocal {
	return p.local
}

// prepareCluster 准备集群
func (p *CosmosProcess) prepareCluster(runnable *CosmosRunnable) *Error {
	if runnable.config == nil {
		return NewError(ErrCosmosConfigInvalid, "Cosmos: Config is nil.").AddStack(nil)
	}

	// Initialize the basic information to prevent panic.
	p.cluster.remoteCosmos = map[string]*CosmosRemote{}
	p.cluster.remoteTrackIDMap = map[uint64]*IDTracker{}

	// Check if it is a cluster process.
	cluster := runnable.config.EnableCluster
	if cluster == nil {
		p.cluster.enable = false
		return nil
	}

	// Prepare cluster.
	err := p.prepareAtomosRemote(cluster.EtcdEndpoints, runnable.config.Node, cluster.OptionalPorts)
	if err != nil {
		return err.AddStack(nil)
	}

	return nil
}

// loadedCluster 加载集群
func (p *CosmosProcess) loadedCluster() *Error {
	if !p.cluster.enable {
		return nil
	}

	// Set cluster node info.
	if err := p.etcdSetClusterCosmosNodeInfo(); err != nil {
		return err.AddStack(nil)
	}

	// Set cluster node current version.
	//
	if p.local.runnable.isCurrentVersion {
		if err := p.etcdClusterSetCurrent(p.cluster.etcdClient, p.local.runnable.config.Node, p.cluster.etcdVersion); err != nil {
			return err.AddStack(nil)
		}
	}

	return nil
}

// prepareAtomosRemote 准备Atomos远程
func (p *CosmosProcess) prepareAtomosRemote(endpoints []string, nodeName string, ports []uint32) *Error {
	addrList, er := net.InterfaceAddrs()
	if er != nil {
		return NewErrorf(ErrCosmosRemoteListenFailed, "Cosmos: Failed to get local IP address. err=(%v)", er).AddStack(nil)
	}
	var ip string
	for _, address := range addrList {
		// check for IPv4 address
		if ipNet, ok := address.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ip = ipNet.IP.String()
				break
			}
		}
	}
	if ip == "" {
		return NewError(ErrCosmosRemoteListenFailed, "Cosmos: Failed to get local IP address.").AddStack(nil)
	}

	// Set up a connection to the etcd server.
	cfg := clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	}

	cli, er := clientv3.New(cfg)
	if er != nil {
		return NewErrorf(ErrCosmosEtcdConnectFailed, "Cosmos: Failed to connect etcd. err=(%v)", er).AddStack(nil)
	}

	// Get TLS config if any.
	isTLS, serverOption, dialOption, err := p.etcdGetClusterTLSConfig(cli)
	if err != nil {
		return err.AddStack(nil)
	}

	// Try to get an available port in optionals port list.
	var grpcListenAddress string
	var grpcListener net.Listener
	var grpcServer *grpc.Server
	for _, port := range ports {
		// Try to listen.
		grpcListenAddress = fmt.Sprintf("%s:%d", ip, port)
		listener, er := net.Listen("tcp", grpcListenAddress)
		if er != nil {
			continue
		}
		// Try to start grpc server.
		var svr *grpc.Server
		if isTLS {
			svr = grpc.NewServer(*serverOption)
		} else {
			svr = grpc.NewServer()
		}
		// Register AtomosRemoteService.
		p.cluster.grpcImpl = &atomosRemoteService{
			process: p,
		}
		RegisterAtomosRemoteServiceServer(svr, p.cluster.grpcImpl)
		if err := svr.Serve(listener); err != nil {
			continue
		}
		// Returns available grpc server.
		grpcServer = svr
		grpcListener = listener
		break
	}
	// Check if grpc server is available.
	if grpcServer == nil {
		return NewError(ErrCosmosEtcdGRPCServerFailed, "Cosmos: Failed to start etcd grpc server.").AddStack(nil)
	}

	p.cluster.etcdClient = cli
	p.cluster.etcdVersion = time.Now().Unix()
	p.cluster.etcdInfoCh = make(chan []byte)
	p.cluster.serverOption = serverOption
	p.cluster.dialOption = dialOption
	p.cluster.grpcAddress = grpcListenAddress
	p.cluster.grpcServer = grpcServer
	p.cluster.grpcListener = grpcListener

	if err := p.etcdKeepClusterAlive(cli, nodeName, grpcListenAddress, p.cluster.etcdVersion); err != nil {
		return err.AddStack(nil)
	}

	if err := p.etcdWatchCluster(cli); err != nil {
		return err.AddStack(nil)
	}

	return nil
}

// unloadAtomosRemote 卸载Atomos远程
func (p *CosmosProcess) unloadAtomosRemote() {
	if !p.cluster.enable {
		return
	}
	if err := p.etcdClusterUnsetCurrent(p.cluster.etcdClient, p.local.runnable.config.Node); err != nil {
		// TODO: Log error.
	}
	if err := p.etcdUnsetClusterCosmosNodeInfo(); err != nil {
		// TODO: Log error.
	}
	if p.cluster.etcdClient != nil {
		p.cluster.etcdClient.Close()
		p.cluster.etcdClient = nil
	}
	if p.cluster.grpcServer != nil {
		p.cluster.grpcServer.Stop()
		p.cluster.grpcServer = nil
	}
	if p.cluster.grpcListener != nil {
		p.cluster.grpcListener.Close()
		p.cluster.grpcListener = nil
	}
}

func (p *CosmosProcess) setRemoteTracker(tracker *IDTracker) {
	p.cluster.remoteMutex.Lock()
	p.cluster.remoteTrackCurID += 1
	curID := p.cluster.remoteTrackCurID
	p.cluster.remoteTrackIDMap[curID] = tracker
	p.cluster.remoteMutex.Unlock()

	tracker.id = curID
}

func (p *CosmosProcess) unsetRemoteTracker(id uint64) {
	p.cluster.remoteMutex.Lock()
	delete(p.cluster.remoteTrackIDMap, id)
	p.cluster.remoteMutex.Unlock()
}
