package go_atomos

// CHECKED!

import (
	"container/list"
	"fmt"
	"google.golang.org/protobuf/proto"
	"runtime"
	"runtime/debug"
	"sync"
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
	count int

	// Element的List容器的Element
	nameElement *list.Element

	// 当前实现
	current *ElementImplementation

	// 调用链
	// 调用链用于检测是否有循环调用，在处理message时把fromID的调用链加上自己之后
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
	a.atomos = NewBaseAtomos(id, log, lv, a, current.Developer.AtomConstructor(name), reloads)
	//a.id = current.Interface.AtomIDConstructor(a)
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

//func (a *AtomLocal) MessageByName(from ID, name string, buf []byte, protoOrJSON bool) ([]byte, *ErrorInfo) {
//	decoderFn, has := a.current.AtomDecoders[name]
//	if !has {
//		return nil, NewErrorf(ErrAtomMessageHandlerNotExists, "Atom message decoder not exists, from=(%v),name=(%s)", from, name).AutoStack(nil, nil)
//	}
//	in, err := decoderFn.InDec(buf, protoOrJSON)
//	if err != nil {
//		return nil, err
//	}
//
//	var outBuf []byte
//	out, err := a.pushMessageMail(from, name, in)
//	if out != nil && reflect.TypeOf(out).Kind() == reflect.Pointer && !reflect.ValueOf(out).IsNil() {
//		var e error
//		if protoOrJSON {
//			outBuf, e = proto.Marshal(out)
//		} else {
//			outBuf, e = json.Marshal(out)
//		}
//		if e != nil {
//			return nil, NewErrorf(ErrAtomMessageReplyType, "Reply marshal failed, err=(%v)", err)
//		}
//	}
//	return outBuf, err
//}

func (a *AtomLocal) MessageByName(from ID, name string, in proto.Message) (proto.Message, *ErrorInfo) {
	return a.pushMessageMail(from, name, in)
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
func (a *AtomLocal) Kill(from ID) *ErrorInfo {
	dev := a.element.current.Developer
	elemAuth, ok := dev.(ElementCustomizeAuthorization)
	if ok && elemAuth != nil {
		if err := elemAuth.AtomCanKill(from); err != nil {
			return err
		}
	}
	return a.pushKillMail(from, true)
}

func (a *AtomLocal) SendWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo {
	return a.atomos.PushWormholeMailAndWaitReply(from, wormhole)
}

func (a *AtomLocal) State() AtomosState {
	return a.atomos.state
}

func (a *AtomLocal) IdleDuration() time.Duration {
	//if a.atomos.state != AtomosWaiting {
	//	return 0
	//}
	return time.Now().Sub(a.atomos.lastBusy)
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

func (a *AtomLocal) Parallel(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				a.Log().Fatal("Parallel PANIC, stack=(%s)", string(stack))
			}
		}()
		fn()
	}()
}

// Implementation of AtomSelfID

func (a *AtomLocal) Config() map[string]string {
	return a.element.main.runnable.config.Customize
}

func (a *AtomLocal) Persistence() AtomAutoDataPersistence {
	p, ok := a.element.atomos.instance.(ElementCustomizeAutoDataPersistence)
	if ok || p == nil {
		return nil
	}
	return p.AtomAutoDataPersistence()
}

func (a *AtomLocal) MessageSelfByName(from ID, name string, buf []byte, protoOrJSON bool) ([]byte, *ErrorInfo) {
	handlerFn, has := a.current.AtomHandlers[name]
	if !has {
		return nil, NewErrorf(ErrAtomMessageHandlerNotExists, "Handler not exists, from=(%v),name=(%s)", from, name).AutoStack(nil, nil)
	}
	decoderFn, has := a.current.AtomDecoders[name]
	if !has {
		return nil, NewErrorf(ErrAtomMessageHandlerNotExists, "Atom message self decoder not exists, from=(%v),name=(%s)", from, name).AutoStack(nil, nil)
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
			return nil, NewErrorf(ErrAtomMessageReplyType, "Reply marshal failed, err=(%v)", err)
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

func (a *AtomLocal) Transaction() Transaction {
	return a.atomos.Transaction()
}

// Check chain.

func (a *AtomLocal) checkCallChain(fromIDList []ID) bool {
	for _, fromID := range fromIDList {
		if fromID.GetIDInfo().IsEqual(a.GetIDInfo()) {
			return false
		}
	}
	return true
}

func (a *AtomLocal) addCallChain(fromIDList []ID) {
	a.callChain = append(fromIDList, a)
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
	//var stack []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				_, file, line, ok := runtime.Caller(2)
				if !ok {
					file, line = "???", 0
				}
				if err == nil {
					err = NewErrorf(ErrFrameworkPanic, "OnMessage, Recover from panic, reason=(%s),file=(%s),line=(%d)", r, file, line)
				}
				err.Panic = string(debug.Stack())
				err.AddStack(a, file, fmt.Sprintf("%v", r), line, args)
			}
		}()
		fromID, _ := from.(ID)
		reply, err = handler(fromID, a.atomos.GetInstance(), args)
	}()
	//if len(stack) != 0 {
	//	err = NewErrorf(ErrAtomMessageHandlerPanic,
	//		"AtomLocal: Message handler PANIC, from=(%s),name=(%s),args=(%v)\nstack=(%s)", from, name, args, stack).
	//		AddStack(a.GetIDInfo(), stack)
	//} else if err != nil && len(err.Stacks) > 0 {
	//	err = err.AddStack(a.GetIDInfo(), debug.Stack())
	//}
	return
}

func (a *AtomLocal) OnScaling(from ID, name string, args proto.Message) (id ID, err *ErrorInfo) {
	return nil, NewError(ErrAtomCannotScale, "OnScaling, atom not supported")
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
			_, file, line, ok := runtime.Caller(2)
			if !ok {
				file, line = "???", 0
			}
			if err == nil {
				err = NewErrorf(ErrFrameworkPanic, "OnStopping, Recover from panic, reason=(%s),file=(%s),line=(%d)", r, file, line)
			}
			err.Panic = string(debug.Stack())
			err.AddStack(a, file, fmt.Sprintf("%v", r), line, nil)
		}
		a.element.elementAtomStopping(a)
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
	persistence, ok := impl.Developer.(ElementCustomizeAutoDataPersistence)
	if !ok || persistence == nil {
		err = NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence,
			"AtomHandler: Save data error, no auto data persistence, id=(%s),element=(%+v)", a.atomos.GetIDInfo(), a.element)
		a.Log().Fatal(err.Message)
		return err
	}
	atomPersistence := persistence.AtomAutoDataPersistence()
	if atomPersistence == nil {
		err = NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence,
			"AtomHandler: Save data error, no atom auto data persistence, id=(%s),element=(%+v)", a.atomos.GetIDInfo(), a.element)
		a.Log().Fatal(err.Message)
		return err
	}
	if err = atomPersistence.SetAtomData(a.GetName(), data); err != nil {
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

	newAtom = reload.Developer.AtomConstructor(a.GetName())
	newAtom.Reload(oldAtom)
	return newAtom
}

func (a *AtomLocal) OnWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo {
	holder, ok := a.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		err := NewErrorf(ErrAtomosNotSupportWormhole, "ElementLocal: Not supported wormhole, type=(%T)", a.atomos.instance)
		a.Log().Error(err.Message)
		return err
	}
	return holder.AcceptWormhole(from, wormhole)
}

// Element

func (a *AtomLocal) elementAtomSpawn(current *ElementImplementation, persistence ElementCustomizeAutoDataPersistence, arg proto.Message) *ErrorInfo {
	// Get data and Spawning.
	var data proto.Message
	// 尝试进行自动数据持久化逻辑，如果支持的话，就会被执行。
	// 会从对象中GetAtomData，如果返回错误，证明服务不可用，那将会拒绝Atom的Spawn。
	// 如果GetAtomData拿不出数据，且Spawn没有传入参数，则认为是没有对第一次Spawn的Atom传入参数，属于错误。
	if persistence != nil {
		atomPersistence := persistence.AtomAutoDataPersistence()
		if atomPersistence != nil {
			name := a.GetName()
			d, err := atomPersistence.GetAtomData(name)
			if err != nil {
				return err
			}
			if d == nil && arg == nil {
				return NewErrorf(ErrAtomSpawnArgInvalid, "Spawn atom without arg, name=(%s)", name)
			}
			data = d
		}
	}
	if err := current.Interface.AtomSpawner(a, a.atomos.instance, arg, data); err != nil {
		return err
	}
	return nil
}
