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
	OnMessaging(fromID ID, firstSyncCall, name string, in proto.Message) (out proto.Message, err *Error)
	// OnAsyncMessagingCallback
	// 收到异步消息回调
	OnAsyncMessagingCallback(firstSyncCall string, in proto.Message, err *Error, callback func(reply proto.Message, err *Error))

	// OnScaling
	// 负载均衡决策
	OnScaling(from ID, firstSyncCall, name string, args proto.Message) (id ID, err *Error)

	// OnWormhole
	// 收到Wormhole
	OnWormhole(from ID, firstSyncCall string, wormhole AtomosWormhole) *Error

	// OnStopping
	// 停止中
	OnStopping(from ID, firstSyncCall string, cancelled []uint64) *Error

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
	// Info of first sync call
	fsc idFirstSyncCallLocal
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
		fsc:      idFirstSyncCallLocal{},
	}
	a.mailbox = newMailBox(id.Info(), a, process.logging.accessLog, process.logging.errorLog)
	initAtomosLog(&a.log, a.id, lv, process.logging)
	initAtomosTasksManager(a.log.logging, &a.task, a)
	initAtomosMessageTracker(&a.mt)
	initAtomosIDTracker(a.it, a)
	initAtomosFirstSyncCall(&a.fsc, id)
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

func (a *BaseAtomos) PushMessageMailAndWaitReply(from ID, firstSyncCall, name string, timeout time.Duration, in proto.Message) (reply proto.Message, err *Error) {
	am := allocAtomosMail()
	initMessageMail(am, from, firstSyncCall, name, in)

	if ok := a.mailbox.pushTail(am.mail); !ok {
		return reply, NewErrorf(ErrAtomosIsNotRunning,
			"Atomos is not running. from=(%s),name=(%s),in=(%v)", from, name, in).AddStack(nil)
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

func (a *BaseAtomos) PushAsyncMessageCallbackMailAndWaitReply(name, firstSyncCall string, args proto.Message, err *Error, async func(proto.Message, *Error)) {
	am := allocAtomosMail()
	initAsyncMessageCallbackMail(am, firstSyncCall, name, async, args, err)

	if ok := a.mailbox.pushHead(am.mail); !ok {
		// TODO
	}

	deallocAtomosMail(am)
}

func (a *BaseAtomos) PushScaleMailAndWaitReply(from ID, firstSyncCall, name string, timeout time.Duration, in proto.Message) (ID, *Error) {
	am := allocAtomosMail()
	initScaleMail(am, from, firstSyncCall, name, in)

	if ok := a.mailbox.pushTail(am.mail); !ok {
		return nil, NewErrorf(ErrAtomosIsNotRunning,
			"Atomos is not running. from=(%s),name=(%s),in=(%v)", from, name, in).AddStack(nil)
	}
	id, err := am.waitReplyID(a, timeout)

	deallocAtomosMail(am)
	if err != nil && err.Code == ErrAtomosIsNotRunning {
		return nil, err.AddStack(nil)
	}
	return id, err.AddStack(nil)
}

func (a *BaseAtomos) PushKillMailAndWaitReply(from SelfID, wait bool, timeout time.Duration) (err *Error) {
	var firstSyncCall string
	if !wait {
		firstSyncCall = a.fsc.nextFirstSyncCall()
	}

	am := allocAtomosMail()
	initKillMail(am, from, firstSyncCall)

	if ok := a.mailbox.pushHead(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos is not running. from=(%s),wait=(%v)", from, wait)
	}
	if wait {
		_, err = am.waitReply(a, timeout)
		return err.AddStack(nil)
	}
	return nil
}

func (a *BaseAtomos) PushWormholeMailAndWaitReply(from SelfID, timeout time.Duration, wormhole AtomosWormhole) (err *Error) {
	firstSyncCall, toDefer, err := a.syncGetFirstSyncCallName(from)
	if err != nil {
		return err.AddStack(nil)
	}
	if toDefer {
		defer from.unsetSyncMessageAndFirstCall()
	}

	am := allocAtomosMail()
	initWormholeMail(am, from, firstSyncCall, wormhole)

	if ok := a.mailbox.pushTail(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos is not running. from=(%s),wormhole=(%v)", from, wormhole)
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

func (a *BaseAtomos) setSpawn() {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosWaiting
	a.mt.spawn()
	a.process.onIDSpawn(a.id)
}

func (a *BaseAtomos) setBusy(message string) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosBusy
	a.mt.set(message, a.id, a.process)
}

func (a *BaseAtomos) setWaiting(message string) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosWaiting
	a.mt.unset(message)
}

func (a *BaseAtomos) setStopping() {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosStopping
	a.mt.stopping()
	a.process.onIDStopping(a.id)
}

func (a *BaseAtomos) setHalted(err *Error) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosHalt
	a.mt.halted()
	a.process.onIDHalted(a.id, err, a.mt)
}

// IDTracker

func (a *BaseAtomos) onIDReleased() {
	a.holder.OnIDsReleased()
}

// FirstSyncCall

func (a *BaseAtomos) syncGetFirstSyncCallName(callerID SelfID) (string, bool, *Error) {
	firstSyncCall := ""
	// 获取调用ID的Go ID
	callerLocalGoID := callerID.getGoID()

	// 远程调用
	// 如果调用ID的Go ID为0，证明远程调用，直接返回当前的FirstSyncCall即可。
	if callerLocalGoID == 0 {
		if firstSyncCall = callerID.getCurFirstSyncCall(); firstSyncCall == "" {
			return "", false, NewErrorf(ErrFrameworkInternalError, "IDFirstSyncCall: callerID is invalid. callerID=(%v)", callerID)
		}
		if a.fsc.curFirstSyncCall == firstSyncCall {
			return "", false, NewErrorf(ErrIDFirstSyncCallDeadlock, "IDFirstSyncCall: Call chain deadlock. callerID=(%v)", callerID)
		}
		//// TODO: 因为远程调用的Async循环调用会捕捉不到这种死锁，所以这里需要检查一下。后续还是要反思下这里。
		//if strings.Split(firstSyncCall, "-")[0] == a.id.Info() {
		//	return "", false, NewErrorf(ErrIDFirstSyncCallDeadlock, "IDFirstSyncCall: Call to self deadlock. callerID=(%v)", callerID)
		//}
		return firstSyncCall, false, nil
	}

	// 本地调用
	// 获取调用栈的Go ID
	curLocalGoID := getGoID()
	var toDefer bool
	// 这种情况，调用方的ID和当前的ID是同一个，证明是同步调用。
	if callerLocalGoID == curLocalGoID {
		// 此时需要检查调用方是否有curFirstSyncCall，如果为空，证明是第一个同步调用（如Task中调用的），所以需要创建一个curFirstSyncCall。
		// 因为是同一个Atom，所以直接设置到当前的ID即可。
		if callerFirst := callerID.getCurFirstSyncCall(); callerFirst == "" {
			// 要从调用者开始算起，所以要从调用者的ID中获取。
			firstSyncCall = callerID.nextFirstSyncCall()
			if err := callerID.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
				return "", false, err.AddStack(nil)
			}
			//defer callerID.unsetSyncMessageAndFirstCall()
			toDefer = true
		} else {
			// 如果不为空，则检查是否和push向的ID的当前curFirstSyncCall一样，
			if eFirst := a.fsc.getCurFirstSyncCall(); callerFirst == eFirst {
				// 如果一样，则是循环调用死锁，返回错误。
				return "", false, NewErrorf(ErrIDFirstSyncCallDeadlock, "IDFirstSyncCall: Sync call is dead lock. callerID=(%v),firstSyncCall=(%s)", callerID, callerFirst).AddStack(nil)
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
		firstSyncCall = a.fsc.nextFirstSyncCall()
	}
	return firstSyncCall, toDefer, nil
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
			a.setBusy(am.name)
			defer a.setWaiting(am.name)

			resp, err := a.holder.OnMessaging(am.from, am.firstSyncCall, am.name, am.arg)
			if resp != nil {
				resp = proto.Clone(resp)
			}
			am.sendReply(resp, err)
			// Mail dealloc in AtomCore.pushMessageMail.
		}
	case MailAsyncMessageCallback:
		{
			name := "AsyncMessageCallback-" + am.name
			a.setBusy(name)
			defer a.setWaiting(name)

			a.holder.OnAsyncMessagingCallback(am.firstSyncCall, am.arg, am.err, am.asyncMessageCallbackClosure)
		}
	case MailTask:
		{
			name := "Task-" + am.name
			a.setBusy(name)
			defer a.setWaiting(name)

			a.task.handleTask(am)
			// Mail dealloc in atomosTaskManager.handleTask and cancels.
		}
	case MailWormhole:
		{
			a.setBusy("AcceptWormhole")
			defer a.setWaiting("AcceptWormhole")

			err := a.holder.OnWormhole(am.from, am.firstSyncCall, am.wormhole)
			am.sendReply(nil, err)
			// Mail dealloc in AtomCore.pushWormholeMail.
		}
	case MailScale:
		{
			name := "Scale-" + am.name
			a.setBusy(name)
			defer a.setWaiting(name)

			id, err := a.holder.OnScaling(am.from, am.firstSyncCall, am.name, am.arg)
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

	a.setStopping()
	defer a.setHalted(err)

	defer func() {
		if r := recover(); r != nil {
			err = NewErrorf(ErrFrameworkRecoverFromPanic, "Atomos: Stopping recovers from panic.").AddPanicStack(nil, 2, r)
			if ar, ok := a.instance.(AtomosRecover); ok {
				defer func() {
					recover()
					a.Log().Fatal("Atomos: Stopping recovers from panic. err=(%v)", err)
				}()
				ar.StopRecover(err)
			} else {
				a.Log().Fatal("Atomos: Stopping recovers from panic. err=(%v)", err)
			}
		}
	}()

	cancels := a.task.cancelAllSchedulingTasks()
	for ; remainMail != nil; remainMail = remainMail.next {
		func(remainMail *mail) {
			err := NewErrorf(ErrAtomosIsStopping, "Atomos: Stopping. mail=(%+v)", remainMail).AddStack(nil)
			remainAtomMail := remainMail.mail
			//defer deallocAtomosMail(remainAtomMail)
			switch remainAtomMail.mailType {
			case MailHalt:
				remainAtomMail.sendReply(nil, err)
				// Mail dealloc in AtomCore.pushKillMail.
			case MailMessage:
				remainAtomMail.sendReply(nil, err)
				// Mail dealloc in AtomCore.pushMessageMail.
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
			default:
				a.log.Fatal("Atomos: Stopping unknown message type. type=%v,mail=%+v",
					remainAtomMail.mailType, remainAtomMail)
			}
		}(remainMail)
	}

	// Handle Kill and Reply Kill.
	err = a.holder.OnStopping(killMail.mail.from, killMail.mail.firstSyncCall, cancels)
	if err != nil {
		err = err.AddStack(nil)
	}
	killMail.mail.sendReply(nil, err)

	return err
}
