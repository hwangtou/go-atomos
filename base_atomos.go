package atomos

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"strings"
	"sync"
	"time"
)

var detectDeadlock = true

// BaseAtomosHolder Atomos持有者
// Base Atomos Holder
type BaseAtomosHolder interface {
	// OnSyncMessaging
	// 收到消息
	OnSyncMessaging(fromID ID, name string, in proto.Message) (out proto.Message, err *Error)

	// OnAsyncMessaging
	// 收到异步消息
	OnAsyncMessaging(fromID ID, name string, startupID, asyncID uint64, in proto.Message)

	// OnAsyncMessagingCallback
	// 收到异步消息回调
	OnAsyncMessagingCallback(asyncID uint64, in proto.Message, err *Error)

	OnFnCallback(*atomosCallback)

	// OnWormhole
	// 收到Wormhole
	OnWormhole(from ID, wormhole BaseAtomosWormhole) *Error

	// OnStopping
	// 停止中
	OnStopping(from ID, cancelled []uint64) *Error

	// OnIDsReleased
	// 释放了所有ID
	OnIDsReleased()
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
		return "Stopping"
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

// BaseAtomosWormhole
// 虫洞传送的对象
// Object of Wormhole.
type BaseAtomosWormhole interface{}

// BaseAtomos 基础Atomos
// Base Atomos
type BaseAtomos struct {
	// 进程
	process *CosmosProcess
	// 实现和句柄信息
	impl ID
	id   *IDInfo

	// 状态
	// State
	state BaseAtomosState

	// Atomos邮箱，也是实现Atom无锁队列的关键。
	// Mailbox, the key of lockless queue of Atom.
	mailbox *mailBox

	// 持有者
	holder BaseAtomosHolder
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

	// ID追踪管理器
	// ID Tracker Manager
	it *atomosIDTracker

	asyncCallMutex   sync.Mutex
	asyncCallbackID  uint64
	asyncCallbackMap map[uint64]asyncCallbackWrap

	stoppingChan chan bool
}

type asyncCallbackWrap struct {
	fn  func(out proto.Message, err *Error)
	now time.Time
}

func NewBaseAtomos(impl ID, id *IDInfo, lv LogLevel, holder BaseAtomosHolder, inst Atomos, process *CosmosProcess) *BaseAtomos {
	a := &BaseAtomos{
		process:          process,
		impl:             impl,
		id:               id,
		state:            BaseAtomosHalt,
		mailbox:          nil,
		holder:           holder,
		instance:         inst,
		log:              atomosLogging{},
		task:             atomosTaskManager{},
		mt:               atomosMessageTracker{},
		it:               &atomosIDTracker{},
		asyncCallMutex:   sync.Mutex{},
		asyncCallbackID:  0,
		asyncCallbackMap: map[uint64]asyncCallbackWrap{},
		stoppingChan:     make(chan bool, 1),
	}
	a.mailbox = newMailBox(id.Info(), a, process.logging)
	initAtomosLog(&a.log, a.id, lv, process.logging)
	initAtomosTasksManager(a.log.logging, &a.task, a)
	initAtomosMessageTracker(&a.mt)
	initAtomosIDTracker(a.it, a)
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

func (a *BaseAtomos) Task() Task {
	return &a.task
}

// go:noinline
func (a *BaseAtomos) GetGoID() uint64 {
	return a.mailbox.goID
}

func (a *BaseAtomos) PushSyncMessage(from ID, name string, in proto.Message, ext []ArgsForBaseAtomos) (reply proto.Message, err *Error) {
	if from == nil {
		return nil, NewErrorf(ErrFrameworkIncorrectUsage, "Atomos: from ID is nil.").AddStack(nil)
	}

	helper := createBaseAtomosHelper(BaseAtomosMailSync, ext)
	if helper.hasErrors() {
		return nil, helper.getError()
	}

	am := allocBaseAtomosMail()
	initBaseAtomosMailSync(am, from, name, in)

	var ok bool
	if helper.appendToHead {
		ok = a.mailbox.pushHead(am.mail)
	} else {
		ok = a.mailbox.pushTail(am.mail)
	}
	if !ok {
		return reply, NewErrorf(ErrAtomosIsNotRunning,
			"Atomos is not running. from=(%s),name=(%s),in=(%v)", from, name, in).AddStack(nil)
	}

	replyInterface, err := am.waitReply(a, helper)
	if err != nil && err.Code == ErrAtomosIsNotRunning {
		return nil, err.AddStack(nil)
	}
	reply, ok = replyInterface.(proto.Message)
	if !ok {
		return nil, err.AddStack(nil)
	}
	return reply, err.AddStack(nil)
}

func (a *BaseAtomos) PushAsyncMessage(from ID, name string, in proto.Message, callback func(reply proto.Message, err *Error), ext []ArgsForBaseAtomos) (errBeforeExec *Error) {
	if from == nil {
		return NewErrorf(ErrFrameworkIncorrectUsage, "Atomos: from ID is nil.").AddStack(nil)
	}

	helper := createBaseAtomosHelper(BaseAtomosMailAsync, ext)
	if helper.hasErrors() {
		return helper.getError()
	}

	var startupID, asyncID uint64
	if callback != nil {
		startupID, asyncID = from.asyncSet(callback)
		if startupID == 0 || asyncID == 0 {
			a.log.coreFatal("PushAsyncMessage: asyncSet returns zero ID, but it should not happen. startupID=(%d),asyncID=(%d)", startupID, asyncID)
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
		return NewErrorf(ErrAtomosIsNotRunning,
			"Atomos is not running. from=(%s),name=(%s),in=(%v)", from, name, in).AddStack(nil)
	}
	return nil
}

func (a *BaseAtomos) PushAsyncMessageCallback(callerID ID, name string, startupID, asyncID uint64, reply proto.Message, err *Error) {
	if asyncID == 0 {
		a.Log().Debug("PushAsyncMessageCallback called with empty asyncID. name=(%s),reply=(%v),err=(%v)")
		return
	}
	am := allocBaseAtomosMail()
	initAsyncMessageCallbackMail(am, callerID, name, startupID, asyncID, reply, err)

	if ok := a.mailbox.pushTail(am.mail); !ok {
		a.log.logging.pushFrameworkErrorLog("PushAsyncMessageCallback: Atomos is not running. name=(%s),args=(%v)", name, reply)
	}

	//deallocAtomosMail(am)
}

func (a *BaseAtomos) PushKillMail(from ID, ext []ArgsForBaseAtomos) (err *Error) {
	if from == nil {
		return NewErrorf(ErrFrameworkIncorrectUsage, "Atomos: from ID is nil.").AddStack(nil)
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
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos is not running. from=(%s),wait=(%v)", from, helper.waitKilled).AddStack(nil)
	}
	if helper.waitKilled {
		_, err = am.waitReply(a, helper)
		return err.AddStack(nil)
	}
	return nil
}

func (a *BaseAtomos) PushWormholeMailAndWaitReply(from ID, wormhole BaseAtomosWormhole, ext []ArgsForBaseAtomos) (err *Error) {
	if from == nil {
		return NewErrorf(ErrFrameworkIncorrectUsage, "Atomos: from ID is nil.").AddStack(nil)
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
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos is not running. from=(%s),wormhole=(%v)", from, wormhole).AddStack(nil)
	}
	_, err = am.waitReply(a, helper)

	//deallocAtomosMail(am)
	return err.AddStack(nil)
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

func (a *BaseAtomos) OnSyncMessaging(fromID ID, name string, handler MessageHandler, in proto.Message) (out proto.Message, err *Error) {
	if fromID == nil {
		a.Log().coreFatal("BaseAtomos: OnSyncMessaging called with nil fromID, but it should not happen.")
		return nil, NewErrorf(ErrFrameworkInternalError, "BaseAtomos: OnSyncMessaging called with nil fromID, but it should not happen.").AddStack(nil)
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
					err = NewErrorf(ErrFrameworkRecoverFromPanic, "BaseAtomos: Messaging recovers from panic.").AddPanicStack(nil, 3, r)
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
		out, err = handler(fromID, a.instance, in)
		if err != nil {
			err = err.AddStack(nil)
		}
	}()
	return
}

func (a *BaseAtomos) OnAsyncMessaging(fromID, toID ID, name string, handler MessageHandler, startupID, asyncID uint64, in proto.Message) {
	if fromID == nil {
		a.Log().coreFatal("BaseAtomos: OnAsyncMessaging called with nil fromID, but it should not happen.")
		return
	}

	var err *Error
	var out proto.Message
	func() {
		defer func() {
			if r := recover(); r != nil {
				defer func() {
					if r2 := recover(); r2 != nil {
						a.Log().Fatal("BaseAtomos: OnAsyncMessaging critical problem again. err=(%v)", err)
					}
				}()
				if err == nil {
					err = NewErrorf(ErrFrameworkRecoverFromPanic, "BaseAtomos: OnAsyncMessaging recovers from panic.").AddPanicStack(nil, 3, r)
				} else {
					err = err.AddPanicStack(nil, 3, r)
				}
				// Hook or Log
				if ar, ok := a.instance.(AtomosRecover); ok {
					ar.MessageRecover(name, in, err)
				} else {
					a.Log().Fatal("BaseAtomos: OnAsyncMessaging critical problem. err=(%v)", err)
				}
				// Global hook
				a.process.onRecoverHook(a.id, err)
			}
		}()
		out, err = handler(fromID, a.instance, in)
		if err != nil {
			err = err.AddStack(nil)
		}
		fromID.asyncCallback(toID, name, startupID, asyncID, out, err)
	}()
}

func (a *BaseAtomos) OnAsyncMessagingCallback(asyncID uint64, in proto.Message, err *Error) {
	if asyncID == 0 {
		a.Log().Error("BaseAtomos: OnAsyncMessagingCallback called with asyncID zero. in=(%v),err=(%v)", in, err)
		return
	}
	handler := a.asyncPop(asyncID)
	if handler == nil {
		a.Log().Error("BaseAtomos: OnAsyncMessagingCallback handler not found. asyncID=(%d),in=(%v),err=(%v)", asyncID, in, err)
		return
	}
	handler(in, err)
}

func (a *BaseAtomos) asyncSet(callback func(out proto.Message, err *Error)) (startupID, callbackID uint64) {
	a.asyncCallMutex.Lock()
	defer a.asyncCallMutex.Unlock()
	a.asyncCallbackID += 1
	asyncID := a.asyncCallbackID
	a.asyncCallbackMap[asyncID] = asyncCallbackWrap{
		fn:  callback,
		now: time.Now(),
	}
	return a.process.startupID, asyncID
}

func (a *BaseAtomos) asyncPop(asyncID uint64) func(out proto.Message, err *Error) {
	a.asyncCallMutex.Lock()
	defer a.asyncCallMutex.Unlock()
	callback, ok := a.asyncCallbackMap[asyncID]
	if ok {
		delete(a.asyncCallbackMap, asyncID)
		return callback.fn
	}
	return nil
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
	defer a.mailbox.mutex.Unlock()
	a.state = BaseAtomosSpawning
	a.mt.spawning()
	a.process.onIDSpawning(a.id)
}

func (a *BaseAtomos) setSpawn() {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = BaseAtomosWaiting
	a.mt.spawn()
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

func (a *BaseAtomos) setStopping() {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = BaseAtomosStopping
	a.mt.stopping()
	a.process.onIDStopping(a.id)
}

func (a *BaseAtomos) setHalted(err *Error) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = BaseAtomosHalt
	a.mt.halted()
	a.process.onIDHalted(a.id, err, a.mt)
	a.stoppingChan <- true
}

// IDTracker

func (a *BaseAtomos) onIDReleased() {
	a.holder.OnIDsReleased()
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

			resp, err := a.holder.OnSyncMessaging(am.from, am.name, am.arg)
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

			a.holder.OnAsyncMessaging(am.from, am.name, am.startupID, am.asyncID, am.arg)
		}
	case BaseAtomosMailOnAsyncCallback:
		{
			name := "AsyncMessageCallback-" + am.name
			a.setBusy(name, am.arg)
			defer a.setWaiting(name)

			a.holder.OnAsyncMessagingCallback(am.asyncID, am.arg, am.err)
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

			err := a.holder.OnWormhole(am.from, am.wormhole)
			am.sendReply(nil, err)
			// Mail dealloc in AtomCore.pushWormholeMail.
		}
	case BaseAtomosMailTaskCallback:
		{
			name := "Callback-" + am.name
			a.setBusy(name, nil)
			defer a.setWaiting(name)

			a.holder.OnFnCallback(am.atomosCallback)
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
	defer a.setHalted(err)

	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					a.Log().Fatal("Atomos: Stopping recovers from panic. err=(%v)", err)
				}
			}()
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "Atomos: Stopping recovers from panic.").AddPanicStack(nil, 2, r)
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
					a.log.Info("Atomos: Stopping meets mail exit command. mail=(%v)", remainMail)
					remainAtomMail := unwrapBaseAtomosFromKillMail(value)
					switch remainAtomMail.mailType {
					case BaseAtomosMailKill:
						remainAtomMail.sendReply(nil, err)
						// TODO ? Mail dealloc in AtomCore.pushKillMail.
					}
					releaseBaseAtomosMail(remainAtomMail)
				}
			case *baseAtomosMail:
				{
					remainAtomMail := value
					err := NewErrorf(ErrAtomosIsStopping, "Atomos: Stopping. mail=(%v),mail=(%v)", remainMail, remainAtomMail).AddStack(nil)
					//defer deallocAtomosMail(remainAtomMail)
					switch remainAtomMail.mailType {
					case BaseAtomosMailKill:
						remainAtomMail.sendReply(nil, err)
						// TODO ? Mail dealloc in AtomCore.pushKillMail.
					case BaseAtomosMailSync:
						remainAtomMail.sendReply(nil, err)
						// TODO ? Mail dealloc in AtomCore.pushMessageMail.
					case BaseAtomosMailAsync:
						if remainAtomMail.asyncID != 0 {
							a.log.Info("Atomos: Stopping so async message mail will callback with error. id=(%s),mail=(%+v)", a.String(), remainAtomMail)
							remainAtomMail.from.asyncCallback(a.impl, remainAtomMail.name, remainAtomMail.startupID, remainAtomMail.asyncID, nil, err)
						}
					case BaseAtomosMailOnAsyncCallback:
						a.log.Info("Atomos: Stopping so async message callback mail will log but cannot callback. id=(%s),mail=(%+v)", a.String(), remainAtomMail)
					case BaseAtomosMailTask:
						// 正常，因为可能因为断点等原因阻塞，导致在执行关闭atomos的过程中，有任务的计时器到达时间，从而导致此逻辑。
						// Is it needed? It just for preventing new mails receive after cancelAllSchedulingTasks,
						// but it's impossible to add task after locking.
						a.log.Info("Atomos: Stopping task mails have been sent after start closing. id=(%s),mail=(%+v)", a.String(), remainMail)
						if err := a.task.cancelTask(remainMail.id, true, value.atomosTask); err == nil {
							cancels = append(cancels, remainMail.id)
						}
						// TODO ? Mail dealloc in atomosTaskManager.cancelTask.
					case BaseAtomosMailWormhole:
						remainAtomMail.sendReply(nil, err)
						// TODO ? Mail dealloc in AtomCore.pushWormholeMail.
					case BaseAtomosMailTaskCallback:
						a.holder.OnFnCallback(remainAtomMail.atomosCallback)
					default:
						a.log.coreFatal("Atomos: Stopping unknown message type. type=%v,mail=%+v",
							remainAtomMail.mailType, remainAtomMail)
					}
					releaseBaseAtomosMail(remainAtomMail)
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
	err = a.holder.OnStopping(am.from, cancels)
	if err != nil {
		err = err.AddStack(nil)
	}
	am.sendReply(nil, err)

	releaseBaseAtomosMail(am)

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
		return NewErrorf(ErrAtomosInvalidArguments, "BaseAtomos: Arguments Error. %s", errReason)
	}
	return nil
}
