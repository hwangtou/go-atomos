package go_atomos

// CHECKED!

import (
	"google.golang.org/protobuf/proto"
	"runtime/debug"
	"sync"

	"github.com/hwangtou/go-atomos/core"
)

// Atom Error

//var (
//	ErrRemoteNotAllowed = errors.New("atomos remote not allowed")
//
//	ErrFromNotFound     = errors.New("atomos fromId not found")
//	ErrAtomNotFound     = errors.New("atomos not found")
//	ErrAtomExists       = errors.New("atomos exists")
//	ErrAtomCannotSpawn  = errors.New("atomos cannot spawn")
//	ErrAtomIsNotRunning = errors.New("atomos is not running")
//	ErrAtomCannotKill   = errors.New("atomos cannot be killed")
//	ErrNotWormhole      = errors.New("atomos is not a wormhole")
//
//	ErrAtomType             = errors.New("atomos type error")
//	ErrAtomMessageAtomType  = errors.New("atomos message atomos type error")
//	ErrAtomMessageArgType   = errors.New("atomos message arg type error")
//	ErrAtomMessageReplyType = errors.New("atomos message reply type error")
//)

// Actual Atom
// The implementations of a local Atom type.

type AtomLocal struct {
	// 对目前ElementLocal实例的引用。
	// 具体指向的ElementLocal对象对AtomLocal是只读的，因此对其所有的操作都需要加上读锁。
	// 指向ElementLocal引用只可以在Atom加载和Cosmos被更新时改变。
	//
	// Reference to current ElementLocal instance.
	// The concrete ElementLocal instance should be read-only, so read-lock is required when access to it.
	// The reference only be set when atomos load and cosmos upgrade.
	element *ElementLocal

	//// ElementInterface的版本
	//// Version of ElementInterface.
	//version uint64

	//// ElementLocal中的唯一Name。
	//// Unique Name of atomos in the ElementLocal.
	//name string

	//// 开发者实现的Atom类型的实例，通过ElementLocal中的AtomCreator方法创建。
	////
	//// Atom-type instance that has been created by AtomConstructor method of ElementLocal,
	//// which is implemented by developer.
	//instance Atomos

	//// 邮箱，也是实现Atom无锁队列的关键。
	//// Mailbox, the key of lockless queue of Atom.
	//mailbox *mailBox
	atomos *core.BaseAtomos

	//// 任务管理器，用于处理来自Atom内部的任务调派。
	//// Task Manager, uses to handle Task from inner Atom.
	//task atomTasksManager
	//
	//// 日志管理器，用于处理来自Atom内部的日志。
	//// Logs Manager, uses to handle Log from inner Atom.
	//log atomLogsManager

	//// Atom信息的protobuf对象，以便于Atom信息的序列化。
	//// Protobuf instance of Atom information, for a convenience serialization of Atom information.
	//atomId *IDInfo

	//// 引用计数，所以任何GetId操作之后都需要Release。
	//// Reference count, thus we have to Release any Id after GetId.
	//count int

	// 升级次数版本
	upgrades int

	// 实际的Id类型
	id ID

	// 当前实现
	current *ElementImplementation
}

func (a *AtomLocal) CosmosSelf() *CosmosProcess {
	return a.element.cosmos
}

func (a *AtomLocal) ElementSelf() Element {
	return a.element
}

func (a *AtomLocal) Log() core.Logging {
	return &a.atomos.log
}

func (a *AtomLocal) Task() core.Task {
	return &a.atomos.task
}

func (a *AtomLocal) String() string {
	return a.atomos.id.str()
}

func (a *AtomLocal) GetVersion() uint64 {
	return a.current.Interface.Config.Version
}

// Atom对象的内存池
// Atom instance pools.
var atomLocalPool = sync.Pool{
	New: func() interface{} {
		return &AtomLocal{}
	},
}

//
// Implementation of Id
//
// Id，相当于Atom的句柄的概念。
// 通过Id，可以访问到Atom所在的Cosmos、Element、Name，以及发送Kill信息，但是否能成功Kill，还需要AtomCanKill函数的认证。
// 直接用AtomLocal继承Id，因此本地的Id直接使用AtomLocal的引用即可。
//
// Id, a concept similar to file descriptor of an atomos.
// With Id, we can access the Cosmos, Element and Name of the Atom. We can also send Kill signal to the Atom,
// then the AtomCanKill method judge kill it or not.
// AtomLocal implements Id interface directly, so local Id is able to use AtomLocal reference directly.

func (a *AtomLocal) Release() {
	a.atomos.holder.atomosRelease(a.atomos)
}

func (a *AtomLocal) Cosmos() CosmosNode {
	return a.element.cosmos.runtime
}

func (a *AtomLocal) Element() Element {
	return a.element
}

func (a *AtomLocal) GetName() string {
	return a.atomos.id.Atomos
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

//
// Implementation of AtomSelf
//
// AtomSelf，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// AtomSelf, a concept that provide Atom resource access to inner Atom.
// With AtomSelf, Atom can access its self-cosmos with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provide Log and Tasks method to inner Atom.

//func (a *AtomLocal) CosmosSelf() *CosmosSelf {
//	return a.element.cosmos
//}
//
//func (a *AtomLocal) ElementSelf() Element {
//	return a.element
//}

// KillSelf
// Atom kill itself from inner
func (a *AtomLocal) KillSelf() {
	//id, elem := a.id, a.element
	if err := a.pushKillMail(a, false); err != nil {
		a.atomos.log.Error("KillSelf error, err=%v", err)
		return
	}
	a.atomos.log.Info("KillSelf")
}

//func (a *AtomLocal) Log() *atomLogsManager {
//	return &a.log
//}
//
//func (a *AtomLocal) Task() TaskManager {
//	return &a.task
//}

//func (a *AtomLocal) CallNameWithProtoBuffer(name string, buf []byte) (proto.Message, *ErrorInfo) {
//	handler, has := a.element.current.Interface.AtomMessages[name]
//	if !has {
//		return nil, ErrAtomMessageAtomType
//	}
//	in, err := handler.InDec(buf)
//	if err != nil {
//		return nil, ErrAtomMessageArgType
//	}
//	return a.pushMessageMail(a, name, in)
//}

//func (a *AtomLocal) Parallel(fn ParallelFn, msg proto.Message, ids ...ID) {
//	go func() {
//		defer func() {
//			if r := recover(); r != nil {
//				a.atomos.log.Fatal("Atom.Parallel: Panic, id=%s,reason=%s", a.atomId.str(), r)
//			}
//		}()
//		fn(a, msg, ids...)
//	}()
//}

// 内部实现
// INTERNAL

// 生命周期相关
// Life Cycle
// Objective-C likes coding style: Alloc/Init/Release/Dealloc

func allocAtomLocal() *AtomLocal {
	return atomLocalPool.Get().(*AtomLocal)
}

func initAtomLocal(name string, a *AtomLocal, e *ElementLocal, current *ElementImplementation, log *loggingMailBox, lv LogLevel) {
	id := &IDInfo{
		Type:    IDType_Atomos,
		Cosmos:  e.cosmos.GetName(),
		Element: e.GetElementName(),
		Atomos:  name,
	}
	a.element = e
	a.atomos = allocBaseAtomos()
	initBaseAtomos(a.atomos, id, log, lv, e, current.Developer.AtomConstructor())
	a.id = current.Interface.AtomIdConstructor(a)
	a.current = current
	//a.element = es
	//a.current = current
	//a.version = current.Interface.Config.Version
	//a.name = name
	//a.instance = inst
	//a.state = AtomosSpawning
	//a.baseAtomos.id = &IDInfo{
	//	Type:    IDType_Atomos,
	//	Cosmos:  a.CosmosSelf().GetName(),
	//	Element: a.Element().GetElementName(),
	//	Atomos:  a.name,
	//}
	//a.count = 1
	//a.upgrades = upgrade
	//a.id = current.Interface.AtomIdConstructor(a)
	//initAtomLog(&a.log, a)
	//initAtomTasksManager(&a.task, a)
}

func releaseAtomLocal(a *AtomLocal) {
	releaseAtomos(a.atomos)
	//releaseAtomTask(&a.task)
	//releaseAtomLog(&a.log)
}

func deallocAtomLocal(a *AtomLocal) {
	atomLocalPool.Put(a)
}

// 邮箱控制器相关
// Mailbox Handler
// TODO: Performance tracer.

// 推送邮件，并管理邮件对象的生命周期。
// 处理邮件，并设置Atom的运行状态。
//
// Push Mail, and manage life cycle of Mail instance.
// Handle Mail, and set the state of Atom.

// Message Mail

// TODO: dead-lock loop checking
func (a *AtomLocal) pushMessageMail(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	am := allocAtomosMail()
	initMessageMail(am, from, name, args)
	if ok := a.atomos.mailbox.pushTail(am.mail); !ok {
		return reply, NewErrorf(ErrAtomosIsNotRunning,
			"Atomos is not running, from=(%s),name=(%s),args=(%v)", from, name, args)
	}
	replyInterface, err := am.waitReply()
	deallocAtomosMail(am)
	if err.Code == ErrAtomosIsNotRunning {
		return nil, err
	}
	reply, ok := replyInterface.(proto.Message)
	if !ok {
		//return reply, fmt.Errorf("Atom.Mail: Reply type error, name=%s,args=%+v,reply=%+v",
		//	name, args, replyInterface)
		return nil, err
	}
	return reply, err
}

func (a *AtomLocal) handleMessage(from ID, name string, args proto.Message) (out proto.Message, err *ErrorInfo) {
	a.atomos.setBusy()
	defer a.atomos.setWaiting()
	handler := a.current.AtomHandlers[name]
	if handler == nil {
		return nil, NewErrorf(ErrAtomMessageHandlerNotExists,
			"Message handler not found, from=(%s),name=(%s),args=(%v)", from, name, args)
	}
	var stack []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				stack = debug.Stack()
			}
		}()
		out, err = handler(from, a.atomos.instance, args)
	}()
	if len(stack) != 0 {
		err = NewErrorfWithStack(ErrAtomMessageHandlerPanic, stack,
			"Message handler PANIC, from=(%s),name=(%s),args=(%v)", from, name, args)
	}
	return
}

// Kill Mail

func (a *AtomLocal) pushKillMail(from ID, wait bool) *ErrorInfo {
	am := allocAtomosMail()
	initKillMail(am, from)
	if ok := a.atomos.mailbox.pushHead(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos is not running, from=(%s),wait=(%v)", from, wait)
	}
	if wait {
		_, err := am.waitReply()
		deallocAtomosMail(am)
		return err
	}
	return nil
}

// 有状态的Atom会在Halt被调用之后调用AtomSaver函数保存状态，期间Atom状态为Stopping。
// Stateful Atom will save data after Halt method has been called, while is doing this, Atom is set to Stopping.
func (a *AtomLocal) handleKill(killAtomMail *atomosMail, cancels map[uint64]CancelledTask) (err *ErrorInfo) {
	a.atomos.setStopping()
	defer func() {
		if r := recover(); r != nil {
			err = NewErrorfWithStack(ErrAtomKillHandlerPanic, debug.Stack(),
				"Kill RECOVERED, id=(%s),instance=(%+v),reason=(%s)", a.atomos.id, a.atomos.instance, r)
			a.atomos.log.Error(err.Message)
		}
	}()
	data := a.atomos.instance.Halt(killAtomMail.from, cancels)
	if impl := a.element.current; impl != nil {
		if p, ok := impl.Developer.(ElementCustomizeAutoDataPersistence); ok && p != nil {
			if err = p.Persistence().SetAtomData(a.atomos.id.Atomos, data); err != nil {
				a.atomos.log.Error("Kill save atomos failed, id=(%s),instance=(%+v),reason=(%s)", a.atomos.id, a.atomos.instance)
				return err
			}
		}
	}
	return err
}

// Reload Mail

// 重载邮件，指定Atom的版本。
// Reload Mail with specific version.
func (a *AtomLocal) pushReloadMail(elem *ElementImplementation, upgradeCount int) *ErrorInfo {
	am := allocAtomosMail()
	initReloadMail(am, elem, upgradeCount)
	if ok := a.atomos.mailbox.pushHead(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos is not running, upgrade=(%d)", upgradeCount)
	}
	_, err := am.waitReply()
	deallocAtomosMail(am)
	return err
}

// TODO: Test.
// 注意：Reload伴随着整个Element的Reload，会先调用Atom的Halt，再Spawn。但不会删除正在执行的任务。
// Notice: Atom reload goes with the Element reload, Halt and Spawn will be called in order. It won't delete tasks.
func (a *AtomLocal) handleReload(am *atomosMail) *ErrorInfo {
	a.atomos.setBusy()
	defer a.atomos.setWaiting()

	// 如果没有新的Element，就用旧的Element。
	// Use old Element if there is no new Element.
	if am.upgrade == nil {
		return NewErrorf(ErrAtomUpgradeInvalid, "Upgrade is invalid, mail=(%v)", am)
	}
	if am.upgradeCount == a.upgrades {
		return nil
	}
	a.upgrades = am.upgradeCount
	upgrade := am.upgrade
	// 释放邮件。
	// Dealloc Atom Mail.
	deallocAtomosMail(am)

	// Save old data.
	//data := a.atomos.instance.Halt(a.element.cosmos.runtime.mainAtom, map[uint64]CancelledTask{})
	data := a.atomos.instance.Halt(a.element, map[uint64]CancelledTask{})
	// Restoring data and replace instance.
	a.atomos.instance = upgrade.Developer.AtomConstructor()
	if err := upgrade.Interface.AtomSpawner(a, a.atomos.instance, nil, data); err != nil {
		a.atomos.log.Info("Reload atom failed, id=(%s),inst=(%+v),data=(%+v)", a.atomos.id, a.atomos.instance, data)
		return err
	}
	return nil
}

// Wormhole Mail

const (
	// 接受新的wormhole
	wormholeAccept = iota

	// 关闭旧的Wormhole
	wormholeClose
)

//// WormholeElement向WormholeAtom发送WormholeDaemon。
//// WormholeElement sends WormholeDaemon to WormholeAtom.
//func (a *AtomLocal) pushWormholeMail(action int, wormhole WormholeDaemon) error {
//	am := allocAtomosMail()
//	initWormholeMail(am, action, wormhole)
//	if ok := a.atomos.mailbox.pushHead(am.mail); !ok {
//		return ErrAtomIsNotRunning
//	}
//	_, err := am.waitReply()
//	deallocAtomosMail(am)
//	return err
//}
//
//func (a *AtomLocal) handleWormhole(action int, wormhole WormholeDaemon) *ErrorInfo {
//	a.atomos.setBusy()
//	defer a.atomos.setWaiting()
//
//	wa, ok := a.atomos.instance.(WormholeAtom)
//	if !ok {
//		return ErrNotWormhole
//	}
//	var err error
//	switch action {
//	case wormholeAccept:
//		err = wa.AcceptWorm(wormhole)
//	case wormholeClose:
//		wa.CloseWorm(wormhole)
//	default:
//		err = ErrAtomMessageArgType
//	}
//	return err
//}
//
//// WormholeId Implementation.
//func (a *AtomLocal) Accept(daemon WormholeDaemon) error {
//	if err := a.pushWormholeMail(wormholeAccept, daemon); err != nil {
//		return err
//	}
//	// Daemon read until return.
//	go func() {
//		defer func() {
//			if r := recover(); r != nil {
//				a.atomos.log.Fatal("Atom.Wormhole: Panic, id=%s,reason=%s", a.atomId.str(), r)
//			}
//		}()
//		a.atomos.log.Info("Atom.Wormhole: Daemon, id=%s", a.atomId.str())
//		if err := daemon.Daemon(a); err != nil {
//			if err = a.pushWormholeMail(wormholeClose, daemon); err != nil {
//				a.atomos.log.Error("Atom.Wormhole: Daemon close error, id=%s,err=%s", a.atomId.str(), err)
//			}
//		}
//	}()
//	return nil
//}
