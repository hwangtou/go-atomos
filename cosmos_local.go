package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"reflect"
	"sync"
	"time"
)

type CosmosLocal struct {
	process  *CosmosProcess
	runnable *CosmosRunnable

	atomos *BaseAtomos

	// Elements & Remote
	mutex    sync.RWMutex
	elements map[string]*ElementLocal
}

// Implementation of ID

func (c *CosmosLocal) GetIDContext() IDContext {
	return &c.atomos.ctx
}

func (c *CosmosLocal) GetIDInfo() *IDInfo {
	if c == nil {
		return nil
	}
	return c.atomos.GetIDInfo()
}

func (c *CosmosLocal) String() string {
	return c.atomos.String()
}

func (c *CosmosLocal) Cosmos() CosmosNode {
	return c
}

func (c *CosmosLocal) State() AtomosState {
	return c.atomos.GetState()
}

func (c *CosmosLocal) IdleTime() time.Duration {
	return c.atomos.idleTime()
}

func (c *CosmosLocal) SyncMessagingByName(_ SelfID, _ string, _ time.Duration, _ proto.Message) (out proto.Message, err *Error) {
	panic("not supported")
}

func (c *CosmosLocal) AsyncMessagingByName(_ SelfID, _ string, _ time.Duration, _ proto.Message, _ func(out proto.Message, err *Error)) {
	panic("not supported")
}

func (c *CosmosLocal) DecoderByName(_ string) (MessageDecoder, MessageDecoder) {
	return nil, nil
}

func (c *CosmosLocal) Kill(_ SelfID, _ time.Duration) *Error {
	return NewError(ErrCosmosCannotKill, "Cosmos: Cannot kill local.").AddStack(c)
}

func (c *CosmosLocal) SendWormhole(_ SelfID, _ time.Duration, _ AtomosWormhole) *Error {
	return NewError(ErrCosmosCannotSendWormhole, "Cosmos: Cannot send wormhole to local.").AddStack(c)
}

func (c *CosmosLocal) getGoID() uint64 {
	return c.atomos.GetGoID()
}

// Implementation of AtomosUtilities

func (c *CosmosLocal) Log() Logging {
	return c.atomos.Log()
}

func (c *CosmosLocal) Task() Task {
	return c.atomos.Task()
}

// Implementation of atomos.SelfID
//
// SelfID，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// SelfID, a concept that provide Atom resource access to inner Atom.
// With SelfID, Atom can access its self-main with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provides Log and Tasks method to inner Atom.

func (c *CosmosLocal) CosmosMain() *CosmosLocal {
	return c
}

// KillSelf
// Atom kill itself from inner
func (c *CosmosLocal) KillSelf() {
	c.Log().Info("Cosmos: Cannot KillSelf.")
}

func (c *CosmosLocal) Parallel(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				var err *Error
				defer func() {
					if r2 := recover(); r2 != nil {
						c.Log().Fatal("Cosmos: Parallel critical problem again. err=(%v)", err)
					}
				}()
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Cosmos: Parallel recovers from panic.").AddPanicStack(c, 3, r)
				// Hook or Log
				if ar, ok := c.atomos.instance.(AtomosRecover); ok {
					ar.ParallelRecover(err)
				} else {
					c.Log().Fatal("Cosmos: Parallel critical problem. err=(%v)", err)
				}
				// Global hook
				c.process.onRecoverHook(c.atomos.id, err)
			}
		}()
		fn()
	}()
}

func (c *CosmosLocal) Config() map[string][]byte {
	return c.runnable.config.Customize
}

func (c *CosmosLocal) getAtomos() *BaseAtomos {
	return c.atomos
}

// Implementation of CosmosNode

func (c *CosmosLocal) GetNodeName() string {
	return c.GetIDInfo().Node
}

func (c *CosmosLocal) CosmosIsLocal() bool {
	return true
}

func (c *CosmosLocal) CosmosGetElementID(elemName string) (ID, *Error) {
	e, err := c.getGlobalElement(elemName, "")
	if err != nil {
		return nil, err.AddStack(c)
	}
	return e, nil
}

func (c *CosmosLocal) CosmosGetAtomID(elemName, name string) (id ID, tracker *IDTracker, err *Error) {
	e, err := c.getGlobalElement(elemName, name)
	if err != nil {
		return nil, nil, err.AddStack(c)
	}
	id, tracker, err = e.GetAtomID(name, NewIDTrackerInfoFromLocalGoroutine(3), true)
	if err != nil {
		return nil, nil, err.AddStack(c)
	}
	return id, tracker, nil
}

func (c *CosmosLocal) CosmosGetScaleAtomID(callerID SelfID, elemName, message string, timeout time.Duration, args proto.Message) (id ID, tracker *IDTracker, err *Error) {
	e, err := c.getGlobalElement(elemName, "")
	if err != nil {
		return nil, nil, err.AddStack(c)
	}
	id, tracker, err = e.ScaleGetAtomID(callerID, message, timeout, args, NewIDTrackerInfoFromLocalGoroutine(3), true)
	if err != nil {
		return nil, nil, err.AddStack(c)
	}
	if reflect.ValueOf(id).IsNil() {
		return nil, nil, NewErrorf(ErrAtomNotExists, "Cosmos: ScaleGetAtomID not exists. name=(%s)", elemName).AddStack(c)
	}
	return id, tracker, nil
}

func (c *CosmosLocal) CosmosSpawnAtom(callerID SelfID, elemName, name string, arg proto.Message) (ID, *IDTracker, *Error) {
	e, err := c.getGlobalElement(elemName, name)
	if err != nil {
		return nil, nil, err.AddStack(c)
	}
	return e.SpawnAtom(callerID, name, arg, NewIDTrackerInfoFromLocalGoroutine(3), true)
}

func (c *CosmosLocal) ElementBroadcast(callerID SelfID, key, contentType string, contentBuffer []byte) (err *Error) {
	elems, err := c.getLocalAllElements()
	if err != nil {
		return err.AddStack(c)
	}
	for _, elem := range elems {
		if elem.elemImpl == nil {
			continue
		}
		if _, has := elem.elemImpl.ElementHandlers[ElementBroadcastName]; !has {
			continue
		}
		elem.AsyncMessagingByName(c, ElementBroadcastName, 0, &ElementBroadcastI{
			Key:           key,
			ContentType:   contentType,
			ContentBuffer: contentBuffer,
		}, func(message proto.Message, err *Error) {
			if err != nil {
				c.Log().Error("Cosmos: ElementBroadcast error. err=(%v)", err)
			}
		})
	}
	return nil
}

// Main as an Atomos

func (c *CosmosLocal) Halt(_ ID, _ []uint64) (save bool, data proto.Message) {
	c.Log().Fatal("Cosmos: Stopping of CosmosLocal should not be called.")
	return false, nil
}

// 邮箱控制器相关
// Mailbox Handler

func (c *CosmosLocal) OnMessaging(_ ID, _ string, _ proto.Message) (out proto.Message, err *Error) {
	return nil, NewError(ErrCosmosCannotMessage, "Cosmos: Cannot send cosmos message.").AddStack(c)
}

func (c *CosmosLocal) OnAsyncMessagingCallback(in proto.Message, err *Error, callback func(reply proto.Message, err *Error)) {
	callback(in, err)
}

func (c *CosmosLocal) OnScaling(_ ID, _ string, _ proto.Message) (id ID, err *Error) {
	return nil, NewError(ErrCosmosCannotScale, "Cosmos: Cannot scale.").AddStack(c)
}

func (c *CosmosLocal) OnWormhole(from ID, wormhole AtomosWormhole) *Error {
	holder, ok := c.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		err := NewErrorf(ErrAtomosNotSupportWormhole, "Cosmos: Not supported wormhole. type=(%T)", c.atomos.instance)
		c.Log().Error(err.Message)
		return err
	}
	if err := holder.AcceptWormhole(from, wormhole); err != nil {
		return err.AddStack(c)
	}
	return nil
}

func (c *CosmosLocal) OnStopping(from ID, cancelled []uint64) (err *Error) {
	c.Log().Info("Cosmos: Now exiting.")

	// Unload local elements and its atomos.
	for i := len(c.runnable.spawnOrder) - 1; i >= 0; i -= 1 {
		name := c.runnable.spawnOrder[i]
		elem, has := c.elements[name]
		if !has {
			continue
		}
		if err = elem.atomos.PushKillMailAndWaitReply(c, true, 0); err != nil {
			c.Log().Error("Cosmos: Exiting kill element error. element=(%s),err=(%v)", name, err.Message)
		}
	}

	//c.elements = nil
	//c.runnable = nil
	//c.process = nil

	return nil
}

func (c *CosmosLocal) OnIDsReleased() {
}

// 内部实现
// INTERNAL

func (c *CosmosLocal) getClusterElementsInfo() map[string]*IDInfo {
	c.mutex.RLock()
	m := make(map[string]*IDInfo, len(c.elements))
	for name, elem := range c.elements {
		m[name] = elem.GetIDInfo()
	}
	c.mutex.RUnlock()
	return m
}

func (c *CosmosLocal) getGlobalElement(elemName, atomName string) (Element, *Error) {
	if !c.process.cluster.enable {
		return c.getLocalElement(elemName)
	}
	router := c.runnable.mainRouter
	if router == nil {
		e, err := c.getLocalElement(elemName)
		if err != nil {
			return nil, err.AddStack(c)
		}
		return e, nil
	}

	nodeName, has := router.GetCosmosNodeName(c.GetNodeName(), elemName, atomName)
	if !has {
		return nil, NewErrorf(ErrCosmosElementNotFound, "Cosmos: Local element not found. name=(%s)", elemName).AddStack(c)
	}

	if nodeName == c.GetNodeName() {
		e, err := c.getLocalElement(elemName)
		if err != nil {
			return nil, err.AddStack(c)
		}
		return e, nil
	} else {
		c.process.cluster.remoteMutex.RLock()
		defer c.process.cluster.remoteMutex.RUnlock()
		cr, has := c.process.cluster.remoteCosmos[nodeName]
		if !has {
			return nil, NewErrorf(ErrCosmosElementNotFound, "Cosmos: Remote element not found. name=(%s)", elemName).AddStack(c)
		}
		e, err := cr.getElement(elemName)
		if err != nil {
			return nil, err.AddStack(c)
		}
		return e, nil
	}
}

func (c *CosmosLocal) getLocalElement(name string) (elem *ElementLocal, err *Error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.runnable == nil {
		return nil, NewError(ErrCosmosRunnableNotFound, "Cosmos: It's not running.").AddStack(c)
	}
	elem, has := c.elements[name]
	if !has {
		return nil, NewErrorf(ErrCosmosElementNotFound, "Cosmos: Local element not found. name=(%s)", name).AddStack(c)
	}
	return elem, nil
}

func (c *CosmosLocal) getLocalAllElements() (elems []*ElementLocal, err *Error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.runnable == nil {
		return nil, NewError(ErrCosmosRunnableNotFound, "Cosmos: It's not running.").AddStack(c)
	}
	for _, elem := range c.elements {
		elems = append(elems, elem)
	}
	return elems, nil
}

func (c *CosmosLocal) trySpawningElements() (err *Error) {
	// Spawn
	// TODO 有个问题，如果这里的Spawn逻辑需要用到新的helper里面的配置，那就会有问题，所以Spawn尽量不要做对其它Cosmos的操作，延后到Script。
	var loaded []*ElementLocal
	for _, name := range c.runnable.spawnOrder {
		impl := c.runnable.implements[name]
		if impl == nil {
			err = NewErrorf(ErrCosmosElementNotFound, "Cosmos: Element not found. name=(%s)", name).AddStack(c)
			c.Log().Fatal("Cosmos: Spawning element failed. name=(%s),err=(%s)", name, err.Message)
			break
		}
		elem, e := c.cosmosElementSpawn(c.runnable, impl)
		if e != nil {
			err = e.AddStack(c)
			c.Log().Fatal("Cosmos: Spawning element failed. name=(%s),err=(%s)", impl.Interface.Config.Name, err.Message)
			break
		}
		loaded = append(loaded, elem)
	}
	if err != nil {
		for _, elem := range loaded {
			if e := elem.atomos.PushKillMailAndWaitReply(c, true, 0); e != nil {
				c.Log().Fatal("Cosmos: Spawning element failed, kill failed. name=(%s),err=(%v)", elem.atomos.id.Element, e.AddStack(c))
			}
		}
		c.Log().Fatal("Cosmos: Spawning element has rollback.")
		return
	}
	for _, elem := range loaded {
		s, ok := elem.atomos.instance.(ElementStartRunning)
		if !ok || s == nil {
			continue
		}
		go func(s ElementStartRunning, elemID *IDInfo) {
			defer func() {
				if r := recover(); r != nil {
					var err *Error
					defer func() {
						if r2 := recover(); r2 != nil {
							c.Log().Fatal("Cosmos: StartRunning recovers from panic. err=(%v)", err)
						}
					}()
					err = NewErrorf(ErrCosmosStartRunningPanic, "Cosmos: StartRunning recovers from panic.").AddPanicStack(c, 3, r)
					// Hook or Log
					if ar, ok := c.atomos.instance.(AtomosRecover); ok {
						ar.ParallelRecover(err)
					} else {
						c.Log().Fatal("Cosmos: StartRunning recovers from panic. err=(%v)", err)
					}
					// Global hook
					c.process.onRecoverHook(elemID, err)
				}
			}()
			s.StartRunning()
		}(s, elem.atomos.id)
	}
	return nil
}

func (c *CosmosLocal) cosmosElementSpawn(r *CosmosRunnable, i *ElementImplementation) (elem *ElementLocal, err *Error) {
	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					c.Log().Fatal("Element: Spawn recovers from panic. err=(%v)", err)
				}
			}()
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Element: Spawn Element recovers from panic.").AddPanicStack(c, 4, r)
			} else {
				err = err.AddPanicStack(c, 4, r)
			}
			// Hook or Log
			if ar, ok := c.atomos.instance.(AtomosRecover); ok {
				ar.SpawnRecover(nil, err)
			} else {
				c.Log().Fatal("Element: Spawn recovers from panic. err=(%v)", err)
			}
			// Global hook
			c.process.onRecoverHook(c.process.local.atomos.id, err)
		}
	}()
	name := i.Interface.Config.Name

	elem = newElementLocal(c, r, i)

	c.mutex.Lock()
	_, has := c.elements[name]
	if !has {
		c.elements[name] = elem
	}
	c.mutex.Unlock()
	if has {
		return nil, NewErrorf(ErrElementLoaded, "Cosmos: Spawn Element exists. name=(%s)", name).AddStack(c)
	}

	// Element的Spawn逻辑。
	//callChain := c.atomos.ctx.CallChain()
	if err = elem.atomos.start(func() *Error {
		if err := elem.cosmosElementSpawn(c, r, i); err != nil {
			return err.AddStack(elem)
		}
		return nil
	}); err != nil {
		c.mutex.Lock()
		delete(c.elements, name)
		c.mutex.Unlock()
		return nil, err.AddStack(elem)
	}
	return elem, nil
}

func (c *CosmosLocal) GetCosmosNode(name string) *CosmosRemote {
	c.process.cluster.remoteMutex.RLock()
	defer c.process.cluster.remoteMutex.RUnlock()
	return c.process.cluster.remoteCosmos[name]
}
