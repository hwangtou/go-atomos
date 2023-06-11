package go_atomos

import (
	"container/list"
	"fmt"
	"regexp"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// ElementLocal
// 本地Element实现。
// Implementation of local Element.

type ElementLocal struct {
	// CosmosSelf引用。
	// Reference to CosmosSelf.
	main *CosmosLocal

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

	//// Available or Reloading
	//avail bool
	// 当前ElementImplementation的引用。
	// Reference to current in use ElementImplementation.
	current *ElementImplementation

	*idFirstSyncCallLocal
	*idTrackerManager
	*messageTrackerManager
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
	}
	e := &ElementLocal{
		main:                  main,
		atomos:                nil,
		atoms:                 nil,
		names:                 list.New(),
		lock:                  sync.RWMutex{},
		current:               impl,
		idFirstSyncCallLocal:  &idFirstSyncCallLocal{},
		idTrackerManager:      &idTrackerManager{},
		messageTrackerManager: &messageTrackerManager{},
	}
	e.atomos = NewBaseAtomos(id, impl.Interface.Config.LogLevel, e, impl.Developer.ElementConstructor(), main.process.logging)
	e.idTrackerManager.init(e)
	e.messageTrackerManager.init(e.atomos, len(impl.ElementHandlers))

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
	return e.main
}

func (e *ElementLocal) State() AtomosState {
	return e.atomos.GetState()
}

// SyncMessagingByName
// 同步调用，通过名字调用Element的消息处理函数。
func (e *ElementLocal) SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error) {
	if callerID == nil {
		return nil, NewError(ErrFrameworkIncorrectUsage, "Element: SyncMessagingByName without fromID.").AddStack(e)
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
				return nil, err.AddStack(e)
			}
			defer callerID.unsetSyncMessageAndFirstCall()
		} else {
			// 如果不为空，则检查是否和push向的ID的当前curFirstSyncCall一样，
			if eFirst := e.getCurFirstSyncCall(); callerFirst == eFirst {
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
		firstSyncCall = e.nextFirstSyncCall()
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
		callerID.pushAsyncMessageCallbackMailAndWaitReply(name, out, err, callback)
	})
}

func (e *ElementLocal) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := e.current.Interface.ElementDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

func (e *ElementLocal) Kill(callerID SelfID, timeout time.Duration) *Error {
	return NewError(ErrFrameworkIncorrectUsage, "Element: Cannot kill an element.").AddStack(e)
}

func (e *ElementLocal) SendWormhole(callerID SelfID, timeout time.Duration, wormhole AtomosWormhole) *Error {
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
				return err.AddStack(e)
			}
			defer callerID.unsetSyncMessageAndFirstCall()
		} else {
			// 如果不为空，则检查是否和push向的ID的当前curFirstSyncCall一样，
			if eFirst := e.getCurFirstSyncCall(); callerFirst == eFirst {
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
		firstSyncCall = e.nextFirstSyncCall()
	}

	return e.atomos.PushWormholeMailAndWaitReply(callerID, firstSyncCall, timeout, wormhole)
}

func (e *ElementLocal) getIDTrackerManager() *idTrackerManager {
	return e.idTrackerManager
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

// Implementation of atomos.SelfID
//
// SelfID，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// SelfID, a concept that provide Atom resource access to inner Atom.
// With SelfID, Atom can access its self-main with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provides Log and Tasks method to inner Atom.

func (e *ElementLocal) CosmosMain() *CosmosLocal {
	return e.main
}

// KillSelf
// Atom kill itself from inner
func (e *ElementLocal) KillSelf() {
	if err := e.pushKillMail(e, false, 0); err != nil {
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
	return e.main.runnable.config.Customize
}

func (e *ElementLocal) pushAsyncMessageCallbackMailAndWaitReply(name string, in proto.Message, err *Error, callback func(out proto.Message, err *Error)) {
	e.atomos.PushAsyncMessageCallbackMailAndWaitReply(name, in, err, callback)
}

// Implementation of ElementSelfID

func (e *ElementLocal) Persistence() AutoDataPersistence {
	p, ok := e.atomos.instance.(AutoDataPersistence)
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

func (e *ElementLocal) GetAtomID(name string, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	if tracker == nil {
		return nil, nil, NewErrorf(ErrFrameworkInternalError, "IDTrackerInfo: IDTrackerInfo is nil.").AddStack(e)
	}
	e.lock.RLock()
	atom, hasAtom := e.atoms[name]
	e.lock.RUnlock()
	if hasAtom && atom.atomos.isNotHalt() {
		return atom, atom.idTrackerManager.addIDTracker(tracker), nil
	}
	// Auto data persistence.
	persistence, ok := e.current.Developer.(AutoDataPersistence)
	if !ok || persistence == nil {
		return nil, nil, NewErrorf(ErrAtomNotExists, "Atom: Atom not exists. name=(%s)", name).AddStack(e)
	}
	return e.elementAtomSpawn(name, nil, e.current, persistence, tracker)
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
		info[atomLocal.String()] = fmt.Sprintf(" -> %s\n", atomLocal.idTrackerManager)
	}
	return info
}

func (e *ElementLocal) SpawnAtom(name string, arg proto.Message, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	// Auto data persistence.
	persistence, _ := e.current.Developer.(AutoDataPersistence)
	return e.elementAtomSpawn(name, arg, e.current, persistence, tracker)
}

func (e *ElementLocal) ScaleGetAtomID(callerID SelfID, name string, timeout time.Duration, in proto.Message, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	if callerID == nil {
		return nil, nil, NewError(ErrFrameworkIncorrectUsage, "Element: ScaleGetAtomID without fromID.").AddStack(e)
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
				return nil, nil, err.AddStack(e)
			}
			defer callerID.unsetSyncMessageAndFirstCall()
		} else {
			// 如果不为空，则检查是否和push向的ID的当前curFirstSyncCall一样，
			if eFirst := e.getCurFirstSyncCall(); callerFirst == eFirst {
				// 如果一样，则是循环调用死锁，返回错误。
				return nil, nil, NewErrorf(ErrIDFirstSyncCallDeadlock, "IDFirstSyncCall: Sync call is dead lock. callerID=(%v),firstSyncCall=(%s)", callerID, callerFirst).AddStack(nil)
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
		firstSyncCall = e.nextFirstSyncCall()
	}

	idTracker := tracker.newScaleIDTracker()
	id, err := e.atomos.PushScaleMailAndWaitReply(callerID, firstSyncCall, name, timeout, in, idTracker)
	if err != nil {
		return nil, nil, err.AddStack(e, &String{S: name}, in)
	}
	return id, idTracker, nil
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

	handler := e.current.ElementHandlers[name]
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

func (e *ElementLocal) OnSyncMessagingCallback(in proto.Message, err *Error, callback func(reply proto.Message, err *Error)) {
	callback(in, err)
}

func (e *ElementLocal) OnScaling(fromID ID, firstSyncCall, name string, in proto.Message, tracker *IDTracker) (id ID, err *Error) {
	if fromID == nil {
		return nil, NewError(ErrFrameworkInternalError, "Element: OnScaling without fromID.").AddStack(e)
	}
	if tracker == nil {
		return nil, NewError(ErrFrameworkInternalError, "Element: OnScaling without tracker.").AddStack(e)
	}
	if err = e.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
		return nil, err.AddStack(e)
	}
	defer e.unsetSyncMessageAndFirstCall()

	handler := e.current.ScaleHandlers[name]
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
		// Retain New.
		id.getIDTrackerManager().addScaleIDTracker(tracker)
		// Release Old.
		releasable, ok := id.(ReleasableID)
		if ok {
			releasable.Release()
		}
	}()
	return
}

func (e *ElementLocal) OnWormhole(from ID, wormhole AtomosWormhole) *Error {
	holder, ok := e.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		return NewErrorf(ErrAtomosNotSupportWormhole, "Element: Not supports wormhole. type=(%T)", e.atomos.instance).AddStack(e)
	}
	return holder.AcceptWormhole(from, wormhole)
}

func (e *ElementLocal) OnStopping(from ID, cancelled []uint64) (err *Error) {
	e.main.hookStopping(e.atomos.id)
	defer e.main.hookHalt(e.atomos.id, err, e.idTrackerManager, e.messageTrackerManager)

	// Send Kill to all atoms.
	var stopTimeout, stopGap time.Duration
	elemExit, ok := e.current.Developer.(ElementAtomExit)
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
			if err := a.pushKillMail(e, true, stopTimeout); err != nil {
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
	var persistence AutoDataPersistence
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
	persistence, ok = e.current.Developer.(AutoDataPersistence)
	if !ok || persistence == nil {
		err = NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence,
			"Element: OnStopping, saving data error, no auto data persistence. id=(%s)", e.GetIDInfo()).AddStack(e)
		e.Log().Fatal(err.Error())
		goto autoLoad
	}
	elemPersistence = persistence.ElementAutoDataPersistence()
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
	pa, ok := e.current.Developer.(AutoDataPersistenceLoading)
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

// 内部实现
// INTERNAL

func (e *ElementLocal) elementAtomSpawn(name string, arg proto.Message, current *ElementImplementation, persistence AutoDataPersistence, t *IDTrackerInfo) (*AtomLocal, *IDTracker, *Error) {
	if t == nil {
		return nil, nil, NewErrorf(ErrFrameworkInternalError, "Element: Spawn atom failed, id tracker is nil. name=(%s)", name).AddStack(e)
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
				tracker := oldAtom.idTrackerManager.addIDTracker(t)
				return oldAtom, tracker, NewErrorf(ErrAtomIsRunning, "Atom: Atom is running, returns this atom. id=(%s),name=(%s)", e.GetIDInfo(), name).AddStack(oldAtom, arg)
			} else { // 如果正在停止运行，则返回这个状态。TODO 尝试等待Stopping之后再去Spawn（但也需要担心等待时间，Spawn没有timeout）。
				return nil, nil, NewErrorf(ErrAtomIsStopping, "Atom: Atom is stopping. id=(%s),name=(%s)", e.GetIDInfo(), name).AddStack(oldAtom, arg)
			}
		}
		// 如果旧的存在且不再运行，则用旧的Atom的结构体，创建一个新的Atom内容。
		// 先将旧的Atom的内容拷贝到新的Atom。
		oldLock := &oldAtom.atomos.mailbox.mutex
		atom.nameElement = oldAtom.nameElement
		atom.idFirstSyncCallLocal.curCallCounter = oldAtom.idFirstSyncCallLocal.curCallCounter
		atom.idTrackerManager = oldAtom.idTrackerManager // TODO: 验证这种情况下，IDTrackerManager下面还有引用，引用Release的情况。
		// 将新的Atom内容替换到旧的Atom。
		*oldAtom = *atom
		// 将旧的Atom解锁。
		oldLock.Unlock()
		// 把新创建的Atom的指针退换成旧的，这样就可以保证持有旧的Atom的ID能够继续使用。
		atom = oldAtom
	}

	// Atom的Spawn逻辑。
	if err := atom.atomos.start(func() *Error {
		e.main.hookSpawning(atom.atomos.id)
		if err := atom.elementAtomSpawn(current, persistence, arg); err != nil {
			e.main.hookSpawn(atom.atomos.id, err)
			return err.AddStack(nil)
		}
		e.main.hookSpawn(atom.atomos.id, nil)
		return nil
	}); err != nil {
		e.elementAtomRelease(atom, nil)
		return nil, nil, err.AddStack(nil)
	}
	tracker := atom.idTrackerManager.addIDTracker(t)
	return atom, tracker, nil
}

func (e *ElementLocal) elementAtomRelease(atom *AtomLocal, tracker *IDTracker) {
	atom.idTrackerManager.Release(tracker)
	if atom.atomos.isNotHalt() {
		return
	}
	if atom.idTrackerManager.refCount() > 0 {
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
		e.main.process.logging.pushFrameworkErrorLog("Atom: Try releasing a mailbox which is still running. name=(%s)", name)
	}
}

func (e *ElementLocal) elementAtomStopping(atom *AtomLocal) {
	if atom.idTrackerManager.refCount() > 0 {
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
		e.main.process.logging.pushFrameworkErrorLog("Atom: Try stopping a mailbox which is still running. name=(%s)", name)
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
	pa, ok := current.Developer.(AutoDataPersistenceLoading)
	if ok && pa != nil {
		if err = pa.Load(e, runnable.config.Customize); err != nil {
			return err.AddStack(e)
		}
	}
	persistence, ok := current.Developer.(AutoDataPersistence)
	if ok && persistence != nil {
		elemPersistence := persistence.ElementAutoDataPersistence()
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

func (e *ElementLocal) pushKillMail(callerID SelfID, wait bool, timeout time.Duration) *Error {
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
					return err.AddStack(e)
				}
				defer callerID.unsetSyncMessageAndFirstCall()
			} else {
				// 如果不为空，则检查是否和push向的ID的当前curFirstSyncCall一样，
				if eFirst := e.getCurFirstSyncCall(); callerFirst == eFirst {
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
			firstSyncCall = e.nextFirstSyncCall()
		}
	}
	return e.atomos.PushKillMailAndWaitReply(callerID, firstSyncCall, wait, true, timeout)
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
