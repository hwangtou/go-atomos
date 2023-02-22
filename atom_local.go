package go_atomos

// CHECKED!

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

	// 调用链
	// 调用链用于检测是否有循环调用，在处理message时把fromID的调用链加上自己之后
	callChain []ID

	messageTracker *MessageTrackerManager
	idTracker      *IDTrackerManager
}

// 生命周期相关
// Life Cycle

func newAtomLocal(name string, e *ElementLocal, current *ElementImplementation, lv LogLevel) *AtomLocal {
	id := &IDInfo{
		Type:    IDType_Atom,
		Cosmos:  e.Cosmos().GetNodeName(),
		Element: e.GetElementName(),
		Atom:    name,
	}
	a := &AtomLocal{
		element:        e,
		atomos:         nil,
		nameElement:    nil,
		current:        current,
		callChain:      nil,
		messageTracker: NewMessageTrackerManager(id, len(current.AtomHandlers)),
		idTracker:      nil,
	}
	a.atomos = NewBaseAtomos(id, lv, a, current.Developer.AtomConstructor(name))
	a.idTracker = NewIDTrackerManager(a)

	return a
}

//
// Implementation of ID
//

// ID，相当于Atom的句柄的概念。
// 通过ID，可以访问到Atom所在的Cosmos、Element、Name，以及发送Kill信息，但是否能成功Kill，还需要AtomCanKill函数的认证。
// 直接用AtomLocal继承ID，因此本地的ID直接使用AtomLocal的引用即可。
//
// ID, a concept similar to file descriptor of an atomos.
// With ID, we can access the Cosmos, Element and Name of the Atom. We can also send Kill signal to the Atom,
// then the AtomCanKill method judge kill it or not.
// AtomLocal implements ID interface directly, so local ID is able to use AtomLocal reference directly.

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

func (a *AtomLocal) Release(tracker *IDTracker) {
	a.element.elementAtomRelease(a, tracker)
}

func (a *AtomLocal) Cosmos() CosmosNode {
	return a.element.main
}

func (a *AtomLocal) Element() Element {
	return a.element
}

func (a *AtomLocal) GetName() string {
	return a.atomos.GetIDInfo().Atom
}

func (a *AtomLocal) State() AtomosState {
	return a.atomos.GetState()
}

func (a *AtomLocal) IdleTime() time.Duration {
	a.atomos.mailbox.mutex.Lock()
	defer a.atomos.mailbox.mutex.Unlock()
	return a.messageTracker.idleTime()
}

func (a *AtomLocal) MessageByName(from ID, name string, timeout time.Duration, in proto.Message) (proto.Message, *Error) {
	return a.pushMessageMail(from, name, timeout, in)
}

func (a *AtomLocal) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := a.current.AtomDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

// Kill
// 从另一个AtomLocal，或者从Main Script发送Kill消息给Atom。
// write Kill signal from other AtomLocal or from Main Script.
// 如果不实现ElementCustomizeAuthorization，则说明没有Kill的ID限制。
func (a *AtomLocal) Kill(from ID, timeout time.Duration) *Error {
	dev := a.element.current.Developer
	elemAuth, ok := dev.(ElementCustomizeAuthorization)
	if ok && elemAuth != nil {
		if err := elemAuth.AtomCanKill(from); err != nil {
			return err.AddStack(a)
		}
	}
	return a.pushKillMail(from, true, timeout)
}

func (a *AtomLocal) SendWormhole(from ID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	return a.atomos.PushWormholeMailAndWaitReply(from, timeout, wormhole)
}

func (a *AtomLocal) getCallChain() []ID {
	a.atomos.mailbox.mutex.Lock()
	defer a.atomos.mailbox.mutex.Unlock()
	idList := make([]ID, 0, len(a.callChain)+1)
	for _, id := range a.callChain {
		idList = append(idList, id)
	}
	idList = append(idList, a)
	return idList
}

func (a *AtomLocal) getElementLocal() *ElementLocal {
	return nil
}

func (a *AtomLocal) getAtomLocal() *AtomLocal {
	return a
}

func (a *AtomLocal) getElementRemote() *ElementRemote {
	return nil
}

func (a *AtomLocal) getAtomRemote() *AtomRemote {
	return nil
}

func (a *AtomLocal) getIDTrackerManager() *IDTrackerManager {
	return a.idTracker
}

// Implementation of SelfID
//
// SelfID，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// SelfID, a concept that provide Atom resource access to inner Atom.
// With SelfID, Atom can access its self-main with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provides Log and Tasks method to inner Atom.

func (a *AtomLocal) CosmosMain() *CosmosMain {
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
				err := NewErrorf(ErrFrameworkPanic, "Atom: Parallel recovers from panic.").AddPanicStack(a, 3, r)
				if ar, ok := a.atomos.instance.(AtomosRecover); ok {
					defer func() {
						recover()
						a.Log().Fatal("Atom: Parallel recovers from panic. err=(%v)", err)
					}()
					ar.ParallelRecover(err)
				} else {
					a.Log().Fatal("Atom: Parallel recovers from panic. err=(%v)", err)
				}
			}
		}()
		fn()
	}()
}

// Implementation of AtomSelfID

func (a *AtomLocal) Config() map[string][]byte {
	return a.element.main.runnable.config.Customize
}

func (a *AtomLocal) Persistence() AtomAutoDataPersistence {
	p, ok := a.element.atomos.instance.(ElementCustomizeAutoDataPersistence)
	if ok || p == nil {
		return nil
	}
	return p.AtomAutoDataPersistence()
}

func (a *AtomLocal) MessageSelfByName(from ID, name string, buf []byte, protoOrJSON bool) ([]byte, *Error) {
	handlerFn, has := a.current.AtomHandlers[name]
	if !has {
		return nil, NewErrorf(ErrAtomMessageHandlerNotExists, "Atom: Handler not exists. from=(%v),name=(%s)", from, name).AddStack(a)
	}
	decoderFn, has := a.current.AtomDecoders[name]
	if !has {
		return nil, NewErrorf(ErrAtomMessageDecoderNotExists, "Atom: Decoder not exists. from=(%v),name=(%s)", from, name).AddStack(a)
	}
	in, err := decoderFn.InDec(buf, protoOrJSON)
	if err != nil {
		return nil, err
	}
	var outBuf []byte
	out, err := handlerFn(from, a.atomos.instance, in)
	if out != nil {
		var e error
		outBuf, e = proto.Marshal(out)
		if e != nil {
			return nil, NewErrorf(ErrAtomMessageReplyType, "Atom: Reply marshal failed. err=(%v)", err).AddStack(a)
		}
	}
	return outBuf, err
}

// Implementation of AtomosUtilities

func (a *AtomLocal) Log() Logging {
	return a.atomos.Log()
}

func (a *AtomLocal) Task() Task {
	return a.atomos.Task()
}

// Check chain.

func (a *AtomLocal) isInChain(fromIDList []ID) bool {
	a.atomos.mailbox.mutex.Lock()
	defer a.atomos.mailbox.mutex.Unlock()

	for _, fromID := range fromIDList {
		if fromID.GetIDInfo().IsEqual(a.GetIDInfo()) {
			return true
		}
	}
	return false
}

func (a *AtomLocal) setMessageAndCallChain(fromID ID, message string) *Error {
	if fromID == nil {
		return NewError(ErrAtomNoFromID, "Element: No fromID.").AddStack(a)
	}
	a.atomos.mailbox.mutex.Lock()
	defer a.atomos.mailbox.mutex.Unlock()
	if a.callChain != nil {
		return NewError(ErrFrameworkPanic, "OnMessage set chain meets non nil chain.").AddStack(a)
	}
	a.callChain = fromID.getCallChain()
	return nil
}

func (a *AtomLocal) unsetMessageAndCallChain(message string) {
	a.atomos.mailbox.mutex.Lock()
	defer a.atomos.mailbox.mutex.Unlock()
	a.callChain = nil
}

// 内部实现
// INTERNAL

// 推送邮件，并管理邮件对象的生命周期。
// 处理邮件，并设置Atom的运行状态。
//
// Push Mail, and manage life cycle of Mail instance.
// Handle Mail, and set the state of Atom.

// Message Mail

func (a *AtomLocal) pushMessageMail(from ID, name string, timeout time.Duration, arg proto.Message) (reply proto.Message, err *Error) {
	// Dead Lock Checker.
	// OnMessaging处理消息的时候，才做addChain操作
	if from == nil {
		return nil, NewError(ErrAtomNoFromID, "Atom: No fromID.").AddStack(a)
	}
	fromChain := from.getCallChain()
	if a.isInChain(fromChain) {
		return reply, NewErrorf(ErrAtomosCallDeadLock, "Atom: Message Deadlock. chain=(%v),to(%s),name=(%s),arg=(%v)",
			fromChain, a, name, arg).AddStack(a)
	}
	return a.atomos.PushMessageMailAndWaitReply(from, name, timeout, arg)
}

func (a *AtomLocal) OnMessaging(from ID, name string, arg proto.Message) (reply proto.Message, err *Error) {
	if err = a.setMessageAndCallChain(from, name); err != nil {
		return nil, err.AddStack(a)
	}
	defer a.unsetMessageAndCallChain(name)
	handler := a.current.AtomHandlers[name]
	if handler == nil {
		return nil, NewErrorf(ErrAtomMessageHandlerNotExists,
			"Atom: Message handler not found. from=(%s),name=(%s),arg=(%v)", from, name, arg).AddStack(a)
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				if err == nil {
					err = NewErrorf(ErrFrameworkPanic, "Atom: Messaging recovers from panic.").AddPanicStack(a, 3, r)
					if ar, ok := a.atomos.instance.(AtomosRecover); ok {
						defer func() {
							recover()
							a.Log().Fatal("Atom: Messaging recovers from panic. err=(%v)", err)
						}()
						ar.MessageRecover(name, arg, err)
					} else {
						a.Log().Fatal("Atom: Messaging recovers from panic. err=(%v)", err)
					}
				}
			}
		}()
		fromID, _ := from.(ID)
		reply, err = handler(fromID, a.atomos.GetInstance(), arg)
	}()
	return
}

func (a *AtomLocal) OnScaling(from ID, name string, arg proto.Message, tracker *IDTracker) (id ID, err *Error) {
	return nil, NewError(ErrAtomCannotScale, "Atom: Atom not supported scaling")
}

// Kill Mail

func (a *AtomLocal) pushKillMail(from ID, wait bool, timeout time.Duration) *Error {
	// Dead Lock Checker.
	if from != nil && wait {
		fromChain := from.getCallChain()
		if a.isInChain(fromChain) {
			return NewErrorf(ErrAtomosCallDeadLock, "Atom: Kill Deadlock. chain=(%v),to(%s)", fromChain, a).AddStack(a)
		}
	}
	return a.atomos.PushKillMailAndWaitReply(from, wait, true, timeout)
}

// 有状态的Atom会在Halt被调用之后调用AtomSaver函数保存状态，期间Atom状态为Stopping。
// Stateful Atom will save data after Stopping method has been called, while is doing this, Atom is set to Stopping.

func (a *AtomLocal) OnStopping(from ID, cancelled map[uint64]CancelledTask) (err *Error) {
	var save bool
	var data proto.Message
	defer func() {
		if r := recover(); r != nil {
			if err == nil {
				err = NewErrorf(ErrFrameworkPanic, "Atom: Stopping recovers from panic.").AddPanicStack(a, 3, r, data)
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
	persistence, ok := impl.Developer.(ElementCustomizeAutoDataPersistence)
	if !ok || persistence == nil {
		return NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence, "Atom: Stopping save data error. no auto data persistence. id=(%s),element=(%+v)",
			a.atomos.GetIDInfo(), a.element).AddStack(a)
	}
	atomPersistence := persistence.AtomAutoDataPersistence()
	if atomPersistence == nil {
		return NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence, "Atom: Stopping save data error. no atom auto data persistence. id=(%s),element=(%+v)",
			a.atomos.GetIDInfo(), a.element).AddStack(a)
	}
	if err = atomPersistence.SetAtomData(a.GetName(), data); err != nil {
		return err.AddStack(a, data)
	}
	return err
}

// Wormhole Mail

func (a *AtomLocal) OnWormhole(from ID, wormhole AtomosWormhole) *Error {
	holder, ok := a.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		return NewErrorf(ErrAtomosNotSupportWormhole, "Atom: Not supported wormhole. type=(%T)",
			a.atomos.instance).AddStack(a)
	}
	return holder.AcceptWormhole(from, wormhole)
}

// Set & Unset

func (a *AtomLocal) Spawn() {
	a.messageTracker.Start()
	if a.element.main.runnable.hookAtomSpawn != nil {
		a.element.main.runnable.hookAtomSpawn(a.element.atomos.id.Element, a.atomos.id.Atom)
	}
}

func (a *AtomLocal) Set(message string) {
	a.messageTracker.Set(message)
}

func (a *AtomLocal) Unset(message string) {
	a.messageTracker.Unset(message)
}

func (a *AtomLocal) Stopping() {
	a.messageTracker.Stopping()
	if a.element.main.runnable.hookAtomStopping != nil {
		a.element.main.runnable.hookAtomStopping(a.element.atomos.id.Element, a.atomos.id.Atom)
	}
}

func (a *AtomLocal) Halted() {
	a.messageTracker.Halt()
	if a.element.main.runnable.hookAtomHalt != nil {
		a.element.main.runnable.hookAtomHalt(a.element.atomos.id.Element, a.atomos.id.Atom)
	}
}

func (a *AtomLocal) GetMessagingInfo() string {
	a.atomos.mailbox.mutex.Lock()
	defer a.atomos.mailbox.mutex.Unlock()
	return a.messageTracker.dump()
}

// Element

func (a *AtomLocal) elementAtomSpawn(current *ElementImplementation, persistence ElementCustomizeAutoDataPersistence, arg proto.Message) (err *Error) {
	defer func() {
		if r := recover(); r != nil {
			if err == nil {
				err = NewErrorf(ErrFrameworkPanic, "Atom: Spawn recovers from panic.").AddPanicStack(a, 3, r)
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
			name := a.GetName()
			data, err = atomPersistence.GetAtomData(name)
			if err != nil {
				return err.AddStack(a)
			}
			if data == nil && arg == nil {
				return NewErrorf(ErrAtomSpawnArgInvalid, "Spawn atom without arg, name=(%s)",
					name).AddStack(a)
			}
		}
	}
	if err = current.Interface.AtomSpawner(a, a.atomos.instance, arg, data); err != nil {
		return err.AddStack(a)
	}
	return nil
}
