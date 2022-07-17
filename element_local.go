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
	mainFn *CosmosMainFn

	// 基础Atomos，也是实现Atom无锁队列的关键。
	// Base atomos, the key of lockless queue of Atom.
	atomos *BaseAtomos

	//// 所有添加过的不同版本的ElementImplementation的容器。
	//// Container of all added versions of ElementImplementation.
	//implements map[uint64]*ElementImplementation

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

	// Available or Reloading
	avail bool
	// 当前ElementImplementation的引用。
	// Reference to current in use ElementImplementation.
	current, reloading *ElementImplementation

	// 调用链
	// 调用链用于检测是否有循环调用，在处理message时把fromId的调用链加上自己之后
	callChain []ID

	//log      *LoggingAtomos
	//logLevel LogLevel
}

// Implementation of ID

func (e *ElementLocal) GetIDInfo() *IDInfo {
	return e.atomos.GetIDInfo()
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

func (e *ElementLocal) getCallChain() []ID {
	return e.callChain
}

func (e *ElementLocal) Release() {
}

func (e *ElementLocal) Cosmos() CosmosNode {
	return e.mainFn
}

func (e *ElementLocal) Element() Element {
	return e
}

func (e *ElementLocal) GetName() string {
	return e.atomos.GetIDInfo().Element
}

func (e *ElementLocal) Kill(from ID) *ErrorInfo {
	return NewError(ErrElementCannotKill, "Cannot kill element")
}

func (e *ElementLocal) String() string {
	return e.atomos.Description()
}

// Implementation of atomos.AtomSelf
// Implementation of atomos.ParallelSelf
//
// AtomSelf，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// AtomSelf, a concept that provide Atom resource access to inner Atom.
// With AtomSelf, Atom can access its self-mainFn with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provides Log and Tasks method to inner Atom.

func (e *ElementLocal) CosmosMainFn() *CosmosMainFn {
	return e.mainFn
}

func (e *ElementLocal) ElementSelf() *ElementLocal {
	return e
}

// KillSelf
// Atom kill itself from inner
func (e *ElementLocal) KillSelf() {
	//id, elem := a.id, a.element
	if err := e.pushKillMail(e, false); err != nil {
		e.Log().Error("KillSelf error, err=%v", err)
		return
	}
	e.Log().Info("KillSelf")
}

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

// Implementation of atomos.Element

func (e *ElementLocal) GetElementName() string {
	return e.current.Interface.Config.Name
}

func (e *ElementLocal) GetAtomId(name string) (ID, *ErrorInfo) {
	atom, err := e.elementGetAtom(name)
	if err != nil {
		return nil, err
	}
	return atom.id, nil
}

func (e *ElementLocal) SpawnAtom(name string, arg proto.Message) (*AtomLocal, *ErrorInfo) {
	return e.elementCreateAtom(name, arg)
}

func (e *ElementLocal) MessageAtom(fromId, toId ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	if fromId == nil {
		return reply, NewErrorf(ErrAtomFromIDInvalid, "From ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)",
			fromId, toId, name, args)
	}
	a, ok := toId.(*AtomLocal)
	if !ok || a == nil {
		return reply, NewErrorf(ErrAtomToIDInvalid, "To ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)",
			fromId, toId, name, args)
	}
	// Dead Lock Checker.
	if !a.checkCallChain(fromId.getCallChain()) {
		return reply, NewErrorf(ErrAtomCallDeadLock, "Call Dead Lock, chain=(%v),to(%s),name=(%s),args=(%v)",
			fromId.getCallChain(), toId, name, args)
	}
	a.addCallChain(fromId.getCallChain())
	defer a.delCallChain()
	// PushProcessLog.
	return a.pushMessageMail(fromId, name, args)
}

func (e *ElementLocal) KillAtom(fromId, toId ID) *ErrorInfo {
	if fromId == nil {
		return NewErrorf(ErrAtomFromIDInvalid, "From ID invalid, from=(%s),to=(%s)", fromId, toId)
	}
	a, ok := toId.(*AtomLocal)
	if !ok || a == nil {
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

// 生命周期相关
// Life Cycle

// 本地Element创建，用于本地Cosmos的创建过程。
// Create of the Local Element, uses in Local Cosmos creation.
func newElementLocal(mainFn *CosmosMainFn, impl *ElementImplementation) *ElementLocal {
	id := &IDInfo{
		Type:    IDType_Element,
		Cosmos:  mainFn.config.Node,
		Element: impl.Interface.Name,
		Atomos:  "",
	}
	elem := &ElementLocal{
		mainFn:    mainFn,
		atomos:    nil,
		atoms:     nil,
		names:     list.New(),
		lock:      sync.RWMutex{},
		avail:     false,
		current:   nil,
		reloading: nil,
		callChain: nil,
	}
	log, logLevel := mainFn.process.sharedLog, impl.Interface.Config.LogLevel
	elem.atomos = NewBaseAtomos(id, log, logLevel, elem, impl.Developer.ElementConstructor(), 0)
	if atomsInitNum, ok := impl.Developer.(ElementCustomizeAtomsInitNum); ok {
		elem.atoms = make(map[string]*AtomLocal, atomsInitNum.GetElementAtomsInitNum())
	} else {
		elem.atoms = map[string]*AtomLocal{}
	}
	return elem
}

func (e *ElementLocal) pushMessageMail(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
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
		reply, err = handler(from.(ID), e.atomos.GetInstance(), args)
	}()
	if len(stack) != 0 {
		err = NewErrorfWithStack(ErrElementMessageHandlerPanic, stack,
			"ElementLocal: Message handler PANIC, from=(%s),name=(%s),args=(%v)", from, name, args)
	}
	return
}

func (e *ElementLocal) pushKillMail(from ID, wait bool) *ErrorInfo {
	return e.atomos.PushKillMailAndWaitReply(from, wait)
}

func (e *ElementLocal) OnStopping(from ID, cancelled map[uint64]CancelledTask) (err *ErrorInfo) {
	defer func() {
		if r := recover(); r != nil {
			err = NewErrorfWithStack(ErrElementKillHandlerPanic, debug.Stack(),
				"Kill RECOVERED, id=(%s),instance=(%+v),reason=(%s)", e.atomos.GetIDInfo(), e.atomos.Description(), r)
			e.Log().Error(err.Message)
		}
	}()
	save, data := e.atomos.GetInstance().Halt(from, cancelled)
	if !save {
		return nil
	}
	// Save data.
	impl := e.current
	if impl == nil {
		err = NewErrorf(ErrAtomKillElementNoImplement,
			"Save data error, no element implement, id=(%s),element=(%+v)", e.atomos.GetIDInfo(), e)
		e.Log().Fatal(err.Message)
		return err
	}
	p, ok := impl.Developer.(ElementCustomizeAutoDataPersistence)
	if !ok || p == nil {
		err = NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence,
			"Save data error, no element auto data persistence, id=(%s),element=(%+v)", e.atomos.GetIDInfo(), e)
		e.Log().Fatal(err.Message)
		return err
	}
	if err = p.ElementAutoDataPersistence().SetElementData(e.GetName(), data); err != nil {
		e.Log().Error("Save data failed, set atom data error, id=(%s),instance=(%+v),err=(%s)",
			e.atomos.GetIDInfo(), e.atomos.Description(), err)
		return err
	}
	return err
}

func (e *ElementLocal) pushReloadMail(from ID, elem *ElementImplementation, upgrades int) *ErrorInfo {
	return e.atomos.PushReloadMailAndWaitReply(from, elem, upgrades)
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
	return newElement
}

// TODO

func (e *ElementLocal) loadElementSetDefine(define *ElementImplementation, reload bool) *ErrorInfo {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.avail = false

	if !reload {
		e.current = define
		e.reloading = nil
		//e.implements[define.Interface.Config.Version] = define
	} else {
		e.reloading = define
	}

	if wh, ok := define.Developer.(ElementLoadable); ok {
		if !reload {
			return wh.Load(e.atomos)
		} else {
			return wh.Reload(e.atomos, e.atomos)
		}
	}
	return nil
}

func (e *ElementLocal) rollback(isReload, loadFailed bool) {
	e.lock.Lock()
	defer e.lock.Unlock()
	if !loadFailed {
		var dev ElementDeveloper
		if e.reloading != nil {
			dev = e.reloading.Developer
		} else {
			dev = e.current.Developer
		}
		if wh, ok := dev.(ElementLoadable); ok {
			wh.Unload()
		}
	}
	if isReload {
		e.avail = true
		e.reloading = nil
	}
}

// For reloading only.
func (e *ElementLocal) commit(isReload bool) {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.avail = true
	if isReload {
		e.current = e.reloading
		e.reloading = nil
		//if _, has := e.implements[e.current.Interface.Config.Version]; !has {
		//	e.implements[e.current.Interface.Config.Version] = e.current
		//}
	}
}

// 重载Element，需要指定一个版本的ElementImplementation。
// Reload element, specific version of ElementImplementation is needed.
func (e *ElementLocal) pushReload(reloads int) {
	e.lock.Lock()
	atomNameList := make([]string, 0, e.names.Len())
	for nameElem := e.names.Front(); nameElem != nil; nameElem = nameElem.Next() {
		atomNameList = append(atomNameList, nameElem.Value.(string))
	}
	e.lock.Unlock()
	wg := sync.WaitGroup{}
	for _, name := range atomNameList {
		wg.Add(1)
		go func(name string) {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					e.Log().Fatal("Element.Reload: Panic, name=%s,reason=%s", name, r)
				}
			}()
			e.lock.Lock()
			atom, has := e.atoms[name]
			e.lock.Unlock()
			if !has {
				return
			}
			e.Log().Info("Element.Reload: Reloading atomos, name=%s", name)
			err := atom.pushReloadMail(e, e.current, reloads)
			if err != nil {
				e.Log().Error("Element.Reload: PushProcessLog reload failed, name=%s,err=%v", name, err)
			}
		}(name)
	}
	wg.Wait()
}

// Internal

func (e *ElementLocal) elementGetAtom(name string) (*AtomLocal, *ErrorInfo) {
	e.lock.RLock()
	current := e.current
	atom, hasAtom := e.atoms[name]
	e.lock.RUnlock()
	if hasAtom && atom.atomos.IsNotHalt() {
		atom.count += 1
		return atom, nil
	}
	persistence, ok := current.Developer.(ElementCustomizeAutoDataPersistence)
	if !ok || persistence == nil {
		return nil, NewErrorf(ErrAtomNotExists, "Atom not exists, name=(%s)", name)
	}
	return e.elementCreateAtom(name, nil)
}

func (e *ElementLocal) elementCreateAtom(name string, arg proto.Message) (*AtomLocal, *ErrorInfo) {
	e.lock.RLock()
	//current, reload := e.current, e.reloads
	current := e.current
	e.lock.RUnlock()
	// Alloc an atomos and try setting.
	atom := newAtomLocal(name, e, e.reloads, current, e.log, e.logLevel)
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
	if has && oldAtom.atomos.IsNotHalt() {
		atom.deleteAtomLocal()
		return nil, NewErrorf(ErrAtomExists, "Atom exists, name=(%s),arg=(%v)", name, arg)
	}
	// If exists and not running, release new and use old.
	if has {
		atom.deleteAtomLocal()
		atom = oldAtom
		atom.count += 1
	}
	// Get data and Spawning.
	var data proto.Message
	if p, ok := current.Developer.(ElementCustomizeAutoDataPersistence); ok && p != nil {
		d, err := p.AtomAutoDataPersistence().GetAtomData(name)
		if err != nil {
			atom.atomos.SetHalt()
			e.atomosRelease(atom)
			return nil, err
		}
		if d == nil && arg == nil {
			atom.atomos.SetHalt()
			e.atomosRelease(atom)
			return nil, NewErrorf(ErrAtomSpawnArgInvalid, "Spawn atom without arg, name=(%s)", name)
		}
		data = d
	}
	if err := e.elementSpawningAtom(atom, current, arg, data); err != nil {
		e.atomosRelease(atom)
		return nil, err
	}
	return atom, nil
}

func (e *ElementLocal) elementSpawningAtom(a *AtomLocal, impl *ElementImplementation, arg, data proto.Message) *ErrorInfo {
	//initMailBox(a.atomos)
	a.atomos.mailbox.start()
	if err := impl.Interface.AtomSpawner(a, a.atomos.instance, arg, data); err != nil {
		a.atomos.setHalt()
		//delMailBox(a.mailbox)
		a.atomos.mailbox.stop()
		return err
	}
	a.atomos.setWaiting()
	return nil
}

func (e *ElementLocal) atomosRelease(atom *AtomLocal) {
	atom.count -= 1
	if atom.atomos.IsNotHalt() {
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
	atom.deleteAtomLocal()
}

func (e *ElementLocal) atomsNum() int {
	e.lock.RLock()
	num := len(e.atoms)
	e.lock.RUnlock()
	return num
}

func (e *ElementLocal) unload() {
	e.lock.Lock()
	atomNameList := make([]string, 0, e.names.Len())
	for nameElem := e.names.Front(); nameElem != nil; nameElem = nameElem.Next() {
		atomNameList = append(atomNameList, nameElem.Value.(string))
	}
	e.lock.Unlock()
	wg := sync.WaitGroup{}
	for _, name := range atomNameList {
		wg.Add(1)
		go func(name string) {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					e.Log().Fatal("Element.Unload: Panic, name=%s,reason=%s", name, r)
				}
			}()
			e.lock.Lock()
			atom, has := e.atoms[name]
			e.lock.Unlock()
			if !has {
				return
			}
			e.Log().Info("Element.Unload: Kill atomos, name=%s", name)
			err := atom.pushKillMail(e, true)
			if err != nil {
				e.Log().Error("Element.Unload: Kill atomos error, name=%s,err=%v", name, err)
			}
		}(name)
	}
	wg.Wait()
}
