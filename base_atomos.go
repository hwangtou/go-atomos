package atomos

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// baseAtomosHolder Atomos持有者
// Base Atomos Holder
type baseAtomosHolder interface {
	// onSyncMessaging
	// 收到消息
	onSyncMessaging(fromID ID, name string, in proto.Message) (out proto.Message, err *Error)

	// onAsyncMessaging
	// 收到异步消息
	onAsyncMessaging(fromID ID, name string, startupID, asyncID uint64, in proto.Message)

	// onAsyncMessagingCallback
	// 收到异步消息回调
	onAsyncMessagingCallback(asyncID uint64, in proto.Message, err error)

	onFnCallback(*atomosCallback)

	// onWormhole
	// 收到Wormhole
	onWormhole(from ID, wormhole BaseAtomosWormhole) *Error

	// onStopping
	// 停止中
	onStopping(from ID, cancelled []uint64) *Error
}

// BaseAtom状态
// BaseAtomosState

type BaseAtomosState int

const (
	BaseAtomosInvalidState BaseAtomosState = 0

	// BaseAtomosSpawning
	// 启动中
	// Atom is starting up.
	BaseAtomosSpawning BaseAtomosState = 1

	// BaseAtomosWaiting
	// 启动成功，等待消息
	// Atom is started and waiting for message.
	BaseAtomosWaiting BaseAtomosState = 2

	// BaseAtomosBusy
	// 启动成功，正在处理消息
	// Atom is started and busy processing message.
	BaseAtomosBusy BaseAtomosState = 3

	// BaseAtomosStopping
	// 停止中
	// Atom is stopping.
	BaseAtomosStopping BaseAtomosState = 4

	// BaseAtomosHalt
	// 停止
	// Atom is stopped.
	BaseAtomosHalt BaseAtomosState = 5
)

func (as BaseAtomosState) String() string {
	switch as {
	case BaseAtomosHalt:
		return "Halted"
	case BaseAtomosSpawning:
		return "Spawning"
	case BaseAtomosWaiting:
		return "Waiting"
	case BaseAtomosBusy:
		return "Busy"
	case BaseAtomosStopping:
		return "Stopping"
	}
	return "Unknown"
}

// BaseAtomosWormhole is the interface that values passed through a wormhole must implement.
// Wormholes carry arbitrary data between actors; implementing this interface explicitly
// marks a type as wormhole-safe and prevents accidentally passing untyped values.
type BaseAtomosWormhole interface {
	IsWormhole()
}

// BaseAtomos 基础Atomos
// Base Atomos
type BaseAtomos struct {
	// 进程
	process *CosmosProcess
	// 实现和句柄信息
	impl ID
	id   *IDInfo
	// instanceID is a process-unique, monotonically increasing id assigned at
	// construction. It distinguishes a respawned instance from its predecessor,
	// enabling per-instance reference counting (atomRefState) to keep a stale
	// (pre-respawn) tracker's Release separate from the new instance's count.
	instanceID uint64

	// 状态
	// State
	state BaseAtomosState

	// Atomos邮箱，也是实现Atom无锁队列的关键。
	// Mailbox, the key of lockless queue of Atom.
	mailbox *mailBox

	// 持有者
	holder baseAtomosHolder
	// 实际上运行的对象
	instance Atomos

	// 日志管理器，用于处理来自Atom内部的日志。
	// Logs Manager, uses to handle Log from inner Atom.
	log atomosLogging

	// 任务管理器，用于处理来自Atom内部的任务调派。
	// Task Manager, uses to handle Task from inner Atom.
	task atomosTaskManager

	// 消息追踪，用于处理来自Atom内部的消息追踪。
	// Message Tracker, uses to handle Message Tracker from inner Atom.
	mt atomosMessageTracker

	asyncCallMutex   sync.Mutex
	asyncCallbackID  uint64
	asyncCallbackMap map[uint64]asyncCallbackWrap
	// asyncCleared 标记停机清账已完成:此后到达的 asyncSet 不再入表(避免清账后插入成为
	// 永不终结的泄漏条目),改为同步以 ErrAsyncCallbackCancelledByHalt 终结回调。
	// 由 asyncCallMutex 保护。
	asyncCleared bool

	stoppingChan chan bool
}

type asyncCallbackWrap struct {
	fn  func(out proto.Message, err error)
	now time.Time
}

func newBaseAtomos(impl ID, id *IDInfo, lv LogLevel, holder baseAtomosHolder, inst Atomos, process *CosmosProcess) *BaseAtomos {
	a := &BaseAtomos{
		process:          process,
		impl:             impl,
		id:               id,
		instanceID:       process.allocInstanceID(),
		state:            BaseAtomosHalt,
		mailbox:          nil,
		holder:           holder,
		instance:         inst,
		log:              atomosLogging{},
		task:             atomosTaskManager{},
		mt:               atomosMessageTracker{},
		asyncCallMutex:   sync.Mutex{},
		asyncCallbackID:  0,
		asyncCallbackMap: map[uint64]asyncCallbackWrap{},
		stoppingChan:     make(chan bool, 1),
	}
	a.mailbox = newMailBox(id.Info(), a, process.logging)
	initAtomosLog(&a.log, a.id, lv, process.logging)
	initAtomosTasksManager(a.log.logging, &a.task, a)
	initAtomosMessageTracker(&a.mt)
	return a
}

func (a *BaseAtomos) GetIDInfo() *IDInfo {
	return a.id
}

func (a *BaseAtomos) String() string {
	return a.id.Info()
}

func (a *BaseAtomos) GetInstance() Atomos {
	return a.instance
}

func (a *BaseAtomos) Log() Logging {
	return &a.log
}

// coreLog returns the internal logging extension of this atomos's logger.
// Framework loggers are always *atomosLogging, so no assertion is needed.
func (a *BaseAtomos) coreLog() loggingInternal {
	return &a.log
}

func (a *BaseAtomos) Task() Task {
	return &a.task
}

// go:noinline
func (a *BaseAtomos) GetGoID() uint64 {
	return a.mailbox.goID
}

func (a *BaseAtomos) PushSyncMessage(from ID, name string, in proto.Message, ext ...ArgsForBaseAtomos) (reply proto.Message, err error) {
	if from == nil {
		return nil, NewErrorf(ErrCodeFrameworkIncorrectUsage, "Atomos: from ID is nil.").AddStack(nil)
	}

	helper := createBaseAtomosHelper(BaseAtomosMailSync, ext)
	if helper.hasErrors() {
		return nil, helper.getError()
	}

	// Deadlock detection: check whether this sync call would close a cycle in
	// the local wait-graph. The executing goroutine (getGoID) is about to block
	// in waitReply waiting for the target Atom's mailbox goroutine (a.GetGoID).
	// If the target can already reach the caller via existing wait-edges, this
	// push would deadlock — return immediately instead of waiting 10s.
	// Remote targets (goID==0) cannot form local cycles and are skipped.
	callerGoID := getGoID()
	targetGoID := a.GetGoID()
	waitKey := a.process.beginSyncWait(from.GetIDInfo())
	defer a.process.endSyncWait(waitKey)
	hasWaitEdge := false
	if callerGoID != 0 && targetGoID != 0 {
		if callerGoID == targetGoID {
			err := NewErrorf(ErrCodeIDFirstSyncCallDeadlock,
				"Atomos: Sync call deadlock (self). caller=(%s) target=(%s) name=(%s)",
				from.GetIDInfo().Info(), a.id.Info(), name).AddStack(nil)
			a.process.onDeadlockEvent(&DeadlockEvent{
				Kind: DeadlockEventCycle, Caller: from.GetIDInfo().Info(), Target: a.id.Info(),
				Name: name, Err: errOrNil(err),
			})
			return nil, err
		}
		if a.process.detectDeadlockAndWait(callerGoID, targetGoID, a.id) {
			err := NewErrorf(ErrCodeIDFirstSyncCallDeadlock,
				"Atomos: Sync call deadlock (cycle). caller=(%s) target=(%s) name=(%s) chain=%s%s",
				from.GetIDInfo().Info(), a.id.Info(), name,
				a.process.waitChain(targetGoID, callerGoID),
				a.process.waitChainStacks(targetGoID, callerGoID)).AddStack(nil)
			a.process.onDeadlockEvent(&DeadlockEvent{
				Kind: DeadlockEventCycle, Caller: from.GetIDInfo().Info(), Target: a.id.Info(),
				Name: name, Chain: a.process.waitChain(targetGoID, callerGoID), Err: errOrNil(err),
			})
			return nil, err
		}
		hasWaitEdge = true
	}
	// Ensure the wait edge is removed on every exit path after this point.
	removeEdge := func() {
		if hasWaitEdge {
			a.process.removeWaitEdge(callerGoID)
			hasWaitEdge = false
		}
	}

	am := allocBaseAtomosMail()
	initBaseAtomosMailSync(am, from, name, in)
	// A remote-originated call already includes its caller. A nested local hop
	// inherits the ambient chain and adds the local caller before the next hop.
	if helper.waitChain != nil {
		am.waitChain = helper.waitChain
	} else if chain := a.process.currentSyncChain(callerGoID); len(chain) > 0 {
		am.waitChain = make([]string, 0, len(chain)+1)
		am.waitChain = append(am.waitChain, chain...)
		am.waitChain = append(am.waitChain, syncChainLink(from.GetIDInfo()))
	}

	var ok bool
	if helper.appendToHead {
		ok = a.mailbox.pushHead(am.mail)
	} else {
		ok = a.mailbox.pushTail(am.mail)
	}
	if !ok {
		removeEdge()
		return reply, NewErrorf(ErrCodeAtomosIsNotRunning,
			"Atomos is not running. from=(%s),name=(%s),in=(%v)", from, name, in).AddStack(nil)
	}

	replyInterface, ierr := am.waitReply(a, helper)
	removeEdge()
	if ierr != nil && ierr.Code == ErrCodeAtomosIsNotRunning {
		return nil, ierr.AddStack(nil)
	}
	reply, ok = replyInterface.(proto.Message)
	if !ok {
		return nil, errOrNil(ierr.AddStack(nil))
	}
	return reply, errOrNil(ierr.AddStack(nil))
}

func (a *BaseAtomos) PushAsyncMessage(from ID, name string, in proto.Message, callback func(reply proto.Message, err error), ext ...ArgsForBaseAtomos) (errBeforeExec error) {
	if from == nil {
		return NewErrorf(ErrCodeFrameworkIncorrectUsage, "Atomos: from ID is nil.").AddStack(nil)
	}

	helper := createBaseAtomosHelper(BaseAtomosMailAsync, ext)
	if helper.hasErrors() {
		return helper.getError()
	}

	var startupID, asyncID uint64
	var fromInternal idInternal
	if callback != nil {
		var intErr *Error
		fromInternal, intErr = asIDInternal(from)
		if intErr != nil {
			return intErr.AddStack(nil)
		}
		startupID, asyncID = fromInternal.asyncSet(callback)
		// asyncID==0 表示发送方实例已完成停机清账:回调已被同步以
		// ErrAsyncCallbackCancelledByHalt 终结。此时不得再投递消息——
		// 否则调用方以为已取消,消息却在对端真实执行。返回 nil:回调已被终结,
		// 调用方不会挂起,正常路径(asyncID 从 1 开始递增)不受影响。
		if asyncID == 0 {
			return nil
		}
		if startupID == 0 {
			// 不应发生;coreFatal 后同样不得投递,避免调用方以为已取消而消息真实执行。
			a.log.coreFatal("PushAsyncMessage: asyncSet returns zero startupID, but it should not happen. startupID=(%d),asyncID=(%d)", startupID, asyncID)
			return nil
		}
	}

	am := allocBaseAtomosMail()
	initBaseAtomosMailAsync(am, from, name, startupID, asyncID, in)

	var ok bool
	if helper.appendToHead {
		ok = a.mailbox.pushHead(am.mail)
	} else {
		ok = a.mailbox.pushTail(am.mail)
	}
	if !ok {
		if asyncID != 0 {
			fromInternal.asyncPop(asyncID)
		}
		releaseMail(am.mail)
		return NewErrorf(ErrCodeAtomosIsNotRunning,
			"Atomos is not running. from=(%s),name=(%s),in=(%v)", from, name, in).AddStack(nil)
	}
	return nil
}

func (a *BaseAtomos) PushAsyncMessageCallback(callerID ID, name string, startupID, asyncID uint64, reply proto.Message, err error) {
	if asyncID == 0 {
		a.Log().Debug("PushAsyncMessageCallback called with empty asyncID. name=(%s),reply=(%v),err=(%v)")
		return
	}
	am := allocBaseAtomosMail()
	initAsyncMessageCallbackMail(am, callerID, name, startupID, asyncID, reply, asError(err))

	if ok := a.mailbox.pushTail(am.mail); !ok {
		a.log.logging.pushFrameworkErrorLog("PushAsyncMessageCallback: Atomos is not running. name=(%s),args=(%v)", name, reply)
	}

	//deallocAtomosMail(am)
}

func (a *BaseAtomos) PushKillMail(from ID, ext ...ArgsForBaseAtomos) (err error) {
	if from == nil {
		return NewErrorf(ErrCodeFrameworkIncorrectUsage, "Atomos: from ID is nil.").AddStack(nil)
	}

	helper := createBaseAtomosHelper(BaseAtomosMailKill, ext)
	if helper.hasErrors() {
		return helper.getError()
	}

	_, am, m := allocBaseAtomosKillMail()
	initAtomosKillMail(am, from)

	var ok bool
	if helper.appendToHead {
		ok = a.mailbox.pushHead(m)
	} else {
		ok = a.mailbox.pushTail(m)
	}
	if !ok {
		return NewErrorf(ErrCodeAtomosIsNotRunning, "Atomos is not running. from=(%s),wait=(%v)", from, helper.waitKilled).AddStack(nil)
	}
	if helper.waitKilled {
		_, ierr := am.waitReply(a, helper)
		return errOrNil(ierr.AddStack(nil))
	}
	return nil
}

func (a *BaseAtomos) PushWormholeMailAndWaitReply(from ID, wormhole BaseAtomosWormhole, ext ...ArgsForBaseAtomos) (err error) {
	if from == nil {
		return NewErrorf(ErrCodeFrameworkIncorrectUsage, "Atomos: from ID is nil.").AddStack(nil)
	}

	helper := createBaseAtomosHelper(BaseAtomosMailWormhole, ext)
	if helper.hasErrors() {
		return helper.getError()
	}

	am := allocBaseAtomosMail()
	initWormholeMail(am, from, wormhole)

	var ok bool
	if helper.appendToHead {
		ok = a.mailbox.pushHead(am.mail)
	} else {
		ok = a.mailbox.pushTail(am.mail)
	}
	if !ok {
		return NewErrorf(ErrCodeAtomosIsNotRunning, "Atomos is not running. from=(%s),wormhole=(%v)", from, wormhole).AddStack(nil)
	}
	_, ierr := am.waitReply(a, helper)

	//deallocAtomosMail(am)
	return errOrNil(ierr.AddStack(nil))
}

func (a *BaseAtomos) PushTaskCallbackMail(name string, callback func(), recoverFn func(any)) {
	am := allocBaseAtomosMail()
	initCallbackMail(am, name, callback, recoverFn)

	// THINKING: PushHead or PushTail? It is better to PushTail to keep the order.
	if ok := a.mailbox.pushTail(am.mail); !ok {
		a.log.logging.pushFrameworkErrorLog("PushTaskCallbackMail: Atomos is not running. name=(%s)", name)
	}

	//deallocAtomosMail(am)
}

// Callbacks

func (a *BaseAtomos) onSyncMessaging(fromID ID, name string, handler MessageHandler, in proto.Message) (out proto.Message, err *Error) {
	if fromID == nil {
		a.coreLog().coreFatal("BaseAtomos: onSyncMessaging called with nil fromID, but it should not happen.")
		return nil, NewErrorf(ErrCodeFrameworkInternalError, "BaseAtomos: onSyncMessaging called with nil fromID, but it should not happen.").AddStack(nil)
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				defer func() {
					if r2 := recover(); r2 != nil {
						a.Log().Fatal("BaseAtomos: Messaging critical problem again. err=(%v)", err)
					}
				}()
				if err == nil {
					err = NewErrorf(ErrCodeFrameworkRecoverFromPanic, "BaseAtomos: Messaging recovers from panic.").AddPanicStack(nil, 3, r)
				} else {
					err = err.AddPanicStack(nil, 3, r)
				}
				// Hook or Log
				if ar, ok := a.instance.(AtomosRecover); ok {
					ar.MessageRecover(name, in, err)
				} else {
					a.Log().Fatal("BaseAtomos: Messaging critical problem. err=(%v)", err)
				}
				// Global hook
				a.process.onRecoverHook(a.id, err)
			}
		}()
		var herr error
		out, herr = handler(fromID, a.instance, in)
		err = asError(herr)
		if err != nil {
			err = err.AddStack(nil)
		}
	}()
	return
}

func (a *BaseAtomos) onAsyncMessaging(fromID, toID ID, name string, handler MessageHandler, startupID, asyncID uint64, in proto.Message) {
	if fromID == nil {
		a.coreLog().coreFatal("BaseAtomos: onAsyncMessaging called with nil fromID, but it should not happen.")
		return
	}

	var err *Error
	var out proto.Message
	func() {
		defer func() {
			if r := recover(); r != nil {
				defer func() {
					if r2 := recover(); r2 != nil {
						a.Log().Fatal("BaseAtomos: onAsyncMessaging critical problem again. err=(%v)", err)
					}
				}()
				if err == nil {
					err = NewErrorf(ErrCodeFrameworkRecoverFromPanic, "BaseAtomos: onAsyncMessaging recovers from panic.").AddPanicStack(nil, 3, r)
				} else {
					err = err.AddPanicStack(nil, 3, r)
				}
				// Hook or Log
				if ar, ok := a.instance.(AtomosRecover); ok {
					ar.MessageRecover(name, in, err)
				} else {
					a.Log().Fatal("BaseAtomos: onAsyncMessaging critical problem. err=(%v)", err)
				}
				// Global hook
				a.process.onRecoverHook(a.id, err)
			}
		}()
		var herr error
		out, herr = handler(fromID, a.instance, in)
		err = asError(herr)
		if err != nil {
			err = err.AddStack(nil)
		}
		fromInternal, intErr := asIDInternal(fromID)
		if intErr != nil {
			a.log.coreFatal("BaseAtomos: onAsyncMessaging caller ID invalid, reply dropped. err=(%v)", intErr)
		} else {
			fromInternal.asyncCallback(toID, name, startupID, asyncID, out, errOrNil(err))
		}
	}()
}

func (a *BaseAtomos) onAsyncMessagingCallback(asyncID uint64, in proto.Message, err error) {
	if asyncID == 0 {
		a.Log().Error("BaseAtomos: onAsyncMessagingCallback called with asyncID zero. in=(%v),err=(%v)", in, err)
		return
	}
	handler := a.asyncPop(asyncID)
	if handler == nil {
		// instance 为当前实例 id：与 asyncSet 日志中的注册实例比对，
		// 不一致即说明回复落到了同名的新实例（注册实例已换代或已死）。
		a.Log().Error("BaseAtomos: onAsyncMessagingCallback handler not found. asyncID=(%d),instance=(%d),in=(%v),err=(%v)", asyncID, a.instanceID, in, err)
		return
	}
	a.Log().Debug("BaseAtomos: onAsyncMessagingCallback consumed. asyncID=(%d),instance=(%d)", asyncID, a.instanceID)
	handler(in, err)
}

func (a *BaseAtomos) asyncSet(callback func(out proto.Message, err error)) (startupID, callbackID uint64) {
	// state 在 mailbox 锁内快照,避免与 setStopping/setHalted 的加锁写构成数据竞争
	state := a.GetState()
	a.asyncCallMutex.Lock()
	// 停机清账已完成:不入表(否则成为清账后插入的泄漏条目),同步以专用错误码终结回调
	if a.asyncCleared {
		a.asyncCallMutex.Unlock()
		a.Log().Warn("BaseAtomos: asyncSet on halted atom, terminate with cancelled error. instance=(%d),name=(%s)", a.instanceID, a.id.Info())
		a.asyncTerminateSync(callback)
		return a.process.startupID, 0
	}
	a.asyncCallbackID += 1
	asyncID := a.asyncCallbackID
	a.asyncCallbackMap[asyncID] = asyncCallbackWrap{
		fn:  callback,
		now: time.Now(),
	}
	a.asyncCallMutex.Unlock()
	// 记录注册实例的 id 与状态：若实例已 Halt（发送方持有已死实例的 ID），
	// 回调必然丢失，直接打 Warn 便于发现。
	if state == BaseAtomosHalt {
		a.Log().Warn("BaseAtomos: asyncSet on halted atom, callback will be lost. asyncID=(%d),instance=(%d),name=(%s)", asyncID, a.instanceID, a.id.Info())
	} else {
		a.Log().Debug("BaseAtomos: asyncSet registered. asyncID=(%d),instance=(%d),state=(%d),name=(%s)", asyncID, a.instanceID, state, a.id.Info())
	}
	return a.process.startupID, asyncID
}

// asyncTerminateSync 清账后到达的注册:在调用方 goroutine 上同步终结回调(带 recover),
// 语义与停机清账一致——丢失的回调确定性以错误终结,而非静默消亡。
func (a *BaseAtomos) asyncTerminateSync(fn func(out proto.Message, err error)) {
	defer func() {
		if r := recover(); r != nil {
			a.Log().Error("BaseAtomos: late async callback panics on terminate. err=(%v)", r)
		}
	}()
	fn(nil, NewErrorf(ErrCodeAsyncCallbackCancelledByHalt,
		"BaseAtomos: Async callback cancelled by halt (late registration). atom=(%s),instance=(%d)", a.id.Info(), a.instanceID).AddStack(nil))
}

func (a *BaseAtomos) asyncPop(asyncID uint64) func(out proto.Message, err error) {
	a.asyncCallMutex.Lock()
	defer a.asyncCallMutex.Unlock()
	callback, ok := a.asyncCallbackMap[asyncID]
	if ok {
		delete(a.asyncCallbackMap, asyncID)
		return callback.fn
	}
	return nil
}

// AsyncCallbackPending 返回在途异步回调数量。业务侧(如空闲清理定时器)
// 可在 KillSelf 前调用,非空时推迟自杀,给在途回复留排空窗口。
func (a *BaseAtomos) AsyncCallbackPending() int {
	a.asyncCallMutex.Lock()
	defer a.asyncCallMutex.Unlock()
	return len(a.asyncCallbackMap)
}

// asyncCancelAll 在 atom 停机时清账:把所有在途回调以 ErrAsyncCallbackCancelledByHalt 终结,
// 避免"回调闭包随实例静默消亡"。每个回调独立 recover,回调 panic 不得阻断停机流程。
// 同时收编 PushAsyncMessage 发送失败路径注册后无人消费的 map 泄漏条目。
func (a *BaseAtomos) asyncCancelAll() {
	a.asyncCallMutex.Lock()
	// 先落 cleared 标记:并发的 asyncSet 在锁上排队,拿到锁后看到标记即走同步终结路径,
	// 从根上关闭"清账快照之后插入、永不终结"的泄漏窗口
	a.asyncCleared = true
	pending := make([]func(out proto.Message, err error), 0, len(a.asyncCallbackMap))
	for id, wrap := range a.asyncCallbackMap {
		if wrap.fn != nil {
			pending = append(pending, wrap.fn)
		}
		delete(a.asyncCallbackMap, id)
	}
	a.asyncCallMutex.Unlock()
	if len(pending) == 0 {
		return
	}
	a.Log().Info("BaseAtomos: cancel pending async callbacks on halt. count=(%d)", len(pending))
	err := NewErrorf(ErrCodeAsyncCallbackCancelledByHalt,
		"BaseAtomos: Async callback cancelled by halt. atom=(%s),instance=(%d)", a.id.Info(), a.instanceID).AddStack(nil)
	for _, fn := range pending {
		func() {
			defer func() {
				if r := recover(); r != nil {
					a.Log().Error("BaseAtomos: pending async callback panics on cancel. err=(%v)", r)
				}
			}()
			fn(nil, err)
		}()
	}
}

// State
// 各种状态
// State of Atom

func (a *BaseAtomos) idleTime() time.Duration {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	return a.mt.idleTime()
}

// TODO: Handle Halting state, check if need to wait halt then restart.
func (a *BaseAtomos) isNotHalt() bool {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	return BaseAtomosInvalidState < a.state && a.state < BaseAtomosHalt
}

func (a *BaseAtomos) GetState() BaseAtomosState {
	a.mailbox.mutex.Lock()
	state := a.state
	a.mailbox.mutex.Unlock()
	return state
}

func (a *BaseAtomos) IsInState(states ...BaseAtomosState) bool {
	state := a.GetState()
	for _, atomosState := range states {
		if atomosState == state {
			return true
		}
	}
	return false
}

func (a *BaseAtomos) setSpawning() {
	a.mailbox.mutex.Lock()
	a.state = BaseAtomosSpawning
	a.mt.spawning()
	a.mailbox.mutex.Unlock()
	a.process.onIDSpawning(a.id)
}

func (a *BaseAtomos) setSpawn() {
	a.mailbox.mutex.Lock()
	a.state = BaseAtomosWaiting
	a.mt.spawn()
	a.mailbox.mutex.Unlock()
	a.process.onIDSpawn(a.id)
}

func (a *BaseAtomos) setBusy(message string, arg proto.Message) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = BaseAtomosBusy
	a.mt.set(message, a.id, a.process, arg)
}

func (a *BaseAtomos) setWaiting(message string) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = BaseAtomosWaiting
	a.mt.unset(message)
}

// currentHandlingMessage returns the message this Atomos is currently
// handling, or "" when idle. The tracker is written by the mailbox goroutine
// under the mailbox mutex (setBusy/setWaiting); readers such as
// waitReply's timeout diagnostics must take the same lock — an unsynchronized
// read races with the mailbox completing a mail exactly when the timeout
// fires.
func (a *BaseAtomos) currentHandlingMessage() string {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	return a.mt.current
}

func (a *BaseAtomos) setStopping() {
	a.mailbox.mutex.Lock()
	a.state = BaseAtomosStopping
	a.mt.stopping()
	a.mailbox.mutex.Unlock()
	a.process.onIDStopping(a.id)
}

func (a *BaseAtomos) setHalted(err *Error) {
	// Only the state transition is guarded by the mailbox mutex. The user hook
	// (onIDHalted) and the stoppingChan signal are invoked *after* releasing the
	// lock, so that a slow/recursive hook or a full channel cannot deadlock the
	// whole mailbox (which would block every subsequent push/waitPop).
	a.mailbox.mutex.Lock()
	a.state = BaseAtomosHalt
	a.mt.halted()
	a.mailbox.mutex.Unlock()

	a.process.onIDHalted(a.id, err, a.mt)
	// stoppingChan is buffered (cap 1); use a non-blocking send so that a repeat
	// halt path can never wedge here while holding no lock either.
	select {
	case a.stoppingChan <- true:
	default:
	}
}

// Mailbox

func (a *BaseAtomos) start(fn func() *Error) *Error {
	return a.mailbox.start(fn)
}

// 处理邮箱启动
func (a *BaseAtomos) mailboxOnStartUp(fn func() *Error) *Error {
	a.setSpawning()
	if fn != nil {
		if err := fn(); err != nil {
			a.setHalted(err.AddStack(nil))
			return err
		}
	}
	a.setSpawn()
	return nil
}

// 处理邮箱消息。
// Handle mailbox messages.
func (a *BaseAtomos) mailboxOnReceive(mail *mail) {
	am := unwrapBaseAtomosMail(mail)
	if !a.IsInState(BaseAtomosWaiting) {
		a.log.logging.pushFrameworkErrorLog("Atomos: onReceive meets non-waiting status. atomos=(%v),state=(%d),mail=(%v)",
			a, a.GetState(), mail)
	}
	switch am.mailType {
	case BaseAtomosMailSync:
		{
			a.setBusy(am.name, am.arg)
			defer a.setWaiting(am.name)
			if am.waitChain != nil {
				// While this (remote-originated or chain-carrying) sync mail
				// executes, expose its chain as the mailbox goroutine's
				// ambient chain: remote calls from the handler extend it.
				a.process.setSyncChain(a.mailbox.goID, am.waitChain)
				defer a.process.clearSyncChain(a.mailbox.goID)
			}

			resp, err := a.holder.onSyncMessaging(am.from, am.name, am.arg)
			if resp != nil {
				resp = proto.Clone(resp)
			}
			am.sendReply(resp, err)
			// Mail dealloc in AtomCore.pushMessageMail.
		}
	case BaseAtomosMailAsync:
		{
			a.setBusy(am.name, am.arg)
			defer a.setWaiting(am.name)

			a.holder.onAsyncMessaging(am.from, am.name, am.startupID, am.asyncID, am.arg)
		}
	case BaseAtomosMailOnAsyncCallback:
		{
			name := "AsyncMessageCallback-" + am.name
			a.setBusy(name, am.arg)
			defer a.setWaiting(name)

			a.holder.onAsyncMessagingCallback(am.asyncID, am.arg, errOrNil(am.err))
		}
	case BaseAtomosMailTask:
		{
			name := "Task-" + am.name
			a.setBusy(name, nil)
			defer a.setWaiting(name)

			a.task.handleTask(am)
			// Mail dealloc in atomosTaskManager.handleTask and cancels.
		}
	case BaseAtomosMailWormhole:
		{
			a.setBusy("AcceptWormhole", am.arg)
			defer a.setWaiting("AcceptWormhole")

			err := a.holder.onWormhole(am.from, am.wormhole)
			am.sendReply(nil, err)
			// Mail dealloc in AtomCore.pushWormholeMail.
		}
	case BaseAtomosMailTaskCallback:
		{
			name := "Callback-" + am.name
			a.setBusy(name, nil)
			defer a.setWaiting(name)

			a.holder.onFnCallback(am.atomosCallback)
		}
	default:
		a.log.Fatal("Atomos: Received unknown message type, type=(%v),mail=(%+v)", am.mailType, am)
	}

	releaseBaseAtomosMail(am)
}

// 处理邮箱退出。
// Handle mailbox stops.
func (a *BaseAtomos) mailboxOnStop(killMail, remainMail *mail, num uint32) (err *Error) {
	defer releaseAtomosMessageTracker(&a.mt)
	defer releaseAtomosTasksManager(&a.task)

	// Fail any outstanding async callbacks. Once this atomos is halting it will
	// never receive replies for them, so without this cleanup each entry (which
	// captures the caller's closure) would leak forever. Terminate each callback
	// with ErrAsyncCallbackCancelledByHalt, then drop the entry; a callback that
	// panics must not interrupt the clearing.
	// 停机清账:在途回调逐一以 ErrAsyncCallbackCancelledByHalt 同步终结。
	// 注意:清账发生在 setStopping 之前(保持本线既有顺序),asyncCancelAll 会先落
	// asyncCleared 标记,此后到达的注册一律在 asyncSet 内同步终结,不依赖停机状态。
	a.asyncCancelAll()

	a.task.stopLock()
	defer a.task.stopUnlock()

	// State is thread-safe get.
	state := a.GetState()
	switch state {
	case BaseAtomosHalt:
		if a.mailbox.running {
			a.log.logging.pushFrameworkErrorLog("Atomos: onStop meets halted but mailbox running status. atomos=(%v)", a)
		}
		return

	case BaseAtomosSpawning:
		a.log.logging.pushFrameworkErrorLog("Atomos: onStop meets spawning status. atomos=(%v)", a)

	case BaseAtomosBusy:
		a.log.logging.pushFrameworkErrorLog("Atomos: onStop meets busy status. atomos=(%v)", a)

	case BaseAtomosStopping:
		a.log.logging.pushFrameworkErrorLog("Atomos: onStop meets stopping status. atomos=(%v)", a)
	}

	a.setStopping()

	killMails := make([]*baseAtomosMail, 0, 1)
	defer func() {
		for _, killMail := range killMails {
			killMail.sendReply(nil, cloneError(err))
			releaseBaseAtomosMail(killMail)
		}
	}()
	defer func() {
		a.setHalted(err)
	}()

	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					a.Log().Fatal("Atomos: Stopping recovers from panic. err=(%v)", err)
				}
			}()
			if err == nil {
				err = NewErrorf(ErrCodeFrameworkRecoverFromPanic, "Atomos: Stopping recovers from panic.").AddPanicStack(nil, 2, r)
			} else {
				err = err.AddPanicStack(nil, 2, r)
			}
			// Hook or Log
			if ar, ok := a.instance.(AtomosRecover); ok {
				ar.StopRecover(err)
			} else {
				a.Log().Fatal("Atomos: Stopping recovers from panic. err=(%v)", err)
			}
			// Global hook
			a.process.onRecoverHook(a.id, err)
		}
	}()

	cancels := a.task.cancelAllSchedulingTasks()
	if len(cancels) > 0 {
		a.log.Info("Atomos: Stopping cancels all scheduling tasks. id=(%s),cancelled=(%v)", a.String(), cancels)
	}
	for ; remainMail != nil; remainMail = remainMail.next {
		func(remainMail *mail) {
			switch value := remainMail.data.(type) {
			case *mailExitCommand:
				{
					a.log.Info("Atomos: Stopping meets mail exit command. mailID=(%d)", remainMail.id)
					remainAtomMail := unwrapBaseAtomosFromKillMail(value)
					switch remainAtomMail.mailType {
					case BaseAtomosMailKill:
						killMails = append(killMails, remainAtomMail)
					}
				}
			case *baseAtomosMail:
				{
					remainAtomMail := value
					// 注意:不能用 %v/%+v 打印 remainMail/remainAtomMail——fmt 反射会读到
					// baseAtomosMail.mutex,而发送方 goroutine 正在 waitReply 里操作同一把锁,
					// 构成数据竞争。这里只记录标量字段。
					err := NewErrorf(ErrCodeAtomosIsStopping, "Atomos: Stopping. mailID=(%d),mailType=(%d),name=(%s)", remainMail.id, remainAtomMail.mailType, remainAtomMail.name).AddStack(nil)
					//defer deallocAtomosMail(remainAtomMail)
					switch remainAtomMail.mailType {
					case BaseAtomosMailKill:
						killMails = append(killMails, remainAtomMail)
					case BaseAtomosMailSync:
						remainAtomMail.sendReply(nil, err)
						// TODO ? Mail dealloc in AtomCore.pushMessageMail.
					case BaseAtomosMailAsync:
						if remainAtomMail.asyncID != 0 {
							a.log.Info("Atomos: Stopping so async message mail will callback with error. id=(%s),mailID=(%d),name=(%s),asyncID=(%d)", a.String(), remainMail.id, remainAtomMail.name, remainAtomMail.asyncID)
							if fromInternal, intErr := asIDInternal(remainAtomMail.from); intErr != nil {
								a.log.coreError("Atomos: Stopping async reply dropped, caller ID invalid. err=(%v)", intErr)
							} else {
								fromInternal.asyncCallback(a.impl, remainAtomMail.name, remainAtomMail.startupID, remainAtomMail.asyncID, nil, err)
							}
						}
					case BaseAtomosMailOnAsyncCallback:
						a.log.Info("Atomos: Stopping so async message callback mail will log but cannot callback. id=(%s),mailID=(%d),name=(%s),asyncID=(%d)", a.String(), remainMail.id, remainAtomMail.name, remainAtomMail.asyncID)
					case BaseAtomosMailTask:
						// 正常，因为可能因为断点等原因阻塞，导致在执行关闭atomos的过程中，有任务的计时器到达时间，从而导致此逻辑。
						// Is it needed? It just for preventing new mails receive after cancelAllSchedulingTasks,
						// but it's impossible to add task after locking.
						a.log.Info("Atomos: Stopping task mails have been sent after start closing. id=(%s),mailID=(%d)", a.String(), remainMail.id)
						if err := a.task.cancelTask(remainMail.id, true, value.atomosTask); err == nil {
							cancels = append(cancels, remainMail.id)
						}
						// TODO ? Mail dealloc in atomosTaskManager.cancelTask.
					case BaseAtomosMailWormhole:
						remainAtomMail.sendReply(nil, err)
						// TODO ? Mail dealloc in AtomCore.pushWormholeMail.
					case BaseAtomosMailTaskCallback:
						a.holder.onFnCallback(remainAtomMail.atomosCallback)
					default:
						a.log.coreFatal("Atomos: Stopping unknown message type. type=%v,mail=%+v",
							remainAtomMail.mailType, remainAtomMail)
					}
					if remainAtomMail.mailType != BaseAtomosMailKill {
						releaseBaseAtomosMail(remainAtomMail)
					}
				}
			default:
				{
					a.log.coreFatal("Atomos: Stopping unknown message type. mail=(%v)", remainMail)
				}
			}
		}(remainMail)
	}

	// Handle Kill and Reply Kill.
	em, ok := killMail.data.(*mailExitCommand)
	if !ok {
		a.log.Fatal("Atomos: onStop received invalid kill mail data type, data=(%v),mail=(%v)", killMail.data, killMail)
		return
	}
	am, ok := em.data.(*baseAtomosMail)
	if !ok {
		a.log.Fatal("Atomos: onStop received invalid kill mail baseAtomosMail type, data=(%v),mail=(%v)", em.data, killMail)
		return
	}
	killMails = append(killMails, am)
	err = a.holder.onStopping(am.from, cancels)
	if err != nil {
		err = err.AddStack(nil)
	}

	return err
}

// baseAtomosHelper

type baseAtomosHelper struct {
	ext []ArgsForBaseAtomos

	invalidArgs  []baseAtomosExtInvalidArg
	conflictArgs []baseAtomosExtInvalidArg

	fields []int32

	timeout      time.Duration
	appendToHead bool
	waitKilled   bool
	// waitChain is the cross-node wait chain carried by a remote-originated
	// sync call (internal arg; not recorded in fields — it is runtime
	// metadata, not a user argument).
	waitChain []string
}

type baseAtomosExtInvalidArg struct {
	idx    int
	arg    ArgsForBaseAtomos
	reason string
}

func createBaseAtomosHelper(mailType BaseAtomosMailType, ext []ArgsForBaseAtomos) *baseAtomosHelper {
	helper := &baseAtomosHelper{
		ext: ext,
	}
	for i, v := range ext {
		if v == nil {
			helper.addInvalidArg(i, v, "Argument is nil")
			continue
		}
		switch v.getArgType() {
		case ArgTypeBaseAtomosTimeout:
			if !helper.checkMailTypeIn(mailType, []BaseAtomosMailType{BaseAtomosMailKill, BaseAtomosMailSync, BaseAtomosMailWormhole}) {
				helper.addInvalidArg(i, v, "Timeout argument is only valid for Sync and Wormhole mails")
				continue
			}
			if helper.timeout != 0 {
				helper.addConflictArg(i, v, "Timeout argument conflict")
			} else {
				helper.fields = append(helper.fields, int32(ArgTypeBaseAtomosTimeout))
				switch arg := v.(type) {
				case argBaseAtomosTimeout:
					helper.timeout = arg.timeout
				case *argBaseAtomosTimeout:
					helper.timeout = arg.timeout
				default:
					helper.addInvalidArg(i, v, "Timeout argument type is invalid")
					continue
				}
			}
		case ArgTypeBaseAtomosAppendToHead:
			if !helper.checkMailTypeIn(mailType, []BaseAtomosMailType{BaseAtomosMailKill, BaseAtomosMailSync, BaseAtomosMailAsync, BaseAtomosMailWormhole}) {
				helper.addInvalidArg(i, v, "AppendToHead argument is only valid for Sync, Async and Wormhole mails")
				continue
			}
			if helper.appendToHead {
				helper.addConflictArg(i, v, "AppendToHead argument conflict")
			} else {
				helper.fields = append(helper.fields, int32(ArgTypeBaseAtomosAppendToHead))
				helper.appendToHead = true
			}
		case ArgTypeBaseAtomosWaitKilled:
			if !helper.checkMailTypeIn(mailType, []BaseAtomosMailType{BaseAtomosMailKill}) {
				helper.addInvalidArg(i, v, "WaitKilled argument is only valid for Halt mails")
				continue
			}
			if helper.waitKilled {
				helper.addConflictArg(i, v, "WaitKilled argument conflict")
			} else {
				helper.fields = append(helper.fields, int32(ArgTypeBaseAtomosWaitKilled))
				helper.waitKilled = true
			}
		case ArgTypeBaseAtomosWaitChain:
			if !helper.checkMailTypeIn(mailType, []BaseAtomosMailType{BaseAtomosMailSync}) {
				helper.addInvalidArg(i, v, "WaitChain argument is only valid for Sync mails")
				continue
			}
			if helper.waitChain != nil {
				helper.addConflictArg(i, v, "WaitChain argument conflict")
			} else {
				switch arg := v.(type) {
				case argBaseAtomosWaitChain:
					helper.waitChain = arg.chain
				case *argBaseAtomosWaitChain:
					helper.waitChain = arg.chain
				default:
					helper.addInvalidArg(i, v, "WaitChain argument type is invalid")
					continue
				}
			}
		default:
			helper.addInvalidArg(i, v, "Argument type is invalid")
		}
	}
	return helper
}

func createExtForCosmosArgs(arg *CosmosArgs) []ArgsForBaseAtomos {
	var ext []ArgsForBaseAtomos
	if arg == nil {
		return ext
	}
	if arg.BaseAtomosTimeoutInNano != 0 {
		ext = append(ext, ArgBaseAtomosTimeout(time.Duration(arg.BaseAtomosTimeoutInNano)))
	}
	if arg.BaseAtomosAppendToHead {
		ext = append(ext, ArgBaseAtomosAppendToHead())
	}
	if arg.BaseAtomosWaitKilled {
		ext = append(ext, ArgBaseAtomosWaitKilled())
	}
	if len(arg.WaitChain) > 0 {
		ext = append(ext, argBaseAtomosWaitChain{chain: arg.WaitChain})
	}
	return ext
}

func (h *baseAtomosHelper) getRemoteArg() *CosmosArgs {
	return &CosmosArgs{
		Types:                   h.fields,
		BaseAtomosTimeoutInNano: h.timeout.Nanoseconds(),
		BaseAtomosAppendToHead:  h.appendToHead,
		BaseAtomosWaitKilled:    h.waitKilled,
	}
}

func (h *baseAtomosHelper) checkMailTypeIn(mailType BaseAtomosMailType, inTypes []BaseAtomosMailType) bool {
	for _, t := range inTypes {
		if t == mailType {
			return true
		}
	}
	return false
}

func (h *baseAtomosHelper) addInvalidArg(idx int, arg ArgsForBaseAtomos, reason string) {
	h.invalidArgs = append(h.invalidArgs, baseAtomosExtInvalidArg{
		idx:    idx,
		arg:    arg,
		reason: reason,
	})
}

func (h *baseAtomosHelper) addConflictArg(idx int, arg ArgsForBaseAtomos, reason string) {
	h.conflictArgs = append(h.conflictArgs, baseAtomosExtInvalidArg{
		idx:    idx,
		arg:    arg,
		reason: reason,
	})
}

func (h *baseAtomosHelper) hasErrors() bool {
	return len(h.invalidArgs) > 0 || len(h.conflictArgs) > 0
}

func (h *baseAtomosHelper) getError() *Error {
	var builder strings.Builder
	if len(h.invalidArgs) > 0 {
		builder.WriteString("Invalid Arguments:")
		for _, ia := range h.invalidArgs {
			builder.WriteString(fmt.Sprintf("\nAt index %d is invalid: %s.", ia.idx, ia.reason))
		}
		builder.WriteString("\n")
	}
	if len(h.conflictArgs) > 0 {
		builder.WriteString("Conflict Arguments:")
		for _, ca := range h.conflictArgs {
			builder.WriteString(fmt.Sprintf("\nAt index %d is conflict: %s.", ca.idx, ca.reason))
		}
		builder.WriteString("\n")
	}
	errReason := builder.String()
	if errReason != "" {
		return NewErrorf(ErrCodeAtomosInvalidArguments, "BaseAtomos: Arguments Error. %s", errReason)
	}
	return nil
}
