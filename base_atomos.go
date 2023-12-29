package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"time"
)

// AtomosHolder Atomos持有者
// Atomos Holder
type AtomosHolder interface {
	// OnMessaging
	// 收到消息
	OnMessaging(fromID ID, name string, in proto.Message) (out proto.Message, err *Error)

	// OnAsyncMessaging
	// 收到异步消息
	OnAsyncMessaging(fromID ID, name string, in proto.Message, callback func(reply proto.Message, err *Error))

	// OnAsyncMessagingCallback
	// 收到异步消息回调
	OnAsyncMessagingCallback(in proto.Message, err *Error, callback func(reply proto.Message, err *Error))

	// OnScaling
	// 负载均衡决策
	OnScaling(from ID, name string, args proto.Message) (id ID, err *Error)

	// OnWormhole
	// 收到Wormhole
	OnWormhole(from ID, wormhole AtomosWormhole) *Error

	// OnStopping
	// 停止中
	OnStopping(from ID, cancelled []uint64) *Error

	// OnIDsReleased
	// 释放了所有ID
	OnIDsReleased()
}

// Atom状态
// AtomosState

type AtomosState int

const (
	// AtomosHalt
	// 停止
	// Atom is stopped.
	AtomosHalt AtomosState = 0

	// AtomosSpawning
	// 启动中
	// Atom is starting up.
	AtomosSpawning AtomosState = 1

	// AtomosWaiting
	// 启动成功，等待消息
	// Atom is started and waiting for message.
	AtomosWaiting AtomosState = 2

	// AtomosBusy
	// 启动成功，正在处理消息
	// Atom is started and busy processing message.
	AtomosBusy AtomosState = 3

	// AtomosStopping
	// 停止中
	// Atom is stopping.
	AtomosStopping AtomosState = 4
)

func (as AtomosState) String() string {
	switch as {
	case AtomosHalt:
		return "Stopping"
	case AtomosSpawning:
		return "Spawning"
	case AtomosWaiting:
		return "Waiting"
	case AtomosBusy:
		return "Busy"
	case AtomosStopping:
		return "Stopping"
	}
	return "Unknown"
}

// AtomosWormhole
// 虫洞传送的对象
// Object of Wormhole.
type AtomosWormhole interface{}

// BaseAtomos 基础Atomos
// Base Atomos
type BaseAtomos struct {
	// 进程
	process *CosmosProcess
	// 句柄信息
	id *IDInfo

	// 状态
	// State
	state AtomosState

	// Atomos邮箱，也是实现Atom无锁队列的关键。
	// Mailbox, the key of lockless queue of Atom.
	mailbox *mailBox

	// 持有者
	holder AtomosHolder
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

	// 首个同步调用的信息
	// Info of ID Context
	ctx atomosIDContextLocal
}

func NewBaseAtomos(id *IDInfo, lv LogLevel, holder AtomosHolder, inst Atomos, process *CosmosProcess) *BaseAtomos {
	a := &BaseAtomos{
		process:  process,
		id:       id,
		state:    AtomosHalt,
		mailbox:  nil,
		holder:   holder,
		instance: inst,
		log:      atomosLogging{},
		task:     atomosTaskManager{},
		mt:       atomosMessageTracker{},
		it:       &atomosIDTracker{},
		ctx:      atomosIDContextLocal{},
	}
	a.mailbox = newMailBox(id.Info(), a, process.logging.accessLog, process.logging.errorLog)
	initAtomosLog(&a.log, a.id, lv, process.logging)
	initAtomosTasksManager(a.log.logging, &a.task, a)
	initAtomosMessageTracker(&a.mt)
	initAtomosIDTracker(a.it, a)
	initAtomosIDContextLocal(&a.ctx, a)
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

func (a *BaseAtomos) PushMessageMailAndWaitReply(callerID SelfID, name string, async bool, timeout time.Duration, in proto.Message) (reply proto.Message, err *Error) {
	if callerID == nil {
		return nil, NewError(ErrFrameworkIncorrectUsage, "Atomos: SyncMessagingByName without fromID.").AddStack(nil)
	}

	var fromCallChain []string
	if !async {
		fromCallChain = callerID.GetIDContext().FromCallChain()
		err = a.ctx.isLoop(fromCallChain, callerID, false)
		if err != nil {
			return nil, NewErrorf(ErrAtomosIDCallLoop, "Atomos: Loop call detected. target=(%s),chain=(%s)", callerID, fromCallChain).AddStack(nil)
		}
		if callerID.GetIDInfo().Type > IDType_Cosmos {
			fromCallChain = append(fromCallChain, callerID.GetIDInfo().Info())
		}
	}

	am := allocAtomosMail()
	initMessageMail(am, callerID, fromCallChain, name, in)

	if ok := a.mailbox.pushTail(am.mail); !ok {
		return reply, NewErrorf(ErrAtomosIsNotRunning,
			"Atomos: It is not running. callerID=(%s),name=(%s),in=(%v)", callerID, name, in).AddStack(nil)
	}

	replyInterface, err := am.waitReply(a, timeout)
	deallocAtomosMail(am)
	if err != nil && err.Code == ErrAtomosIsNotRunning {
		return nil, err.AddStack(nil)
	}
	reply, ok := replyInterface.(proto.Message)
	if !ok {
		return nil, err.AddStack(nil)
	}
	return reply, err.AddStack(nil)
}

func (a *BaseAtomos) PushAsyncMessageMail(callerID, selfID SelfID, name string, timeout time.Duration, in proto.Message, callback func(proto.Message, *Error)) {
	if callerID == nil {
		if callback != nil {
			callback(nil, NewError(ErrFrameworkIncorrectUsage, "Atomos: AsyncMessagingByName without fromID.").AddStack(nil))
		}
		return
	}

	am := allocAtomosMail()
	initAsyncMessageMail(am, callerID, selfID, name, timeout, callback, in)

	if ok := a.mailbox.pushTail(am.mail); !ok {
		a.log.logging.pushFrameworkErrorLog("Atomos: PushAsyncMessageMail, Atomos: It is not running. name=(%s),args=(%v)", name, in)
	}

	deallocAtomosMail(am)
}

func (a *BaseAtomos) PushAsyncMessageCallbackMailAndWaitReply(callerID SelfID, name string, args proto.Message, err *Error, async func(proto.Message, *Error)) {
	am := allocAtomosMail()
	initAsyncMessageCallbackMail(am, callerID, name, async, args, err)

	if ok := a.mailbox.pushHead(am.mail); !ok {
		a.log.logging.pushFrameworkErrorLog("Atomos: PushAsyncMessageCallbackMailAndWaitReply, Atomos: It is not running. name=(%s),args=(%v)", name, args)
	}

	deallocAtomosMail(am)
}

func (a *BaseAtomos) PushScaleMailAndWaitReply(callerID SelfID, name string, timeout time.Duration, in proto.Message) (ID, *Error) {
	fromCallChain := callerID.GetIDContext().FromCallChain()
	err := a.ctx.isLoop(fromCallChain, callerID, false)
	if err != nil {
		return nil, NewErrorf(ErrAtomosIDCallLoop, "Atomos: Loop call detected. target=(%s),chain=(%s)", callerID, fromCallChain).AddStack(nil)
	}

	if callerID.GetIDInfo().Type > IDType_Cosmos {
		fromCallChain = append(fromCallChain, callerID.GetIDInfo().Info())
	}

	am := allocAtomosMail()
	initScaleMail(am, callerID, fromCallChain, name, in)

	if ok := a.mailbox.pushTail(am.mail); !ok {
		return nil, NewErrorf(ErrAtomosIsNotRunning,
			"Atomos: It is not running. from=(%s),name=(%s),in=(%v)", callerID, name, in).AddStack(nil)
	}
	id, err := am.waitReplyID(a, timeout)

	deallocAtomosMail(am)
	if err != nil && err.Code == ErrAtomosIsNotRunning {
		return nil, err.AddStack(nil)
	}
	return id, err.AddStack(nil)
}

func (a *BaseAtomos) PushKillMailAndWaitReply(callerID SelfID, wait bool, timeout time.Duration) (err *Error) {
	var fromCallChain []string
	if wait {
		fromCallChain = callerID.GetIDContext().FromCallChain()
		err = a.ctx.isLoop(fromCallChain, callerID, false)
		if err != nil {
			return NewErrorf(ErrAtomosIDCallLoop, "Atomos: Loop call detected. target=(%s),chain=(%s)", callerID, fromCallChain).AddStack(nil)
		}
	}

	if callerID.GetIDInfo().Type > IDType_Cosmos {
		fromCallChain = append(fromCallChain, callerID.GetIDInfo().Info())
	}

	am := allocAtomosMail()
	initKillMail(am, callerID, fromCallChain)

	if ok := a.mailbox.pushHead(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos: It is not running. from=(%s),wait=(%v)", callerID, wait).AddStack(nil)
	}
	if wait {
		_, err = am.waitReply(a, timeout)
		return err.AddStack(nil)
	}
	return nil
}

func (a *BaseAtomos) cosmosProcessPushKillMailAndWaitReply(callerID SelfID, timeout time.Duration) (err *Error) {
	am := allocAtomosMail()
	initKillMail(am, callerID, []string{})

	if ok := a.mailbox.pushHead(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos: It is not running. from=(%s)", callerID).AddStack(nil)
	}
	_, err = am.waitReply(a, timeout)
	return err.AddStack(nil)
}

func (a *BaseAtomos) PushWormholeMailAndWaitReply(callerID SelfID, timeout time.Duration, wormhole AtomosWormhole) (err *Error) {
	fromCallChain := callerID.GetIDContext().FromCallChain()
	err = a.ctx.isLoop(fromCallChain, callerID, false)
	if err != nil {
		return NewErrorf(ErrAtomosIDCallLoop, "Atomos: Loop call detected. target=(%s),chain=(%s)", callerID, fromCallChain).AddStack(nil)
	}

	if callerID.GetIDInfo().Type > IDType_Cosmos {
		fromCallChain = append(fromCallChain, callerID.GetIDInfo().Info())
	}

	am := allocAtomosMail()
	initWormholeMail(am, callerID, fromCallChain, wormhole)

	if ok := a.mailbox.pushTail(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos: It is not running. from=(%s),wormhole=(%v)", callerID, wormhole).AddStack(nil)
	}
	_, err = am.waitReply(a, timeout)

	deallocAtomosMail(am)
	return err.AddStack(nil)
}

// State
// 各种状态
// State of Atom

func (a *BaseAtomos) idleTime() time.Duration {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	return a.mt.idleTime()
}

func (a *BaseAtomos) isNotHalt() bool {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	return a.state > AtomosHalt
}

func (a *BaseAtomos) GetState() AtomosState {
	a.mailbox.mutex.Lock()
	state := a.state
	a.mailbox.mutex.Unlock()
	return state
}

func (a *BaseAtomos) IsInState(states ...AtomosState) bool {
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
	a.state = AtomosSpawning
	a.mt.spawning()
	a.process.onIDSpawning(a.id)
}

func (a *BaseAtomos) setSpawningFromChain(fromCallChain []string) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.ctx.context.IdChain = fromCallChain
}

func (a *BaseAtomos) setSpawn() {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosWaiting
	a.mt.spawn()
	a.process.onIDSpawn(a.id)
}

func (a *BaseAtomos) setBusyWithContext(message string, arg proto.Message, fromCallChain []string) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosBusy
	a.ctx.context.IdChain = fromCallChain
	a.mt.set(message, a.id, a.process, arg)
}

func (a *BaseAtomos) setWaitingWithContext(message string) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosWaiting
	a.ctx.context.IdChain = nil
	a.mt.unset(message)
}

func (a *BaseAtomos) setStopping(fromCallChain []string) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosStopping
	a.ctx.context.IdChain = fromCallChain
	a.mt.stopping()
	a.process.onIDStopping(a.id)
}

func (a *BaseAtomos) setHalted(err *Error) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosHalt
	a.ctx.context.IdChain = nil
	a.mt.halted()
	a.process.onIDHalted(a.id, err, a.mt)
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
	am := mail.mail
	if !a.IsInState(AtomosWaiting) {
		a.log.logging.pushFrameworkErrorLog("Atomos: onReceive meets non-waiting status. atomos=(%v),state=(%d),mail=(%v)",
			a, a.GetState(), mail)
	}
	switch am.mailType {
	case MailMessage:
		{
			a.setBusyWithContext(am.name, am.arg, am.fromCallChain)
			defer a.setWaitingWithContext(am.name)

			resp, err := a.holder.OnMessaging(am.from, am.name, am.arg)
			if resp != nil {
				resp = proto.Clone(resp)
			}
			am.sendReply(resp, err)
			// Mail dealloc in AtomCore.pushMessageMail.
		}
	case MailAsyncMessage:
		{
			name := "AsyncMessage-" + am.name
			a.setBusyWithContext(name, nil, nil)
			defer a.setWaitingWithContext(name)

			a.holder.OnAsyncMessaging(am.from, am.name, am.arg, am.asyncMessageCallbackClosure)
		}
	case MailAsyncMessageCallback:
		{
			name := "AsyncMessageCallback-" + am.name
			a.setBusyWithContext(name, nil, nil)
			defer a.setWaitingWithContext(name)

			a.holder.OnAsyncMessagingCallback(am.arg, am.err, am.asyncMessageCallbackClosure)
		}
	case MailTask:
		{
			name := "Task-" + am.name
			a.setBusyWithContext(name, nil, nil)
			defer a.setWaitingWithContext(name)

			a.task.handleTask(am)
			// Mail dealloc in atomosTaskManager.handleTask and cancels.
		}
	case MailWormhole:
		{
			a.setBusyWithContext("AcceptWormhole", nil, am.fromCallChain)
			defer a.setWaitingWithContext("AcceptWormhole")

			err := a.holder.OnWormhole(am.from, am.wormhole)
			am.sendReply(nil, err)
			// Mail dealloc in AtomCore.pushWormholeMail.
		}
	case MailScale:
		{
			name := "Scale-" + am.name
			a.setBusyWithContext(name, nil, am.fromCallChain)
			defer a.setWaitingWithContext(name)

			id, err := a.holder.OnScaling(am.from, am.name, am.arg)
			am.sendReplyID(id, err)
			// Mail dealloc in AtomCore.pushScaleMail.
		}
	default:
		a.log.Fatal("Atomos: Received unknown message type, type=(%v),mail=(%+v)", am.mailType, am)
	}
}

// 处理邮箱退出。
// Handle mailbox stops.
func (a *BaseAtomos) mailboxOnStop(killMail, remainMail *mail, num uint32) (err *Error) {
	defer releaseAtomosMessageTracker(&a.mt)
	defer releaseAtomosTasksManager(&a.task)

	a.task.stopLock()
	defer a.task.stopUnlock()

	state := a.GetState()
	if state == AtomosHalt {
		if a.mailbox.running {
			a.log.logging.pushFrameworkErrorLog("Atomos: onStop meets halted but mailbox running status. atomos=(%v)", a)
		}
		return
	}
	if state != AtomosWaiting {
		a.log.logging.pushFrameworkErrorLog("Atomos: onStop meets non-waiting status. atomos=(%v)", a)
	}

	a.setStopping(killMail.mail.fromCallChain)
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
	for ; remainMail != nil; remainMail = remainMail.next {
		func(remainMail *mail) {
			err := NewErrorf(ErrAtomosIsStopping, "Atomos: Stopping. mail=(%v),mail=(%v),log=(%v)", remainMail, remainMail.mail, remainMail.log).AddStack(nil)
			remainAtomMail := remainMail.mail
			//defer deallocAtomosMail(remainAtomMail)
			switch remainAtomMail.mailType {
			case MailHalt:
				remainAtomMail.sendReply(nil, err)
				// Mail dealloc in AtomCore.pushKillMail.
			case MailMessage:
				remainAtomMail.sendReply(nil, err)
				// Mail dealloc in AtomCore.pushMessageMail.
			case MailAsyncMessage:
				remainAtomMail.asyncReply(nil, err)
			case MailAsyncMessageCallback:
				remainAtomMail.asyncReply(nil, err)
			case MailTask:
				// 正常，因为可能因为断点等原因阻塞，导致在执行关闭atomos的过程中，有任务的计时器到达时间，从而导致此逻辑。
				// Is it needed? It just for preventing new mails receive after cancelAllSchedulingTasks,
				// but it's impossible to add task after locking.
				a.log.Fatal("Atomos: Stopping task mails have been sent after start closing. id=(%s),mail=(%+v)", a.String(), remainMail)
				if err := a.task.cancelTask(remainMail.id, nil); err == nil {
					cancels = append(cancels, remainMail.id)
				}
				// Mail dealloc in atomosTaskManager.cancelTask.
			case MailWormhole:
				remainAtomMail.sendReply(nil, err)
				// Mail dealloc in AtomCore.pushWormholeMail.
			case MailScale:
				remainAtomMail.sendReplyID(nil, err)
				// Mail dealloc in AtomCore.pushScaleMail.
			default:
				a.log.Fatal("Atomos: Stopping unknown message type. type=%v,mail=%+v",
					remainAtomMail.mailType, remainAtomMail)
			}
		}(remainMail)
	}

	// Handle Kill and Reply Kill.
	err = a.holder.OnStopping(killMail.mail.from, cancels)
	if err != nil {
		err = err.AddStack(nil)
	}
	killMail.mail.sendReply(nil, err)

	return err
}
