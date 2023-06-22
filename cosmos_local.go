package go_atomos

import (
	"google.golang.org/protobuf/proto"
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

	*idFirstSyncCallLocal
	*idTrackerManager
}

// Implementation of ID

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

func (c *CosmosLocal) SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error) {
	panic("not supported")
}

func (c *CosmosLocal) AsyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message, callback func(out proto.Message, err *Error)) {
	panic("not supported")
}

func (c *CosmosLocal) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	return nil, nil
}

func (c *CosmosLocal) Kill(callerID SelfID, timeout time.Duration) *Error {
	return NewError(ErrMainCannotKill, "Cosmos: Cannot kill local.").AddStack(c)
}

func (c *CosmosLocal) SendWormhole(callerID SelfID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	return NewError(ErrMainCannotSendWormhole, "Cosmos: Cannot send wormhole to local.").AddStack(c)
}

func (c *CosmosLocal) getIDTrackerManager() *idTrackerManager {
	return c.idTrackerManager
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
				err := NewErrorf(ErrFrameworkRecoverFromPanic, "Cosmos: Parallel recovers from panic.").AddPanicStack(c, 3, r)
				if ar, ok := c.atomos.instance.(AtomosRecover); ok {
					defer func() {
						recover()
						c.Log().Fatal("Cosmos: Parallel critical problem again. err=(%v)", err)
					}()
					ar.ParallelRecover(err)
				} else {
					c.Log().Fatal("Cosmos: Parallel critical problem. err=(%v)", err)
				}
			}
		}()
		fn()
	}()
}

func (c *CosmosLocal) Config() map[string][]byte {
	return c.runnable.config.Customize
}

func (c *CosmosLocal) pushAsyncMessageCallbackMailAndWaitReply(name string, in proto.Message, err *Error, callback func(out proto.Message, err *Error)) {
	c.atomos.PushAsyncMessageCallbackMailAndWaitReply(name, in, err, callback)
}

// Implementation of CosmosNode

func (c *CosmosLocal) GetNodeName() string {
	return c.GetIDInfo().Node
}

func (c *CosmosLocal) CosmosIsLocal() bool {
	return true
}

func (c *CosmosLocal) CosmosGetElementID(elemName string) (ID, *Error) {
	e, err := c.getLocalElement(elemName)
	if err != nil {
		return nil, err.AddStack(c)
	}
	return e, nil
}

func (c *CosmosLocal) CosmosGetAtomID(elemName, name string) (id ID, tracker *IDTracker, err *Error) {
	e, err := c.getLocalElement(elemName)
	if err != nil {
		return nil, nil, err.AddStack(c)
	}
	return e.GetAtomID(name, NewIDTrackerInfoFromLocalGoroutine(3))
}

func (c *CosmosLocal) CosmosGetScaleAtomID(callerID SelfID, elemName, message string, timeout time.Duration, args proto.Message) (ID ID, tracker *IDTracker, err *Error) {
	e, err := c.getLocalElement(elemName)
	if err != nil {
		return nil, nil, err.AddStack(c)
	}
	return e.ScaleGetAtomID(callerID, message, timeout, args, NewIDTrackerInfoFromLocalGoroutine(3))
}

func (c *CosmosLocal) CosmosSpawnAtom(elemName, name string, arg proto.Message) (ID, *IDTracker, *Error) {
	e, err := c.getLocalElement(elemName)
	if err != nil {
		return nil, nil, err.AddStack(c)
	}
	return e.SpawnAtom(name, arg, NewIDTrackerInfoFromLocalGoroutine(3))
}

func (c *CosmosLocal) ElementBroadcast(callerID ID, key, contentType string, contentBuffer []byte) (err *Error) {
	elems, err := c.getLocalAllElements()
	if err != nil {
		return err.AddStack(c)
	}
	for _, elem := range elems {
		if elem.current == nil {
			continue
		}
		if _, has := elem.current.ElementHandlers[ElementBroadcastName]; !has {
			continue
		}
		elem.AsyncMessagingByName(c, ElementBroadcastName, 0, &ElementBroadcastI{
			Key:           key,
			ContentType:   contentType,
			ContentBuffer: contentBuffer,
		}, func(message proto.Message, err *Error) {
			if err != nil {
				// TODO
			}
		})
	}
	return nil
}

// Main as an Atomos

func (c *CosmosLocal) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	c.Log().Fatal("Cosmos: Stopping of CosmosLocal should not be called.")
	return false, nil
}

// 邮箱控制器相关
// Mailbox Handler

func (c *CosmosLocal) OnMessaging(fromID ID, firstSyncCall, name string, in proto.Message) (out proto.Message, err *Error) {
	return nil, NewError(ErrMainCannotMessage, "Cosmos: Cannot send cosmos message.")
}

func (c *CosmosLocal) OnAsyncMessagingCallback(in proto.Message, err *Error, callback func(reply proto.Message, err *Error)) {
	callback(in, err)
}

func (c *CosmosLocal) OnScaling(from ID, firstSyncCall, name string, args proto.Message, tracker *IDTracker) (id ID, err *Error) {
	return nil, NewError(ErrMainCannotScale, "Cosmos: Cannot scale.").AddStack(c)
}

func (c *CosmosLocal) OnWormhole(from ID, wormhole AtomosWormhole) *Error {
	holder, ok := c.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		err := NewErrorf(ErrAtomosNotSupportWormhole, "Cosmos: Not supported wormhole. type=(%T)", c.atomos.instance)
		c.Log().Error(err.Message)
		return err
	}
	return holder.AcceptWormhole(from, wormhole)
}

func (c *CosmosLocal) OnStopping(from ID, cancelled []uint64) (err *Error) {
	c.Log().Info("Cosmos: Now exiting.")

	// Unload local elements and its atomos.
	for i := len(c.runnable.implementOrder) - 1; i >= 0; i -= 1 {
		name := c.runnable.implementOrder[i].Interface.Config.Name
		elem, has := c.elements[name]
		if !has {
			continue
		}
		if err = elem.pushKillMail(c, true, 0); err != nil {
			c.Log().Error("Cosmos: Exiting kill element error. element=(%s),err=(%v)", name, err.Message)
		}
	}

	//c.elements = nil
	//c.runnable = nil
	//c.process = nil

	return nil
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

func (c *CosmosLocal) getLocalElement(name string) (elem *ElementLocal, err *Error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.runnable == nil {
		return nil, NewError(ErrMainRunnableNotFound, "Cosmos: It's not running.").AddStack(c)
	}
	elem, has := c.elements[name]
	if !has {
		return nil, NewErrorf(ErrMainElementNotFound, "Cosmos: Local element not found. name=(%s)", name).AddStack(c)
	}
	return elem, nil
}

func (c *CosmosLocal) getLocalAllElements() (elems []*ElementLocal, err *Error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.runnable == nil {
		return nil, NewError(ErrMainRunnableNotFound, "Cosmos: It's not running.").AddStack(c)
	}
	for _, elem := range c.elements {
		elems = append(elems, elem)
	}
	return elems, nil
}

//func (c *CosmosLocal) trySpawningElements(helper *runnableLoadingHelper) (err *Error) {
func (c *CosmosLocal) trySpawningElements() (err *Error) {
	// Spawn
	// TODO 有个问题，如果这里的Spawn逻辑需要用到新的helper里面的配置，那就会有问题，所以Spawn尽量不要做对其它Cosmos的操作，延后到Script。
	var loaded []*ElementLocal
	for _, impl := range c.runnable.implementOrder {
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
			if e := elem.pushKillMail(c, true, 0); e != nil {
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
		go func(s ElementStartRunning) {
			defer func() {
				if r := recover(); r != nil {
					err := NewErrorf(ErrMainStartRunningPanic, "Cosmos: StartRunning recovers from panic.").AddPanicStack(c, 3, r)
					if ar, ok := c.atomos.instance.(AtomosRecover); ok {
						defer func() {
							recover()
							c.Log().Fatal("Cosmos: StartRunning recovers from panic. err=(%v)", err)
						}()
						ar.ParallelRecover(err)
					} else {
						c.Log().Fatal("Cosmos: StartRunning recovers from panic. err=(%v)", err)
					}
				}
			}()
			s.StartRunning()
		}(s)
	}
	return nil
}

func (c *CosmosLocal) cosmosElementSpawn(r *CosmosRunnable, i *ElementImplementation) (elem *ElementLocal, err *Error) {
	defer func() {
		if r := recover(); r != nil {
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Element: Spawn Element recovers from panic.").AddPanicStack(c, 4, r)
				if ar, ok := c.atomos.instance.(AtomosRecover); ok {
					defer func() {
						recover()
						c.Log().Fatal("Element: Spawn recovers from panic. err=(%v)", err)
					}()
					ar.SpawnRecover(nil, err)
				} else {
					c.Log().Fatal("Element: Spawn recovers from panic. err=(%v)", err)
				}
			}
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
	if err = elem.atomos.start(func() *Error {
		if err := elem.cosmosElementSpawn(r, i); err != nil {
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

func (c *CosmosLocal) pushKillMail(callerID SelfID, wait bool, timeout time.Duration) *Error {
	firstSyncCall := ""

	if callerID != nil && wait {
		// 获取调用ID的Go ID
		callerLocalGoID := callerID.getGoID()
		// 获取调用栈的Go ID
		curLocalGoID := getGoID()

		// 这种情况，调用方的ID和当前的ID是同一个，证明是同步调用。
		if callerLocalGoID == curLocalGoID {
			// 此时需要检查调用方是否有curFirstSyncCall，如果为空，证明是第一个同步调用（如Task中调用的），所以需要创建一个curFirstSyncCall。
			// 因为是同一个Atom，所以直接设置到当前的ID即可。
			if callerFirst := callerID.getCurFirstSyncCall(); callerFirst == "" {
				// 要从调用者开始算起，所以要从调用者的ID中获取。
				firstSyncCall = callerID.nextFirstSyncCall()
				if err := callerID.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
					return err.AddStack(c)
				}
				defer callerID.unsetSyncMessageAndFirstCall()
			} else {
				// 如果不为空，则检查是否和push向的ID的当前curFirstSyncCall一样，
				if eFirst := c.getCurFirstSyncCall(); callerFirst == eFirst {
					// 如果一样，则是循环调用死锁，返回错误。
					return NewErrorf(ErrIDFirstSyncCallDeadlock, "IDFirstSyncCall: Sync call is dead lock. callerID=(%v),firstSyncCall=(%s)", callerID, callerFirst).AddStack(nil)
				} else {
					// 这些情况都检查过，则可以正常调用。 如果是同一个，则证明调用ID就是在自己的同步调用中调用的，需要把之前的同步调用链传递下去。
					// （所以一定要保护好SelfID，只应该让当前atomos去持有）。
					// 继续传递调用链。
					firstSyncCall = callerFirst
				}
			}
		} else {
			// 例如在Parallel和被其它框架调用的情况，就是这种。
			// 因为是其它goroutine发起的，所以可以不用把caller设置成firstSyncCall。
			firstSyncCall = c.nextFirstSyncCall()
		}
	}
	return c.atomos.PushKillMailAndWaitReply(callerID, firstSyncCall, wait, true, timeout)
}
