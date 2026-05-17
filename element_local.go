package atomos

import (
	"container/list"
	"fmt"
	"regexp"
	"runtime"
	"runtime/debug"
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

	spawnLockMap Map[string, *sync.Mutex]

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
	}
	e := &ElementLocal{
		cosmosLocal:  main,
		atomos:       nil,
		atoms:        nil,
		names:        list.New(),
		lock:         sync.RWMutex{},
		spawnLockMap: NewMapGoWithRefCount[string, *sync.Mutex](),
		elemImpl:     impl,
	}
	var logLevel LogLevel
	if customizeLogLevel, ok := impl.Developer.(ElementLogLevel); ok {
		logLevel = customizeLogLevel.GetElementLogLevel()
	} else {
		logLevel = runnable.config.LogLevel
	}
	e.atomos = NewBaseAtomos(e, id, logLevel, e, impl.Developer.ElementConstructor(), main.process)

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
	return e.atomos.GetIDInfo()
}

func (e *ElementLocal) String() string {
	return e.atomos.String()
}

func (e *ElementLocal) Cosmos() CosmosNode {
	return e.cosmosLocal
}

func (e *ElementLocal) State() BaseAtomosState {
	return e.atomos.GetState()
}

func (e *ElementLocal) IdleTime() time.Duration {
	return e.atomos.idleTime()
}

// SyncMessagingByName
// 同步调用，通过名字调用Element的消息处理函数。
func (e *ElementLocal) SyncMessagingByName(callerID ID, name string, in proto.Message, ext []ArgsForBaseAtomos) (out proto.Message, err *Error) {
	return e.atomos.PushSyncMessage(callerID, name, in, ext)
}

// AsyncMessagingByName
// 异步调用，通过名字调用Element的消息处理函数。
func (e *ElementLocal) AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(proto.Message, *Error), ext []ArgsForBaseAtomos) (errBeforeExec *Error) {
	return e.atomos.PushAsyncMessage(callerID, name, in, callback, ext)
}

// asyncCallback
// 内部使用，仅供ID实现调用。
// Internal use only, for ID implementation only.
func (e *ElementLocal) asyncCallback(callerID ID, name string, startupID, asyncID uint64, reply proto.Message, err *Error) {
	e.atomos.PushAsyncMessageCallback(callerID, name, startupID, asyncID, reply, err)
}

func (e *ElementLocal) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := e.elemImpl.Interface.ElementDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

func (e *ElementLocal) Kill(callerID ID, ext []ArgsForBaseAtomos) *Error {
	return NewError(ErrFrameworkIncorrectUsage, "Element: Cannot kill an element.").AddStack(e)
}

func (e *ElementLocal) SendWormhole(callerID ID, wormhole BaseAtomosWormhole, ext []ArgsForBaseAtomos) *Error {
	return e.atomos.PushWormholeMailAndWaitReply(callerID, wormhole, ext)
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
	return e.cosmosLocal
}

// KillSelf
// Atom kill itself from inner
func (e *ElementLocal) KillSelf() {
	if err := e.atomos.PushKillMail(e, nil); err != nil {
		e.Log().Error("Element: KillSelf failed. err=(%v)", err.AddStack(e))
		return
	}
	e.Log().Info("Element: KillSelf.")
}

func (e *ElementLocal) Parallel(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				var err *Error
				defer func() {
					if r2 := recover(); r2 != nil {
						e.Log().Fatal("Element: Parallel critical problem again. err=(%v)", err)
					}
				}()
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Element: Parallel recovered from panic.").AddPanicStack(e, 3, r)
				// Hook or Log
				if ar, ok := e.atomos.instance.(AtomosRecover); ok {
					ar.ParallelRecover(err)
				} else {
					e.Log().Fatal("Element: Parallel critical problem. err=(%v)", err)
				}
				// Global hook
				e.cosmosLocal.process.onRecoverHook(e.atomos.id, err)
			}
		}()
		fn()
	}()
}

func (e *ElementLocal) Config() map[string][]byte {
	return e.cosmosLocal.runnable.config.Customize
}

func (e *ElementLocal) asyncSet(callback func(out proto.Message, err *Error)) (startupID, callbackID uint64) {
	return e.atomos.asyncSet(callback)
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
		if atomLocal.atomos.IsInState(BaseAtomosSpawning, BaseAtomosWaiting, BaseAtomosBusy) {
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

		if atomLocal.atomos.IsInState(BaseAtomosSpawning, BaseAtomosWaiting, BaseAtomosBusy) {
			atoms = append(atoms, atomLocal)
		}
	}
	e.lock.RUnlock()
	return atoms
}

// Implementation of Element

func (e *ElementLocal) GetAtomID(name string, tracker *IDTrackerInfo, fromLocalOrRemote bool, args ...any) (ID, *IDTracker, *Error) {
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
	return e.elementAtomSpawn(e, name, nil, e.elemImpl, persistence, tracker, false, fromLocalOrRemote)
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
		if atomLocal.atomos.IsInState(BaseAtomosSpawning, BaseAtomosWaiting, BaseAtomosBusy) {
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
		if atomLocal.atomos.IsInState(BaseAtomosHalt) {
			atoms = append(atoms, atomLocal)
		}
	}
	e.lock.RUnlock()
	for _, atomLocal := range atoms {
		info[atomLocal.String()] = fmt.Sprintf(" -> %s\n", atomLocal.atomos.it.String())
	}
	return info
}

func (e *ElementLocal) SpawnAtom(callerID ID, name string, arg proto.Message, tracker *IDTrackerInfo, fromLocalOrRemote bool, args ...ArgsForBaseAtomos) (ID, *IDTracker, *Error) {
	// Auto data persistence.
	persistence, _ := e.elemImpl.Developer.(AutoData)
	id, t, err := e.elementAtomSpawn(callerID, name, arg, e.elemImpl, persistence, tracker, true, fromLocalOrRemote, args...)
	if err != nil {
		return id, t, err.AddStack(e)
	}
	return id, t, nil
}

// 邮箱控制器相关
// Mailbox Handler

func (e *ElementLocal) OnSyncMessaging(fromID ID, name string, in proto.Message) (out proto.Message, err *Error) {
	handler := e.elemImpl.ElementHandlers[name]
	if handler == nil {
		return nil, NewErrorf(ErrElementMessageHandlerNotExists,
			"Element: Message handler not found. from=(%s),name=(%s),in=(%v)", fromID, name, in).AddStack(e)
	}
	return e.atomos.OnSyncMessaging(fromID, name, handler, in)
}

func (e *ElementLocal) OnAsyncMessaging(fromID ID, name string, startupID, asyncID uint64, in proto.Message) {
	handler := e.elemImpl.ElementHandlers[name]
	if handler == nil {
		fromID.asyncCallback(e, name, startupID, asyncID, nil, NewErrorf(ErrAtomMessageHandlerNotExists, "Element: OnAsyncMessaging handler not found. from=(%s),name=(%s),in=(%v)", fromID, name, in).AddStack(e))
		return
	}
	e.atomos.OnAsyncMessaging(fromID, e, name, handler, startupID, asyncID, in)
}

func (e *ElementLocal) OnAsyncMessagingCallback(asyncID uint64, in proto.Message, err *Error) {
	e.atomos.OnAsyncMessagingCallback(asyncID, in, err)
}

func (e *ElementLocal) OnFnCallback(callback *atomosCallback) {
	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					e.atomos.log.Fatal("ElementLocal: Recover from panic again when handling task. reason=(%v), stack=(%s)\n", r2, string(debug.Stack()))
				}
			}()
			if f := callback.recoverFn; f != nil {
				f(r)
			} else {
				e.atomos.log.Fatal("ElementLocal: Recover from panic when handling task. reason=(%v), stack=(%s)\n", r, string(debug.Stack()))
			}
		}
	}()
	callback.callback()
}

func (e *ElementLocal) OnWormhole(from ID, wormhole BaseAtomosWormhole) *Error {
	holder, ok := e.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		return NewErrorf(ErrAtomosNotSupportWormhole, "Element: Not supports wormhole. type=(%T)", e.atomos.instance).AddStack(e)
	}
	if err := holder.AcceptWormhole(from, wormhole); err != nil {
		return err.AddStack(e)
	}
	return nil
}

func (e *ElementLocal) OnStopping(from ID, cancelled []uint64) (err *Error) {
	// Send Kill to all atoms.
	var stopTimeout, stopGap time.Duration
	elemExit, ok := e.elemImpl.Developer.(ElementAtomExit)
	if ok && elemExit != nil {
		stopTimeout = elemExit.StopTimeout()
		stopGap = elemExit.StopGap()
	}

	sem := make(chan struct{}, runtime.NumCPU()) // 信号量，用于控制并发goroutine的数量。
	exitWG := sync.WaitGroup{}

	for nameElem := e.names.Back(); nameElem != nil; nameElem = nameElem.Prev() {
		name := nameElem.Value.(string)
		e.lock.RLock()
		atom, has := e.atoms[name]
		e.lock.RUnlock()
		if !has || atom == nil {
			continue
		}
		e.Log().Info("Element: OnStopping, killing atom. name=(%s)", name)

		// 获取信号量的一个槽位。如果信号量已满，则这里将阻塞，直到信号量中有可用的槽位。
		sem <- struct{}{}
		exitWG.Add(1)

		// 启动一个goroutine来处理任务
		go func(a *AtomLocal, n string) {
			var err *Error
			defer func() {
				if r := recover(); r != nil {
					e.Log().Fatal("Element: OnStopping, killing atom recovers from panic. err=(%v)", err.AddPanicStack(e, 3, r))
				}
			}()
			defer exitWG.Done()
			defer func() { <-sem }() // 任务完成，释放信号量的一个槽位。
			err = a.atomos.PushKillMail(e, []ArgsForBaseAtomos{
				&argBaseAtomosWaitKilled{},
				&argBaseAtomosTimeout{timeout: stopTimeout},
			})
			if err != nil {
				e.Log().Error("Element: Kill atom failed. name=(%s),err=(%v)", n, err)
			}
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
			defer func() {
				if r2 := recover(); r2 != nil {
					e.Log().Fatal("Element: Stopping recovers from panic. err=(%v)", err)
				}
			}()
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Element: Stopping recovers from panic.").AddPanicStack(e, 3, r, data)
			} else {
				err = err.AddPanicStack(e, 3, r, data)
			}
			// Hook or Log
			if ar, ok := e.atomos.instance.(AtomosRecover); ok {
				ar.StopRecover(err)
			} else {
				e.Log().Fatal("Element: Stopping recovers from panic. err=(%v)", err)
			}
			// Global hook
			e.cosmosLocal.process.onRecoverHook(e.atomos.id, err)
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
			"Element: OnStopping, saving data error, no auto data persistence. id=(%s)", e.GetIDInfo().Info()).AddStack(e)
		e.Log().Fatal(err.Error())
		goto autoLoad
	}
	elemPersistence = persistence.ElementAutoData()
	if elemPersistence == nil {
		err = NewErrorf(ErrAtomKillElementNotImplementAutoDataPersistence,
			"Element: OnStopping, saving data error, no element auto data persistence. id=(%s)", e.GetIDInfo().Info()).AddStack(e)
		e.Log().Fatal(err.Error())
		return err
	}
	if err = elemPersistence.SetElementData(data); err != nil {
		e.Log().Error("Element: OnStopping, saving data failed, set atom data error. id=(%s),instance=(%+v),err=(%s)",
			e.GetIDInfo().Info(), e.atomos.String(), err.AddStack(e))
		goto autoLoad
	}

autoLoad:

	// Auto Load
	pa, ok := e.elemImpl.Developer.(ElementLoader)
	if !ok || pa == nil {
		return nil
	}
	if err = pa.Unload(); err != nil {
		e.Log().Error("Element: OnStopping, unload failed. id=(%s),instance=(%+v),err=(%s)",
			e.GetIDInfo().Info(), e.atomos.String(), err.AddStack(e))
		return err.AddStack(e)
	}
	return err
}

func (e *ElementLocal) OnIDsReleased() {

}

// 内部实现
// INTERNAL

func (e *ElementLocal) elementAtomSpawn(callerID ID, name string, arg proto.Message, current *ElementImplementation, persistence AutoData, t *IDTrackerInfo, spawnOrGet, fromLocalOrRemote bool, args ...ArgsForBaseAtomos) (*AtomLocal, *IDTracker, *Error) {
	if fromLocalOrRemote && t == nil {
		return nil, nil, NewErrorf(ErrFrameworkInternalError, "Element: Spawn atom failed, id tracker is nil. name=(%s)", name).AddStack(e)
	}

	lock, _ := e.spawnLockMap.GetOrPut(name, &sync.Mutex{})
	defer e.spawnLockMap.Remove(name)

	lock.Lock()
	defer lock.Unlock()

	return e.elementAtomSpawnUnderLocking(callerID, name, arg, current, persistence, t, spawnOrGet, fromLocalOrRemote, args...)
}

func (e *ElementLocal) elementAtomSpawnUnderLocking(callerID ID, name string, arg proto.Message, current *ElementImplementation, persistence AutoData, t *IDTrackerInfo, spawnOrGet, fromLocalOrRemote bool, args ...ArgsForBaseAtomos) (*AtomLocal, *IDTracker, *Error) {
	// Element的容器逻辑。
	// Alloc an atomos and try setting.
	// If not exist, lock and set a new one.
	e.lock.Lock()
	oldAtom, has := e.atoms[name]
	e.lock.Unlock()
	// If exists and running, release new and return error.
	// 不用担心两个Atom同时创建的问题，因为Atom创建的时候就是AtomSpawning了，除非其中一个在极端短的时间内AtomHalt了
	if has {
		return e.elementAtomSpawnMeetsExistAtom(name, oldAtom, arg, current, persistence, t, spawnOrGet, fromLocalOrRemote, args...)
	} else {
		return e.elementAtomSpawnNewAtom(name, nil, oldAtom, arg, current, persistence, t, spawnOrGet, fromLocalOrRemote)
	}
}

func (e *ElementLocal) elementAtomSpawnMeetsExistAtom(name string, oldAtom *AtomLocal, arg proto.Message, current *ElementImplementation, persistence AutoData, t *IDTrackerInfo, spawnOrGet bool, fromLocalOrRemote bool, args ...ArgsForBaseAtomos) (*AtomLocal, *IDTracker, *Error) {
	oldLock := &oldAtom.atomos.mailbox.mutex
	oldLock.Lock()
	switch oldAtom.atomos.state {
	case BaseAtomosSpawning, BaseAtomosWaiting, BaseAtomosBusy:
		// TODO 如果邮箱里面有kill mail，那就说明这个Atom正在被停止，这时候应该当成Stopping来处理。但一定要确保kill mail不会被删除。
		return e.elementAtomSpawnInternalFoundRunning(name, oldLock, oldAtom, arg, current, persistence, t, spawnOrGet, fromLocalOrRemote)
	case BaseAtomosStopping:
		return e.elementAtomSpawnInternalFoundStopping(name, oldLock, oldAtom, arg, current, persistence, t, spawnOrGet, fromLocalOrRemote)
	case BaseAtomosHalt:
		return e.elementAtomSpawnNewAtom(name, oldLock, oldAtom, arg, current, persistence, t, spawnOrGet, fromLocalOrRemote)
	default:
		e.Log().Fatal("Element: Atom in unknown state. name=(%s), state=(%d)", name, oldAtom.atomos.state)
		return e.elementAtomSpawnNewAtom(name, oldLock, oldAtom, arg, current, persistence, t, spawnOrGet, fromLocalOrRemote)
	}
}

func (e *ElementLocal) elementAtomSpawnInternalFoundRunning(name string, oldLock *sync.Mutex, oldAtom *AtomLocal, arg proto.Message, current *ElementImplementation, persistence AutoData, t *IDTrackerInfo, spawnOrGet bool, fromLocalOrRemote bool) (*AtomLocal, *IDTracker, *Error) {
	if oldLock != nil {
		defer oldLock.Unlock()
	}

	if spawnOrGet {
		return oldAtom, nil, NewErrorf(ErrAtomSpawningAnExistedAtom, "Atom: Spawning an existed atom. name=(%s)", name).AddStack(e)
	}

	if fromLocalOrRemote {
		return oldAtom, oldAtom.atomos.it.addIDTracker(t, fromLocalOrRemote), nil
	} else {
		return oldAtom, nil, nil
	}
}

func (e *ElementLocal) elementAtomSpawnInternalFoundStopping(name string, oldLock *sync.Mutex, oldAtom *AtomLocal, arg proto.Message, current *ElementImplementation, persistence AutoData, t *IDTrackerInfo, spawnOrGet bool, fromLocalOrRemote bool, args ...ArgsForBaseAtomos) (toReturn *AtomLocal, idTracker *IDTracker, err *Error) {
	if oldLock != nil {
		oldLock.Unlock()
	}

	select {
	case <-oldAtom.atomos.stoppingChan:
		// 思考：如果spawn不是under locking保护的话，这里就有可能出现竞争条件，导致没得到到stoppingChan的通知就继续往下走了，还是有可能会spawn两次。但如果是under locking保护的话，就不会有这个问题了。
		e.Log().Info("Element: AtomSpawn meets an atom in stopping state, but it stopped successfully. name=(%s)", name)
	case <-time.After(time.Second * 10):
		e.Log().Warn("Element: AtomSpawn meets an atom in stopping state, and it seems that the atom is stuck in stopping. name=(%s)", name)
	}
	return e.elementAtomSpawnNewAtom(name, nil, oldAtom, arg, current, persistence, t, spawnOrGet, fromLocalOrRemote, args...)
}

func (e *ElementLocal) elementAtomSpawnNewAtom(name string, oldLock *sync.Mutex, oldAtom *AtomLocal, arg proto.Message, current *ElementImplementation, persistence AutoData, t *IDTrackerInfo, spawnOrGet bool, fromLocalOrRemote bool, args ...ArgsForBaseAtomos) (*AtomLocal, *IDTracker, *Error) {
	if oldLock != nil {
		oldLock.Unlock()
	}

	atom, err := newAtomLocal(name, e, current, e.atomos.log.level)
	if err != nil {
		return nil, nil, err.AddStack(e)
	}

	e.lock.Lock()
	e.atoms[name] = atom
	if oldAtom != nil {
		//// 因为这里的锁是为了保证旧的Atom的内容不会被修改，但是这里的锁是在旧的Atom已经不再运行的情况下上锁，所以不会有并发修改的问题。
		//// 完成后再对其解锁，以避免有并发Spawn的情况下，后面的Spawn出现死锁。
		//oldLock := &oldAtom.atomos.mailbox.mutex
		// 将旧的Atom的Name元素复制到新的Atom。
		atom.nameElement = oldAtom.nameElement
		// 将旧的Atom的IDTrackerManager复制到新的Atom，但AtomosRelease用新的。
		atom.atomos.it = atom.atomos.it.fromOld(oldAtom.atomos.it)   // TODO: 验证这种情况下，IDTrackerManager下面还有引用，引用Release的情况。
		atom.atomos.asyncCallbackID = oldAtom.atomos.asyncCallbackID // TODO: 不复制asyncCallbackMap以防止回调数据出错，要实现的话方案再议。

		// 将新的Atom内容替换到旧的Atom。
		*oldAtom = *atom
		//// 将旧的Atom解锁。
		//oldLock.Unlock()
		// 把新创建的Atom的指针退换成旧的，这样就可以保证持有旧的Atom的ID能够继续使用。
		atom = oldAtom

	} else {
		atom.nameElement = e.names.PushBack(name)
	}
	e.lock.Unlock()

	// 如果旧的存在且不再运行，则用旧的Atom的结构体，创建一个新的Atom内容。
	// 先将旧的Atom的内容拷贝到新的Atom。

	// Atom的Spawn逻辑。
	if err = atom.atomos.start(func() *Error {
		if err := atom.elementAtomSpawn(current, persistence, arg, args...); err != nil {
			return err.AddStack(nil)
		}
		return nil
	}); err != nil {
		//e.atomos.stop() // TODO: 这里的stop是不是应该放到start里面去？
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

func (e *ElementLocal) cosmosElementSpawn(c *CosmosLocal, runnable *CosmosRunnable, current *ElementImplementation, args ...ArgsForBaseAtomos) (err *Error) {
	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					e.Log().Fatal("Element: Spawn critical problem again. err=(%v)", err)
				}
			}()
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Element: Spawn recovers from panic.").AddPanicStack(e, 3, r)
			} else {
				err = err.AddPanicStack(e, 3, r)
			}
			// Hook or Log
			if ar, ok := e.atomos.instance.(AtomosRecover); ok {
				ar.SpawnRecover(nil, err)
			} else {
				e.Log().Fatal("Element: Spawn critical problem. err=(%v)", err)
			}
			// Global hook
			e.cosmosLocal.process.onRecoverHook(e.atomos.id, err)
		}
	}()

	// Get data and Spawning.
	var data proto.Message
	// 尝试进行自动数据持久化逻辑，如果支持的话，就会被执行。
	// 会从对象中GetAtomData，如果返回错误，证明服务不可用，那将会拒绝Atom的Spawn。
	// 如果GetAtomData拿不出数据，且Spawn没有传入参数，则认为是没有对第一次Spawn的Atom传入参数，属于错误。
	pa, ok := current.Developer.(ElementLoader)
	if ok && pa != nil {
		if err = pa.Load(e, runnable.config.Customize, args...); err != nil {
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
	if err := current.Interface.ElementSpawner(e, e.atomos.instance, data, args...); err != nil {
		return err.AddStack(e)
	}
	return nil
}

func (e *ElementLocal) getAtomFromRemote(name string) (*AtomLocal, *Error) {
	e.lock.RLock()
	atom, hasAtom := e.atoms[name]
	e.lock.RUnlock()
	if hasAtom && atom.atomos.isNotHalt() {
		return atom, nil
	}
	// Auto data persistence.
	persistence, ok := e.elemImpl.Developer.(AutoData)
	if !ok || persistence == nil {
		return nil, nil
	}
	atom, _, err := e.elementAtomSpawn(e, name, nil, e.elemImpl, persistence, nil, false, false)
	if err != nil {
		return nil, NewErrorf(ErrAtomNotExists, "Atom: Atom not exists. name=(%s),err=(%v)", name, err).AddStack(e)
	}
	return atom, nil
}
