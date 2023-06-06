package go_atomos

import (
	"container/list"
	"google.golang.org/protobuf/proto"
	"time"
)

// Actual Atom
// The implementations of a local Atom type.

type AtomLocal struct {
	// 对目前ElementLocal实例的引用。
	// 具体指向的ElementLocal对象对AtomLocal是只读的，因此对其所有的操作都需要加上读锁。
	// 指向ElementLocal引用只可以在Atom加载和Cosmos被更新时改变。
	//
	// Reference to current ElementLocal instance.
	// The concrete ElementLocal instance should be read-only, so read-lock is required when access to it.
	// The reference only be set when atomos load and main reload.
	element *ElementLocal

	// 基础Atomos，也是实现Atom无锁队列的关键。
	// Base atomos, the key of lockless queue of Atom.
	atomos *BaseAtomos

	// 引用计数，所以任何GetID操作之后都需要Release。
	// Reference count, thus we have to Release any ID after GetID.

	// Element的List容器的Element
	nameElement *list.Element

	// 当前实现
	current *ElementImplementation

	*idFirstSyncCallLocal
	*idTrackerManager
	*messageTrackerManager
}

// 生命周期相关
// Life Cycle

func newAtomLocal(name string, e *ElementLocal, current *ElementImplementation, lv LogLevel) (*AtomLocal, *Error) {
	id := &IDInfo{
		Type:    IDType_Atom,
		Cosmos:  e.atomos.id.Cosmos,
		Element: e.atomos.id.Element,
		Atom:    name,
	}
	a := &AtomLocal{
		element:               e,
		atomos:                nil,
		nameElement:           nil,
		current:               current,
		idFirstSyncCallLocal:  &idFirstSyncCallLocal{},
		idTrackerManager:      &idTrackerManager{},
		messageTrackerManager: &messageTrackerManager{},
	}
	instance, err := func() (at Atomos, err *Error) {
		defer func() {
			if r := recover(); r != nil {
				err = NewError(ErrFrameworkRecoverFromPanic, "Atom: AtomConstructor panic.").AddStack(a)
				a.element.main.Log().Error("AtomLocal: AtomConstructor panic. err=(%v)", r)
			}
		}()
		at = current.Developer.AtomConstructor(name)
		if at == nil {
			return nil, NewError(ErrFrameworkIncorrectUsage, "Atom: AtomConstructor return nil.").AddStack(a)
		}
		return at, nil
	}()
	if err != nil {
		return nil, err.AddStack(nil)
	}
	a.atomos = NewBaseAtomos(id, lv, a, instance)
	a.idFirstSyncCallLocal.init(e.atomos.id)
	a.idTrackerManager.init(a)
	a.messageTrackerManager.init(e.atomos, len(e.current.AtomHandlers))

	return a, nil
}

//
// Implementation of ID
//

func (a *AtomLocal) GetIDInfo() *IDInfo {
	if a == nil {
		return nil
	}
	return a.atomos.GetIDInfo()
}

func (a *AtomLocal) String() string {
	if a == nil {
		return "nil"
	}
	return a.atomos.String()
}

func (a *AtomLocal) Cosmos() CosmosNode {
	return a.element.main
}

func (a *AtomLocal) State() AtomosState {
	return a.atomos.GetState()
}

// SyncMessagingByName
// 同步调用，通过名字调用Atom的消息处理函数。
func (a *AtomLocal) SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error) {
	if callerID == nil {
		return nil, NewError(ErrFrameworkIncorrectUsage, "Atom: SyncMessagingByName without fromID.").AddStack(a)
	}

	firstSyncCall := ""
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
				return nil, err.AddStack(a)
			}
			defer callerID.unsetSyncMessageAndFirstCall()
		} else {
			// 如果不为空，则检查是否和push向的ID的当前curFirstSyncCall一样，
			if eFirst := a.getCurFirstSyncCall(); callerFirst == eFirst {
				// 如果一样，则是循环调用死锁，返回错误。
				return nil, NewErrorf(ErrIDFirstSyncCallDeadlock, "IDFirstSyncCall: Sync call is dead lock. callerID=(%v),firstSyncCall=(%s)", callerID, callerFirst).AddStack(nil)
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
		firstSyncCall = a.nextFirstSyncCall()
	}

	out, err = a.atomos.PushMessageMailAndWaitReply(callerID, firstSyncCall, name, timeout, in)
	if err != nil {
		err = err.AddStack(a)
	}
	return
}

// AsyncMessagingByName
// 异步调用，通过名字调用Atom的消息处理函数。
func (a *AtomLocal) AsyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message, callback func(proto.Message, *Error)) {
	if callerID == nil {
		callback(nil, NewError(ErrFrameworkIncorrectUsage, "Atom: AsyncMessagingByName without fromID.").AddStack(a))
		return
	}

	// 这种情况需要创建新的FirstSyncCall，因为这是一个新的调用链，调用的开端是push向的ID。
	firstSyncCall := a.nextFirstSyncCall()

	a.Parallel(func() {
		out, err := a.atomos.PushMessageMailAndWaitReply(callerID, firstSyncCall, name, timeout, in)
		if err != nil {
			err = err.AddStack(a)
		}
		callerID.pushAsyncMessageCallbackMailAndWaitReply(name, out, err, callback)
	})
}

func (a *AtomLocal) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := a.current.Interface.AtomDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

// Kill
// 从另一个AtomLocal，或者从Main Script发送Kill消息给Atom。
// write Kill signal from other AtomLocal or from Main Script.
// 如果不实现ElementCustomizeAuthorization，则说明没有Kill的ID限制。
func (a *AtomLocal) Kill(callerID SelfID, timeout time.Duration) *Error {
	dev := a.element.current.Developer
	elemAuth, ok := dev.(ElementAuthorization)
	if ok && elemAuth != nil {
		if err := elemAuth.AtomCanKill(callerID); err != nil {
			return err.AddStack(a)
		}
	}
	return a.pushKillMail(callerID, true, timeout)
}

func (a *AtomLocal) SendWormhole(callerID SelfID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	firstSyncCall := ""
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
				return err.AddStack(a)
			}
			defer callerID.unsetSyncMessageAndFirstCall()
		} else {
			// 如果不为空，则检查是否和push向的ID的当前curFirstSyncCall一样，
			if eFirst := a.getCurFirstSyncCall(); callerFirst == eFirst {
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
		firstSyncCall = a.nextFirstSyncCall()
	}

	return a.atomos.PushWormholeMailAndWaitReply(callerID, firstSyncCall, timeout, wormhole)
}

func (a *AtomLocal) getIDTrackerManager() *idTrackerManager {
	return a.idTrackerManager
}

func (a *AtomLocal) getGoID() uint64 {
	return a.atomos.GetGoID()
}

// Implementations of AtomosRelease

func (a *AtomLocal) Release(tracker *IDTracker) {
	a.element.elementAtomRelease(a, tracker)
}

// Implementation of AtomosUtilities

func (a *AtomLocal) Log() Logging {
	return a.atomos.Log()
}

func (a *AtomLocal) Task() Task {
	return a.atomos.Task()
}

// Implementation of SelfID
//
// SelfID，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// SelfID, a concept that provide Atom resource access to inner Atom.
// With SelfID, Atom can access its self-main with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provides Log and Tasks method to inner Atom.

func (a *AtomLocal) CosmosMain() *CosmosLocal {
	return a.element.main
}

// KillSelf
// Atom kill itself from inner
func (a *AtomLocal) KillSelf() {
	//id, elem := a.id, a.element
	if err := a.pushKillMail(a, false, 0); err != nil {
		a.Log().Error("Atom: KillSelf failed. err=(%v)", err.AddStack(a))
		return
	}
	a.Log().Info("Atom: KillSelf.")
}

func (a *AtomLocal) Parallel(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				err := NewErrorf(ErrFrameworkRecoverFromPanic, "Atom: Parallel recovers from panic.").AddPanicStack(a, 3, r)
				if ar, ok := a.atomos.instance.(AtomosRecover); ok {
					defer func() {
						recover()
						a.Log().Fatal("Atom: Parallel critical problem again. err=(%v)", err)
					}()
					ar.ParallelRecover(err)
				} else {
					a.Log().Fatal("Atom: Parallel critical problem. err=(%v)", err)
				}
			}
		}()
		fn()
	}()
}

func (a *AtomLocal) Config() map[string][]byte {
	return a.element.main.runnable.config.Customize
}

func (a *AtomLocal) pushAsyncMessageCallbackMailAndWaitReply(name string, in proto.Message, err *Error, callback func(out proto.Message, err *Error)) {
	a.atomos.PushAsyncMessageCallbackMailAndWaitReply(name, in, err, callback)
}

// Implementation of AtomSelfID

func (a *AtomLocal) Persistence() AtomAutoData {
	p, ok := a.element.atomos.instance.(AutoDataPersistence)
	if ok || p == nil {
		return nil
	}
	return p.AtomAutoDataPersistence()
}

// 邮箱控制器相关
// Mailbox Handler

func (a *AtomLocal) OnMessaging(fromID ID, firstSyncCall, name string, in proto.Message) (out proto.Message, err *Error) {
	if fromID == nil {
		return nil, NewError(ErrFrameworkInternalError, "Atom: OnMessaging without fromID.").AddStack(a)
	}
	if err = a.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
		return nil, err.AddStack(a)
	}
	defer a.unsetSyncMessageAndFirstCall()

	handler := a.current.AtomHandlers[name]
	if handler == nil {
		return nil, NewErrorf(ErrAtomMessageHandlerNotExists,
			"Atom: Message handler not found. from=(%s),name=(%s),in=(%v)", fromID, name, in).AddStack(a)
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				if err == nil {
					err = NewErrorf(ErrFrameworkRecoverFromPanic, "Atom: Messaging recovers from panic.").AddPanicStack(a, 3, r)
					if ar, ok := a.atomos.instance.(AtomosRecover); ok {
						defer func() {
							recover()
							a.Log().Fatal("Atom: Messaging critical problem again. err=(%v)", err)
						}()
						ar.MessageRecover(name, in, err)
					} else {
						a.Log().Fatal("Atom: Messaging critical problem. err=(%v)", err)
					}
				}
			}
		}()
		out, err = handler(fromID, a.atomos.GetInstance(), in)
	}()
	return
}

func (a *AtomLocal) OnSyncMessagingCallback(in proto.Message, err *Error, callback func(reply proto.Message, err *Error)) {
	callback(in, err)
}

func (a *AtomLocal) OnScaling(from ID, firstSyncCall, name string, args proto.Message, tracker *IDTracker) (id ID, err *Error) {
	return nil, NewError(ErrFrameworkInternalError, "Atom: Atom not supported scaling.").AddStack(a)
}

func (a *AtomLocal) OnWormhole(from ID, wormhole AtomosWormhole) *Error {
	holder, ok := a.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		return NewErrorf(ErrAtomosNotSupportWormhole, "Atom: Not supported wormhole. type=(%T)", a.atomos.instance).AddStack(a)
	}
	return holder.AcceptWormhole(from, wormhole)
}

// 有状态的Atom会在Halt被调用之后调用AtomSaver函数保存状态，期间Atom状态为Stopping。
// Stateful Atom will save data after Stopping method has been called, while is doing this, Atom is set to Stopping.

func (a *AtomLocal) OnStopping(from ID, cancelled []uint64) (err *Error) {
	// Atoms
	a.element.main.hookStopping(a.atomos.id)
	defer a.element.main.hookHalt(a.element.atomos.id, err, a.element.idTrackerManager, a.element.messageTrackerManager)

	var save bool
	var data proto.Message
	defer func() {
		if r := recover(); r != nil {
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Atom: Stopping recovers from panic.").AddPanicStack(a, 3, r, data)
				if ar, ok := a.atomos.instance.(AtomosRecover); ok {
					defer func() {
						recover()
						a.Log().Fatal("Atom: Stopping recovers from panic. err=(%v)", err)
					}()
					ar.StopRecover(err)
				} else {
					a.Log().Fatal("Atom: Stopping recovers from panic. err=(%v)", err)
				}
			}
		}
		a.element.elementAtomStopping(a)
	}()
	save, data = a.atomos.GetInstance().Halt(from, cancelled)
	if !save {
		return nil
	}

	// Save data.
	impl := a.element.current
	if impl == nil {
		return NewErrorf(ErrAtomKillElementNoImplement, "Atom: Stopping save data error. no element implement. id=(%s),element=(%+v)",
			a.atomos.GetIDInfo(), a.element).AddStack(a)
	}
	persistence, ok := impl.Developer.(AutoDataPersistence)
	if !ok || persistence == nil {
		return NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence, "Atom: Stopping save data error. no auto data persistence. id=(%s),element=(%+v)",
			a.atomos.GetIDInfo(), a.element).AddStack(a)
	}
	atomPersistence := persistence.AtomAutoDataPersistence()
	if atomPersistence == nil {
		return NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence, "Atom: Stopping save data error. no atom auto data persistence. id=(%s),element=(%+v)",
			a.atomos.GetIDInfo(), a.element).AddStack(a)
	}
	if err = atomPersistence.SetAtomData(a.atomos.id.Atom, data); err != nil {
		return err.AddStack(a, data)
	}
	return err
}

// 内部实现
// INTERNAL

func (a *AtomLocal) elementAtomSpawn(current *ElementImplementation, persistence AutoDataPersistence, arg proto.Message) (err *Error) {
	defer func() {
		if r := recover(); r != nil {
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Atom: Spawn recovers from panic.").AddPanicStack(a, 3, r)
				if ar, ok := a.atomos.instance.(AtomosRecover); ok {
					defer func() {
						recover()
						a.Log().Fatal("Atom: Spawn recovers from panic. err=(%v)", err)
					}()
					ar.SpawnRecover(arg, err)
				} else {

				}
			}
		}
	}()
	// Get data and Spawning.
	var data proto.Message
	// 尝试进行自动数据持久化逻辑，如果支持的话，就会被执行。
	// 会从对象中GetAtomData，如果返回错误，证明服务不可用，那将会拒绝Atom的Spawn。
	// 如果GetAtomData拿不出数据，且Spawn没有传入参数，则认为是没有对第一次Spawn的Atom传入参数，属于错误。
	if persistence != nil {
		atomPersistence := persistence.AtomAutoDataPersistence()
		if atomPersistence != nil {
			name := a.atomos.id.Atom
			data, err = atomPersistence.GetAtomData(name)
			if err != nil {
				return err.AddStack(a)
			}
			if data == nil && arg == nil {
				return NewErrorf(ErrAtomDataNotFound, "Atom data not found. name=(%s)", name).AddStack(a)
			}
		}
	}
	if err = current.Interface.AtomSpawner(a, a.atomos.instance, arg, data); err != nil {
		return err.AddStack(a)
	}
	return nil
}

func (a *AtomLocal) pushKillMail(callerID SelfID, wait bool, timeout time.Duration) *Error {
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
					return err.AddStack(a)
				}
				defer callerID.unsetSyncMessageAndFirstCall()
			} else {
				// 如果不为空，则检查是否和push向的ID的当前curFirstSyncCall一样，
				if eFirst := a.getCurFirstSyncCall(); callerFirst == eFirst {
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
			firstSyncCall = a.nextFirstSyncCall()
		}
	}
	return a.atomos.PushKillMailAndWaitReply(callerID, firstSyncCall, wait, true, timeout)
}
