package atomos

import (
	"container/list"
	"fmt"
	"regexp"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
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
	// refStates tracks outstanding IDTracker references per atom name. Unlike
	// `atoms`, a refState outlives a halt+respawn cycle so that references
	// acquired against a halted instance can still be Release()d correctly.
	// Guarded by `lock`.
	refStates map[string]*atomRefState
	// Element的List容器的
	names *list.List
	// Lock.
	lock sync.RWMutex

	spawnOperations    map[string]*atomSpawnOperation
	atomSpawnAccepting bool
	atomSpawnPending   int
	atomSpawnDrained   chan struct{}

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
		cosmosLocal:        main,
		atomos:             nil,
		atoms:              nil,
		names:              list.New(),
		lock:               sync.RWMutex{},
		spawnOperations:    map[string]*atomSpawnOperation{},
		atomSpawnAccepting: true,
		elemImpl:           impl,
	}
	var logLevel LogLevel
	if customizeLogLevel, ok := impl.Developer.(ElementLogLevel); ok {
		logLevel = customizeLogLevel.GetElementLogLevel()
	} else {
		logLevel = runnable.config.LogLevel
	}
	// Populate the element's message-schema metadata (Config.Messages) from
	// the registered decoders; idempotent, no-op when already filled.
	fillElementMessages(impl.Interface)
	e.atomos = newBaseAtomos(e, id, logLevel, e, impl.Developer.ElementConstructor(), main.process)

	// 如果实现了ElementCustomizeAtomInitNum接口，那么就使用接口中定义的数量。
	if atomsInitNum, ok := impl.Developer.(ElementAtomInitNum); ok {
		num := atomsInitNum.GetElementAtomsInitNum()
		e.atoms = make(map[string]*AtomLocal, num)
	} else {
		e.atoms = map[string]*AtomLocal{}
	}
	e.refStates = map[string]*atomRefState{}
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
func (e *ElementLocal) SyncMessagingByName(callerID ID, name string, in proto.Message, ext ...ArgsForBaseAtomos) (out proto.Message, err error) {
	return e.atomos.PushSyncMessage(callerID, name, in, ext...)
}

// AsyncMessagingByName
// 异步调用，通过名字调用Element的消息处理函数。
func (e *ElementLocal) AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(proto.Message, error), ext ...ArgsForBaseAtomos) (errBeforeExec error) {
	return e.atomos.PushAsyncMessage(callerID, name, in, callback, ext...)
}

// asyncCallback
// 内部使用，仅供ID实现调用。
// Internal use only, for ID implementation only.
func (e *ElementLocal) asyncCallback(callerID ID, name string, startupID, asyncID uint64, reply proto.Message, err error) {
	e.atomos.PushAsyncMessageCallback(callerID, name, startupID, asyncID, reply, err)
}

func (e *ElementLocal) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := e.elemImpl.Interface.ElementDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

func (e *ElementLocal) Kill(callerID ID, ext ...ArgsForBaseAtomos) error {
	return NewError(ErrCodeFrameworkIncorrectUsage, "Element: Cannot kill an element.").AddStack(e)
}

func (e *ElementLocal) SendWormhole(callerID ID, wormhole BaseAtomosWormhole, ext ...ArgsForBaseAtomos) error {
	return e.atomos.PushWormholeMailAndWaitReply(callerID, wormhole, ext...)
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

func (e *ElementLocal) CosmosMain() CosmosNode {
	return e.cosmosLocal
}

// KillSelf
// Atom kill itself from inner
func (e *ElementLocal) KillSelf() {
	if err := e.atomos.PushKillMail(e); err != nil {
		e.Log().Error("Element: KillSelf failed. err=(%v)", asError(err).AddStack(e))
		return
	}
	e.Log().Info("Element: KillSelf.")
}

func (e *ElementLocal) AsyncCallbackPending() int {
	return e.atomos.AsyncCallbackPending()
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
				err = NewErrorf(ErrCodeFrameworkRecoverFromPanic, "Element: Parallel recovered from panic.").AddPanicStack(e, 3, r)
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

func (e *ElementLocal) asyncSet(callback func(out proto.Message, err error)) (startupID, callbackID uint64) {
	return e.atomos.asyncSet(callback)
}

func (e *ElementLocal) asyncPop(asyncID uint64) func(out proto.Message, err error) {
	return e.atomos.asyncPop(asyncID)
}

// Implementation of ElementSelfID

func (e *ElementLocal) Persistence() AutoData {
	p, ok := e.atomos.instance.(AutoData)
	if !ok || p == nil {
		return nil
	}
	return p
}

func (e *ElementLocal) GetAtoms() []*AtomLocal {
	e.lock.RLock()
	atoms := make([]*AtomLocal, 0, len(e.atoms))
	for _, atomLocal := range e.atoms {
		atoms = append(atoms, atomLocal)
	}
	e.lock.RUnlock()

	active := atoms[:0]
	for _, atomLocal := range atoms {
		if atomLocal.atomos.IsInState(BaseAtomosSpawning, BaseAtomosWaiting, BaseAtomosBusy) {
			active = append(active, atomLocal)
		}
	}
	return active
}

func (e *ElementLocal) GetAtomsInPattern(pattern string) []*AtomLocal {
	type entry struct {
		name string
		atom *AtomLocal
	}
	e.lock.RLock()
	entries := make([]entry, 0, len(e.atoms))
	for name, atomLocal := range e.atoms {
		entries = append(entries, entry{name: name, atom: atomLocal})
	}
	e.lock.RUnlock()

	atoms := make([]*AtomLocal, 0, len(entries))
	for _, ent := range entries {
		matched, err := regexp.MatchString(pattern, ent.name)
		if err != nil || !matched {
			continue
		}
		if ent.atom.atomos.IsInState(BaseAtomosSpawning, BaseAtomosWaiting, BaseAtomosBusy) {
			atoms = append(atoms, ent.atom)
		}
	}
	return atoms
}

// Implementation of Element

func (e *ElementLocal) GetAtomID(name string, tracker *IDTrackerInfo, args ...ArgsForBaseAtomos) (ID, *IDTracker, error) {
	fromLocalOrRemote := argsFromLocalOrRemote(args)
	if fromLocalOrRemote && tracker == nil {
		return nil, nil, NewErrorf(ErrCodeFrameworkInternalError, "IDTrackerInfo: IDTrackerInfo is nil.").AddStack(e)
	}
	e.lock.RLock()
	atom, hasAtom := e.atoms[name]
	spawnOp := e.spawnOperations[name]
	e.lock.RUnlock()
	if hasAtom && atom.atomos.isNotHalt() {
		if fromLocalOrRemote {
			return atom, e.addRefAtom(atom, tracker), nil
		} else {
			return atom, nil, nil
		}
	}
	if spawnOp != nil {
		if spawnOp.owner.Load() == getGoID() {
			return nil, nil, NewErrorf(ErrCodeAtomosIsNotRunning, "Atom: Get cannot wait for its own spawn. name=(%s)", name).AddStack(e)
		}
		atom, err := e.waitAtomSpawnReady(name, spawnOp, atomSpawnReadyTimeout)
		result, t, err := e.atomSpawnResult(atom, tracker, fromLocalOrRemote, err)
		if err != nil && result == nil {
			return nil, nil, err
		}
		return result, t, errOrNil(err)
	}
	// Auto data persistence.
	persistence, ok := e.elemImpl.Developer.(AutoData)
	if !ok || persistence == nil {
		return nil, nil, NewErrorf(ErrCodeAtomNotExists, "Atom: Atom not exists. name=(%s)", name).AddStack(e)
	}
	atom, t, serr := e.elementAtomSpawn(e, name, nil, e.elemImpl, persistence, tracker, false, fromLocalOrRemote)
	if serr != nil && atom == nil {
		return nil, nil, serr
	}
	return atom, t, errOrNil(serr)
}

func (e *ElementLocal) GetAtomsNum() int {
	e.lock.RLock()
	num := len(e.atoms)
	e.lock.RUnlock()
	return num
}

func (e *ElementLocal) GetActiveAtomsNum() int {
	e.lock.RLock()
	atoms := make([]*AtomLocal, 0, len(e.atoms))
	for _, atomLocal := range e.atoms {
		atoms = append(atoms, atomLocal)
	}
	e.lock.RUnlock()

	num := 0
	for _, atomLocal := range atoms {
		if atomLocal.atomos.IsInState(BaseAtomosSpawning, BaseAtomosWaiting, BaseAtomosBusy) {
			num += 1
		}
	}
	return num
}

func (e *ElementLocal) GetAllInactiveAtomsIDTrackerInfo() map[string]string {
	type entry struct {
		atom *AtomLocal
		st   *atomRefState
	}
	e.lock.RLock()
	entries := make([]entry, 0, len(e.atoms))
	for _, atomLocal := range e.atoms {
		entries = append(entries, entry{atomLocal, e.refStates[atomLocal.GetIDInfo().Atom]})
	}
	e.lock.RUnlock()

	info := make(map[string]string, len(entries))
	for _, ent := range entries {
		if !ent.atom.atomos.IsInState(BaseAtomosHalt) {
			continue
		}
		// A halted atom with a live refState means it still has outstanding
		// IDTracker references; dump them. If no refState exists, there are
		// no outstanding references.
		dump := "No IDTracker remain"
		if ent.st != nil {
			dump = ent.st.String()
		}
		info[ent.atom.String()] = fmt.Sprintf(" -> %s\n", dump)
	}
	return info
}

func (e *ElementLocal) SpawnAtom(callerID ID, name string, arg proto.Message, tracker *IDTrackerInfo, args ...ArgsForBaseAtomos) (ID, *IDTracker, error) {
	fromLocalOrRemote := argsFromLocalOrRemote(args)
	// Auto data persistence.
	persistence, _ := e.elemImpl.Developer.(AutoData)
	id, t, err := e.elementAtomSpawn(callerID, name, arg, e.elemImpl, persistence, tracker, true, fromLocalOrRemote, args...)
	if err != nil {
		if id == nil {
			return nil, nil, err.AddStack(e)
		}
		return id, t, err.AddStack(e)
	}
	return id, t, nil
}

// 邮箱控制器相关
// Mailbox Handler

func (e *ElementLocal) onSyncMessaging(fromID ID, name string, in proto.Message) (out proto.Message, err *Error) {
	handler := e.elemImpl.ElementHandlers[name]
	if handler == nil {
		return nil, NewErrorf(ErrCodeElementMessageHandlerNotExists,
			"Element: Message handler not found. from=(%s),name=(%s),in=(%v)", fromID, name, in).AddStack(e)
	}
	return e.atomos.onSyncMessaging(fromID, name, handler, in)
}

func (e *ElementLocal) onAsyncMessaging(fromID ID, name string, startupID, asyncID uint64, in proto.Message) {
	handler := e.elemImpl.ElementHandlers[name]
	if handler == nil {
		if fromInternal, intErr := asIDInternal(fromID); intErr != nil {
			e.atomos.log.coreError("Element: onAsyncMessaging caller ID invalid, error reply dropped. err=(%v)", intErr)
		} else {
			fromInternal.asyncCallback(e, name, startupID, asyncID, nil, NewErrorf(ErrCodeAtomMessageHandlerNotExists, "Element: onAsyncMessaging handler not found. from=(%s),name=(%s),in=(%v)", fromID, name, in).AddStack(e))
		}
		return
	}
	e.atomos.onAsyncMessaging(fromID, e, name, handler, startupID, asyncID, in)
}

func (e *ElementLocal) onAsyncMessagingCallback(asyncID uint64, in proto.Message, err error) {
	e.atomos.onAsyncMessagingCallback(asyncID, in, err)
}

func (e *ElementLocal) onFnCallback(callback *atomosCallback) {
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

func (e *ElementLocal) onWormhole(from ID, wormhole BaseAtomosWormhole) *Error {
	holder, ok := e.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		return NewErrorf(ErrCodeAtomosNotSupportWormhole, "Element: Not supports wormhole. type=(%T)", e.atomos.instance).AddStack(e)
	}
	if err := holder.AcceptWormhole(from, wormhole); err != nil {
		return asError(err).AddStack(e)
	}
	return nil
}

func (e *ElementLocal) onStopping(from ID, cancelled []uint64) (err *Error) {
	e.closeAtomSpawnAdmission()
	e.waitAtomSpawnDrained()

	// Pending is zero and admission is closed, so this is the final snapshot.
	// Send Kill to all atoms.
	var stopTimeout, stopGap time.Duration
	elemExit, ok := e.elemImpl.Developer.(ElementAtomExit)
	if ok && elemExit != nil {
		stopTimeout = elemExit.StopTimeout()
		stopGap = elemExit.StopGap()
	}

	sem := make(chan struct{}, runtime.NumCPU()) // 信号量，用于控制并发goroutine的数量。
	exitWG := sync.WaitGroup{}

	// Snapshot the atom names under the read lock before iterating. container/list
	// is not safe for concurrent traversal while elementAtomStopping (running on a
	// kill goroutine) removes nodes from e.names. Iterating a static slice avoids
	// that data race.
	e.lock.RLock()
	names := make([]string, 0, len(e.atoms))
	for nameElem := e.names.Back(); nameElem != nil; nameElem = nameElem.Prev() {
		names = append(names, nameElem.Value.(string))
	}
	e.lock.RUnlock()

	for _, name := range names {
		e.lock.RLock()
		atom, has := e.atoms[name]
		e.lock.RUnlock()
		if !has || atom == nil {
			continue
		}
		e.Log().Info("Element: onStopping, killing atom. name=(%s)", name)

		// 获取信号量的一个槽位。如果信号量已满，则这里将阻塞，直到信号量中有可用的槽位。
		sem <- struct{}{}
		exitWG.Add(1)

		// 启动一个goroutine来处理任务
		go func(a *AtomLocal, n string) {
			var err error
			defer func() {
				if r := recover(); r != nil {
					e.Log().Fatal("Element: onStopping, killing atom recovers from panic. err=(%v)", asError(err).AddPanicStack(e, 3, r))
				}
			}()
			defer exitWG.Done()
			defer func() { <-sem }() // 任务完成，释放信号量的一个槽位。
			err = a.atomos.PushKillMail(e,
				&argBaseAtomosWaitKilled{},
				&argBaseAtomosTimeout{timeout: stopTimeout},
			)
			if err != nil {
				e.Log().Error("Element: Kill atom failed. name=(%s),err=(%v)", n, err)
			}
		}(atom, name)
		if stopGap > 0 {
			<-time.After(stopGap)
		}
	}
	exitWG.Wait()
	e.Log().Info("Element: onStopping, all atoms killed. element=(%s)", e.atomos.id.Element)

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
				err = NewErrorf(ErrCodeFrameworkRecoverFromPanic, "Element: Stopping recovers from panic.").AddPanicStack(e, 3, r, data)
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
		err = NewErrorf(ErrCodeAtomKillElementNotImplementAutoDataPersistence,
			"Element: onStopping, saving data error, no auto data persistence. id=(%s)", e.GetIDInfo().Info()).AddStack(e)
		e.Log().Fatal(err.Error())
		goto autoLoad
	}
	elemPersistence = persistence.ElementAutoData()
	if elemPersistence == nil {
		err = NewErrorf(ErrCodeAtomKillElementNotImplementAutoDataPersistence,
			"Element: onStopping, saving data error, no element auto data persistence. id=(%s)", e.GetIDInfo().Info()).AddStack(e)
		e.Log().Fatal(err.Error())
		return err
	}
	if err = asError(elemPersistence.SetElementData(data)); err != nil {
		e.Log().Error("Element: onStopping, saving data failed, set atom data error. id=(%s),instance=(%+v),err=(%s)",
			e.GetIDInfo().Info(), e.atomos.String(), err.AddStack(e))
		goto autoLoad
	}

autoLoad:

	// Auto Load
	pa, ok := e.elemImpl.Developer.(ElementLoader)
	if !ok || pa == nil {
		return nil
	}
	if err = asError(pa.Unload()); err != nil {
		e.Log().Error("Element: onStopping, unload failed. id=(%s),instance=(%+v),err=(%s)",
			e.GetIDInfo().Info(), e.atomos.String(), err.AddStack(e))
		return err.AddStack(e)
	}
	return err
}

// 内部实现
// INTERNAL

type atomSpawnOperation struct {
	done  chan struct{}
	owner atomic.Uint64
	atom  *AtomLocal
	err   *Error
}

const (
	atomSpawnStoppingTimeout = 10 * time.Second
	atomSpawnReadyTimeout    = 10 * time.Second
)

func (e *ElementLocal) beginAtomSpawn(name string) *Error {
	p := e.cosmosLocal.process
	if p != nil {
		if err := p.beginAtomSpawn(); err != nil {
			return err.AddStack(e)
		}
	}

	e.lock.Lock()
	if !e.atomSpawnAccepting {
		e.lock.Unlock()
		if p != nil {
			p.endAtomSpawn()
		}
		return NewErrorf(ErrCodeAtomosIsStopping, "Element: Atom spawn rejected, element is stopping. element=(%s),atom=(%s)", e.atomos.id.Element, name).AddStack(e)
	}
	if e.atomSpawnPending == 0 {
		e.atomSpawnDrained = make(chan struct{})
	}
	e.atomSpawnPending++
	e.lock.Unlock()
	return nil
}

func (e *ElementLocal) endAtomSpawn() {
	e.lock.Lock()
	if e.atomSpawnPending <= 0 {
		e.lock.Unlock()
		return
	}
	e.atomSpawnPending--
	if e.atomSpawnPending == 0 && e.atomSpawnDrained != nil {
		close(e.atomSpawnDrained)
		e.atomSpawnDrained = nil
	}
	e.lock.Unlock()

	if p := e.cosmosLocal.process; p != nil {
		p.endAtomSpawn()
	}
}

func (e *ElementLocal) closeAtomSpawnAdmission() {
	e.lock.Lock()
	e.atomSpawnAccepting = false
	e.lock.Unlock()
}

func (e *ElementLocal) waitAtomSpawnDrained() {
	for {
		e.lock.RLock()
		if e.atomSpawnPending == 0 {
			e.lock.RUnlock()
			return
		}
		drained := e.atomSpawnDrained
		e.lock.RUnlock()
		<-drained
	}
}

func (e *ElementLocal) elementAtomSpawnInternalFoundStopping(name string, oldLock *sync.Mutex, oldAtom *AtomLocal, arg proto.Message, current *ElementImplementation, persistence AutoData, t *IDTrackerInfo, spawnOrGet, fromLocalOrRemote bool, timeout time.Duration, args ...ArgsForBaseAtomos) (*AtomLocal, *IDTracker, *Error) {
	if oldLock != nil {
		oldLock.Unlock()
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-oldAtom.atomos.stoppingChan:
	case <-timer.C:
		return nil, nil, NewErrorf(ErrCodeAtomosIsStopping, "Element: AtomSpawn timed out waiting for the previous instance to halt. name=(%s),timeout=(%v)", name, timeout).AddStack(e)
	}
	return e.elementAtomSpawn(nil, name, arg, current, persistence, t, spawnOrGet, fromLocalOrRemote, args...)
}

func (e *ElementLocal) elementAtomSpawn(callerID ID, name string, arg proto.Message, current *ElementImplementation, persistence AutoData, t *IDTrackerInfo, spawnOrGet, fromLocalOrRemote bool, args ...ArgsForBaseAtomos) (*AtomLocal, *IDTracker, *Error) {
	if err := e.beginAtomSpawn(name); err != nil {
		return nil, nil, err
	}
	defer e.endAtomSpawn()
	if fromLocalOrRemote && t == nil {
		return nil, nil, NewErrorf(ErrCodeFrameworkInternalError, "Element: Spawn atom failed, id tracker is nil. name=(%s)", name).AddStack(e)
	}
	return e.elementAtomSpawnAdmitted(callerID, name, arg, current, persistence, t, spawnOrGet, fromLocalOrRemote, args...)
}

func (e *ElementLocal) elementAtomSpawnAdmitted(callerID ID, name string, arg proto.Message, current *ElementImplementation, persistence AutoData, t *IDTrackerInfo, spawnOrGet, fromLocalOrRemote bool, args ...ArgsForBaseAtomos) (*AtomLocal, *IDTracker, *Error) {
	e.lock.Lock()
	if op := e.spawnOperations[name]; op != nil {
		e.lock.Unlock()
		if spawnOrGet {
			return nil, nil, NewErrorf(ErrCodeAtomSpawningAnExistedAtom, "Atom: Spawning an existed atom. name=(%s)", name).AddStack(e)
		}
		if op.owner.Load() == getGoID() {
			return nil, nil, NewErrorf(ErrCodeAtomosIsNotRunning, "Atom: Get cannot wait for its own spawn. name=(%s)", name).AddStack(e)
		}
		atom, err := e.waitAtomSpawnReady(name, op, atomSpawnReadyTimeout)
		return e.atomSpawnResult(atom, t, fromLocalOrRemote, err)
	}
	op := &atomSpawnOperation{done: make(chan struct{})}
	op.owner.Store(getGoID())
	if e.spawnOperations == nil {
		e.spawnOperations = map[string]*atomSpawnOperation{}
	}
	e.spawnOperations[name] = op
	oldAtom, has := e.atoms[name]
	e.lock.Unlock()

	if has {
		oldLock := &oldAtom.atomos.mailbox.mutex
		oldLock.Lock()
		state := oldAtom.atomos.state
		oldLock.Unlock()
		switch state {
		case BaseAtomosSpawning, BaseAtomosWaiting, BaseAtomosBusy:
			if spawnOrGet {
				err := NewErrorf(ErrCodeAtomSpawningAnExistedAtom, "Atom: Spawning an existed atom. name=(%s)", name).AddStack(e)
				e.finishAtomSpawn(name, op, oldAtom, nil)
				return oldAtom, nil, err
			}
			e.finishAtomSpawn(name, op, oldAtom, nil)
			return e.atomSpawnResult(oldAtom, t, fromLocalOrRemote, nil)
		case BaseAtomosStopping:
			timer := time.NewTimer(atomSpawnStoppingTimeout)
			select {
			case <-oldAtom.atomos.stoppingChan:
				timer.Stop()
			case <-timer.C:
				err := NewErrorf(ErrCodeAtomosIsStopping, "Element: AtomSpawn timed out waiting for the previous instance to halt. name=(%s),timeout=(%v)", name, atomSpawnStoppingTimeout).AddStack(e)
				e.finishAtomSpawn(name, op, nil, err)
				return nil, nil, err
			}
		case BaseAtomosHalt:
		default:
			e.Log().Fatal("Element: Atom in unknown state. name=(%s), state=(%d)", name, state)
		}
	}
	return e.elementAtomSpawnNewAtom(name, op, oldAtom, arg, current, persistence, t, fromLocalOrRemote, args...)
}

func (e *ElementLocal) waitAtomSpawnReady(name string, op *atomSpawnOperation, timeout time.Duration) (*AtomLocal, *Error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-op.done:
		return op.atom, op.err
	case <-timer.C:
		return nil, NewErrorf(ErrCodeAtomosPushTimeoutReject, "Element: Get timed out waiting for atom to become ready. name=(%s),timeout=(%v)", name, timeout).AddStack(e)
	}
}

func (e *ElementLocal) atomSpawnResult(atom *AtomLocal, t *IDTrackerInfo, fromLocalOrRemote bool, err *Error) (*AtomLocal, *IDTracker, *Error) {
	if err != nil {
		return nil, nil, err
	}
	if fromLocalOrRemote {
		return atom, e.addRefAtom(atom, t), nil
	}
	return atom, nil, nil
}

func (e *ElementLocal) finishAtomSpawn(name string, op *atomSpawnOperation, atom *AtomLocal, err *Error) {
	e.lock.Lock()
	if e.spawnOperations[name] != op {
		e.lock.Unlock()
		return
	}
	delete(e.spawnOperations, name)
	op.atom = atom
	op.err = err
	e.lock.Unlock()
	close(op.done)
}

func (e *ElementLocal) publishAtomSpawn(name string, op *atomSpawnOperation, oldAtom, atom *AtomLocal) {
	e.lock.Lock()
	if e.spawnOperations[name] != op {
		e.lock.Unlock()
		return
	}
	e.atoms[name] = atom
	if oldAtom != nil {
		atom.nameElement = oldAtom.nameElement
		atom.atomos.asyncCallbackID = oldAtom.atomos.asyncCallbackID
	} else {
		atom.nameElement = e.names.PushBack(name)
	}
	delete(e.spawnOperations, name)
	op.atom = atom
	e.lock.Unlock()
	close(op.done)
}

func (e *ElementLocal) elementAtomSpawnNewAtom(name string, op *atomSpawnOperation, oldAtom *AtomLocal, arg proto.Message, current *ElementImplementation, persistence AutoData, t *IDTrackerInfo, fromLocalOrRemote bool, args ...ArgsForBaseAtomos) (*AtomLocal, *IDTracker, *Error) {
	atom, err := newAtomLocal(name, e, current, e.atomos.log.level)
	if err != nil {
		e.finishAtomSpawn(name, op, nil, err)
		return nil, nil, err.AddStack(e)
	}

	if err = atom.atomos.start(func() *Error {
		op.owner.Store(getGoID())
		if err := atom.elementAtomSpawn(current, persistence, arg, args...); err != nil {
			return err.AddStack(nil)
		}
		return nil
	}, func() {
		e.publishAtomSpawn(name, op, oldAtom, atom)
	}); err != nil {
		e.finishAtomSpawn(name, op, nil, err)
		e.elementAtomRelease(atom)
		return nil, nil, err.AddStack(nil)
	}
	return e.atomSpawnResult(atom, t, fromLocalOrRemote, nil)
}

// addRefAtom records a new IDTracker reference against atom's instanceID and
// returns a tracker whose Release will decrement that instance's cell. The
// refState is created lazily under e.lock.
func (e *ElementLocal) addRefAtom(atom *AtomLocal, info *IDTrackerInfo) *IDTracker {
	name := atom.GetIDInfo().Atom
	e.lock.Lock()
	st, ok := e.refStates[name]
	if !ok {
		st = newAtomRefState(e, name)
		e.refStates[name] = st
	}
	e.lock.Unlock()
	return st.addRef(atom.atomos.instanceID, info)
}

// refStateFor returns the refState for a name (nil if absent). Caller must
// hold e.lock (RLock is sufficient).
func (e *ElementLocal) refStateFor(name string) *atomRefState {
	return e.refStates[name]
}

// onInstanceRefsDrained is invoked by IDTracker.Release when an instance's
// reference count reaches zero. If that instance has halted, it becomes
// eligible for collection; otherwise this is a no-op (a live instance simply
// has no outstanding references right now).
func (e *ElementLocal) onInstanceRefsDrained(st *atomRefState, instID uint64) {
	e.lock.RLock()
	atom, has := e.atoms[st.name]
	e.lock.RUnlock()
	if has {
		if atom.atomos.instanceID != instID || atom.atomos.isNotHalt() {
			return
		}
	}

	if e.removeAtomIfCurrent(st.name, instID) && atom.atomos.mailbox.isRunning() {
		e.cosmosLocal.process.logging.pushFrameworkErrorLog("Atom: Try releasing a mailbox which is still running. name=(%s)", st.name)
	}
}

// removeAtomIfCurrent removes a container entry only if it still belongs to
// instID. The name list node is shared across respawns, so stale cleanup must
// leave both that node and any refState used by the replacement untouched.
func (e *ElementLocal) removeAtomIfCurrent(name string, instID uint64) bool {
	e.lock.Lock()
	defer e.lock.Unlock()

	atom, has := e.atoms[name]
	if has {
		if atom == nil || atom.atomos.instanceID != instID {
			return false
		}
		delete(e.atoms, name)
		if atom.nameElement != nil {
			e.names.Remove(atom.nameElement)
			atom.nameElement = nil
		}
	}
	if st := e.refStates[name]; st != nil && st.isEmpty() {
		delete(e.refStates, name)
	}
	return has
}

func (e *ElementLocal) elementAtomRelease(atom *AtomLocal) {
	// Now driven by per-instance refState. A halted atom with no outstanding
	// references is collected here on the explicit-release path (e.g. spawn
	// failure cleanup) as well as via onInstanceRefsDrained.
	if atom.atomos.isNotHalt() {
		return
	}
	name := atom.GetIDInfo().Atom
	instID := atom.atomos.instanceID

	// If this instance still has outstanding references, keep it.
	e.lock.RLock()
	st := e.refStates[name]
	e.lock.RUnlock()
	if st != nil && st.refCount(instID) > 0 {
		return
	}

	if e.removeAtomIfCurrent(name, instID) && atom.atomos.mailbox.isRunning() {
		e.cosmosLocal.process.logging.pushFrameworkErrorLog("Atom: Try releasing a mailbox which is still running. name=(%s)", name)
	}
}

func (e *ElementLocal) elementAtomStopping(atom *AtomLocal) {
	name := atom.GetIDInfo().Atom
	instID := atom.atomos.instanceID

	// If this instance still has outstanding references, keep it in the map
	// until they drain (onInstanceRefsDrained will then collect it).
	e.lock.RLock()
	st := e.refStates[name]
	e.lock.RUnlock()
	if st != nil && st.refCount(instID) > 0 {
		return
	}

	if e.removeAtomIfCurrent(name, instID) && atom.atomos.mailbox.isRunning() {
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
				err = NewErrorf(ErrCodeFrameworkRecoverFromPanic, "Element: Spawn recovers from panic.").AddPanicStack(e, 3, r)
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
		if err = asError(pa.Load(e, runnable.config.Customize, args...)); err != nil {
			return err.AddStack(e)
		}
	}
	persistence, ok := current.Developer.(AutoData)
	if ok && persistence != nil {
		elemPersistence := persistence.ElementAutoData()
		if elemPersistence != nil {
			var gerr error
			data, gerr = elemPersistence.GetElementData()
			if err = asError(gerr); err != nil {
				return err.AddStack(e)
			}
		}
	}
	if serr := current.Interface.ElementSpawner(e, e.atomos.instance, data, args...); serr != nil {
		return asError(serr).AddStack(e)
	}
	return nil
}

// getAtomFromRemote resolves the live Atom instance for a remote caller.
//
// callerInstanceID is the per-instance epoch the caller holds (from the IDInfo
// it received when resolving the ID). The strict policy (M5-0):
//   - callerInstanceID == 0 → reject (ErrAtomInstanceMismatch): a remote call
//     must carry a real epoch. Legacy clients without instance_id must rebind.
//   - callerInstanceID != live instance's instanceID → reject: the caller's
//     handle points at a previous (now-replaced) instance; rebind required.
//
// A cache miss no longer triggers an implicit respawn for remote callers: a
// respawn would produce a NEW instanceID that the caller's stale epoch cannot
// match, so implicit respawn would only hand back a different instance than the
// caller expects (a dirty-data source). Remote callers must rebind explicitly
// (GetAtomID/SpawnAtom) to obtain the current instance.
func (e *ElementLocal) getAtomFromRemote(name string, callerInstanceID uint64) (*AtomLocal, *Error) {
	// Strict policy: a remote call must carry a real instance_id.
	if callerInstanceID == 0 {
		return nil, NewErrorf(ErrCodeAtomInstanceMismatch,
			"Element: Remote call missing instance_id (legacy client?). name=(%s)", name).AddStack(e)
	}
	e.lock.RLock()
	atom, hasAtom := e.atoms[name]
	e.lock.RUnlock()
	if hasAtom && atom.atomos.isNotHalt() {
		if atom.atomos.instanceID != callerInstanceID {
			return nil, NewErrorf(ErrCodeAtomInstanceMismatch,
				"Element: Atom instance mismatch. name=(%s),caller_inst=(%d),live_inst=(%d)",
				name, callerInstanceID, atom.atomos.instanceID).AddStack(e)
		}
		return atom, nil
	}
	// No live instance under this name. Do NOT implicitly respawn for a remote
	// caller: the caller's epoch is necessarily stale (any respawn would yield a
	// new instanceID), so the caller must rebind.
	return nil, NewErrorf(ErrCodeAtomInstanceMismatch,
		"Element: Atom not live and caller epoch is stale. name=(%s),caller_inst=(%d)",
		name, callerInstanceID).AddStack(e)
}
