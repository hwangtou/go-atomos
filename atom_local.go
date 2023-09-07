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
	elemImpl *ElementImplementation
}

// 生命周期相关
// Life Cycle

func newAtomLocal(name string, e *ElementLocal, current *ElementImplementation, lv LogLevel) (*AtomLocal, *Error) {
	id := &IDInfo{
		Type:    IDType_Atom,
		Cosmos:  e.atomos.id.Cosmos,
		Node:    e.atomos.id.Node,
		Element: e.atomos.id.Element,
		Atom:    name,
		//GoId:    0,
	}
	a := &AtomLocal{
		element:     e,
		atomos:      nil,
		nameElement: nil,
		elemImpl:    current,
	}
	instance, err := func() (at Atomos, err *Error) {
		defer func() {
			if r := recover(); r != nil {
				err = NewError(ErrFrameworkRecoverFromPanic, "Atom: AtomConstructor panic.").AddStack(a)
				a.element.cosmosLocal.Log().Error("AtomLocal: AtomConstructor panic. err=(%v)", r)
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
	a.atomos = NewBaseAtomos(id, lv, a, instance, e.cosmosLocal.process)

	return a, nil
}

//
// Implementation of ID
//

func (a *AtomLocal) GetIDContext() IDContext {
	return &a.atomos.ctx
}

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
	return a.element.cosmosLocal
}

func (a *AtomLocal) State() AtomosState {
	return a.atomos.GetState()
}

func (a *AtomLocal) IdleTime() time.Duration {
	return a.atomos.idleTime()
}

// SyncMessagingByName
// 同步调用，通过名字调用Atom的消息处理函数。
func (a *AtomLocal) SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error) {
	out, err = a.atomos.PushMessageMailAndWaitReply(callerID, name, false, timeout, in)
	if err != nil {
		err = err.AddStack(a)
	}
	return
}

// AsyncMessagingByName
// 异步调用，通过名字调用Atom的消息处理函数。
func (a *AtomLocal) AsyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message, callback func(proto.Message, *Error)) {
	a.atomos.PushAsyncMessageMail(callerID, a, name, timeout, in, callback)
}

func (a *AtomLocal) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := a.elemImpl.Interface.AtomDecoders[name]
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
	dev := a.element.elemImpl.Developer
	elemAuth, ok := dev.(ElementAuthorization)
	if ok && elemAuth != nil {
		if err := elemAuth.AtomCanKill(callerID); err != nil {
			return err.AddStack(a)
		}
	}

	if err := a.atomos.PushKillMailAndWaitReply(callerID, true, timeout); err != nil {
		return err.AddStack(a)
	}
	return nil
}

func (a *AtomLocal) SendWormhole(callerID SelfID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	if err := a.atomos.PushWormholeMailAndWaitReply(callerID, timeout, wormhole); err != nil {
		return err.AddStack(a)
	}
	return nil
}

func (a *AtomLocal) getGoID() uint64 {
	return a.atomos.GetGoID()
}

func (a *AtomLocal) asyncCallback(callerID SelfID, name string, reply proto.Message, err *Error, callback func(reply proto.Message, err *Error)) {
	a.atomos.PushAsyncMessageCallbackMailAndWaitReply(callerID, name, reply, err, callback)
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
	return a.element.cosmosLocal
}

// KillSelf
// Atom kill itself from inner
func (a *AtomLocal) KillSelf() {
	if err := a.atomos.PushKillMailAndWaitReply(a, false, 0); err != nil {
		a.Log().Error("Atom: KillSelf failed. err=(%v)", err.AddStack(a))
		return
	}
	a.Log().Info("Atom: KillSelf.")
}

func (a *AtomLocal) Parallel(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				var err *Error
				defer func() {
					if r2 := recover(); r2 != nil {
						a.Log().Fatal("Atom: Parallel critical problem again. err=(%v)", err)
					}
				}()
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Atom: Parallel recovers from panic.").AddPanicStack(a, 3, r)
				// Hook or Log
				if ar, ok := a.atomos.instance.(AtomosRecover); ok {
					ar.ParallelRecover(err)
				} else {
					a.Log().Fatal("Atom: Parallel critical problem. err=(%v)", err)
				}
				// Global hook
				a.element.cosmosLocal.process.onRecoverHook(a.atomos.id, err)
			}
		}()
		fn()
	}()
}

func (a *AtomLocal) Config() map[string][]byte {
	return a.element.cosmosLocal.runnable.config.Customize
}

func (a *AtomLocal) getAtomos() *BaseAtomos {
	return a.atomos
}

// Implementation of AtomSelfID

func (a *AtomLocal) Persistence() AtomAutoData {
	p, ok := a.element.atomos.instance.(AutoData)
	if ok || p == nil {
		return nil
	}
	return p.AtomAutoData()
}

// 邮箱控制器相关
// Mailbox Handler

func (a *AtomLocal) OnMessaging(fromID ID, name string, in proto.Message) (out proto.Message, err *Error) {
	if fromID == nil {
		return nil, NewError(ErrFrameworkInternalError, "Atom: OnMessaging without fromID.").AddStack(a)
	}

	handler := a.elemImpl.AtomHandlers[name]
	if handler == nil {
		return nil, NewErrorf(ErrAtomMessageHandlerNotExists,
			"Atom: Message handler not found. from=(%s),name=(%s),in=(%v)", fromID, name, in).AddStack(a)
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				defer func() {
					if r2 := recover(); r2 != nil {
						a.Log().Fatal("Atom: Messaging critical problem again. err=(%v)", err)
					}
				}()
				if err == nil {
					err = NewErrorf(ErrFrameworkRecoverFromPanic, "Atom: Messaging recovers from panic.").AddPanicStack(a, 3, r)
				} else {
					err = err.AddPanicStack(a, 3, r)
				}
				// Hook or Log
				if ar, ok := a.atomos.instance.(AtomosRecover); ok {
					ar.MessageRecover(name, in, err)
				} else {
					a.Log().Fatal("Atom: Messaging critical problem. err=(%v)", err)
				}
				// Global hook
				a.element.cosmosLocal.process.onRecoverHook(a.atomos.id, err)
			}
		}()
		out, err = handler(fromID, a.atomos.GetInstance(), in)
		if err != nil {
			err = err.AddStack(a)
		}
	}()
	return
}

func (a *AtomLocal) OnAsyncMessaging(fromID ID, name string, in proto.Message, callback func(reply proto.Message, err *Error)) {
	if fromID == nil {
		if callback != nil {
			callback(nil, NewError(ErrFrameworkInternalError, "Atom: OnAsyncMessaging without fromID.").AddStack(a))
		}
		a.Log().Error("Atom: OnAsyncMessaging without fromID.")
		return
	}

	handler := a.elemImpl.AtomHandlers[name]
	if handler == nil {
		if callback != nil {
			callback(nil, NewErrorf(ErrAtomMessageHandlerNotExists,
				"Atom: OnAsyncMessaging handler not found. from=(%s),name=(%s),in=(%v)", fromID, name, in).AddStack(a))
		}
		a.Log().Error("Atom: OnAsyncMessaging handler not found. from=(%s),name=(%s),in=(%v)", fromID, name, in)
		return
	}

	var err *Error
	var out proto.Message
	func() {
		defer func() {
			if r := recover(); r != nil {
				defer func() {
					if r2 := recover(); r2 != nil {
						a.Log().Fatal("Atom: OnAsyncMessaging critical problem again. err=(%v)", err)
					}
				}()
				if err == nil {
					err = NewErrorf(ErrFrameworkRecoverFromPanic, "Atom: OnAsyncMessaging recovers from panic.").AddPanicStack(a, 3, r)
				} else {
					err = err.AddPanicStack(a, 3, r)
				}
				// Hook or Log
				if ar, ok := a.atomos.instance.(AtomosRecover); ok {
					ar.MessageRecover(name, in, err)
				} else {
					a.Log().Fatal("Atom: OnAsyncMessaging critical problem. err=(%v)", err)
				}
				// Global hook
				a.element.cosmosLocal.process.onRecoverHook(a.atomos.id, err)
			}
		}()
		out, err = handler(fromID, a.atomos.GetInstance(), in)
		if err != nil {
			err = err.AddStack(a)
		}
		if callback != nil {
			fromID.asyncCallback(a, name, out, err, callback)
		} else {
			a.Log().Debug("Atom: OnAsyncMessaging without callback. from=(%s),name=(%s),in=(%v),out=(%v),err=(%v)", fromID, name, in, out, err)
		}
	}()
}

func (a *AtomLocal) OnAsyncMessagingCallback(in proto.Message, err *Error, callback func(reply proto.Message, err *Error)) {
	callback(in, err.AddStack(a))
}

func (a *AtomLocal) OnScaling(_ ID, _ string, _ proto.Message) (id ID, err *Error) {
	return nil, NewError(ErrFrameworkInternalError, "Atom: Atom not supported scaling.").AddStack(a)
}

func (a *AtomLocal) OnWormhole(from ID, wormhole AtomosWormhole) *Error {
	holder, ok := a.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		return NewErrorf(ErrAtomosNotSupportWormhole, "Atom: Not supported wormhole. type=(%T)", a.atomos.instance).AddStack(a)
	}
	if err := holder.AcceptWormhole(from, wormhole); err != nil {
		return err.AddStack(a)
	}
	return nil
}

// 有状态的Atom会在Halt被调用之后调用AtomSaver函数保存状态，期间Atom状态为Stopping。
// Stateful Atom will save data after Stopping method has been called, while is doing this, Atom is set to Stopping.

func (a *AtomLocal) OnStopping(from ID, cancelled []uint64) (err *Error) {
	var save bool
	var data proto.Message
	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					a.Log().Fatal("Atom: Stopping recovers from panic. err=(%v)", err)
				}
			}()
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Atom: Stopping recovers from panic.").AddPanicStack(a, 3, r, data)
			} else {
				err = err.AddPanicStack(a, 3, r, data)
			}
			// Hook or Log
			if ar, ok := a.atomos.instance.(AtomosRecover); ok {
				ar.StopRecover(err)
			} else {
				a.Log().Fatal("Atom: Stopping recovers from panic. err=(%v)", err)
			}
			// Global hook
			a.element.cosmosLocal.process.onRecoverHook(a.atomos.id, err)
		}
		a.element.elementAtomStopping(a)
	}()
	save, data = a.atomos.GetInstance().Halt(from, cancelled)
	if !save {
		return nil
	}

	// Save data.
	impl := a.element.elemImpl
	if impl == nil {
		a.Log().Fatal("Atom: Stopping save data error. no element implement. id=(%s),element=(%+v),data=(%v)", a.atomos.GetIDInfo().Info(), a.element, data)
		return NewErrorf(ErrAtomKillElementNoImplement, "Atom: Stopping save data error. no element implement. id=(%s),element=(%+v)",
			a.atomos.GetIDInfo().Info(), a.element).AddStack(a)
	}
	persistence, ok := impl.Developer.(AutoData)
	if !ok || persistence == nil {
		a.Log().Fatal("Atom: Stopping save data error. no auto data persistence. id=(%s),element=(%+v),data=(%v)", a.atomos.GetIDInfo().Info(), a.element, data)
		return NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence, "Atom: Stopping save data error. no auto data persistence. id=(%s),element=(%+v)",
			a.atomos.GetIDInfo().Info(), a.element).AddStack(a)
	}
	atomPersistence := persistence.AtomAutoData()
	if atomPersistence == nil {
		a.Log().Fatal("Atom: Stopping save data error. no atom auto data persistence. id=(%s),element=(%+v),data=(%v)", a.atomos.GetIDInfo().Info(), a.element, data)
		return NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence, "Atom: Stopping save data error. no atom auto data persistence. id=(%s),element=(%+v)",
			a.atomos.GetIDInfo().Info(), a.element).AddStack(a)
	}
	if err = atomPersistence.SetAtomData(a.atomos.id.Atom, data); err != nil {
		a.Log().Fatal("Atom: Stopping save data error. id=(%s),element=(%+v),data=(%v),err=(%v)", a.atomos.GetIDInfo().Info(), a.element, data, err)
		return err.AddStack(a, data)
	}
	return nil
}

func (a *AtomLocal) OnIDsReleased() {
	a.element.elementAtomRelease(a)
}

// 内部实现
// INTERNAL

func (a *AtomLocal) elementAtomSpawn(current *ElementImplementation, persistence AutoData, arg proto.Message) (err *Error) {
	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					a.Log().Fatal("Atom: Spawn recovers from panic. err=(%v)", err)
				}
			}()
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Atom: Spawn recovers from panic.").AddPanicStack(a, 3, r)
			} else {
				err = err.AddPanicStack(a, 3, r)
			}
			// Hook or Log
			if ar, ok := a.atomos.instance.(AtomosRecover); ok {
				ar.SpawnRecover(arg, err)
			} else {
				a.Log().Fatal("Atom: Spawn recovers from panic. err=(%v)", err)
			}
			// Global hook
			a.element.cosmosLocal.process.onRecoverHook(a.atomos.id, err)
		}
	}()

	// Get data and Spawning.
	var data proto.Message
	// 尝试进行自动数据持久化逻辑，如果支持的话，就会被执行。
	// 会从对象中GetAtomData，如果返回错误，证明服务不可用，那将会拒绝Atom的Spawn。
	// 如果GetAtomData拿不出数据，且Spawn没有传入参数，则认为是没有对第一次Spawn的Atom传入参数，属于错误。
	if persistence != nil {
		atomPersistence := persistence.AtomAutoData()
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
