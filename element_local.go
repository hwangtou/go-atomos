package go_atomos

// CHECKED!

import (
	"container/list"
	"runtime/debug"
	"sync"

	"google.golang.org/protobuf/proto"
)

// ElementLocal
// 本地Element实现。
// Implementation of local Element.
type ElementLocal struct {
	// CosmosSelf引用。
	// Reference to CosmosSelf.
	main *CosmosMain

	// 基础Atomos，也是实现Atom无锁队列的关键。
	// Base atomos, the key of lockless queue of Atom.
	atomos *BaseAtomos

	//// 实际的Id类型
	//id ID

	// 该Element所有Atom的容器。
	// Container of all atoms.
	// 思考：要考虑在频繁变动的情景下，迭代不全的问题。
	// 两种情景：更新&关闭。
	atoms map[string]*AtomLocal
	// Element的List容器的
	names *list.List
	// Lock.
	lock sync.RWMutex

	//// Available or Reloading
	//avail bool
	// 当前ElementImplementation的引用。
	// Reference to current in use ElementImplementation.
	current *ElementImplementation

	// 调用链
	// 调用链用于检测是否有循环调用，在处理message时把fromId的调用链加上自己之后
	callChain []ID
}

// 生命周期相关
// Life Cycle

// 本地Element创建，用于本地Cosmos的创建过程。
// Create of the Local Element, uses in Local Cosmos creation.
func newElementLocal(main *CosmosMain, runnable *CosmosRunnable, impl *ElementImplementation) *ElementLocal {
	id := &IDInfo{
		Type:    IDType_Element,
		Cosmos:  runnable.config.Node,
		Element: impl.Interface.Config.Name,
		Atomos:  "",
	}
	elem := &ElementLocal{
		main:      main,
		atomos:    nil,
		atoms:     nil,
		names:     list.New(),
		lock:      sync.RWMutex{},
		current:   impl,
		callChain: nil,
	}
	log, logLevel := main.process.sharedLog, impl.Interface.Config.LogLevel
	elem.atomos = NewBaseAtomos(id, log, logLevel, elem, impl.Developer.ElementConstructor(), 0)
	if atomsInitNum, ok := impl.Developer.(ElementCustomizeAtomInitNum); ok {
		elem.atoms = make(map[string]*AtomLocal, atomsInitNum.GetElementAtomsInitNum())
	} else {
		elem.atoms = map[string]*AtomLocal{}
	}
	return elem
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

func (e *ElementLocal) GetIDInfo() *IDInfo {
	return e.atomos.GetIDInfo()
}

func (e *ElementLocal) Release() {
}

func (e *ElementLocal) Cosmos() CosmosNode {
	return e.main
}

func (e *ElementLocal) Element() Element {
	return e
}

func (e *ElementLocal) GetName() string {
	return e.GetIDInfo().Element
}

func (e *ElementLocal) Kill(from ID) *ErrorInfo {
	return NewError(ErrElementCannotKill, "Cannot kill element")
}

func (e *ElementLocal) String() string {
	return e.atomos.Description()
}

func (e *ElementLocal) getCallChain() []ID {
	return e.callChain
}

func (a *ElementLocal) getElementLocal() *ElementLocal {
	return a
}

func (a *ElementLocal) getAtomLocal() *AtomLocal {
	return nil
}

// Implementation of atomos.SelfID
// Implementation of atomos.ParallelSelf
//
// SelfID，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// SelfID, a concept that provide Atom resource access to inner Atom.
// With SelfID, Atom can access its self-main with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provides Log and Tasks method to inner Atom.

func (e *ElementLocal) CosmosMain() *CosmosMain {
	return e.main
}

// KillSelf
// Atom kill itself from inner
func (e *ElementLocal) KillSelf() {
	if err := e.pushKillMail(e, false); err != nil {
		e.Log().Error("KillSelf error, err=%v", err)
		return
	}
	e.Log().Info("KillSelf")
}

// Implementation of Element

func (e *ElementLocal) GetElementName() string {
	return e.GetIDInfo().Element
}

func (e *ElementLocal) GetAtomId(name string) (ID, *ErrorInfo) {
	return e.elementAtomGet(name)
}

func (e *ElementLocal) GetAtomsNum() int {
	e.lock.RLock()
	num := len(e.atoms)
	e.lock.RUnlock()
	return num
}

func (e *ElementLocal) SpawnAtom(name string, arg proto.Message) (*AtomLocal, *ErrorInfo) {
	e.lock.RLock()
	current := e.current
	e.lock.RUnlock()
	// Auto data persistence.
	persistence, _ := current.Developer.(ElementCustomizeAutoDataPersistence)
	return e.elementAtomSpawn(name, arg, current, persistence)
}

func (e *ElementLocal) MessageElement(fromId, toId ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	if fromId == nil {
		return reply, NewErrorf(ErrAtomFromIDInvalid, "From ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)",
			fromId, toId, name, args)
	}
	a := toId.getElementLocal()
	if a == nil {
		return reply, NewErrorf(ErrAtomToIDInvalid, "To ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)",
			fromId, toId, name, args)
	}
	// PushProcessLog.
	return a.pushMessageMail(fromId, name, args)
}

func (e *ElementLocal) MessageAtom(fromId, toId ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	if fromId == nil {
		return reply, NewErrorf(ErrAtomFromIDInvalid, "From ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)",
			fromId, toId, name, args)
	}
	a := toId.getAtomLocal()
	if a == nil {
		return reply, NewErrorf(ErrAtomToIDInvalid, "To ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)",
			fromId, toId, name, args)
	}
	// PushProcessLog.
	return a.pushMessageMail(fromId, name, args)
}

func (e *ElementLocal) KillAtom(fromId, toId ID) *ErrorInfo {
	if fromId == nil {
		return NewErrorf(ErrAtomFromIDInvalid, "From ID invalid, from=(%s),to=(%s)", fromId, toId)
	}
	a := toId.getElementLocal()
	if a == nil {
		return NewErrorf(ErrAtomToIDInvalid, "To ID invalid, from=(%s),to=(%s)", fromId, toId)
	}
	// Dead Lock Checker.
	if !a.checkCallChain(fromId.getCallChain()) {
		return NewErrorf(ErrAtomCallDeadLock, "Call Dead Lock, chain=(%v),to(%s)", fromId.getCallChain(), toId)
	}
	a.addCallChain(fromId.getCallChain())
	defer a.delCallChain()
	// PushProcessLog.
	return a.pushKillMail(fromId, true)
}

// Implementation of AtomosUtilities

func (e *ElementLocal) Log() Logging {
	return e.atomos.Log()
}

func (e *ElementLocal) Task() Task {
	return e.atomos.Task()
}

// Check chain.

func (e *ElementLocal) checkCallChain(fromIdList []ID) bool {
	for _, fromId := range fromIdList {
		if fromId.GetIDInfo().IsEqual(e.GetIDInfo()) {
			return false
		}
	}
	return true
}

func (e *ElementLocal) addCallChain(fromIdList []ID) {
	e.callChain = append(fromIdList, e)
}

func (e *ElementLocal) delCallChain() {
	e.callChain = nil
}

// 内部实现
// INTERNAL

// 邮箱控制器相关
// Mailbox Handler
// TODO: Performance tracer.

func (e *ElementLocal) pushMessageMail(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	// Dead Lock Checker.
	if from != nil {
		if !e.checkCallChain(from.getCallChain()) {
			return reply, NewErrorf(ErrAtomCallDeadLock, "Call Dead Lock, chain=(%v),to(%s),name=(%s),args=(%v)",
				from.getCallChain(), e, name, args)
		}
		e.addCallChain(from.getCallChain())
		defer e.delCallChain()
	}
	return e.atomos.PushMessageMailAndWaitReply(from, name, args)
}

func (e *ElementLocal) OnMessaging(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	handler := e.current.ElementHandlers[name]
	if handler == nil {
		return nil, NewErrorf(ErrElementMessageHandlerNotExists,
			"ElementLocal: Message handler not found, from=(%s),name=(%s),args=(%v)", from, name, args)
	}
	var stack []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				stack = debug.Stack()
			}
		}()
		fromID, _ := from.(ID)
		reply, err = handler(fromID, e.atomos.GetInstance(), args)
	}()
	if len(stack) != 0 {
		err = NewErrorf(ErrElementMessageHandlerPanic,
			"ElementLocal: Message handler PANIC, from=(%s),name=(%s),args=(%v)", from, name, args).
			AddStack(e.GetIDInfo(), stack)
	} else if err != nil && len(err.Stacks) > 0 {
		err = err.AddStack(e.GetIDInfo(), debug.Stack())
	}
	return
}

func (e *ElementLocal) pushKillMail(from ID, wait bool) *ErrorInfo {
	return e.atomos.PushKillMailAndWaitReply(from, wait)
}

func (e *ElementLocal) OnStopping(from ID, cancelled map[uint64]CancelledTask) (err *ErrorInfo) {
	//defer func() {
	//	if r := recover(); r != nil {
	//		err = NewErrorf(ErrElementKillHandlerPanic,
	//			"ElementHandler: Kill RECOVERED, id=(%s),instance=(%+v),reason=(%s)", e.GetIDInfo(), e.atomos.Description(), r).
	//			AddStack(e.GetIDInfo(), debug.Stack())
	//		e.Log().Error(err.Message)
	//	}
	//}()

	// Atomos
	// Send Kill to all atoms.
	for nameElem := e.names.Back(); nameElem != nil; nameElem = nameElem.Prev() {
		name := nameElem.Value.(string)
		atom := e.atoms[name]
		e.Log().Info("ElementLocal: Kill atomos, name=(%s)", name)
		err := atom.pushKillMail(e, true)
		if err != nil {
			e.Log().Error("ElementLocal: Kill atomos failed, name=(%s),err=(%v)", name, err)
		}
	}
	e.Log().Info("ElementLocal: Atoms killed, element=(%s)", e.GetName())

	// Element
	save, data := e.atomos.GetInstance().Halt(from, cancelled)
	if !save {
		return nil
	}

	// Save data.
	impl := e.current
	if impl == nil {
		err = NewErrorf(ErrAtomKillElementNoImplement,
			"ElementHandler: Save data error, no element implement, id=(%s),element=(%+v)", e.GetIDInfo(), e)
		e.Log().Fatal(err.Message)
		return err
	}
	p, ok := impl.Developer.(ElementCustomizeAutoDataPersistence)
	if !ok || p == nil {
		err = NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence,
			"ElementHandler: Save data error, no element auto data persistence, id=(%s),element=(%+v)", e.GetIDInfo(), e)
		e.Log().Fatal(err.Message)
		return err
	}
	if err = p.ElementAutoDataPersistence().SetElementData(e.GetName(), data); err != nil {
		e.Log().Error("ElementHandler: Save data failed, set atom data error, id=(%s),instance=(%+v),err=(%s)",
			e.GetIDInfo(), e.atomos.Description(), err)
		return err
	}

	return err
}

func (e *ElementLocal) pushReloadMail(from ID, impl *ElementImplementation, reloads int) *ErrorInfo {
	return e.atomos.PushReloadMailAndWaitReply(from, impl, reloads)
}

func (e *ElementLocal) OnReloading(oldElement Atomos, reloadObject AtomosReloadable) (newElement Atomos) {
	// 如果没有新的Element，就用旧的Element。
	// Use old Element if there is no new Element.
	reload, ok := reloadObject.(*ElementImplementation)
	if !ok || reload == nil {
		err := NewErrorf(ErrElementReloadInvalid, "Reload is invalid, reload=(%v),reloads=(%d)", reload, e.atomos.reloads)
		e.Log().Fatal(err.Message)
		return
	}

	newElement = reload.Developer.ElementConstructor()
	newElement.Reload(oldElement)

	// Send Reload to all atoms.
	// 重载Element，需要指定一个版本的ElementImplementation。
	// Reload element, specific version of ElementImplementation is needed.
	for nameElem := e.names.Front(); nameElem != nil; nameElem = nameElem.Next() {
		name := nameElem.Value.(string)
		atom := e.atoms[name]
		e.Log().Info("ElementLocal: Reloading atomos, name=(%s)", name)
		err := atom.pushReloadMail(e, reload, e.atomos.reloads)
		if err != nil {
			e.Log().Error("ElementLocal: Reloading atomos failed, name=(%s),err=(%v)", name, err)
		}
	}
	e.Log().Info("ElementLocal: Atoms reloaded, element=(%s)", e.GetName())
	return newElement
}

// Internal

func (e *ElementLocal) elementAtomGet(name string) (*AtomLocal, *ErrorInfo) {
	e.lock.RLock()
	current := e.current
	atom, hasAtom := e.atoms[name]
	e.lock.RUnlock()
	if hasAtom && atom.atomos.isNotHalt() {
		atom.count += 1
		return atom, nil
	}
	// Auto data persistence.
	persistence, ok := current.Developer.(ElementCustomizeAutoDataPersistence)
	if !ok || persistence == nil {
		return nil, NewErrorf(ErrAtomNotExists, "Atom not exists, name=(%s)", name)
	}
	return e.elementAtomSpawn(name, nil, current, persistence)
}

func (e *ElementLocal) elementAtomSpawn(name string, arg proto.Message, current *ElementImplementation, persistence ElementCustomizeAutoDataPersistence) (*AtomLocal, *ErrorInfo) {
	// Element的容器逻辑。

	// Alloc an atomos and try setting.
	atom := newAtomLocal(name, e, e.atomos.reloads, current, e.atomos.logging, current.Interface.Config.LogLevel)
	//atom := newAtomLocal(name, e, e.reloads, current, e.log, e.logLevel)
	// If not exist, lock and set an new one.
	e.lock.Lock()
	oldAtom, has := e.atoms[name]
	if !has {
		e.atoms[name] = atom
		atom.nameElement = e.names.PushBack(name)
	}
	e.lock.Unlock()
	// If exists and running, release new and return error.
	// 不用担心两个Atom同时创建的问题，因为Atom创建的时候就是AtomSpawning了，除非其中一个在极端短的时间内AtomHalt了
	if has && oldAtom.atomos.isNotHalt() {
		atom.deleteAtomLocal(false)
		return nil, NewErrorf(ErrAtomExists, "Atom exists, name=(%s),arg=(%v)", name, arg)
	}
	// If exists and not running, release new and use old.
	// 如果已经存在，那就不需要新的，用旧的。
	if has {
		atom.deleteAtomLocal(false)
		atom = oldAtom
	}
	atom.count += 1

	// Atom的Spawn逻辑。
	atom.atomos.setSpawning()
	err := atom.elementAtomSpawn(current, persistence, arg)
	if err != nil {
		atom.atomos.setHalt()
		e.elementAtomRelease(atom)
		return nil, err
	}
	atom.atomos.setWaiting()
	return atom, nil
}

func (e *ElementLocal) elementAtomRelease(atom *AtomLocal) {
	atom.count -= 1
	if atom.atomos.isNotHalt() {
		return
	}
	if atom.count > 0 {
		return
	}
	e.lock.Lock()
	name := atom.GetName()
	_, has := e.atoms[name]
	if has {
		delete(e.atoms, name)
	}
	if atom.nameElement != nil {
		e.names.Remove(atom.nameElement)
		atom.nameElement = nil
	}
	e.lock.Unlock()
	atom.deleteAtomLocal(false)
}

func (a *ElementLocal) cosmosElementSpawn(current *ElementImplementation) *ErrorInfo {
	// Get data and Spawning.
	var data proto.Message
	// 尝试进行自动数据持久化逻辑，如果支持的话，就会被执行。
	// 会从对象中GetAtomData，如果返回错误，证明服务不可用，那将会拒绝Atom的Spawn。
	// 如果GetAtomData拿不出数据，且Spawn没有传入参数，则认为是没有对第一次Spawn的Atom传入参数，属于错误。
	persistence, ok := current.Developer.(ElementCustomizeAutoDataPersistence)
	if ok && persistence != nil {
		name := a.GetName()
		d, err := persistence.ElementAutoDataPersistence().GetElementData(name)
		if err != nil {
			return err
		}
		//if d == nil && arg == nil {
		//	return NewErrorf(ErrElementSpawnArgInvalid, "Spawn element without arg, name=(%s)", name)
		//}
		data = d
	}
	if err := current.Interface.ElementSpawner(a, a.atomos.instance, data); err != nil {
		return err
	}
	return nil
}
