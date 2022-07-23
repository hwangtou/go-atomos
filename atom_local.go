package go_atomos

// CHECKED!

import (
	"container/list"
	"google.golang.org/protobuf/proto"
	"runtime/debug"
	"sync"
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

	//// 实际的Id类型
	//id ID
	// 引用计数，所以任何GetId操作之后都需要Release。
	// Reference count, thus we have to Release any ID after GetId.
	count int

	// Element的List容器的Element
	nameElement *list.Element

	// 当前实现
	current *ElementImplementation

	// 调用链
	// 调用链用于检测是否有循环调用，在处理message时把fromId的调用链加上自己之后
	callChain []ID
}

// 生命周期相关
// Life Cycle

func newAtomLocal(name string, e *ElementLocal, reloads int, current *ElementImplementation, log *LoggingAtomos, lv LogLevel) *AtomLocal {
	id := &IDInfo{
		Type:    IDType_Atomos,
		Cosmos:  e.Cosmos().GetNodeName(),
		Element: e.GetElementName(),
		Atomos:  name,
	}
	a := atomLocalPool.Get().(*AtomLocal)
	a.element = e
	a.atomos = NewBaseAtomos(id, log, lv, a, current.Developer.AtomConstructor(), reloads)
	//a.id = current.Interface.AtomIdConstructor(a)
	a.count = 0
	a.nameElement = nil
	a.current = current
	a.callChain = nil

	return a
}

func (a *AtomLocal) deleteAtomLocal(wait bool) {
	a.atomos.DeleteAtomos(wait)
	atomLocalPool.Put(a)
}

// Atom对象的内存池
// Atom instance pools.
var atomLocalPool = sync.Pool{
	New: func() interface{} {
		return &AtomLocal{}
	},
}

//
// Implementation of ID
//

// ID，相当于Atom的句柄的概念。
// 通过Id，可以访问到Atom所在的Cosmos、Element、Name，以及发送Kill信息，但是否能成功Kill，还需要AtomCanKill函数的认证。
// 直接用AtomLocal继承Id，因此本地的Id直接使用AtomLocal的引用即可。
//
// ID, a concept similar to file descriptor of an atomos.
// With ID, we can access the Cosmos, Element and Name of the Atom. We can also send Kill signal to the Atom,
// then the AtomCanKill method judge kill it or not.
// AtomLocal implements ID interface directly, so local ID is able to use AtomLocal reference directly.

func (a *AtomLocal) GetIDInfo() *IDInfo {
	return a.atomos.GetIDInfo()
}

func (a *AtomLocal) Release() {
	a.element.elementAtomRelease(a)
}

func (a *AtomLocal) Cosmos() CosmosNode {
	return a.element.main
}

func (a *AtomLocal) Element() Element {
	return a.element
}

func (a *AtomLocal) GetName() string {
	return a.atomos.GetIDInfo().Atomos
}

// Kill
// 从另一个AtomLocal，或者从Main Script发送Kill消息给Atom。
// write Kill signal from other AtomLocal or from Main Script.
// 如果不实现ElementCustomizeAuthorization，则说明没有Kill的ID限制。
func (a *AtomLocal) Kill(from ID) *ErrorInfo {
	dev := a.element.current.Developer
	elemAuth, ok := dev.(ElementCustomizeAuthorization)
	if !ok || elemAuth == nil {
		return nil
	}
	if err := elemAuth.AtomCanKill(from); err != nil {
		return err
	}
	return a.pushKillMail(from, true)
}

func (a *AtomLocal) String() string {
	return a.atomos.String()
}

func (a *AtomLocal) getCallChain() []ID {
	return a.callChain
}

func (a *AtomLocal) getElementLocal() *ElementLocal {
	return nil
}

func (a *AtomLocal) getAtomLocal() *AtomLocal {
	return a
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

func (a *AtomLocal) ElementLocal() *ElementLocal {
	return a.element
}

// KillSelf
// Atom kill itself from inner
func (a *AtomLocal) KillSelf() {
	//id, elem := a.id, a.element
	if err := a.pushKillMail(a, false); err != nil {
		a.Log().Error("KillSelf error, err=%v", err)
		return
	}
	a.Log().Info("KillSelf")
}

// Implementation of AtomosUtilities

func (a *AtomLocal) Log() Logging {
	return a.atomos.Log()
}

func (a *AtomLocal) Task() Task {
	return a.atomos.Task()
}

// Check chain.

func (a *AtomLocal) checkCallChain(fromIdList []ID) bool {
	for _, fromId := range fromIdList {
		if fromId.GetIDInfo().IsEqual(a.GetIDInfo()) {
			return false
		}
	}
	return true
}

func (a *AtomLocal) addCallChain(fromIdList []ID) {
	a.callChain = append(fromIdList, a)
}

func (a *AtomLocal) delCallChain() {
	a.callChain = nil
}

// 内部实现
// INTERNAL

// 邮箱控制器相关
// Mailbox Handler
// TODO: Performance tracer.

// 推送邮件，并管理邮件对象的生命周期。
// 处理邮件，并设置Atom的运行状态。
//
// PushProcessLog Mail, and manage life cycle of Mail instance.
// Handle Mail, and set the state of Atom.

// Message Mail

func (a *AtomLocal) pushMessageMail(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	// Dead Lock Checker.
	if from != nil {
		if !a.checkCallChain(from.getCallChain()) {
			return reply, NewErrorf(ErrAtomCallDeadLock, "Call Dead Lock, chain=(%v),to(%s),name=(%s),args=(%v)",
				from.getCallChain(), a, name, args)
		}
		a.addCallChain(from.getCallChain())
		defer a.delCallChain()
	}
	return a.atomos.PushMessageMailAndWaitReply(from, name, args)
}

func (a *AtomLocal) OnMessaging(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	handler := a.current.AtomHandlers[name]
	if handler == nil {
		return nil, NewErrorf(ErrAtomMessageHandlerNotExists,
			"AtomHandler: Message handler not found, from=(%s),name=(%s),args=(%v)", from, name, args)
	}
	var stack []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				stack = debug.Stack()
			}
		}()
		fromID, _ := from.(ID)
		reply, err = handler(fromID, a.atomos.GetInstance(), args)
	}()
	if len(stack) != 0 {
		err = NewErrorf(ErrAtomMessageHandlerPanic,
			"Message handler PANIC, from=(%s),name=(%s),args=(%v)", from, name, args).
			AddStack(a.GetIDInfo(), stack)
	} else if err != nil && len(err.Stacks) > 0 {
		err = err.AddStack(a.GetIDInfo(), debug.Stack())
	}
	return
}

// Kill Mail

func (a *AtomLocal) pushKillMail(from ID, wait bool) *ErrorInfo {
	return a.atomos.PushKillMailAndWaitReply(from, wait)
}

// 有状态的Atom会在Halt被调用之后调用AtomSaver函数保存状态，期间Atom状态为Stopping。
// Stateful Atom will save data after Halt method has been called, while is doing this, Atom is set to Stopping.

func (a *AtomLocal) OnStopping(from ID, cancelled map[uint64]CancelledTask) (err *ErrorInfo) {
	defer func() {
		if r := recover(); r != nil {
			err = NewErrorf(ErrAtomKillHandlerPanic,
				"AtomHandler: Kill RECOVERED, id=(%s),instance=(%+v),reason=(%s)", a.atomos.GetIDInfo(), a.atomos.Description(), r).
				AddStack(a.GetIDInfo(), debug.Stack())
			a.Log().Error(err.Message)
		}
	}()
	save, data := a.atomos.GetInstance().Halt(from, cancelled)
	if !save {
		return nil
	}
	// Save data.
	impl := a.element.current
	if impl == nil {
		err = NewErrorf(ErrAtomKillElementNoImplement,
			"AtomHandler: Save data error, no element implement, id=(%s),element=(%+v)", a.atomos.GetIDInfo(), a.element)
		a.Log().Fatal(err.Message)
		return err
	}
	p, ok := impl.Developer.(ElementCustomizeAutoDataPersistence)
	if !ok || p == nil {
		err = NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence,
			"AtomHandler: Save data error, no element auto data persistence, id=(%s),element=(%+v)", a.atomos.GetIDInfo(), a.element)
		a.Log().Fatal(err.Message)
		return err
	}
	if err = p.AtomAutoDataPersistence().SetAtomData(a.GetName(), data); err != nil {
		a.Log().Error("AtomHandler: Save data failed, set atom data error, id=(%s),instance=(%+v),err=(%s)",
			a.atomos.GetIDInfo(), a.atomos.Description(), err)
		return err
	}
	return err
}

// Reload Mail

// 重载邮件，指定Atom的版本。
// Reload Mail with specific version.
func (a *AtomLocal) pushReloadMail(from ID, elem *ElementImplementation, upgrades int) *ErrorInfo {
	return a.atomos.PushReloadMailAndWaitReply(from, elem, upgrades)
}

// 注意：Reload伴随着整个Element的Reload，会先调用Atom的Halt，再Spawn。但不会删除正在执行的任务。
// Notice: Atom reload goes with the Element reload, Halt and Spawn will be called in order. It won't delete tasks.

func (a *AtomLocal) OnReloading(oldAtom Atomos, reloadObject AtomosReloadable) (newAtom Atomos) {
	// 如果没有新的Element，就用旧的Element。
	// Use old Element if there is no new Element.
	reload, ok := reloadObject.(*ElementImplementation)
	if !ok || reload == nil {
		err := NewErrorf(ErrAtomReloadInvalid, "Reload is invalid, reload=(%v),reloads=(%d)", reload, a.atomos.reloads)
		a.Log().Fatal(err.Message)
		return
	}

	newAtom = reload.Developer.AtomConstructor()
	newAtom.Reload(oldAtom)
	return newAtom
}

// Element

func (a *AtomLocal) elementAtomSpawn(current *ElementImplementation, persistence ElementCustomizeAutoDataPersistence, arg proto.Message) *ErrorInfo {
	// Get data and Spawning.
	var data proto.Message
	// 尝试进行自动数据持久化逻辑，如果支持的话，就会被执行。
	// 会从对象中GetAtomData，如果返回错误，证明服务不可用，那将会拒绝Atom的Spawn。
	// 如果GetAtomData拿不出数据，且Spawn没有传入参数，则认为是没有对第一次Spawn的Atom传入参数，属于错误。
	if persistence != nil {
		name := a.GetName()
		d, err := persistence.AtomAutoDataPersistence().GetAtomData(name)
		if err != nil {
			return err
		}
		if d == nil && arg == nil {
			return NewErrorf(ErrAtomSpawnArgInvalid, "Spawn atom without arg, name=(%s)", name)
		}
		data = d
	}
	if err := current.Interface.AtomSpawner(a, a.atomos.instance, arg, data); err != nil {
		return err
	}
	return nil
}
