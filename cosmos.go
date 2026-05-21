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
	CosmosGetElementID(elem string, args ...ArgsForBaseAtomos) (ID, *Error)

	// CosmosGetAtomID returns the ID for a specific Atom on this node.
	// The returned IDTracker must be released via defer tracker.Release()
	// when done to allow the Atom to be garbage-collected.
	CosmosGetAtomID(elem, name string, args ...ArgsForBaseAtomos) (ID, *IDTracker, *Error)

	// CosmosSpawnAtom creates a new Atom with the given name and argument.
	// If an Atom with this name already exists and is running, returns ErrAtomSpawningAnExistedAtom.
	CosmosSpawnAtom(callerID SelfID, elem, name string, arg proto.Message, args ...ArgsForBaseAtomos) (ID, *IDTracker, *Error)

	// ElementBroadcast sends an asynchronous broadcast to every Element on this
	// node that has a handler registered for the "Broadcast" message.
	ElementBroadcast(callerID ID, key, contentType string, contentBuffer []byte) (err *Error)
}

// CosmosRunnable is the application builder. Every Atomos process creates one,
// registers Element implementations and hooks, then passes it to Main() or Start().
//
// Typical usage:
//
//	runnable := &atomos.CosmosRunnable{}
//	runnable.SetConfig(config).
//	         SetMainScript(myScript).
//	         AddElementImplementation(myElementImpl, true)
//	atomos.Main(*runnable)
type CosmosRunnable struct {
	config       *Config
	implements   map[string]*ElementImplementation
	spawnElement map[string]bool
	spawnOrder   []string
	mainScript   CosmosMainScript
	mainRouter   CosmosMainGlobalRouter

	// Lifecycle hooks invoked at each stage of an actor's lifecycle.
	// All hooks receive the *IDInfo of the affected actor.
	spawningHook func(id *IDInfo)
	spawnHook    func(id *IDInfo)
	stoppingHook func(id *IDInfo)
	haltedHook   func(id *IDInfo, err *Error, mt *AtomosMessageTrackerExporter)

	// Error hooks invoked when framework-level events occur.
	messageTimeoutHook func(id *IDInfo, timeout time.Duration, message string, args proto.Message)
	recoverHook        func(id *IDInfo, err *Error)
	newErrorHook       func(err *Error)
}

// Check 检查CosmosRunnable是否正确构造。
// Check if CosmosRunnable is constructed correctly.
func (r *CosmosRunnable) Check() *Error {
	// Config
	if r.config == nil {
		return NewError(ErrRunnableConfigNotFound, "Runnable: Config not found.").AddStack(nil)
	}
	if err := r.config.Check(); err != nil {
		return err.AddStack(nil)
	}
	// Implements
	if r.implements == nil {
		r.implements = map[string]*ElementImplementation{}
	}
	// MainScript
	if r.mainScript == nil {
		return NewError(ErrRunnableScriptNotFound, "Runnable: Script not found").AddStack(nil)
	}
	return nil
}

// AddElementImplementation CosmosRunnable构造器方法，用于添加ElementImplementation（实现）。
// Construct method of CosmosRunnable, uses to add ElementImplementation.
func (r *CosmosRunnable) AddElementImplementation(i *ElementImplementation, setSpawn bool) *CosmosRunnable {
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

// SetConfig CosmosRunnable构造器方法，用于设置Config。
// Construct method of CosmosRunnable, uses to set Config.
func (r *CosmosRunnable) SetConfig(config *Config) *CosmosRunnable {
	r.config = config
	return r
}

// SetMainScript CosmosRunnable构造器方法，用于设置MainScript。
// Construct method of CosmosRunnable, uses to set MainScript.
func (r *CosmosRunnable) SetMainScript(script CosmosMainScript) *CosmosRunnable {
	r.mainScript = script
	return r
}

func (r *CosmosRunnable) SetRouter(router CosmosMainGlobalRouter) *CosmosRunnable {
	r.mainRouter = router
	return r
}

func (r *CosmosRunnable) SetSpawningHook(hook func(id *IDInfo)) *CosmosRunnable {
	r.spawningHook = hook
	return r
}

func (r *CosmosRunnable) SetSpawnHook(hook func(id *IDInfo)) *CosmosRunnable {
	r.spawnHook = hook
	return r
}

func (r *CosmosRunnable) SetStoppingHook(hook func(id *IDInfo)) *CosmosRunnable {
	r.stoppingHook = hook
	return r
}

func (r *CosmosRunnable) SetHaltedHook(hook func(id *IDInfo, err *Error, mt *AtomosMessageTrackerExporter)) *CosmosRunnable {
	r.haltedHook = hook
	return r
}

func (r *CosmosRunnable) SetMessageTimeoutHook(hook func(id *IDInfo, timeout time.Duration, message string, args proto.Message)) *CosmosRunnable {
	r.messageTimeoutHook = hook
	return r
}

func (r *CosmosRunnable) SetRecoverHook(hook func(id *IDInfo, err *Error)) *CosmosRunnable {
	r.recoverHook = hook
	return r
}

func (r *CosmosRunnable) SetNewErrorHook(hook func(err *Error)) *CosmosRunnable {
	r.newErrorHook = hook
	return r
}
