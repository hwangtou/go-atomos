package go_atomos

// CHECKED!

import (
	"container/list"
	"google.golang.org/protobuf/proto"
	"runtime/debug"
	"sync"

	"github.com/hwangtou/go-atomos/core"
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
	// The reference only be set when atomos load and mainFn reload.
	element *ElementLocal

	//// 邮箱，也是实现Atom无锁队列的关键。
	//// Mailbox, the key of lockless queue of Atom.
	//mailbox *mailBox
	atomos *core.BaseAtomos

	// 实际的Id类型
	id ID
	// 引用计数，所以任何GetId操作之后都需要Release。
	// Reference count, thus we have to Release any Id after GetId.
	count int

	// Element的List容器的Element
	nameElement *list.Element

	// 升级次数版本
	reloads int
	// 当前实现
	current *ElementImplementation

	// 调用链
	// 调用链用于检测是否有循环调用，在处理message时把fromId的调用链加上自己之后
	callChain []ID
}

// Implementation of core.ID

func (a *AtomLocal) GetIDInfo() *core.IDInfo {
	return a.atomos.GetIDInfo()
}

//
// Implementation of atomos.ID
//
// Id，相当于Atom的句柄的概念。
// 通过Id，可以访问到Atom所在的Cosmos、Element、Name，以及发送Kill信息，但是否能成功Kill，还需要AtomCanKill函数的认证。
// 直接用AtomLocal继承Id，因此本地的Id直接使用AtomLocal的引用即可。
//
// Id, a concept similar to file descriptor of an atomos.
// With Id, we can access the Cosmos, Element and Name of the Atom. We can also send Kill signal to the Atom,
// then the AtomCanKill method judge kill it or not.
// AtomLocal implements Id interface directly, so local Id is able to use AtomLocal reference directly.

func (a *AtomLocal) getCallChain() []ID {
	return a.callChain
}

func (a *AtomLocal) Release() {
	a.element.atomosRelease(a)
	//a.atomos.holder.atomosRelease(a.atomos)
}

func (a *AtomLocal) Cosmos() CosmosNode {
	return a.element.mainFn
}

func (a *AtomLocal) Element() Element {
	return a.element
}

func (a *AtomLocal) GetName() string {
	return a.atomos.GetIDInfo().Atomos
}

//func (a *AtomLocal) GetVersion() uint64 {
//	return a.current.Interface.Config.Version
//}

// Kill
// 从另一个AtomLocal，或者从Main Script发送Kill消息给Atom。
// write Kill signal from other AtomLocal or from Main Script.
// 如果不实现ElementCustomizeAuthorization，则说明没有Kill的ID限制。
func (a *AtomLocal) Kill(from ID) *core.ErrorInfo {
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

// Implementation of atomos.AtomSelf
// Implementation of atomos.ParallelSelf
//
// AtomSelf，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// AtomSelf, a concept that provide Atom resource access to inner Atom.
// With AtomSelf, Atom can access its self-mainFn with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provide Log and Tasks method to inner Atom.

func (a *AtomLocal) CosmosMainFn() *CosmosMainFn {
	return a.element.mainFn
}

func (a *AtomLocal) ElementSelf() *ElementLocal {
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

func (a *AtomLocal) Log() core.Logging {
	return a.atomos.Log()
}

func (a *AtomLocal) Task() core.Task {
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

// Atom对象的内存池
// Atom instance pools.
var atomLocalPool = sync.Pool{
	New: func() interface{} {
		return &AtomLocal{}
	},
}

// 内部实现
// INTERNAL

// 生命周期相关
// Life Cycle

func newAtomLocal(name string, e *ElementLocal, reloads int, current *ElementImplementation, log *core.LoggingAtomos, lv core.LogLevel) *AtomLocal {
	id := &core.IDInfo{
		Type:    core.IDType_Atomos,
		Cosmos:  e.Cosmos().GetNodeName(),
		Element: e.GetElementName(),
		Atomos:  name,
	}
	a := atomLocalPool.Get().(*AtomLocal)
	a.element = e
	a.atomos = core.NewBaseAtomos(id, log, lv, a, current.Developer.AtomConstructor())
	a.reloads = reloads
	a.id = current.Interface.AtomIdConstructor(a)
	a.current = current
	return a
}

func (a *AtomLocal) deleteAtomLocal() {
	a.atomos.DeleteAtomos()
	atomLocalPool.Put(a)
}

// 邮箱控制器相关
// Mailbox Handler
// TODO: Performance tracer.

// 推送邮件，并管理邮件对象的生命周期。
// 处理邮件，并设置Atom的运行状态。
//
// PushProcessLog Mail, and manage life cycle of Mail instance.
// Handle Mail, and set the state of Atom.

// Message Mail

func (a *AtomLocal) pushMessageMail(from ID, name string, args proto.Message) (reply proto.Message, err *core.ErrorInfo) {
	return a.atomos.PushMessageMailAndWaitReply(from, name, args)
}

func (a *AtomLocal) OnMessaging(from core.ID, name string, args proto.Message) (reply proto.Message, err *core.ErrorInfo) {
	a.atomos.SetBusy()
	defer a.atomos.SetWaiting()
	handler := a.current.AtomHandlers[name]
	if handler == nil {
		return nil, core.NewErrorf(core.ErrAtomMessageHandlerNotExists,
			"Message handler not found, from=(%s),name=(%s),args=(%v)", from, name, args)
	}
	var stack []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				stack = debug.Stack()
			}
		}()
		reply, err = handler(from.(ID), a.atomos.GetInstance(), args)
	}()
	if len(stack) != 0 {
		err = core.NewErrorfWithStack(core.ErrAtomMessageHandlerPanic, stack,
			"Message handler PANIC, from=(%s),name=(%s),args=(%v)", from, name, args)
	}
	return
}

// Kill Mail

func (a *AtomLocal) pushKillMail(from ID, wait bool) *core.ErrorInfo {
	return a.atomos.PushKillMailAndWaitReply(from, wait)
}

// 有状态的Atom会在Halt被调用之后调用AtomSaver函数保存状态，期间Atom状态为Stopping。
// Stateful Atom will save data after Halt method has been called, while is doing this, Atom is set to Stopping.

func (a *AtomLocal) OnStopping(from core.ID, cancelled map[uint64]core.CancelledTask) (err *core.ErrorInfo) {
	//a.atomos.SetStopping()
	defer func() {
		if r := recover(); r != nil {
			err = core.NewErrorfWithStack(core.ErrAtomKillHandlerPanic, debug.Stack(),
				"Kill RECOVERED, id=(%s),instance=(%+v),reason=(%s)", a.atomos.GetIDInfo(), a.atomos.Description(), r)
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
		err = core.NewErrorf(core.ErrAtomKillElementNoImplement,
			"Save data error, no element implement, id=(%s),element=(%+v)", a.atomos.GetIDInfo(), a.element)
		a.Log().Fatal(err.Message)
		return err
	}
	p, ok := impl.Developer.(ElementCustomizeAutoDataPersistence)
	if !ok || p == nil {
		err = core.NewErrorf(core.ErrAtomKillElementNotImplementAutoDataPersistence,
			"Save data error, no element auto data persistence, id=(%s),element=(%+v)", a.atomos.GetIDInfo(), a.element)
		a.Log().Fatal(err.Message)
		return err
	}
	if err = p.AtomAutoDataPersistence().SetAtomData(a.GetName(), data); err != nil {
		a.Log().Error("Save data failed, set atom data error, id=(%s),instance=(%+v),err=(%s)",
			a.atomos.GetIDInfo(), a.atomos.Description(), err)
		return err
	}
	return err
}

// Reload Mail

// 重载邮件，指定Atom的版本。
// Reload Mail with specific version.
func (a *AtomLocal) pushReloadMail(from ID, elem *ElementImplementation, upgrades int) *core.ErrorInfo {
	return a.atomos.PushReloadMailAndWaitReply(from, elem, upgrades)
}

// TODO: Test.
// 注意：Reload伴随着整个Element的Reload，会先调用Atom的Halt，再Spawn。但不会删除正在执行的任务。
// Notice: Atom reload goes with the Element reload, Halt and Spawn will be called in order. It won't delete tasks.

func (a *AtomLocal) OnReloading(reloadInterface interface{}, reloads int) {
	a.atomos.SetBusy()
	defer a.atomos.SetWaiting()

	// 如果没有新的Element，就用旧的Element。
	// Use old Element if there is no new Element.
	reload, ok := reloadInterface.(*ElementImplementation)
	if !ok || reload == nil {
		err := core.NewErrorf(core.ErrAtomReloadInvalid, "Reload is invalid, reload=(%v),reloads=(%d)", reload, reloads)
		a.Log().Fatal(err.Message)
		return
	}
	if reloads == a.reloads {
		return
	}
	a.reloads = reloads

	newAtom := reload.Developer.AtomConstructor()
	oldAtom := a.atomos.ReloadInstance(newAtom)
	newAtom.Reload(oldAtom)
}
