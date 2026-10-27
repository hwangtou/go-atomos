package atomos

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// CosmosProcess is the main process runtime. It owns all mutable configuration
// that was previously package-level global state, enabling isolated instances
// for testing and multi-tenant scenarios.
type CosmosProcess struct {
	mutex sync.RWMutex
	state CosmosProcessState

	startupID uint64

	// app is the owning App, set when the process is created via
	// App.NewCosmosProcess. Nil for processes constructed directly in tests;
	// exitAfterDrain falls back to Stop in that case.
	app *App

	// instanceSeq is a process-global monotonic counter assigning a unique
	// instanceID to every BaseAtomos (every atom spawn, element, cosmos).
	// instanceID distinguishes a respawned atom instance from its predecessor
	// so per-instance reference counting (atomRefState) can tell a stale
	// (pre-respawn) tracker from a current one without pointer/manager rewiring.
	instanceSeq atomic.Uint64

	// waitMu guards waitGraph, the sync-call wait-graph used for deadlock
	// detection. Each edge (executingGoID → waitEdge) records that a
	// goroutine is blocked in PushSyncMessage.waitReply waiting for a target
	// Atom's mailbox goroutine to process a sync mail. A cycle in this graph
	// (e.g. A's mailbox blocked on B while B's mailbox is blocked on A) is a
	// deadlock; PushSyncMessage checks this before enqueue and returns
	// ErrIDFirstSyncCallDeadlock immediately instead of waiting out the
	// timeout. Edges carry the target's ID for readable chain diagnostics and,
	// when syncDeadlockDebug is on, the caller's stack at push time.
	waitMu    sync.Mutex
	waitGraph map[uint64]waitEdge
	syncWaits map[string]int

	// syncChainMu guards syncChains, the ambient cross-node wait chain per
	// mailbox goroutine. While a chain-carrying sync mail executes on a
	// mailbox goroutine, syncChains[goID] holds the chain carried by that
	// mail; a remote sync call made from inside the handler extends it (see
	// BaseRemote.PushSyncMessage). Local wait-graph cycles are covered by
	// waitGraph; this map extends detection across nodes.
	syncChainMu sync.Mutex
	syncChains  map[uint64][]string

	// syncDeadlockDebug enables per-edge stack capture for waitGraph (off by
	// default; costs one runtime.Callers per registered edge when on).
	syncDeadlockDebug bool
	// syncCallDefaultTimeout bounds sync calls that do not pass an explicit
	// ArgBaseAtomosTimeout: it is the local waitReply fallback and the remote
	// client-side deadline base. Default 10s.
	syncCallDefaultTimeout time.Duration
	lifecycleMode          LifecycleMode

	// Per-process mutable settings (formerly package-level vars).
	messageTimeoutTracer  bool
	messageTimeoutDefault time.Duration
	muteKeepaliveLog      bool
	// idTrackerDebug toggles IDTracker debug diagnostics. When false (default),
	// the IDTracker fast path uses a plain refcount with no per-reference
	// allocation/map (optimal for production hot paths). When true, IDTracker
	// maintains a debug map of all outstanding references with caller file:line,
	// so leaks can be enumerated via String(). Consumed by a later milestone
	// (count/diagnosis split); added here to be wired in later.
	idTrackerDebug bool
	// draining is set true by Drain(); the drainWatcher goroutine then waits
	// for active atoms and admitted spawn operations to reach zero before
	// triggering graceful exit.
	draining           bool
	atomSpawnAccepting bool
	atomSpawnPending   int
	atomSpawnDrained   chan struct{}
	// drainCancel, when non-nil, is closed by Stop() to signal drainWatcher to
	// exit promptly. Without it, drainWatcher keeps polling (and logging) after
	// the process has stopped, which races with test/teardown logging.
	drainCancel chan struct{}

	logging   *loggingAtomos
	local     *CosmosLocal
	lifecycle *lifecycleManager

	// debugServer is the optional introspection HTTP server, started by Start
	// when Config.Customize carries configKeyDebugServer and stopped first in
	// Stop. Nil when not configured. Guarded by mutex.
	debugServer *debugServer

	// rpcMetrics records client-observed call-class RPC latencies and
	// per-method counts, exposed through the debug server.
	rpcMetrics *rpcMetrics

	cluster struct {
		enable bool
		// ETCD
		etcdClient      *clientv3.Client
		etcdVersion     int64
		etcdInfoSeq     uint64
		etcdInfoCh      chan etcdInfoUpdate
		etcdExitCh      chan struct{}
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
	}
}

type CosmosMainGlobalRouter interface {
	GetCosmosNodeName(selfNode, element, atom string) (string, bool)
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

// String returns the state name used in logs and the debug server.
func (s CosmosProcessState) String() string {
	switch s {
	case CosmosProcessStatePrepare:
		return "Prepare"
	case CosmosProcessStateStartup:
		return "Startup"
	case CosmosProcessStateRunning:
		return "Running"
	case CosmosProcessStateShutdown:
		return "Shutdown"
	case CosmosProcessStateOff:
		return "Off"
	}
	return "Unknown"
}

// newCosmosProcess 创建进程
// Callers: App.NewCosmosProcess (production/atomostest) and test fixtures.
func newCosmosProcess(cosmosName, cosmosNode string, logging appLogging, args ...any) (*CosmosProcess, *Error) {
	process := &CosmosProcess{}
	if err := process.init(cosmosName, cosmosNode, logging, args...); err != nil {
		return nil, err.AddStack(nil)
	}
	return process, nil
}

// init 初始化进程
func (p *CosmosProcess) init(cosmosName, cosmosNode string, logging appLogging, args ...any) *Error {
	p.startupID = newProcessStartupID()

	// Per-process mutable defaults.
	p.messageTimeoutTracer = true
	p.messageTimeoutDefault = 2 * time.Second
	p.muteKeepaliveLog = true
	p.idTrackerDebug = false
	p.syncDeadlockDebug = false
	p.syncCallDefaultTimeout = 10 * time.Second
	p.lifecycleMode = LifecycleModeLegacyUnsafe
	p.waitGraph = map[uint64]waitEdge{}
	p.syncWaits = map[string]int{}
	p.syncChains = map[uint64][]string{}
	p.rpcMetrics = newRPCMetrics()
	p.atomSpawnAccepting = true

	// Init Info.
	id := &IDInfo{Type: IDType_Cosmos, Cosmos: cosmosName, Node: cosmosNode}

	// Init Logging.
	p.logging = &loggingAtomos{}
	if err := p.logging.init(logging); err != nil {
		logging.WriteErrorLog(fmt.Sprintf("CosmosProcess: Init logging failed, exitting. err=(%+v)", err))
		return err.AddStack(nil)
	}
	p.logging.PushLogging(id, LogLevel_CoreInfo, fmt.Sprintf("CosmosProcess: Launching. pid=(%d)", os.Getpid()))

	// Init CosmosLocal.
	p.local = &CosmosLocal{
		process:  p,
		runnable: nil,
		atomos:   nil,
		mutex:    sync.RWMutex{},
		elements: map[string]*ElementLocal{},
	}
	p.local.atomos = newBaseAtomos(p.local, id, LogLevel_Info, p.local, p.local, p)
	if err := p.local.atomos.start(func() *Error { return nil }); err != nil {
		return err.AddStack(nil)
	}

	// Init Cluster.
	// Initialize the basic information to prevent panic.
	p.cluster.remoteCosmos = map[string]*CosmosRemote{}
	//p.cluster.remoteTrackIDMap = map[uint64]*IDTracker{}

	return nil
}

// allocInstanceID returns a process-unique, monotonically increasing id used
// to tag each BaseAtomos instance. Used by atomRefState to distinguish a
// respawned atom instance from its predecessor.
func (p *CosmosProcess) allocInstanceID() uint64 {
	if p == nil {
		return 0
	}
	return p.instanceSeq.Add(1)
}

// waitEdge records one blocked sync call in the wait-graph: the target
// Atom's mailbox goroutine, the target's ID (for readable chain diagnostics),
// and — when syncDeadlockDebug is enabled — the caller's stack captured at
// push time.
type waitEdge struct {
	targetGoID uint64
	targetID   *IDInfo
	frames     []uintptr
}

// detectDeadlockAndWait checks whether adding a wait edge callerGoID →
// targetGoID would close a cycle in the sync-call wait-graph. If it would, it
// returns true (deadlock) and does NOT register the edge. If it would not, it
// registers the edge atomically (under waitMu) and returns false. The
// check+register is atomic so that two concurrent sync pushes that would form a
// cycle cannot both pass the check (the second sees the first's edge).
//
// callerGoID is the goroutine currently executing PushSyncMessage (about to
// block in waitReply); targetGoID is the target Atom's mailbox goroutine;
// targetID identifies the target Atom for readable diagnostics.
func (p *CosmosProcess) detectDeadlockAndWait(callerGoID, targetGoID uint64, targetID *IDInfo) (deadlock bool) {
	if p == nil {
		return false
	}
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	// Walk the wait chain starting from the target: if the target can already
	// reach the caller, adding caller→target closes a cycle.
	cur := targetGoID
	for cur != 0 {
		if cur == callerGoID {
			return true // cycle: target reaches caller
		}
		edge, ok := p.waitGraph[cur]
		if !ok {
			break // chain ended, no cycle
		}
		cur = edge.targetGoID
	}
	// No cycle: register the edge. Each goroutine has at most one outstanding
	// wait edge at a time (it blocks synchronously), so a plain assignment is
	// correct. Lazy-init the map for CosmosProcess instances constructed
	// outside init() (e.g. bare test fixtures).
	if p.waitGraph == nil {
		p.waitGraph = map[uint64]waitEdge{}
	}
	var frames []uintptr
	if p.syncDeadlockDebug {
		pcs := make([]uintptr, 32)
		if n := runtime.Callers(3, pcs); n > 0 {
			frames = pcs[:n]
		}
	}
	p.waitGraph[callerGoID] = waitEdge{targetGoID: targetGoID, targetID: targetID, frames: frames}
	return false
}

// removeWaitEdge removes the wait edge for the given executing goroutine. Called
// after waitReply returns (success, timeout, or error) so the graph does not
// retain stale edges.
func (p *CosmosProcess) removeWaitEdge(executingGoID uint64) {
	if p == nil {
		return
	}
	p.waitMu.Lock()
	delete(p.waitGraph, executingGoID)
	p.waitMu.Unlock()
}

func syncChainLink(id *IDInfo) string {
	if id == nil {
		return "NilID"
	}
	return fmt.Sprintf("%s#%d", id.Info(), id.InstanceId)
}

func (p *CosmosProcess) beginSyncWait(id *IDInfo) string {
	if p == nil || id == nil {
		return ""
	}
	key := syncChainLink(id)
	p.waitMu.Lock()
	if p.syncWaits == nil {
		p.syncWaits = map[string]int{}
	}
	p.syncWaits[key]++
	p.waitMu.Unlock()
	return key
}

func (p *CosmosProcess) endSyncWait(key string) {
	if p == nil || key == "" {
		return
	}
	p.waitMu.Lock()
	if p.syncWaits[key] <= 1 {
		delete(p.syncWaits, key)
	} else {
		p.syncWaits[key]--
	}
	p.waitMu.Unlock()
}

func (p *CosmosProcess) isSyncWaiting(id *IDInfo) bool {
	if p == nil || id == nil {
		return false
	}
	p.waitMu.Lock()
	waiting := p.syncWaits[syncChainLink(id)] > 0
	p.waitMu.Unlock()
	return waiting
}

// waitChain returns a human-readable description of the wait chain from `from`
// to `to` (inclusive), e.g. "g42→Atom(node::Elem::a)[g7]→Atom(node::Elem::b)[g42 (cycle)]".
// Caller must hold no lock; this acquires waitMu briefly. Only meaningful once
// a cycle is already confirmed.
func (p *CosmosProcess) waitChain(from, to uint64) string {
	if p == nil {
		return ""
	}
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	var b strings.Builder
	fmt.Fprintf(&b, "g%d", from)
	cur := from
	for cur != 0 {
		edge, ok := p.waitGraph[cur]
		if !ok {
			break
		}
		fmt.Fprintf(&b, "→%s[g%d]", edge.targetID.Info(), edge.targetGoID)
		cur = edge.targetGoID
		if cur == to {
			b.WriteString(" (cycle)")
			break
		}
	}
	return b.String()
}

// waitChainStacks renders the captured caller stacks along the wait chain from
// `from` toward `to`, one block per edge that has frames. Returns "" when
// syncDeadlockDebug is off (no frames captured).
func (p *CosmosProcess) waitChainStacks(from, to uint64) string {
	if p == nil {
		return ""
	}
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	var b strings.Builder
	cur := from
	for cur != 0 {
		edge, ok := p.waitGraph[cur]
		if !ok {
			break
		}
		if len(edge.frames) > 0 {
			fmt.Fprintf(&b, "\n  goroutine %d waiting for %s[g%d] at:", cur, edge.targetID.Info(), edge.targetGoID)
			renderFrames(&b, edge.frames)
		}
		if edge.targetGoID == to {
			break
		}
		cur = edge.targetGoID
	}
	return b.String()
}

// renderFrames appends a "function\n    file:line" block per frame.
func renderFrames(b *strings.Builder, frames []uintptr) {
	iter := runtime.CallersFrames(frames)
	for {
		f, more := iter.Next()
		if f.Function != "" {
			fmt.Fprintf(b, "\n    %s\n        %s:%d", f.Function, f.File, f.Line)
		}
		if !more {
			return
		}
	}
}

// setSyncChain records `chain` as the ambient cross-node wait chain for the
// goroutine executing a chain-carrying sync mail. Called by the mailbox before
// dispatching the mail; paired with clearSyncChain.
func (p *CosmosProcess) setSyncChain(goID uint64, chain []string) {
	if p == nil || goID == 0 || len(chain) == 0 {
		return
	}
	p.syncChainMu.Lock()
	if p.syncChains == nil {
		p.syncChains = map[uint64][]string{}
	}
	p.syncChains[goID] = chain
	p.syncChainMu.Unlock()
}

// clearSyncChain removes the ambient chain for a goroutine after its mail
// finished executing.
func (p *CosmosProcess) clearSyncChain(goID uint64) {
	if p == nil || goID == 0 {
		return
	}
	p.syncChainMu.Lock()
	delete(p.syncChains, goID)
	p.syncChainMu.Unlock()
}

// currentSyncChain returns the ambient cross-node wait chain for the goroutine
// executing a chain-carrying sync mail, or nil. The returned slice is shared
// (treat as read-only).
func (p *CosmosProcess) currentSyncChain(goID uint64) []string {
	if p == nil || goID == 0 {
		return nil
	}
	p.syncChainMu.Lock()
	defer p.syncChainMu.Unlock()
	return p.syncChains[goID]
}

// syncCallTimeout returns the effective timeout for a sync call: an explicit
// positive timeout wins; otherwise the process-wide default (10s when unset).
func (p *CosmosProcess) syncCallTimeout(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	if p == nil || p.syncCallDefaultTimeout <= 0 {
		return 10 * time.Second
	}
	return p.syncCallDefaultTimeout
}

// Start 启动进程
// 检查runnable是否合法，再根据配置获取网络监听信息，并尝试监听。
func (p *CosmosProcess) Start(runnable *CosmosRunnable) error {
	// Check if in prepare state.
	if err := func() *Error {
		if p == nil {
			return NewError(ErrCodeCosmosProcessHasNotInitialized, "CosmosProcess: Process has not initialized.").
				AddStack(nil)
		}
		p.mutex.Lock()
		defer p.mutex.Unlock()

		if p.state != CosmosProcessStatePrepare {
			return NewError(ErrCodeCosmosProcessHasBeenStarted, "CosmosProcess: Process can only start once.").
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
				err = NewError(ErrCodeCosmosProcessOnStartupPanic, "CosmosProcess: Process startups panic.").
					AddPanicStack(p.local, 1, r)
			}
		}()

		// 检查runnable是否合法。
		// Check if runnable is valid.
		if err = asError(runnable.Check()); err != nil {
			return err.AddStack(p.local)
		}
		p.local.runnable = runnable

		// Apply per-process settings carried by the runnable. Unset options
		// keep the CosmosProcess defaults established in init.
		if runnable.messageTimeoutTracer != nil {
			p.messageTimeoutTracer = *runnable.messageTimeoutTracer
			p.messageTimeoutDefault = runnable.messageTimeoutDefault
		}
		if runnable.muteKeepaliveLog != nil {
			p.muteKeepaliveLog = *runnable.muteKeepaliveLog
		}
		if runnable.idTrackerDebug != nil {
			p.idTrackerDebug = *runnable.idTrackerDebug
		}
		if runnable.syncDeadlockDebug != nil {
			p.syncDeadlockDebug = *runnable.syncDeadlockDebug
		}
		if runnable.syncCallDefaultTimeout > 0 {
			p.syncCallDefaultTimeout = runnable.syncCallDefaultTimeout
		}
		p.lifecycleMode = runnable.lifecycleMode
		if p.lifecycleMode == LifecycleModePerAtomStrict {
			p.lifecycle = newLifecycleManager(p)
			p.lifecycle.startJanitor()
		}

		// 启动时初始化脚本。
		if err := p.mainScriptOnBootProtect(); err != nil {
			p.local.coreLog().coreFatal("CosmosProcess: Main script boot failed. err=(%s)", err.Message)
			return err.AddStack(p.local)
		}

		// 如果是集群进程，尝试通过etcd加载网络配置，再尝试监听。
		// If it is a cluster process, try to load networking configuration via etcd, then try to listen.
		if err = p.prepareCluster(runnable); err != nil {
			p.local.coreLog().coreFatal("CosmosProcess: Prepare cluster failed. err=(%s)", err.Message)
			return err.AddStack(p.local)
		}
		p.local.coreLog().coreInfo("CosmosProcess: Checkpoint - prepareCluster")
		if p.lifecycle != nil {
			if err := p.lifecycle.startWatch(); err != nil {
				return err.AddStack(p.local)
			}
		}

		// 已经准备好集群的本地节点环境，尝试启动元素（Elements）。
		// The local node environment of the cluster is ready, try to start the elements.
		if err = p.local.trySpawningElements(); err != nil {
			p.local.coreLog().coreFatal("CosmosProcess: Spawn elements failed. err=(%s)", err.Message)
			return err.AddStack(p.local)
		}
		p.local.coreLog().coreInfo("CosmosProcess: Checkpoint - trySpawningElements.")

		// 尝试将自己设置为current并保持心跳。首先，如果有其它节点的话，将其退出，退出失败也会导致本程序退出。然后把当前进程信息设置到etcd中，并keepalive 。
		// Try to set yourself as current and keepalive. First, if there are other nodes, exit them, and if the exit fails, the program will exit.
		// Then set the current process information to etcd and keepalive.
		if err := p.trySettingClusterToCurrentAndKeepalive(); err != nil {
			p.local.coreLog().coreFatal("CosmosProcess: Set cluster to current and keepalive failed. err=(%s)", err.Message)
			return err.AddStack(p.local)
		}
		p.local.coreLog().coreInfo("CosmosProcess: Checkpoint - trySettingClusterToCurrentAndKeepalive")

		// 启动主脚本。
		// Start the main script.
		if err = p.mainScriptOnStartUpProtect(); err != nil {
			p.local.coreLog().coreFatal("CosmosProcess: Main script startup failed. err=(%s)", err.Message)
			return err.AddStack(p.local)
		}
		p.local.coreLog().coreInfo("CosmosProcess: Checkpoint - mainScriptOnStartUpProtect")

		return nil
	}(); err != nil {
		p.local.coreLog().coreError("CosmosProcess: Start failed. err=(%v)", err)
		p.lifecycle.close()
		p.handleStartUpFailedLocalCleanUp()
		p.handleStartUpFailedClusterCleanUp()
		p.mutex.Lock()
		p.state = CosmosProcessStateOff
		p.mutex.Unlock()
		p.logging.stop()
		return err.AddStack(p.local)
	}

	p.mutex.Lock()
	p.state = CosmosProcessStateRunning
	// Optional introspection server; runs until Stop. A listen failure is
	// logged inside and leaves the process running without introspection.
	if p.local.runnable != nil && p.local.runnable.config != nil {
		if addr := debugServerAddr(p.local.runnable.config.Customize); addr != "" {
			p.debugServer = startDebugServer(p, addr)
		}
	}
	p.mutex.Unlock()

	return nil
}

func (p *CosmosProcess) stopFromOtherNode() *Error {
	if err := func() *Error {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		switch p.state {
		case CosmosProcessStatePrepare:
			return NewError(ErrCodeCosmosProcessCannotStopPrepareState, "CosmosProcess: Stopping app is preparing.").AddStack(p.local)
		case CosmosProcessStateStartup:
			return NewError(ErrCodeCosmosProcessCannotStopStartupState, "CosmosProcess: Stopping app is starting up.").AddStack(p.local)
		case CosmosProcessStateRunning:
			p.state = CosmosProcessStateShutdown
			p.atomSpawnAccepting = false
			// Signal drainWatcher (if running) to exit promptly so its polling
			// goroutine does not outlive the process and race with teardown.
			if p.drainCancel != nil {
				close(p.drainCancel)
				p.drainCancel = nil
			}
			return nil
		case CosmosProcessStateShutdown:
			return NewError(ErrCodeCosmosProcessCannotStopShutdownState, "CosmosProcess: Stopping app is shutting down.").AddStack(p.local)
		case CosmosProcessStateOff:
			return NewError(ErrCodeCosmosProcessCannotStopOffState, "CosmosProcess: Stopping app is halt.").AddStack(p.local)
		}
		return NewError(ErrCodeCosmosProcessInvalidState, "CosmosProcess: Stopping app is in invalid app state.").AddStack(p.local)
	}(); err != nil {
		return err.AddStack(p.local)
	}

	err := func() (err *Error) {
		defer func() {
			if r := recover(); r != nil {
				err = NewError(ErrCodeCosmosProcessOnShutdownPanic, "CosmosProcess: Process shutdowns panic.").
					AddPanicStack(p.local, 1, r)
			}
		}()
		defer func() {
			ext := []ArgsForBaseAtomos{ArgBaseAtomosWaitKilled()}
			if err := p.local.atomos.PushKillMail(p.local, ext...); err != nil {
				p.local.coreLog().coreFatal("CosmosProcess: Push kill mail failed. err=(%+v)", err)
			}
		}()

		if err := p.tryUnsettingCurrentAndUpdateNodeInfo(); err != nil {
			p.local.coreLog().coreFatal("CosmosProcess: Update cluster info failed. err=(%s)", err.Message)
		}

		if err = p.mainScriptOnShutdownProtect(); err != nil {
			return err.AddStack(p.local)
		}

		return nil
	}()

	return err
}

func (p *CosmosProcess) stopFromOtherNodeAfterResponse() {
	// 关闭集群本地信息
	// Close cluster local info.
	p.unloadClusterLocalNode()

	p.mutex.Lock()
	p.state = CosmosProcessStateOff
	if p.debugServer != nil {
		p.debugServer.stop()
		p.debugServer = nil
	}
	p.mutex.Unlock()
	p.local.coreLog().coreInfo("CosmosProcess: stopFromOtherNodeAfterResponse.")

	p.logging.stop()
}

// Drain tuning constants. Never mutated at runtime (verified across the repo),
// so they are constants rather than package-level variables.
const (
	drainCheckInterval   = 5 * time.Second
	drainDefaultDeadline = 1 * time.Hour
)

// isDraining reports whether the process has entered drain mode.
func (p *CosmosProcess) isDraining() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.draining
}

func (p *CosmosProcess) beginAtomSpawn() *Error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if !p.atomSpawnAccepting {
		if p.draining {
			return NewError(ErrCodeCosmosNodeDraining, "CosmosProcess: Atom spawn rejected, node is draining.").AddStack(nil)
		}
		return NewError(ErrCodeAtomosIsStopping, "CosmosProcess: Atom spawn rejected, process is stopping.").AddStack(nil)
	}
	if p.atomSpawnPending == 0 {
		p.atomSpawnDrained = make(chan struct{})
	}
	p.atomSpawnPending++
	return nil
}

func (p *CosmosProcess) endAtomSpawn() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.atomSpawnPending <= 0 {
		return
	}
	p.atomSpawnPending--
	if p.atomSpawnPending == 0 && p.atomSpawnDrained != nil {
		close(p.atomSpawnDrained)
		p.atomSpawnDrained = nil
	}
}

func (p *CosmosProcess) atomSpawnPendingSnapshot() (int, <-chan struct{}) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.atomSpawnPending, p.atomSpawnDrained
}

// Drain enters drain mode: stop accepting new atoms, keep serving existing
// ones, and exit gracefully once all atoms and admitted spawns finish (or the
// deadline elapses).
//
// deadline <= 0 means use the default (1h). The drain runs in a background
// goroutine; this method returns immediately after marking the state.
func (p *CosmosProcess) Drain(deadline time.Duration) error {
	if p == nil {
		return NewError(ErrCodeCosmosProcessHasNotInitialized, "CosmosProcess: Process has not initialized.").AddStack(nil)
	}
	p.mutex.Lock()
	if p.state != CosmosProcessStateRunning {
		state := p.state
		p.mutex.Unlock()
		return NewErrorf(ErrCodeCosmosProcessInvalidState, "CosmosProcess: Drain requires Running state, got=(%v).", state).AddStack(nil)
	}
	if p.draining {
		p.mutex.Unlock()
		p.local.coreLog().coreInfo("CosmosProcess: Already draining.")
		return nil
	}
	p.draining = true
	p.atomSpawnAccepting = false
	p.drainCancel = make(chan struct{})
	// drainCancel 会被 Stop 并发地 close 并置 nil(锁内)。watcher 里再读这个字段
	// 会与置 nil 构成数据竞争,因此在锁内快照,按参数传给 watcher。
	drainCancel := p.drainCancel
	p.mutex.Unlock()

	if deadline <= 0 {
		deadline = drainDefaultDeadline
	}
	p.local.coreLog().coreInfo("CosmosProcess: Drain started. deadline=(%v)", deadline)

	if p.cluster.enable {
		if err := p.updateNodeState(ClusterNodeState_Draining); err != nil {
			p.local.coreLog().coreError("CosmosProcess: Drain failed to broadcast Draining state via etcd. err=(%v)", err)
		}
	}

	go p.drainWatcher(deadline, drainCancel)
	return nil
}

// drainWatcher polls active atoms and also listens for admitted spawns to
// publish or clean up. drainCancel 是 Drain 时快照的取消通道:Stop 关闭它即退出,
// 不直接读 p.drainCancel(该字段会被 Stop 并发置 nil)。
func (p *CosmosProcess) drainWatcher(deadline time.Duration, drainCancel chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			p.local.coreLog().coreError("CosmosProcess: drainWatcher panicked. reason=(%v)", r)
		}
	}()
	deadlineTimer := time.NewTimer(deadline)
	defer deadlineTimer.Stop()
	ticker := time.NewTicker(drainCheckInterval)
	defer ticker.Stop()

	for {
		// Read pending before active: admission is closed, so a zero pending
		// snapshot guarantees every admitted publication precedes the atom count.
		pending, drained := p.atomSpawnPendingSnapshot()
		active := p.countActiveAtoms()
		select {
		case <-drainCancel:
			p.local.coreLog().coreInfo("CosmosProcess: Drain cancelled by Stop(). Exiting drainWatcher.")
			return
		case <-ticker.C:
			p.local.coreLog().coreInfo("CosmosProcess: Drain polling. active_atoms=(%d),pending_spawns=(%d)", active, pending)
		case <-drained:
		case <-deadlineTimer.C:
			p.local.coreLog().coreInfo("CosmosProcess: Drain deadline reached with %d active atoms and %d pending spawns. Force exiting.", active, pending)
			p.exitAfterDrain()
			return
		}
		if active == 0 && pending == 0 {
			p.local.coreLog().coreInfo("CosmosProcess: Drain complete, all atoms and admitted spawns finished. Exiting.")
			p.exitAfterDrain()
			return
		}
	}
}

// countActiveAtoms sums GetActiveAtomsNum across all spawned elements.
func (p *CosmosProcess) countActiveAtoms() int {
	total := 0
	p.local.mutex.RLock()
	for _, elem := range p.local.elements {
		total += elem.GetActiveAtomsNum()
	}
	p.local.mutex.RUnlock()
	return total
}

// exitAfterDrain triggers graceful shutdown via the app's exit channel.
func (p *CosmosProcess) exitAfterDrain() {
	if p.app != nil {
		p.app.ExitApp()
	} else {
		// No app context (e.g. test); stop directly.
		p.Stop()
	}
}

// updateNodeState writes the given ClusterNodeState to etcd so other nodes
// observe it via watch and adjust routing. No-op when clustering is disabled.
func (p *CosmosProcess) updateNodeState(state ClusterNodeState) *Error {
	if !p.cluster.enable {
		return nil
	}
	_, infoBuf, err := p.etcdNodeVersion(p.local.runnable.config.Node, p.cluster.etcdVersion, &CosmosNodeVersionInfo{
		Node:      p.local.runnable.config.Node,
		Address:   p.cluster.grpcAddress,
		Id:        p.local.GetIDInfo(),
		State:     state,
		Elements:  p.local.getClusterElementsInfo(),
		StartupId: p.startupID,
	})
	if err != nil {
		return err.AddStack(nil)
	}
	p.queueEtcdInfo(infoBuf)
	return nil
}

func (p *CosmosProcess) queueEtcdInfo(info string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	infoCh := p.cluster.etcdInfoCh
	if infoCh == nil {
		return
	}
	p.cluster.etcdInfoSeq++
	updated := etcdInfoUpdate{seq: p.cluster.etcdInfoSeq, info: info}
	select {
	case infoCh <- updated:
		return
	default:
	}
	// The production channel has capacity one. Replace an unconsumed value so
	// the publisher always observes the newest complete node state.
	select {
	case <-infoCh:
	default:
	}
	select {
	case infoCh <- updated:
	default:
	}
}

func (p *CosmosProcess) closeEtcdInfoChannel() <-chan struct{} {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.cluster.etcdCancelWatch != nil {
		p.cluster.etcdCancelWatch()
		p.cluster.etcdCancelWatch = nil
	}
	if p.cluster.etcdInfoCh == nil {
		return nil
	}
	close(p.cluster.etcdInfoCh)
	p.cluster.etcdInfoCh = nil
	return p.cluster.etcdExitCh
}

// Stop 停止进程
// 检查进程状态，如果是运行中，则调用OnShutdown，然后关闭网络监听。
func (p *CosmosProcess) Stop() error {
	// Guard against a nil receiver — a process that was never initialized
	// would panic on Stop. Return an error instead.
	if p == nil {
		return NewError(ErrCodeCosmosProcessHasNotInitialized, "CosmosProcess: Process has not initialized.").AddStack(nil)
	}
	if err := func() *Error {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		switch p.state {
		case CosmosProcessStatePrepare:
			return NewError(ErrCodeCosmosProcessCannotStopPrepareState, "CosmosProcess: Stopping app is preparing.").AddStack(p.local)
		case CosmosProcessStateStartup:
			return NewError(ErrCodeCosmosProcessCannotStopStartupState, "CosmosProcess: Stopping app is starting up.").AddStack(p.local)
		case CosmosProcessStateRunning:
			p.state = CosmosProcessStateShutdown
			p.atomSpawnAccepting = false
			// Signal drainWatcher (if running) to exit promptly so its polling
			// goroutine does not outlive the process and race with teardown.
			if p.drainCancel != nil {
				close(p.drainCancel)
				p.drainCancel = nil
			}
			return nil
		case CosmosProcessStateShutdown:
			return NewError(ErrCodeCosmosProcessCannotStopShutdownState, "CosmosProcess: Stopping app is shutting down.").AddStack(p.local)
		case CosmosProcessStateOff:
			return NewError(ErrCodeCosmosProcessCannotStopOffState, "CosmosProcess: Stopping app is halt.").AddStack(p.local)
		}
		return NewError(ErrCodeCosmosProcessInvalidState, "CosmosProcess: Stopping app is in invalid app state.").AddStack(p.local)
	}(); err != nil {
		return err
	}

	defer p.lifecycle.close()

	// Stop the debug server first: no new introspection requests once
	// shutdown begins (and /healthz already reports unhealthy in Shutdown
	// state).
	func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		if p.debugServer != nil {
			p.debugServer.stop()
			p.debugServer = nil
		}
	}()

	err := func() (err *Error) {
		defer func() {
			if r := recover(); r != nil {
				err = NewError(ErrCodeCosmosProcessOnShutdownPanic, "CosmosProcess: Process shutdowns panic.").
					AddPanicStack(p.local, 1, r)
			}
		}()
		defer func() {
			ext := []ArgsForBaseAtomos{ArgBaseAtomosWaitKilled()}
			if err := p.local.atomos.PushKillMail(p.local, ext...); err != nil {
				p.local.coreLog().coreFatal("CosmosProcess: Push kill mail failed. err=(%+v)", err)
			}
		}()

		if err := p.tryUnsettingCurrentAndUpdateNodeInfo(); err != nil {
			p.local.coreLog().coreFatal("CosmosProcess: Update cluster info failed. err=(%s)", err.Message)
		}

		if err = p.mainScriptOnShutdownProtect(); err != nil {
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
	p.local.coreLog().coreInfo("CosmosProcess: Stopping app is off.")

	// 退出卫生:清空等待图残留边。被阻塞的等待方会经由 kill/stop 回复或超时
	// 唤醒,removeWaitEdge 对缺失键是安全的。
	p.waitMu.Lock()
	p.waitGraph = map[uint64]waitEdge{}
	p.syncWaits = map[string]int{}
	p.waitMu.Unlock()

	<-time.After(100 * time.Millisecond)
	p.logging.stop()
	// Wait for the mailbox loop goroutines to fully exit: their deferred final
	// log runs after the stop acknowledgment, and returning before they exit
	// leaves late log writes racing with whatever the caller does next.
	p.local.atomos.mailbox.waitExit(5 * time.Second)
	p.logging.logBox.waitExit(5 * time.Second)
	return errOrNil(err)
}

func (p *CosmosProcess) mainScriptOnBootProtect() (err *Error) {
	defer func() {
		if r := recover(); r != nil {
			err = NewError(ErrCodeCosmosProcessOnStartupPanic, "CosmosProcess: Process boots panic.").AddPanicStack(p.local, 3, r)
			p.local.coreLog().coreFatal("CosmosProcess: Main script boot failed. err=(%+v)", err)
		}
	}()
	if err = asError(p.local.runnable.mainScript.OnBoot(p)); err != nil {
		return err.AddStack(p.local)
	}
	return nil
}

func (p *CosmosProcess) mainScriptOnStartUpProtect() (err *Error) {
	defer func() {
		if r := recover(); r != nil {
			err = NewError(ErrCodeCosmosProcessOnStartupPanic, "CosmosProcess: Process startups panic.").AddPanicStack(p.local, 3, r)
			p.local.coreLog().coreFatal("CosmosProcess: Main script startup failed. err=(%+v)", r)
		}
	}()
	if err = asError(p.local.runnable.mainScript.OnStartUp(p)); err != nil {
		return err.AddStack(p.local)
	}
	return nil
}

func (p *CosmosProcess) mainScriptOnShutdownProtect() (err *Error) {
	defer func() {
		if r := recover(); r != nil {
			err = NewError(ErrCodeCosmosProcessOnShutdownPanic, "CosmosProcess: Process shutdowns panic.").AddPanicStack(p.local, 3, r)
			p.local.coreLog().coreFatal("CosmosProcess: Main script shutdown failed. err=(%+v)", r)
		}
	}()
	if err = asError(p.local.runnable.mainScript.OnShutdown()); err != nil {
		return err.AddStack(p.local)
	}
	return nil
}

func (p *CosmosProcess) Self() *CosmosLocal {
	// Guard against a nil receiver — callers may hold a nil *CosmosProcess if
	// the process was never initialized.
	if p == nil {
		return nil
	}
	return p.local
}

// IsRunning returns true if the process has completed startup and is in the
// Running state. Safe for concurrent use.
// Returns false if the process is nil, not yet started, or shutting down.
func (p *CosmosProcess) IsRunning() bool {
	if p == nil {
		return false
	}
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.state == CosmosProcessStateRunning
}

// IsHealthy reports whether the process is running and its cluster registration
// and watch are healthy. Standalone processes have no etcd health dependency.
func (p *CosmosProcess) IsHealthy() bool {
	return p.IsRunning() && p.EtcdClusterHealthy()
}

// recoveryMiddleware returns a gRPC unary interceptor that recovers panics in
// remote-service handlers and reports them through this process's logging.
// It replaces the former package-level RecoveryMiddleware, which depended on
// the deleted sharedCosmosProcess singleton.
func (p *CosmosProcess) recoveryMiddleware() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				if p != nil {
					// Recover() needs a SelfID; use the local cosmos as the ID for consistent logging.
					Recover(p.Self())
				} else {
					log.Printf("CosmosProcess: Recovered from gRPC panic. req=(%+v),info=(%+v),recovery=(%v),stack=(%s)", req, info.FullMethod, r, string(debug.Stack()))
				}
				err = status.Errorf(codes.Internal, "CosmosProcess: gRPC error. req=(%+v),info=(%+v),recovery=(%v)", req, info.FullMethod, r)
			}
		}()
		return handler(ctx, req)
	}
}

// prepareCluster 准备集群
func (p *CosmosProcess) prepareCluster(runnable *CosmosRunnable) *Error {
	// Check if it is a cluster process.
	cluster := runnable.config.EnableCluster
	if cluster == nil || !cluster.Enable {
		if p.lifecycleMode == LifecycleModePerAtomStrict {
			return NewError(ErrCodeFencingRequired, "CosmosProcess: PerAtomStrict requires an enabled etcd cluster")
		}
		return nil
	}

	// Prepare cluster local.
	err := p.prepareClusterLocalNode(cluster.EtcdEndpoints, runnable.config.Node, cluster.OptionalPorts, cluster.EnableCert)
	if err != nil {
		return err.AddStack(p.local)
	}
	p.local.coreLog().coreInfo("CosmosProcess: Cluster prepared.")
	p.cluster.enable = true
	return nil
}

// prepareClusterLocalNode 准备集群本地节点
// 注意顺序，先建立etcd链接，再创建gRPC监听，以避免etcd不认可当前节点的情况，也占用了gRPC资源。
// Notice the order, first establish the etcd link, then create the gRPC listener, to avoid the situation that etcd does not recognize the current node, and also occupy the gRPC resources.
func (p *CosmosProcess) prepareClusterLocalNode(endpoints []string, nodeName string, ports []int32, cert *CertConfig) *Error {
	p.cluster.etcdVersion = time.Now().UnixNano()

	// 获取本地IP地址，用于接受集群中其他节点的连接。
	// An explicit _cluster_advertise_ip override wins (containers, loopback
	// test fixtures); otherwise use the first non-loopback IPv4.
	// Get the local IP address, for accepting connections from other nodes in the cluster.
	var ip string
	if p.local.runnable != nil && p.local.runnable.config != nil {
		if v, ok := p.local.runnable.config.Customize[configKeyClusterAdvertiseIP]; ok {
			if candidate := net.ParseIP(strings.TrimSpace(string(v))); candidate != nil {
				ip = candidate.String()
			}
		}
	}
	if ip == "" {
		addrList, er := net.InterfaceAddrs()
		if er != nil {
			return NewErrorf(ErrCodeCosmosRemoteListenFailed, "CosmosProcess: Failed to get local IP address. err=(%v)", er).AddStack(p.local)
		}
		for _, address := range addrList {
			// check for IPv4 address
			if ipNet, ok := address.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				if ipNet.IP.To4() != nil {
					ip = ipNet.IP.String()
					break
				}
			}
		}
	}
	if ip == "" {
		return NewError(ErrCodeCosmosRemoteListenFailed, "CosmosProcess: Failed to get local IP address.").AddStack(p.local)
	}
	p.local.coreLog().coreInfo("CosmosProcess: Using IP. ip=(%s)", ip)

	// 建立到etcd服务器的连接。
	// Set up a connection to the etcd server.
	cli, er := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: etcdDialTime * time.Second,
	})
	if er != nil {
		return NewErrorf(ErrCodeCosmosEtcdConnectFailed, "CosmosProcess: Failed to connect etcd. err=(%v)", er).AddStack(p.local)
	}
	p.cluster.etcdClient = cli
	p.cluster.etcdInfoCh = make(chan etcdInfoUpdate, 1)

	// 检查etcd中同一个cosmos的node有多少个version。因为version是用于热更的设计，而不是为了支持多个版本，所以集群同时只应该有不超过两个version。
	// 当检查到当前cosmos的node已经有两个version，且两个version都不是自己时，应该主动退出。
	// Check how many versions of the node of the same cosmos are in etcd. Because the version is designed for hot updates, not to support multiple versions, there should be no more than two versions in the cluster at the same time.
	// When it is found that the current cosmos node already has two versions, and neither of the two versions is itself, it should actively exit.
	// Key and prefix to be used in conditions
	if err := p.etcdStartUpTryLockingVersionNodeLock(cli, nodeName, p.cluster.etcdVersion); err != nil {
		return err.AddStack(p.local)
	}

	var isTLS bool
	var serverOption *grpc.ServerOption
	var dialOption *grpc.DialOption
	var err *Error
	if cert != nil {
		isTLS, serverOption, dialOption, err = getClusterTLSConfigFromFiles(cert)
	} else {
		isTLS, serverOption, dialOption, err = p.getClusterTLSConfig(cli)
	}
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
		listener, er := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if er != nil {
			continue
		}
		// Try to start grpc server.
		var svr *grpc.Server
		var publicOption = []grpc.ServerOption{
			grpc.InitialWindowSize(grpcServerInitialWindowSize),
			grpc.InitialConnWindowSize(grpcServerInitialConnWindowSize),
			grpc.WriteBufferSize(grpcServerWriteBufferSize),
			grpc.ReadBufferSize(grpcServerReadBufferSize),
		}
		if isTLS {
			opts := []grpc.ServerOption{
				*serverOption,
				grpc.UnaryInterceptor(p.recoveryMiddleware()),
			}
			opts = append(opts, publicOption...)
			svr = grpc.NewServer(opts...)
		} else {
			opts := []grpc.ServerOption{
				grpc.UnaryInterceptor(p.recoveryMiddleware()),
			}
			opts = append(opts, publicOption...)
			svr = grpc.NewServer(opts...)
		}
		// Register AtomosRemoteService.
		p.cluster.grpcImpl = &atomosRemoteService{
			process: p,
		}
		RegisterAtomosRemoteServiceServer(svr, p.cluster.grpcImpl)
		go func() {
			if err := svr.Serve(listener); err != nil {
				p.local.coreLog().coreInfo("CosmosProcess: gRPC server has exited. err=(%v)", err)
			}
		}()
		// Returns available grpc server.
		grpcServer = svr
		grpcListener = listener
		p.local.coreLog().coreInfo("CosmosProcess: gRPC server is listening. addr=(%s)", grpcListenAddress)
		break
	}
	// Check if grpc server is available.
	if grpcServer == nil {
		p.local.coreLog().coreFatal("CosmosProcess: Failed to start gRPC server. No available port in optionals ports list. ports=(%v)", ports)
		return NewError(ErrCodeCosmosEtcdGRPCServerFailed, "CosmosProcess: Failed to start etcd grpc server.").AddStack(p.local)
	}

	p.cluster.grpcServerOption = serverOption
	p.cluster.grpcDialOption = dialOption
	p.cluster.grpcAddress = grpcListenAddress
	p.cluster.grpcServer = grpcServer
	p.cluster.grpcListener = grpcListener

	// Watch cluster.
	// 先拉取一次集群信息，再检测集群变化。
	p.local.coreLog().coreInfo("CosmosProcess: Watching cluster changes. start")
	if err := p.watchCluster(cli); err != nil {
		p.local.coreLog().coreFatal("CosmosProcess: Failed to watch cluster. err=(%v)", err)
		return err.AddStack(p.local)
	}
	p.local.coreLog().coreInfo("CosmosProcess: Watching cluster changes. end")

	return nil
}

// handleStartUpFailedClusterCleanUp 当准备集群本地失败时，做etcd和gRPC的清理工作。
// When preparing the cluster locally fails, do the cleanup work of etcd and gRPC.
func (p *CosmosProcess) handleStartUpFailedClusterCleanUp() {
	// 删除etcd中的watcher。
	if exitCh := p.closeEtcdInfoChannel(); exitCh != nil {
		<-exitCh
		p.cluster.etcdExitCh = nil
	}

	// 删除etcd中的key，刪除version锁和node信息。
	if p.cluster.etcdClient != nil {
		// 解锁version锁。
		if err := p.etcdStartUpFailedNodeVersionUnlock(); err != nil {
			p.local.coreLog().coreFatal("CosmosProcess: Failed to unlock version while 'handleStartUpFailedClusterCleanUp'. err=(%v)", err)
		}

		// 删除node信息。
		key := etcdCosmosNodeVersionURI(p.local.runnable.config.Cosmos, p.local.runnable.config.Node, p.cluster.etcdVersion)
		if err := etcdDelete(p.cluster.etcdClient, key); err != nil {
			p.local.coreLog().coreFatal("CosmosProcess: Failed to delete etcd key while 'handleStartUpFailedClusterCleanUp'. err=(%v)", err)
		}

		// 关闭etcd客户端。
		if err := p.cluster.etcdClient.Close(); err != nil {
			p.local.coreLog().coreFatal("CosmosProcess: Failed to close etcd client while 'handleStartUpFailedClusterCleanUp'. err=(%v)", err)
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
	ext := []ArgsForBaseAtomos{ArgBaseAtomosWaitKilled()}
	if err := p.local.atomos.PushKillMail(p.local, ext...); err != nil {
		p.local.coreLog().coreFatal("CosmosProcess: Failed to kill local cosmos. err=(%v)", err)
	}
}

// unloadClusterLocalNode 卸载集群本地节点。
// Unload cluster local node.
func (p *CosmosProcess) unloadClusterLocalNode() {
	p.lifecycle.close()
	// 删除etcd中的watcher。
	if exitCh := p.closeEtcdInfoChannel(); exitCh != nil {
		<-exitCh
		p.cluster.etcdExitCh = nil
	}

	// 删除etcd中的key，刪除version锁和node信息。
	if p.cluster.etcdClient != nil {
		//// 删除version锁。
		//if err := p.etcdStoppingNodeVersionUnlock(); err != nil {
		//	p.logging.PushLogging(p.local.info, LogLevel_Core, fmt.Sprintf("CosmosProcess: Failed to unlock version while 'handleStartUpFailedClusterCleanUp'. err=(%v)", err))
		//}

		// 删除node信息。
		key := etcdCosmosNodeVersionURI(p.local.runnable.config.Cosmos, p.local.runnable.config.Node, p.cluster.etcdVersion)
		if err := etcdDelete(p.cluster.etcdClient, key); err != nil {
			p.local.coreLog().coreError("CosmosProcess: Failed to delete etcd key while 'handleStartUpFailedClusterCleanUp'. err=(%v)", err)
		}

		// 关闭etcd客户端。避免在正常 Stop 路径下泄漏 etcd 连接和 goroutine。
		// Close the etcd client. Mirrors handleStartUpFailedClusterCleanUp to
		// avoid leaking connections/goroutines on the normal Stop path.
		if err := p.cluster.etcdClient.Close(); err != nil {
			p.local.coreLog().coreError("CosmosProcess: Failed to close etcd client while 'unloadClusterLocalNode'. err=(%v)", err)
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

func (p *CosmosProcess) onIDSpawning(id *IDInfo) {
	runnable := p.local.runnable
	if runnable == nil {
		return
	}
	spawningHook := runnable.spawningHook
	p.local.coreLog().coreInfo("Tracker: Spawning. id=(%s)", id.Info())
	if spawningHook != nil {
		spawningHook(id)
	}
}

func (p *CosmosProcess) onIDSpawn(id *IDInfo) {
	runnable := p.local.runnable
	if runnable == nil {
		return
	}
	spawnHook := runnable.spawnHook
	if spawnHook == nil {
		p.local.coreLog().coreInfo("Tracker: Spawned. id=(%s)", id.Info())
	} else {
		spawnHook(id)
	}
}

func (p *CosmosProcess) onIDStopping(id *IDInfo) {
	runnable := p.local.runnable
	if runnable == nil {
		return
	}
	stoppingHook := runnable.stoppingHook
	if stoppingHook == nil {
		p.local.coreLog().coreInfo("Tracker: Stopping. id=(%s)", id.Info())
	} else {
		stoppingHook(id)
	}
}

func (p *CosmosProcess) onIDHalted(id *IDInfo, err *Error, mt atomosMessageTracker) {
	runnable := p.local.runnable
	if runnable == nil {
		return
	}
	haltedHook := runnable.haltedHook
	p.local.coreLog().coreInfo("Tracker: Halted. id=(%s),err=(%v)", id.Info(), err)
	if haltedHook != nil {
		exporter := mt.Export()
		haltedHook(id, errOrNil(err), exporter)
	}
}

func (p *CosmosProcess) onIDMessageTimeout(info *IDInfo, timeout time.Duration, message string, arg proto.Message) {
	p.local.Log().Warn("Tracker: Message Timeout. id=(%s),duration=(%v),message=(%s),arg=(%v)", info.Info(), timeout, message, arg)
	runnable := p.local.runnable
	if runnable == nil {
		return
	}
	messageTimeoutHook := runnable.messageTimeoutHook
	if messageTimeoutHook == nil {
		return
	}
	messageTimeoutHook(info, timeout, message, arg)
}

func (p *CosmosProcess) onRecoverHook(id *IDInfo, err *Error) {
	runnable := p.local.runnable
	if runnable == nil {
		return
	}
	recoverHook := runnable.recoverHook
	if recoverHook == nil {
		return
	}
	recoverHook(id, errOrNil(err))
}

func (p *CosmosProcess) onNewError(err *Error) {
	runnable := p.local.runnable
	if runnable == nil {
		return
	}
	newErrorHook := runnable.newErrorHook
	if newErrorHook == nil {
		return
	}
	newErrorHook(errOrNil(err))
}

// onDeadlockEvent delivers a sync-call deadlock detection or timeout event to
// the hook registered via CosmosRunnable.SetDeadlockHook. Panics in the hook
// are contained: the hook is an observer and must never take down the
// reporting goroutine (which is often a mailbox).
func (p *CosmosProcess) onDeadlockEvent(e *DeadlockEvent) {
	if p == nil || e == nil {
		return
	}
	runnable := p.local.runnable
	if runnable == nil {
		return
	}
	hook := runnable.deadlockHook
	if hook == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			p.local.coreLog().coreError("CosmosProcess: Deadlock hook panicked. err=(%v)", r)
		}
	}()
	hook(e)
}
