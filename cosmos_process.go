package go_atomos

import (
	"context"
	"fmt"
	"go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"net"
	"os"
	"sync"
	"time"
)

// CosmosProcess
// 这个才是进程的主循环。

type CosmosProcess struct {
	mutex sync.RWMutex
	state CosmosProcessState

	// 进程的日志工具
	// Logging tool of process
	logging *loggingAtomos

	// 本地Cosmos节点
	// Local Cosmos Node
	local *CosmosLocal

	// 全局的Cosmos节点
	// Global Cosmos Node
	cluster struct {
		enable bool
		// ETCD
		etcdClient      *clientv3.Client
		etcdVersion     int64
		etcdInfoCh      chan string
		etcdCancelWatch context.CancelFunc
		// GRPC
		grpcServerOption *grpc.ServerOption
		grpcDialOption   *grpc.DialOption
		grpcAddress      string
		grpcServer       *grpc.Server
		grpcListener     net.Listener
		// GRPC Implementation
		grpcImpl *atomosRemoteService

		// Cluster Info
		remoteMutex  sync.RWMutex
		remoteCosmos map[string]*CosmosRemote

		remoteTrackCurID uint64
		remoteTrackIDMap map[uint64]*IDTracker
	}
}

type CosmosMainGlobalRouter interface {
	GetCosmosNodeName(element, atom string) (string, bool)
	GetCosmosNodeAddress(node string) string
}

// CosmosProcessState
// 进程的状态
type CosmosProcessState int

const (
	CosmosProcessStatePrepare  CosmosProcessState = 0
	CosmosProcessStateStartup  CosmosProcessState = 1
	CosmosProcessStateRunning  CosmosProcessState = 2
	CosmosProcessStateShutdown CosmosProcessState = 3
	CosmosProcessStateOff      CosmosProcessState = 4
)

// newCosmosProcess 创建进程
// 该函数只能被InitCosmosProcess调用。
func newCosmosProcess(cosmosName, cosmosNode string, accessLogFn, errLogFn loggingFn) (*CosmosProcess, *Error) {
	process := &CosmosProcess{}
	if err := process.init(cosmosName, cosmosNode, accessLogFn, errLogFn); err != nil {
		return nil, err.AddStack(nil)
	}
	return process, nil
}

// init 初始化进程
func (p *CosmosProcess) init(cosmosName, cosmosNode string, accessLogFn, errLogFn loggingFn) *Error {
	// Init Info.
	id := &IDInfo{Type: IDType_Cosmos, Cosmos: cosmosName, Node: cosmosNode}

	// Init Logging.
	p.logging = &loggingAtomos{}
	if err := p.logging.init(accessLogFn, errLogFn); err != nil {
		errLogFn(fmt.Sprintf("CosmosProcess: Init logging failed, exitting. err=(%+v)", err))
		return err.AddStack(nil)
	}
	p.logging.PushLogging(id, LogLevel_Info, fmt.Sprintf("CosmosProcess: Launching. pid=(%d)", os.Getpid()))

	// Init CosmosLocal.
	p.local = &CosmosLocal{
		process:               p,
		runnable:              nil,
		atomos:                nil,
		mutex:                 sync.RWMutex{},
		elements:              map[string]*ElementLocal{},
		idFirstSyncCallLocal:  &idFirstSyncCallLocal{},
		idTrackerManager:      &idTrackerManager{},
		messageTrackerManager: &messageTrackerManager{},
	}
	p.local.atomos = NewBaseAtomos(id, LogLevel_Debug, p.local, p.local, p.logging)
	p.local.idFirstSyncCallLocal.init(id)
	p.local.idTrackerManager.init(p.local)
	p.local.messageTrackerManager.init(p.local.atomos, len(p.local.elements))
	if err := p.local.atomos.start(nil); err != nil {
		return err.AddStack(nil)
	}

	// Init Cluster.
	// Initialize the basic information to prevent panic.
	p.cluster.remoteCosmos = map[string]*CosmosRemote{}
	p.cluster.remoteTrackIDMap = map[uint64]*IDTracker{}

	return nil
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

		// 检查runnable是否合法。
		// Check if runnable is valid.
		if err = runnable.Check(); err != nil {
			return err.AddStack(p.local)
		}
		p.local.runnable = runnable

		// 如果是集群进程，尝试通过etcd加载网络配置，再尝试监听。
		// If it is a cluster process, try to load networking configuration via etcd, then try to listen.
		if err = p.prepareCluster(runnable); err != nil {
			p.handleStartUpFailedClusterCleanUp()
			return err.AddStack(p.local)
		}

		// 已经准备好集群的本地节点环境，尝试启动元素（Elements）。
		// The local node environment of the cluster is ready, try to start the elements.
		if err = p.local.trySpawningElements(); err != nil {
			p.handleStartUpFailedClusterCleanUp()
			return err.AddStack(p.local)
		}

		// 尝试将自己设置为current并保持心跳。首先，如果有其它节点的话，将其退出，退出失败也会导致本程序退出。然后把当前进程信息设置到etcd中，并keepalive 。
		// Try to set yourself as current and keepalive. First, if there are other nodes, exit them, and if the exit fails, the program will exit.
		// Then set the current process information to etcd and keepalive.
		if err := p.trySetClusterToCurrentAndKeepalive(); err != nil {
			p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Set cluster to current and keepalive failed. err=(%+v)", err))
			p.handleStartUpFailedLocalCleanUp()
			p.handleStartUpFailedClusterCleanUp()
			return err.AddStack(p.local)
		}

		// 启动主脚本。
		// Start the main script.
		if err = p.local.runnable.mainScript.OnStartUp(p); err != nil {
			p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Main script startup failed. err=(%+v)", err))
			p.handleStartUpFailedLocalCleanUp()
			p.handleStartUpFailedClusterCleanUp()
			return err.AddStack(p.local)
		}

		return nil
	}(); err != nil {
		p.mutex.Lock()
		p.state = CosmosProcessStateOff
		p.mutex.Unlock()
		return err
	}

	p.mutex.Lock()
	p.state = CosmosProcessStateRunning
	p.mutex.Unlock()

	return nil
}

func (p *CosmosProcess) stopFromOtherNode() *Error {
	if err := func() *Error {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		switch p.state {
		case CosmosProcessStatePrepare:
			return NewError(ErrCosmosProcessCannotStopPrepareState, "CosmosProcess: Stopping app is preparing.").AddStack(p.local)
		case CosmosProcessStateStartup:
			return NewError(ErrCosmosProcessCannotStopStartupState, "CosmosProcess: Stopping app is starting up.").AddStack(p.local)
		case CosmosProcessStateRunning:
			p.state = CosmosProcessStateShutdown
			return nil
		case CosmosProcessStateShutdown:
			return NewError(ErrCosmosProcessCannotStopShutdownState, "CosmosProcess: Stopping app is shutting down.").AddStack(p.local)
		case CosmosProcessStateOff:
			return NewError(ErrCosmosProcessCannotStopOffState, "CosmosProcess: Stopping app is halt.").AddStack(p.local)
		}
		return NewError(ErrCosmosProcessInvalidState, "CosmosProcess: Stopping app is in invalid app state.").AddStack(p.local)
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
		defer func() {
			if err := p.local.pushKillMail(p.local, true, 0); err != nil {
				p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Push kill mail failed. err=(%+v)", err))
			}
		}()

		if err := p.tryUnsetCurrentAndUpdateNodeInfo(); err != nil {
			p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Update cluster info failed. err=(%+v)", err))
		}

		if err = p.local.runnable.mainScript.OnShutdown(); err != nil {
			return err
		}

		return nil
	}()

	p.mutex.Lock()
	p.state = CosmosProcessStateOff
	p.mutex.Unlock()

	p.logging.stop()
	return err
}

func (p *CosmosProcess) stopFromOtherNodeAfterResponse() {
	// 关闭集群本地信息
	// Close cluster local info.
	p.unloadClusterLocalNode()

	p.mutex.Lock()
	p.state = CosmosProcessStateOff
	p.mutex.Unlock()

	p.logging.stop()
}

// Stop 停止进程
// 检查进程状态，如果是运行中，则调用OnShutdown，然后关闭网络监听。
func (p *CosmosProcess) Stop() *Error {
	if err := func() *Error {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		switch p.state {
		case CosmosProcessStatePrepare:
			return NewError(ErrCosmosProcessCannotStopPrepareState, "CosmosProcess: Stopping app is preparing.").AddStack(p.local)
		case CosmosProcessStateStartup:
			return NewError(ErrCosmosProcessCannotStopStartupState, "CosmosProcess: Stopping app is starting up.").AddStack(p.local)
		case CosmosProcessStateRunning:
			p.state = CosmosProcessStateShutdown
			return nil
		case CosmosProcessStateShutdown:
			return NewError(ErrCosmosProcessCannotStopShutdownState, "CosmosProcess: Stopping app is shutting down.").AddStack(p.local)
		case CosmosProcessStateOff:
			return NewError(ErrCosmosProcessCannotStopOffState, "CosmosProcess: Stopping app is halt.").AddStack(p.local)
		}
		return NewError(ErrCosmosProcessInvalidState, "CosmosProcess: Stopping app is in invalid app state.").AddStack(p.local)
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
		defer func() {
			if err := p.local.pushKillMail(p.local, true, 0); err != nil {
				p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Push kill mail failed. err=(%+v)", err))
			}
		}()

		if err := p.tryUnsetCurrentAndUpdateNodeInfo(); err != nil {
			p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Update cluster info failed. err=(%+v)", err))
		}

		if err = p.local.runnable.mainScript.OnShutdown(); err != nil {
			return err
		}

		// 关闭集群本地信息
		// Close cluster local info.
		p.unloadClusterLocalNode()

		return nil
	}()

	p.mutex.Lock()
	p.state = CosmosProcessStateOff
	p.mutex.Unlock()

	p.logging.stop()
	return err
}

func (p *CosmosProcess) Self() *CosmosLocal {
	return p.local
}

// prepareCluster 准备集群
func (p *CosmosProcess) prepareCluster(runnable *CosmosRunnable) *Error {
	// Check if it is a cluster process.
	cluster := runnable.config.EnableCluster
	if cluster == nil {
		return nil
	}

	// Prepare cluster local.
	err := p.prepareClusterLocalNode(cluster.EtcdEndpoints, runnable.config.Node, cluster.OptionalPorts)
	if err != nil {
		return err.AddStack(p.local)
	}
	p.cluster.enable = true
	return nil
}

// prepareClusterLocalNode 准备集群本地节点
// 注意顺序，先建立etcd链接，再创建gRPC监听，以避免etcd不认可当前节点的情况，也占用了gRPC资源。
// Notice the order, first establish the etcd link, then create the gRPC listener, to avoid the situation that etcd does not recognize the current node, and also occupy the gRPC resources.
func (p *CosmosProcess) prepareClusterLocalNode(endpoints []string, nodeName string, ports []uint32) *Error {
	p.cluster.etcdVersion = time.Now().Unix()

	// 获取本地IP地址，用于接受集群中其他节点的连接。
	// Get the local IP address, for accepting connections from other nodes in the cluster.
	addrList, er := net.InterfaceAddrs()
	if er != nil {
		return NewErrorf(ErrCosmosRemoteListenFailed, "CosmosProcess: Failed to get local IP address. err=(%v)", er).AddStack(p.local)
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
		return NewError(ErrCosmosRemoteListenFailed, "CosmosProcess: Failed to get local IP address.").AddStack(p.local)
	}
	p.logging.PushLogging(p.local.info, LogLevel_Info, fmt.Sprintf("CosmosProcess: Using IP. ip=(%s)", ip))

	// 建立到etcd服务器的连接。
	// Set up a connection to the etcd server.
	cli, er := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: etcdDialTime * time.Second,
	})
	if er != nil {
		return NewErrorf(ErrCosmosEtcdConnectFailed, "CosmosProcess: Failed to connect etcd. err=(%v)", er).AddStack(p.local)
	}
	p.cluster.etcdClient = cli
	p.cluster.etcdInfoCh = make(chan string)

	// 检查etcd中同一个cosmos的node有多少个version。因为version是用于热更的设计，而不是为了支持多个版本，所以集群同时只应该有不超过两个version。
	// 当检查到当前cosmos的node已经有两个version，且两个version都不是自己时，应该主动退出。
	// Check how many versions of the node of the same cosmos are in etcd. Because the version is designed for hot updates, not to support multiple versions, there should be no more than two versions in the cluster at the same time.
	// When it is found that the current cosmos node already has two versions, and neither of the two versions is itself, it should actively exit.
	// Key and prefix to be used in conditions
	if err := p.etcdStartUpTryLockingVersionNodeLock(cli, nodeName, p.cluster.etcdVersion); err != nil {
		return err.AddStack(p.local)
	}

	// 从etcd获取gRPC TLS配置（如果有）。
	// Get gRPC TLS config from etcd, if any.
	// TODO: Test TLS and non TLS.
	isTLS, serverOption, dialOption, err := p.getClusterTLSConfig(cli)
	if err != nil {
		return err.AddStack(p.local)
	}

	// Try to get an available port in optionals port list for gRPC server listening.
	// 尝试在可选端口列表中获取一个可用端口，用于gRPC服务器侦听。
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
		go func() {
			if err := svr.Serve(listener); err != nil {
				p.logging.PushLogging(p.local.info, LogLevel_Warn, fmt.Sprintf("CosmosProcess: gRPC server has exited. err=(%v)", err))
			}
		}()
		// Returns available grpc server.
		grpcServer = svr
		grpcListener = listener
		break
	}
	// Check if grpc server is available.
	if grpcServer == nil {
		return NewError(ErrCosmosEtcdGRPCServerFailed, "CosmosProcess: Failed to start etcd grpc server.").AddStack(p.local)
	}

	p.cluster.grpcServerOption = serverOption
	p.cluster.grpcDialOption = dialOption
	p.cluster.grpcAddress = grpcListenAddress
	p.cluster.grpcServer = grpcServer
	p.cluster.grpcListener = grpcListener

	// Watch cluster.
	// 先拉取一次集群信息，再检测集群变化。
	if err := p.watchCluster(cli); err != nil {
		p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Failed to watch cluster. err=(%v)", err))
		return err.AddStack(p.local)
	}

	return nil
}

// handleStartUpFailedClusterCleanUp 当准备集群本地失败时，做etcd和gRPC的清理工作。
// When preparing the cluster locally fails, do the cleanup work of etcd and gRPC.
func (p *CosmosProcess) handleStartUpFailedClusterCleanUp() {
	// 删除etcd中的watcher。
	if p.cluster.etcdInfoCh != nil {
		close(p.cluster.etcdInfoCh)
		p.cluster.etcdInfoCh = nil
	}

	// 删除etcd中的key，刪除version锁和node信息。
	if p.cluster.etcdClient != nil {
		// 解锁version锁。
		if err := p.etcdStartUpFailedNodeVersionUnlock(); err != nil {
			p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Failed to unlock version while 'handleStartUpFailedClusterCleanUp'. err=(%v)", err))
		}

		// 删除node信息。
		key := etcdCosmosNodeVersionURI(p.local.runnable.config.Cosmos, p.local.runnable.config.Node, p.cluster.etcdVersion)
		if err := etcdDelete(p.cluster.etcdClient, key); err != nil {
			p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Failed to delete etcd key while 'handleStartUpFailedClusterCleanUp'. err=(%v)", err))
		}

		// 关闭etcd客户端。
		if err := p.cluster.etcdClient.Close(); err != nil {
			p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Failed to close etcd client while 'handleStartUpFailedClusterCleanUp'. err=(%v)", err))
		}
		p.cluster.etcdClient = nil
	}

	// 关闭gRPC服务。
	if p.cluster.grpcServer != nil {
		p.cluster.grpcServer.Stop()
		p.cluster.grpcServer = nil
	}
	// 关闭gRPC监听。
	if p.cluster.grpcListener != nil {
		if err := p.cluster.grpcListener.Close(); err != nil {
			return
		}
		p.cluster.grpcListener = nil
	}
}

// handleStartUpFailedLocalCleanUp 当准备集群本地失败时，如果本地已经加载成功了，就做本地的清理工作。
// When preparing the cluster locally fails, if the local has been loaded successfully, do the local cleanup work.
func (p *CosmosProcess) handleStartUpFailedLocalCleanUp() {
	if err := p.local.pushKillMail(p.local, true, 0); err != nil {
		p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Failed to kill local cosmos. err=(%v)", err))
	}
}

// unloadClusterLocalNode 卸载集群本地节点。
// Unload cluster local node.
func (p *CosmosProcess) unloadClusterLocalNode() {
	// 删除etcd中的watcher。
	if p.cluster.etcdInfoCh != nil {
		close(p.cluster.etcdInfoCh)
		p.cluster.etcdInfoCh = nil
	}

	// 删除etcd中的key，刪除version锁和node信息。
	if p.cluster.etcdClient != nil {
		// 删除version锁。
		if err := p.etcdStoppingNodeVersionUnlock(); err != nil {
			p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Failed to unlock version while 'handleStartUpFailedClusterCleanUp'. err=(%v)", err))
		}

		// 删除node信息。
		key := etcdCosmosNodeVersionURI(p.local.runnable.config.Cosmos, p.local.runnable.config.Node, p.cluster.etcdVersion)
		if err := etcdDelete(p.cluster.etcdClient, key); err != nil {
			p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Failed to delete etcd key while 'handleStartUpFailedClusterCleanUp'. err=(%v)", err))
		}

		// 关闭etcd客户端。
		if err := p.cluster.etcdClient.Close(); err != nil {
			p.logging.PushLogging(p.local.info, LogLevel_Err, fmt.Sprintf("CosmosProcess: Failed to close etcd client while 'handleStartUpFailedClusterCleanUp'. err=(%v)", err))
		}
		p.cluster.etcdClient = nil
	}

	// 关闭gRPC服务。
	if p.cluster.grpcServer != nil {
		p.cluster.grpcServer.Stop()
		p.cluster.grpcServer = nil
	}

	// 关闭gRPC监听。
	if p.cluster.grpcListener != nil {
		if err := p.cluster.grpcListener.Close(); err != nil {
			return
		}
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
