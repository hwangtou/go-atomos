package atomos

import (
	"time"

	"google.golang.org/protobuf/proto"
)

// CosmosNode is a node in the cluster. It can be local (this process) or remote
// (another process connected via gRPC). Use CosmosIsLocal to distinguish.
//
// Obtaining a CosmosNode
//
//   - For the local node: self.CosmosMain() returns the *CosmosLocal (which implements CosmosNode).
//   - For remote nodes: CosmosLocal.CosmosGetElementID / CosmosGetAtomID routes via the global router.
//
// Thread safety: all CosmosNode methods are safe for concurrent use.
type CosmosNode interface {
	ID

	// GetNodeName returns the node's configured name.
	GetNodeName() string

	// CosmosIsLocal returns true if this node is the current process.
	CosmosIsLocal() bool

	// CosmosGetElementID returns the ID for the named Element on this node.
	CosmosGetElementID(elem string, args ...ArgsForBaseAtomos) (ID, error)

	// CosmosGetAtomID returns the ID for a specific Atom on this node.
	// The returned IDTracker must be released via defer tracker.Release()
	// when done to allow the Atom to be garbage-collected.
	CosmosGetAtomID(elem, name string, args ...ArgsForBaseAtomos) (ID, *IDTracker, error)

	// CosmosSpawnAtom creates a new Atom with the given name and argument.
	// If an Atom with this name already exists and is running, returns ErrAtomSpawningAnExistedAtom.
	CosmosSpawnAtom(callerID SelfID, elem, name string, arg proto.Message, args ...ArgsForBaseAtomos) (ID, *IDTracker, error)

	// ElementBroadcast sends an asynchronous broadcast to every Element on this
	// node that has a handler registered for the "Broadcast" message.
	ElementBroadcast(callerID ID, key, contentType string, contentBuffer []byte) (err error)
}

// LifecycleMode selects the atom lifecycle guarantees for a process.
type LifecycleMode int

const (
	// LifecycleModeLegacyUnsafe preserves the existing router-driven lifecycle.
	LifecycleModeLegacyUnsafe LifecycleMode = iota
	// LifecycleModePerAtomStrict enables per-atom etcd ownership, operation
	// deduplication, incarnation checks and lease fencing. It requires a cluster.
	LifecycleModePerAtomStrict
	// LifecycleModePerShardStrict shares a leased fence generation across atoms
	// assigned to the same shard while retaining per-atom incarnations.
	LifecycleModePerShardStrict
)

const defaultLifecycleShardCount = 64

func lifecycleStrictMode(mode LifecycleMode) bool {
	return mode == LifecycleModePerAtomStrict || mode == LifecycleModePerShardStrict
}

// CosmosRunnable is the application builder. Every Atomos process creates one,
// registers Element implementations and hooks, then passes it to Run (or
// atomostest.Run in tests).
//
// Typical usage:
//
//	runnable := (&atomos.CosmosRunnable{}).
//		SetMainScript(myScript).
//		AddElementImplementationWithSpawn(myElementImpl)
//	atomos.Run(runnable)
type CosmosRunnable struct {
	config        *Config
	clock         Clock
	etcdClient    EtcdClient
	grpcTransport GRPCTransport
	implements    map[string]*ElementImplementation
	spawnElement  map[string]bool
	spawnOrder    []string
	mainScript    CosmosMainScript
	mainRouter    CosmosMainGlobalRouter

	// Lifecycle hooks invoked at each stage of an actor's lifecycle.
	// All hooks receive the *IDInfo of the affected actor.
	spawningHook func(id *IDInfo)
	spawnHook    func(id *IDInfo)
	stoppingHook func(id *IDInfo)
	haltedHook   func(id *IDInfo, err error, mt *AtomosMessageTrackerExporter)

	// Error hooks invoked when framework-level events occur.
	messageTimeoutHook func(id *IDInfo, timeout time.Duration, message string, args proto.Message)
	recoverHook        func(id *IDInfo, err error)
	newErrorHook       func(err error)

	// deadlockHook is invoked on every runtime sync-call deadlock detection
	// and sync-call timeout. See SetDeadlockHook.
	deadlockHook func(e *DeadlockEvent)

	// Per-process settings applied to the CosmosProcess at Start. Nil fields
	// keep the CosmosProcess defaults.
	messageTimeoutTracer   *bool
	messageTimeoutDefault  time.Duration
	muteKeepaliveLog       *bool
	idTrackerDebug         *bool
	syncDeadlockDebug      *bool
	syncCallDefaultTimeout time.Duration
	lifecycleMode          LifecycleMode
	lifecycleShardCount    int
	lifecycleShardCountSet bool
}

// Check reports whether the CosmosRunnable is correctly constructed: a Config
// and a MainScript are required.
// 检查CosmosRunnable是否正确构造。
func (r *CosmosRunnable) Check() error {
	// Config
	if r.config == nil {
		return NewError(ErrCodeRunnableConfigNotFound, "Runnable: Config not found.").AddStack(nil)
	}
	if err := r.config.Check(); err != nil {
		return asError(err).AddStack(nil)
	}
	// Implements
	if r.implements == nil {
		r.implements = map[string]*ElementImplementation{}
	}
	if r.lifecycleShardCountSet && r.lifecycleShardCount <= 0 {
		return NewError(ErrCodeCosmosConfigInvalid, "Runnable: lifecycle shard count must be positive").AddStack(nil)
	}
	// MainScript
	if r.mainScript == nil {
		return NewError(ErrCodeRunnableScriptNotFound, "Runnable: Script not found").AddStack(nil)
	}
	return nil
}

// AddElementImplementation registers an ElementImplementation with the
// runnable. The Element is available but not spawned at process start; use
// AddElementImplementationWithSpawn or SetElementSpawn for that.
// CosmosRunnable构造器方法，用于添加ElementImplementation（实现）。
func (r *CosmosRunnable) AddElementImplementation(i *ElementImplementation) *CosmosRunnable {
	return r.addElementImplementation(i, false)
}

// AddElementImplementationWithSpawn is AddElementImplementation plus marking
// the Element as a spawn entry (equivalent to also calling SetElementSpawn):
// the Element is spawned automatically at process start.
// 与 AddElementImplementation 相同，并额外将该 Element 标记为 spawn 入口。
func (r *CosmosRunnable) AddElementImplementationWithSpawn(i *ElementImplementation) *CosmosRunnable {
	return r.addElementImplementation(i, true)
}

func (r *CosmosRunnable) addElementImplementation(i *ElementImplementation, setSpawn bool) *CosmosRunnable {
	//r.AddElementInterface(i.Interface)
	if r.implements == nil {
		r.implements = map[string]*ElementImplementation{}
	}
	if _, has := r.implements[i.Interface.Config.Name]; !has {
		r.implements[i.Interface.Config.Name] = i
		//r.implementOrder = append(r.implementOrder, i.Interface.Config.Name)
	}
	if setSpawn {
		r.SetElementSpawn(i.Interface.Config.Name)
	}
	return r
}

// SetElementSpawn marks a registered Element as a spawn entry: it is spawned
// automatically at process start, in the order the entries were marked.
func (r *CosmosRunnable) SetElementSpawn(name string) *CosmosRunnable {
	if r.spawnElement == nil {
		r.spawnElement = map[string]bool{}
	}
	if _, has := r.implements[name]; !has {
		return r
	}
	if _, has := r.spawnElement[name]; has {
		return r
	}
	r.spawnElement[name] = true
	r.spawnOrder = append(r.spawnOrder, name)
	return r
}

// SetConfig sets the process Config. Run and atomostest.Run may replace it
// with the config loaded from the config file or app environment.
// CosmosRunnable构造器方法，用于设置Config。
func (r *CosmosRunnable) SetConfig(config *Config) *CosmosRunnable {
	r.config = config
	return r
}

// GetConfig returns the Config currently set on the runnable (nil if unset).
func (r *CosmosRunnable) GetConfig() *Config {
	return r.config
}

// SetClock sets the clock used by local timers and timeouts that have been
// migrated to Clock; it does not control all process scheduling. A nil clock
// keeps the production system clock.
func (r *CosmosRunnable) SetClock(clock Clock) *CosmosRunnable {
	r.clock = clock
	return r
}

// SetEtcdClient injects the etcd client used by an enabled cluster. The caller
// retains ownership: stopping this process does not close the client, so it may
// be shared with other processes. A nil client restores normal client creation.
func (r *CosmosRunnable) SetEtcdClient(client EtcdClient) *CosmosRunnable {
	r.etcdClient = client
	return r
}

// SetGRPCTransport injects the cluster listener and dialer. A nil transport
// uses TCP listeners and the standard gRPC client. The transport does not own
// listeners or connections after returning them to the process.
func (r *CosmosRunnable) SetGRPCTransport(transport GRPCTransport) *CosmosRunnable {
	r.grpcTransport = transport
	return r
}

// SetLifecycleMode configures the lifecycle guarantees for the process.
// LifecycleModeLegacyUnsafe is the default. PerAtomStrict and PerShardStrict
// require etcd and route ordinary Spawn/Get/Kill through controlled lifecycle.
func (r *CosmosRunnable) SetLifecycleMode(mode LifecycleMode) *CosmosRunnable {
	r.lifecycleMode = mode
	return r
}

// SetLifecycleShardCount sets the stable shard count used by PerShardStrict.
// The default is 64. Check rejects non-positive explicit values.
func (r *CosmosRunnable) SetLifecycleShardCount(count int) *CosmosRunnable {
	r.lifecycleShardCount = count
	r.lifecycleShardCountSet = true
	return r
}

// SetMessageTimeoutTracer enables or disables the message timeout tracer on
// the CosmosProcess started with this runnable. When enabled, messages that
// exceed timeout are reported via the message timeout hook.
func (r *CosmosRunnable) SetMessageTimeoutTracer(v bool, timeout time.Duration) *CosmosRunnable {
	r.messageTimeoutTracer = &v
	r.messageTimeoutDefault = timeout
	return r
}

// SetMuteKeepaliveLog silences or un-silences the periodic etcd keepalive log
// messages on the CosmosProcess started with this runnable.
func (r *CosmosRunnable) SetMuteKeepaliveLog(b bool) *CosmosRunnable {
	r.muteKeepaliveLog = &b
	return r
}

// SetIDTrackerDebug toggles IDTracker debug diagnostics on the CosmosProcess
// started with this runnable. See CosmosProcess.idTrackerDebug.
func (r *CosmosRunnable) SetIDTrackerDebug(v bool) *CosmosRunnable {
	r.idTrackerDebug = &v
	return r
}

// SetSyncDeadlockDebug enables per-edge stack capture for the sync-call
// wait-graph on the CosmosProcess started with this runnable. When enabled,
// every registered wait edge captures the caller's stack; deadlock errors
// (and the deadlock hook) then carry those stacks. Off by default — the hot
// path pays zero extra cost.
func (r *CosmosRunnable) SetSyncDeadlockDebug(v bool) *CosmosRunnable {
	r.syncDeadlockDebug = &v
	return r
}

// DeadlockEventKind classifies runtime sync-call failure events reported
// through the deadlock hook.
type DeadlockEventKind int

const (
	// DeadlockEventCycle: the sync-call wait-graph detected a cycle (local or
	// cross-node) and rejected the call with ErrIDFirstSyncCallDeadlock.
	DeadlockEventCycle DeadlockEventKind = iota
	// DeadlockEventTimeoutReject: a sync mail timed out while still queued.
	DeadlockEventTimeoutReject
	// DeadlockEventTimeoutHandling: a sync mail timed out while its handler
	// was executing (slow or wedged handler, or an undetectable wait cycle
	// such as one through user locks).
	DeadlockEventTimeoutHandling
)

// DeadlockEvent describes one runtime sync-call deadlock detection or
// timeout. Delivered via the hook set by CosmosRunnable.SetDeadlockHook.
type DeadlockEvent struct {
	// Kind classifies the event.
	Kind DeadlockEventKind
	// Caller is the IDInfo.Info() string of the sync caller, or "" if unknown.
	Caller string
	// Target is the IDInfo.Info() string of the target atom.
	Target string
	// Name is the sync message name.
	Name string
	// Chain is the readable wait chain for cycle events (e.g.
	// "g42→node::Elem::a[g7] (cycle)"); empty for timeout events.
	Chain string
	// Err is the error returned to the sync caller.
	Err error
}

// SetDeadlockHook registers a callback invoked on every runtime sync-call
// deadlock detection (DeadlockEventCycle) and sync-call timeout
// (DeadlockEventTimeoutReject / DeadlockEventTimeoutHandling).
//
// The hook is a diagnostics/ops outlet: count, alert, or dump stacks
// (atomos.DumpStacks). It cannot unblock a detected cycle — Go cannot preempt
// the blocked goroutines; the framework already unblocked the caller by
// returning ErrIDFirstSyncCallDeadlock, and the remaining ring members wake as
// the error propagates back or their timeouts expire. The hook runs
// synchronously on the reporting goroutine; it must be fast, must not panic,
// and must not make sync calls into the framework.
func (r *CosmosRunnable) SetDeadlockHook(fn func(e *DeadlockEvent)) *CosmosRunnable {
	r.deadlockHook = fn
	return r
}

// SetSyncCallDefaultTimeout sets the timeout applied to user sync calls that
// do not pass an explicit ArgBaseAtomosTimeout: it is the local waitReply
// fallback for sync mails and the base of the remote client-side gRPC
// deadline. Framework control mails (kill/wormhole) always keep a 10s floor.
// The default is 10s; non-positive values are ignored.
func (r *CosmosRunnable) SetSyncCallDefaultTimeout(d time.Duration) *CosmosRunnable {
	r.syncCallDefaultTimeout = d
	return r
}

// SetMainScript sets the process-level lifecycle hooks (CosmosMainScript).
// Required: Check fails without it.
// CosmosRunnable构造器方法，用于设置MainScript。
func (r *CosmosRunnable) SetMainScript(script CosmosMainScript) *CosmosRunnable {
	r.mainScript = script
	return r
}

// SetRouter sets the global router used to pick a node for cluster-wide
// lookups when more than one node could serve an Element.
func (r *CosmosRunnable) SetRouter(router CosmosMainGlobalRouter) *CosmosRunnable {
	r.mainRouter = router
	return r
}

// GetRouter returns the global router set by SetRouter, or nil. Testkit
// fixtures use it to install a default router only when none was provided.
func (r *CosmosRunnable) GetRouter() CosmosMainGlobalRouter {
	return r.mainRouter
}

// SetSpawningHook registers a hook invoked when an actor starts spawning.
func (r *CosmosRunnable) SetSpawningHook(hook func(id *IDInfo)) *CosmosRunnable {
	r.spawningHook = hook
	return r
}

// SetSpawnHook registers a hook invoked after an actor has spawned.
func (r *CosmosRunnable) SetSpawnHook(hook func(id *IDInfo)) *CosmosRunnable {
	r.spawnHook = hook
	return r
}

// SetStoppingHook registers a hook invoked when an actor starts stopping.
func (r *CosmosRunnable) SetStoppingHook(hook func(id *IDInfo)) *CosmosRunnable {
	r.stoppingHook = hook
	return r
}

// SetHaltedHook registers a hook invoked after an actor has halted.
func (r *CosmosRunnable) SetHaltedHook(hook func(id *IDInfo, err error, mt *AtomosMessageTrackerExporter)) *CosmosRunnable {
	r.haltedHook = hook
	return r
}

// SetMessageTimeoutHook registers a hook invoked when a message exceeds its
// timeout.
func (r *CosmosRunnable) SetMessageTimeoutHook(hook func(id *IDInfo, timeout time.Duration, message string, args proto.Message)) *CosmosRunnable {
	r.messageTimeoutHook = hook
	return r
}

// SetRecoverHook registers a hook invoked when an actor recovers from a
// panic.
func (r *CosmosRunnable) SetRecoverHook(hook func(id *IDInfo, err error)) *CosmosRunnable {
	r.recoverHook = hook
	return r
}

// SetNewErrorHook registers a hook invoked whenever the framework constructs
// an error.
func (r *CosmosRunnable) SetNewErrorHook(hook func(err error)) *CosmosRunnable {
	r.newErrorHook = hook
	return r
}
