package go_atomos

import (
	"container/list"
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// ElementLocal
// 本地Element实现。
// Implementation of local Element.

type ElementLocal struct {
	cosmosLocal *CosmosLocal

	// 基础Atomos，也是实现Atom无锁队列的关键。
	// Base atomos, the key of lockless queue of Atom.
	atomos *BaseAtomos

	// 该Element所有Atom的容器。
	// Container of all atoms.
	// 思考：要考虑在频繁变动的情景下，迭代不全的问题。
	// 两种情景：更新&关闭。
	atoms map[string]*AtomLocal
	// Element的List容器的
	names *list.List
	// Lock.
	lock sync.RWMutex

	// 当前ElementImplementation的引用。
	// Reference to current in use ElementImplementation.
	elemImpl *ElementImplementation
}

// 生命周期相关
// Life Cycle

// 本地Element创建，用于本地Cosmos的创建过程。
// Create of the Local Element, uses in Local Cosmos creation.
func newElementLocal(main *CosmosLocal, runnable *CosmosRunnable, impl *ElementImplementation) *ElementLocal {
	id := &IDInfo{
		Type:    IDType_Element,
		Cosmos:  runnable.config.Cosmos,
		Node:    runnable.config.Node,
		Element: impl.Interface.Config.Name,
		Atom:    "",
		Version: main.atomos.id.Version,
		//GoId:    0,
	}
	e := &ElementLocal{
		cosmosLocal: main,
		atomos:      nil,
		atoms:       nil,
		names:       list.New(),
		lock:        sync.RWMutex{},
		elemImpl:    impl,
	}
	e.atomos = NewBaseAtomos(id, impl.Interface.Config.LogLevel, e, impl.Developer.ElementConstructor(), main.process)

	// 如果实现了ElementCustomizeAtomInitNum接口，那么就使用接口中定义的数量。
	if atomsInitNum, ok := impl.Developer.(ElementAtomInitNum); ok {
		num := atomsInitNum.GetElementAtomsInitNum()
		e.atoms = make(map[string]*AtomLocal, num)
	} else {
		e.atoms = map[string]*AtomLocal{}
	}
	return e
}

//
// Implementation of ID
//

func (e *ElementLocal) GetIDInfo() *IDInfo {
	if e == nil {
		return nil
	}
	return e.atomos.GetIDInfo()
}

func (e *ElementLocal) String() string {
	if e == nil {
		return "nil"
	}
	return e.atomos.String()
}

func (e *ElementLocal) Cosmos() CosmosNode {
	return e.cosmosLocal
}

func (e *ElementLocal) State() AtomosState {
	return e.atomos.GetState()
}

func (e *ElementLocal) IdleTime() time.Duration {
	return e.atomos.idleTime()
}

// SyncMessagingByName
// 同步调用，通过名字调用Element的消息处理函数。
func (e *ElementLocal) SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error) {
	if callerID == nil {
		return nil, NewError(ErrFrameworkIncorrectUsage, "Element: SyncMessagingByName without fromID.").AddStack(e)
	}

	firstSyncCall, toDefer, err := e.atomos.syncGetFirstSyncCallName(callerID)
	if err != nil {
		return nil, err.AddStack(e)
	}
	if toDefer {
		defer callerID.unsetSyncMessageAndFirstCall()
	}

	out, err = e.atomos.PushMessageMailAndWaitReply(callerID, firstSyncCall, name, timeout, in)
	if err != nil {
		err = err.AddStack(e)
	}
	return
}

// AsyncMessagingByName
// 异步调用，通过名字调用Element的消息处理函数。
func (e *ElementLocal) AsyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message, callback func(proto.Message, *Error)) {
	if callerID == nil {
		callback(nil, NewError(ErrFrameworkIncorrectUsage, "Element: AsyncMessagingByName without fromID.").AddStack(e))
		return
	}

	// 这种情况需要创建新的FirstSyncCall，因为这是一个新的调用链，调用的开端是push向的ID。
	firstSyncCall := e.nextFirstSyncCall()

	e.Parallel(func() {
		out, err := e.atomos.PushMessageMailAndWaitReply(callerID, firstSyncCall, name, timeout, in)
		if err != nil {
			err = err.AddStack(e)
		}
		callerID.pushAsyncMessageCallbackMailAndWaitReply(name, firstSyncCall, out, err, callback)
	})
}

func (e *ElementLocal) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := e.elemImpl.Interface.ElementDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

func (e *ElementLocal) Kill(callerID SelfID, timeout time.Duration) *Error {
	return NewError(ErrFrameworkIncorrectUsage, "Element: Cannot kill an element.").AddStack(e)
}

func (e *ElementLocal) SendWormhole(callerID SelfID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	return e.atomos.PushWormholeMailAndWaitReply(callerID, timeout, wormhole)
}

func (e *ElementLocal) getGoID() uint64 {
	return e.atomos.GetGoID()
}

// Implementation of AtomosUtilities

func (e *ElementLocal) Log() Logging {
	return e.atomos.Log()
}

func (e *ElementLocal) Task() Task {
	return e.atomos.Task()
}

// Implementation of idFirstSyncCall

func (e *ElementLocal) getCurFirstSyncCall() string {
	return e.atomos.fsc.getCurFirstSyncCall()
}

func (e *ElementLocal) setSyncMessageAndFirstCall(s string) *Error {
	return e.atomos.fsc.setSyncMessageAndFirstCall(s)
}

func (e *ElementLocal) unsetSyncMessageAndFirstCall() {
	e.atomos.fsc.unsetSyncMessageAndFirstCall()
}

func (e *ElementLocal) nextFirstSyncCall() string {
	return e.atomos.fsc.nextFirstSyncCall()
}

// Implementation of atomos.SelfID
//
// SelfID，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// SelfID, a concept that provide Atom resource access to inner Atom.
// With SelfID, Atom can access its self-main with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provides Log and Tasks method to inner Atom.

func (e *ElementLocal) CosmosMain() *CosmosLocal {
	return e.cosmosLocal
}

// KillSelf
// Atom kill itself from inner
func (e *ElementLocal) KillSelf() {
	if err := e.atomos.PushKillMailAndWaitReply(e, false, 0); err != nil {
		e.Log().Error("Element: KillSelf failed. err=(%v)", err)
		return
	}
	e.Log().Info("Element: KillSelf.")
}

func (e *ElementLocal) Parallel(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				err := NewErrorf(ErrFrameworkRecoverFromPanic, "Element: Parallel recovered from panic.").AddPanicStack(e, 3, r)
				if ar, ok := e.atomos.instance.(AtomosRecover); ok {
					defer func() {
						recover()
						e.Log().Fatal("Element: Parallel critical problem again. err=(%v)", err)
					}()
					ar.ParallelRecover(err)
				} else {
					e.Log().Fatal("Element: Parallel critical problem. err=(%v)", err)
				}
			}
		}()
		fn()
	}()
}

func (e *ElementLocal) Config() map[string][]byte {
	return e.cosmosLocal.runnable.config.Customize
}

func (e *ElementLocal) pushAsyncMessageCallbackMailAndWaitReply(name, firstSyncCall string, in proto.Message, err *Error, callback func(out proto.Message, err *Error)) {
	e.atomos.PushAsyncMessageCallbackMailAndWaitReply(name, firstSyncCall, in, err, callback)
}

// Implementation of ElementSelfID

func (e *ElementLocal) Persistence() AutoData {
	p, ok := e.atomos.instance.(AutoData)
	if ok || p == nil {
		return nil
	}
	return p
}

func (e *ElementLocal) GetAtoms() []*AtomLocal {
	e.lock.RLock()
	atoms := make([]*AtomLocal, 0, len(e.atoms))
	for _, atomLocal := range e.atoms {
		if atomLocal.atomos.IsInState(AtomosSpawning, AtomosWaiting, AtomosBusy) {
			atoms = append(atoms, atomLocal)
		}
	}
	e.lock.RUnlock()
	return atoms
}

func (e *ElementLocal) GetAtomsInPattern(pattern string) []*AtomLocal {
	e.lock.RLock()
	atoms := make([]*AtomLocal, 0, len(e.atoms))
	for name, atomLocal := range e.atoms {
		matched, err := regexp.MatchString(pattern, name)
		if err != nil {
			continue
		}
		if !matched {
			continue
		}

		if atomLocal.atomos.IsInState(AtomosSpawning, AtomosWaiting, AtomosBusy) {
			atoms = append(atoms, atomLocal)
		}
	}
	e.lock.RUnlock()
	return atoms
}

// Implementation of Element

func (e *ElementLocal) GetAtomID(name string, tracker *IDTrackerInfo, fromLocalOrRemote bool) (ID, *IDTracker, *Error) {
	if fromLocalOrRemote && tracker == nil {
		return nil, nil, NewErrorf(ErrFrameworkInternalError, "IDTrackerInfo: IDTrackerInfo is nil.").AddStack(e)
	}
	e.lock.RLock()
	atom, hasAtom := e.atoms[name]
	e.lock.RUnlock()
	if hasAtom && atom.atomos.isNotHalt() {
		if fromLocalOrRemote {
			return atom, atom.atomos.it.addIDTracker(tracker, fromLocalOrRemote), nil
		} else {
			return atom, nil, nil
		}
	}
	// Auto data persistence.
	persistence, ok := e.elemImpl.Developer.(AutoData)
	if !ok || persistence == nil {
		return nil, nil, NewErrorf(ErrAtomNotExists, "Atom: Atom not exists. name=(%s)", name).AddStack(e)
	}
	return e.elementAtomSpawn(e, name, nil, e.elemImpl, persistence, tracker, fromLocalOrRemote)
}

func (e *ElementLocal) GetAtomsNum() int {
	e.lock.RLock()
	num := len(e.atoms)
	e.lock.RUnlock()
	return num
}

func (e *ElementLocal) GetActiveAtomsNum() int {
	num := 0
	e.lock.RLock()
	for _, atomLocal := range e.atoms {
		if atomLocal.atomos.IsInState(AtomosSpawning, AtomosWaiting, AtomosBusy) {
			num += 1
		}
	}
	e.lock.RUnlock()
	return num
}

func (e *ElementLocal) GetAllInactiveAtomsIDTrackerInfo() map[string]string {
	e.lock.RLock()
	info := make(map[string]string, len(e.atoms))
	atoms := make([]*AtomLocal, 0, len(e.atoms))
	for _, atomLocal := range e.atoms {
		if atomLocal.atomos.IsInState(AtomosHalt) {
			atoms = append(atoms, atomLocal)
		}
	}
	e.lock.RUnlock()
	for _, atomLocal := range atoms {
		info[atomLocal.String()] = fmt.Sprintf(" -> %s\n", atomLocal.atomos.it.String())
	}
	return info
}

func (e *ElementLocal) SpawnAtom(callerID SelfID, name string, arg proto.Message, tracker *IDTrackerInfo, fromLocalOrRemote bool) (ID, *IDTracker, *Error) {
	// Auto data persistence.
	persistence, _ := e.elemImpl.Developer.(AutoData)
	return e.elementAtomSpawn(callerID, name, arg, e.elemImpl, persistence, tracker, fromLocalOrRemote)
}

func (e *ElementLocal) ScaleGetAtomID(callerID SelfID, name string, timeout time.Duration, in proto.Message, tracker *IDTrackerInfo, fromLocalOrRemote bool) (ID, *IDTracker, *Error) {
	if fromLocalOrRemote && callerID == nil {
		return nil, nil, NewError(ErrFrameworkIncorrectUsage, "Element: ScaleGetAtomID without fromID.").AddStack(e)
	}

	firstSyncCall, toDefer, err := e.atomos.syncGetFirstSyncCallName(callerID)
	if err != nil {
		return nil, nil, err.AddStack(e)
	}
	if toDefer {
		defer callerID.unsetSyncMessageAndFirstCall()
	}

	id, err := e.atomos.PushScaleMailAndWaitReply(callerID, firstSyncCall, name, timeout, in)
	if err != nil {
		return nil, nil, err.AddStack(e, &String{S: name}, in)
	}
	if fromLocalOrRemote {
		atom, ok := id.(*AtomLocal)
		if ok {
			return id, atom.atomos.it.addScaleIDTracker(tracker.newScaleIDTracker()), nil
		}
	}
	return id, nil, nil
}

// 邮箱控制器相关
// Mailbox Handler

func (e *ElementLocal) OnMessaging(fromID ID, firstSyncCall, name string, in proto.Message) (out proto.Message, err *Error) {
	if fromID == nil {
		return nil, NewError(ErrFrameworkInternalError, "Element: OnMessaging without fromID.").AddStack(e)
	}
	if err = e.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
		return nil, err.AddStack(e)
	}
	defer e.unsetSyncMessageAndFirstCall()

	handler := e.elemImpl.ElementHandlers[name]
	if handler == nil {
		return nil, NewErrorf(ErrElementMessageHandlerNotExists,
			"Element: Message handler not found. from=(%s),name=(%s),in=(%v)", fromID, name, in)
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				if err == nil {
					err = NewErrorf(ErrFrameworkRecoverFromPanic, "Element: Messaging recovers from panic.").AddPanicStack(e, 3, r)
					if ar, ok := e.atomos.instance.(AtomosRecover); ok {
						defer func() {
							recover()
							e.Log().Fatal("Element: Messaging critical problem again. err=(%v)", err)
						}()
						ar.MessageRecover(name, in, err)
					} else {
						e.Log().Fatal("Element: Messaging critical problem. err=(%v)", err)
					}
				}
			}
		}()
		out, err = handler(fromID, e.atomos.GetInstance(), in)
	}()
	return
}

func (e *ElementLocal) OnAsyncMessagingCallback(firstSyncCall string, in proto.Message, err *Error, callback func(reply proto.Message, err *Error)) {
	if err = e.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
		e.Log().Fatal("Element: OnAsyncMessagingCallback failed. err=(%v)", err.AddStack(e))
		return
	}
	defer e.unsetSyncMessageAndFirstCall()

	callback(in, err)
}

func (e *ElementLocal) OnScaling(fromID ID, firstSyncCall, name string, in proto.Message) (id ID, err *Error) {
	if fromID == nil {
		return nil, NewError(ErrFrameworkInternalError, "Element: OnScaling without fromID.").AddStack(e)
	}
	if err = e.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
		return nil, err.AddStack(e)
	}
	defer e.unsetSyncMessageAndFirstCall()

	handler := e.elemImpl.ScaleHandlers[name]
	if handler == nil {
		return nil, NewErrorf(ErrElementScaleHandlerNotExists,
			"Element: Scale handler not found. fromID=(%s),name=(%s),in=(%v)", fromID, name, in)
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				if err == nil {
					err = NewErrorf(ErrFrameworkRecoverFromPanic, "Element: Scaling recovers from panic.").AddPanicStack(e, 3, r)
					if ar, ok := e.atomos.instance.(AtomosRecover); ok {
						defer func() {
							recover()
							e.Log().Fatal("Element: Scaling critical problem again. err=(%v)", err)
						}()
						ar.ScaleRecover(name, in, err)
					} else {
						e.Log().Fatal("Element: Scaling critical problem. err=(%v)", err)
					}
				}
			}
		}()
		id, err = handler(fromID, e.atomos.instance, name, in)
		if err != nil {
			return
		}
		if reflect.ValueOf(id).IsNil() {
			return
		}
		//// Retain New.
		//id.getIDTrackerManager().addScaleIDTracker(tracker)
		//// Release Old.
		//releasable, ok := id.(ReleasableID)
		//if ok {
		//	releasable.Release()
		//}
	}()
	return
}

func (e *ElementLocal) OnWormhole(from ID, firstSyncCall string, wormhole AtomosWormhole) *Error {
	if err := e.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
		return err.AddStack(e)
	}
	defer e.unsetSyncMessageAndFirstCall()

	holder, ok := e.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		return NewErrorf(ErrAtomosNotSupportWormhole, "Element: Not supports wormhole. type=(%T)", e.atomos.instance).AddStack(e)
	}
	return holder.AcceptWormhole(from, wormhole)
}

func (e *ElementLocal) OnStopping(from ID, firstSyncCall string, cancelled []uint64) (err *Error) {
	if firstSyncCall != "" {
		if err := e.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
			e.Log().Fatal("Element: Stopping failed. err=(%v)", err.AddStack(e))
		} else {
			defer e.unsetSyncMessageAndFirstCall()
		}
	}

	// Send Kill to all atoms.
	var stopTimeout, stopGap time.Duration
	elemExit, ok := e.elemImpl.Developer.(ElementAtomExit)
	if ok && elemExit != nil {
		stopTimeout = elemExit.StopTimeout()
		stopGap = elemExit.StopGap()
	}
	exitWG := sync.WaitGroup{}
	for nameElem := e.names.Back(); nameElem != nil; nameElem = nameElem.Prev() {
		name := nameElem.Value.(string)
		atom, has := e.atoms[name]
		if !has || atom == nil {
			continue
		}
		e.Log().Info("Element: OnStopping, killing atom. name=(%s)", name)
		exitWG.Add(1)
		go func(a *AtomLocal, n string) {
			if err := a.atomos.PushKillMailAndWaitReply(e, true, stopTimeout); err != nil {
				e.Log().Error("Element: Kill atom failed. name=(%s),err=(%v)", n, err)
			}
			exitWG.Done()
		}(atom, name)
		if stopGap > 0 {
			<-time.After(stopGap)
		}
	}
	exitWG.Wait()
	e.Log().Info("Element: OnStopping, all atoms killed. element=(%s)", e.atomos.id.Element)

	// Element
	var save bool
	var data proto.Message
	var persistence AutoData
	var elemPersistence ElementAutoData
	defer func() {
		if r := recover(); r != nil {
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Element: Stopping recovers from panic.").AddPanicStack(e, 3, r, data)
				if ar, ok := e.atomos.instance.(AtomosRecover); ok {
					defer func() {
						recover()
						e.Log().Fatal("Element: Stopping recovers from panic. err=(%v)", err)
					}()
					ar.StopRecover(err)
				} else {
					e.Log().Fatal("Element: Stopping recovers from panic. err=(%v)", err)
				}
			}
		}
	}()

	save, data = e.atomos.GetInstance().Halt(from, cancelled)
	if !save {
		goto autoLoad
	}

	// Save data.
	// Auto Save
	persistence, ok = e.elemImpl.Developer.(AutoData)
	if !ok || persistence == nil {
		err = NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence,
			"Element: OnStopping, saving data error, no auto data persistence. id=(%s)", e.GetIDInfo()).AddStack(e)
		e.Log().Fatal(err.Error())
		goto autoLoad
	}
	elemPersistence = persistence.ElementAutoData()
	if elemPersistence == nil {
		err = NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence,
			"Element: OnStopping, saving data error, no element auto data persistence. id=(%s)", e.GetIDInfo()).AddStack(e)
		e.Log().Fatal(err.Error())
		return err
	}
	if err = elemPersistence.SetElementData(data); err != nil {
		e.Log().Error("Element: OnStopping, saving data failed, set atom data error. id=(%s),instance=(%+v),err=(%s)",
			e.GetIDInfo(), e.atomos.String(), err.AddStack(e))
		goto autoLoad
	}

autoLoad:

	// Auto Load
	pa, ok := e.elemImpl.Developer.(AutoDataLoader)
	if !ok || pa == nil {
		return nil
	}
	if err = pa.Unload(); err != nil {
		e.Log().Error("Element: OnStopping, unload failed. id=(%s),instance=(%+v),err=(%s)",
			e.GetIDInfo(), e.atomos.String(), err.AddStack(e))
		return err
	}
	return err
}

func (e *ElementLocal) OnIDsReleased() {

}

// 内部实现
// INTERNAL

func (e *ElementLocal) elementAtomSpawn(callerID SelfID, name string, arg proto.Message, current *ElementImplementation, persistence AutoData, t *IDTrackerInfo, fromLocalOrRemote bool) (*AtomLocal, *IDTracker, *Error) {
	if fromLocalOrRemote && t == nil {
		return nil, nil, NewErrorf(ErrFrameworkInternalError, "Element: Spawn atom failed, id tracker is nil. name=(%s)", name).AddStack(e)
	}
	firstSyncCall, toDefer, err := e.atomos.syncGetFirstSyncCallName(callerID)
	if err != nil {
		return nil, nil, err.AddStack(e)
	}
	if toDefer {
		defer callerID.unsetSyncMessageAndFirstCall()
	}

	// Element的容器逻辑。
	// Alloc an atomos and try setting.
	atom, err := newAtomLocal(name, e, current, current.Interface.Config.LogLevel)
	if err != nil {
		return nil, nil, err.AddStack(e)
	}
	// If not exist, lock and set a new one.
	e.lock.Lock()
	oldAtom, has := e.atoms[name]
	if !has {
		e.atoms[name] = atom
		atom.nameElement = e.names.PushBack(name)
	}
	e.lock.Unlock()
	// If exists and running, release new and return error.
	// 不用担心两个Atom同时创建的问题，因为Atom创建的时候就是AtomSpawning了，除非其中一个在极端短的时间内AtomHalt了
	if has {
		oldAtom.atomos.mailbox.mutex.Lock()
		if oldAtom.atomos.state > AtomosHalt { // 如果正在运行，则想办法返回正在运行的Atom。
			oldAtom.atomos.mailbox.mutex.Unlock()
			if oldAtom.atomos.state < AtomosStopping { // 如果正在运行，则返回正在运行的Atom，不允许创建新的Atom。
				err = NewErrorf(ErrAtomIsRunning, "Atom: Atom is running, returns this atom. id=(%s),name=(%s)", e.GetIDInfo(), name)
				if fromLocalOrRemote {
					return oldAtom, oldAtom.atomos.it.addIDTracker(t, fromLocalOrRemote), err.AddStack(oldAtom, arg)
				} else {
					return oldAtom, nil, err.AddStack(oldAtom, arg)
				}
			} else { // 如果正在停止运行，则返回这个状态。TODO 尝试等待Stopping之后再去Spawn（但也需要担心等待时间，Spawn没有timeout）。
				return nil, nil, NewErrorf(ErrAtomIsStopping, "Atom: Atom is stopping. id=(%s),name=(%s)", e.GetIDInfo(), name).AddStack(oldAtom, arg)
			}
		}

		// 如果旧的存在且不再运行，则用旧的Atom的结构体，创建一个新的Atom内容。
		// 先将旧的Atom的内容拷贝到新的Atom。

		// 因为这里的锁是为了保证旧的Atom的内容不会被修改，但是这里的锁是在旧的Atom已经不再运行的情况下上锁，所以不会有并发修改的问题。
		// 完成后再对其解锁，以避免有并发Spawn的情况下，后面的Spawn出现死锁。
		oldLock := &oldAtom.atomos.mailbox.mutex
		// 将旧的Atom的Name元素复制到新的Atom。
		atom.nameElement = oldAtom.nameElement
		// 将旧的Atom的firstSyncCall计数器复制到新的Atom。
		atom.atomos.fsc.curCallCounter = oldAtom.atomos.fsc.curCallCounter
		// 将旧的Atom的IDTrackerManager复制到新的Atom，但AtomosRelease用新的。
		atom.atomos.it = atom.atomos.it.fromOld(oldAtom.atomos.it) // TODO: 验证这种情况下，IDTrackerManager下面还有引用，引用Release的情况。

		// 将新的Atom内容替换到旧的Atom。
		*oldAtom = *atom
		// 将旧的Atom解锁。
		oldLock.Unlock()
		// 把新创建的Atom的指针退换成旧的，这样就可以保证持有旧的Atom的ID能够继续使用。
		atom = oldAtom
	}

	// Atom的Spawn逻辑。
	if err = atom.atomos.start(func() *Error {
		if err := atom.elementAtomSpawn(firstSyncCall, current, persistence, arg); err != nil {
			return err.AddStack(nil)
		}
		return nil
	}); err != nil {
		e.elementAtomRelease(atom)
		return nil, nil, err.AddStack(nil)
	}
	if fromLocalOrRemote {
		return atom, atom.atomos.it.addIDTracker(t, fromLocalOrRemote), nil
	} else {
		return atom, nil, nil
	}
}

func (e *ElementLocal) elementAtomRelease(atom *AtomLocal) {
	if atom.atomos.isNotHalt() {
		return
	}
	if atom.atomos.it.refCount() > 0 {
		return
	}
	e.lock.Lock()

	name := atom.GetIDInfo().Atom
	_, has := e.atoms[name]
	if has {
		delete(e.atoms, name)
	} else {
		e.lock.Unlock()
		return
	}
	if atom.nameElement != nil {
		e.names.Remove(atom.nameElement)
		atom.nameElement = nil
	}
	e.lock.Unlock()

	// assert
	if atom.atomos.mailbox.isRunning() {
		e.cosmosLocal.process.logging.pushFrameworkErrorLog("Atom: Try releasing a mailbox which is still running. name=(%s)", name)
	}
}

func (e *ElementLocal) elementAtomStopping(atom *AtomLocal) {
	if atom.atomos.it.refCount() > 0 {
		return
	}
	e.lock.Lock()

	name := atom.GetIDInfo().Atom
	_, has := e.atoms[name]
	if has {
		delete(e.atoms, name)
	}
	if atom.nameElement != nil {
		e.names.Remove(atom.nameElement)
		atom.nameElement = nil
	}
	e.lock.Unlock()

	// assert
	if atom.atomos.mailbox.isRunning() {
		e.cosmosLocal.process.logging.pushFrameworkErrorLog("Atom: Try stopping a mailbox which is still running. name=(%s)", name)
	}
}

func (e *ElementLocal) cosmosElementSpawn(runnable *CosmosRunnable, current *ElementImplementation) (err *Error) {
	defer func() {
		if r := recover(); r != nil {
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Element: Spawn recovers from panic.").AddPanicStack(e, 3, r)
				if ar, ok := e.atomos.instance.(AtomosRecover); ok {
					defer func() {
						recover()
						e.Log().Fatal("Element: Spawn critical problem again. err=(%v)", err)
					}()
					ar.SpawnRecover(nil, err)
				} else {
					e.Log().Fatal("Element: Spawn critical problem. err=(%v)", err)
				}
			}
		}
	}()

	// Get data and Spawning.
	var data proto.Message
	// 尝试进行自动数据持久化逻辑，如果支持的话，就会被执行。
	// 会从对象中GetAtomData，如果返回错误，证明服务不可用，那将会拒绝Atom的Spawn。
	// 如果GetAtomData拿不出数据，且Spawn没有传入参数，则认为是没有对第一次Spawn的Atom传入参数，属于错误。
	pa, ok := current.Developer.(AutoDataLoader)
	if ok && pa != nil {
		if err = pa.Load(e, runnable.config.Customize); err != nil {
			return err.AddStack(e)
		}
	}
	persistence, ok := current.Developer.(AutoData)
	if ok && persistence != nil {
		elemPersistence := persistence.ElementAutoData()
		if elemPersistence != nil {
			data, err = elemPersistence.GetElementData()
			if err != nil {
				return err.AddStack(e)
			}
		}
	}
	if err := current.Interface.ElementSpawner(e, e.atomos.instance, data); err != nil {
		return err.AddStack(e)
	}
	return nil
}

func (e *ElementLocal) getAtom(name string) *AtomLocal {
	e.lock.RLock()
	atom, hasAtom := e.atoms[name]
	e.lock.RUnlock()
	if hasAtom && atom.atomos.isNotHalt() {
		return atom
	}
	return nil
}
