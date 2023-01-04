package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"time"
)

type AtomosHolder interface {
	// OnMessaging
	// 收到消息
	OnMessaging(from ID, name string, args proto.Message) (reply proto.Message, err *Error)

	// OnScaling
	// 负载均衡决策
	OnScaling(from ID, name string, args proto.Message) (id ID, err *Error)

	// OnWormhole
	// 收到Wormhole
	OnWormhole(from ID, wormhole AtomosWormhole) *Error

	// OnStopping
	// 停止中
	OnStopping(from ID, cancelled map[uint64]CancelledTask) *Error

	// Spawn & Set & Unset & Stopping & Halt
	Spawn()
	Set(message string)
	Unset(message string)
	Stopping()
	Halted()
}

type AtomosAcceptWormhole interface {
	AcceptWormhole(from ID, wormhole AtomosWormhole) *Error
}

type AtomosRelease interface {
	Release(tracker *IDTracker)
}

type BaseAtomos struct {
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

	// 任务管理器，用于处理来自Atom内部的任务调派。
	// Task Manager, uses to handle Task from inner Atom.
	task atomosTaskManager

	// 日志管理器，用于处理来自Atom内部的日志。
	// Logs Manager, uses to handle Log from inner Atom.
	log atomosLoggingManager
}

func NewBaseAtomos(id *IDInfo, lv LogLevel, holder AtomosHolder, inst Atomos) *BaseAtomos {
	a := &BaseAtomos{
		id:       id,
		state:    AtomosHalt,
		mailbox:  nil,
		holder:   holder,
		instance: inst,
		task:     atomosTaskManager{},
		log:      atomosLoggingManager{},
	}
	a.mailbox = newMailBox(id.Info(), MailBoxHandler{
		OnReceive: a.onReceive,
		OnStop:    a.onStop,
	})
	initAtomosLog(&a.log, a, lv)
	initAtomosTasksManager(&a.task, a)
	a.mailbox.start()
	a.state = AtomosWaiting
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

func (a *BaseAtomos) PushMessageMailAndWaitReply(from ID, name string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	am := allocAtomosMail()
	initMessageMail(am, from, name, args)

	if ok := a.mailbox.pushTail(am.mail); !ok {
		return reply, NewErrorf(ErrAtomosIsNotRunning,
			"Atomos is not running. from=(%s),name=(%s),args=(%v)", from, name, args).AddStack(nil)
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

func (a *BaseAtomos) PushScaleMailAndWaitReply(from ID, message string, timeout time.Duration, args proto.Message) (ID, *Error) {
	am := allocAtomosMail()
	initScaleMail(am, from, message, args)

	if ok := a.mailbox.pushTail(am.mail); !ok {
		return nil, NewErrorf(ErrAtomosIsNotRunning,
			"Atomos is not running. from=(%s),message=(%s),args=(%v)", from, message, args).AddStack(nil)
	}
	id, err := am.waitReplyID(a, timeout)

	deallocAtomosMail(am)
	if err != nil && err.Code == ErrAtomosIsNotRunning {
		return nil, err.AddStack(nil)
	}
	return id, err.AddStack(nil)
}

func (a *BaseAtomos) PushKillMailAndWaitReply(from ID, wait, executeStop bool, timeout time.Duration) (err *Error) {
	am := allocAtomosMail()
	initKillMail(am, from, executeStop)

	if ok := a.mailbox.pushHead(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos is not running. from=(%s),wait=(%v)", from, wait)
	}
	if wait {
		_, err = am.waitReply(a, timeout)
		return err.AddStack(nil)
	}
	return nil
}

func (a *BaseAtomos) PushWormholeMailAndWaitReply(from ID, timeout time.Duration, wormhole AtomosWormhole) (err *Error) {
	am := allocAtomosMail()
	initWormholeMail(am, from, wormhole)

	if ok := a.mailbox.pushTail(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos is not running. from=(%s),wormhole=(%v)", from, wormhole)
	}
	_, err = am.waitReply(a, timeout)

	deallocAtomosMail(am)
	return err.AddStack(nil)
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

// State
// 各种状态
// State of Atom

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
}

func (a *BaseAtomos) setSpawn() {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosWaiting
	a.holder.Spawn()
}

func (a *BaseAtomos) setBusy(message string) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosBusy
	a.holder.Set(message)
}

func (a *BaseAtomos) setWaiting(message string) {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosWaiting
	a.holder.Unset(message)
}

func (a *BaseAtomos) setStopping() {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosStopping
	a.holder.Stopping()
}

func (a *BaseAtomos) setHalt() {
	a.mailbox.mutex.Lock()
	defer a.mailbox.mutex.Unlock()
	a.state = AtomosHalt
	a.holder.Halted()
}

// Mailbox

// 处理邮箱消息。
// Handle mailbox messages.
func (a *BaseAtomos) onReceive(mail *mail) {
	am := mail.mail
	if !a.IsInState(AtomosWaiting) {
		SharedLogging().pushFrameworkErrorLog("Atomos: onReceive meets non-waiting status. atomos=(%v),mail=(%v)",
			a, mail)
	}
	switch am.mailType {
	case MailMessage:
		{
			a.setBusy(am.name)
			defer a.setWaiting(am.name)

			resp, err := a.holder.OnMessaging(am.from, am.name, am.arg)
			if resp != nil {
				resp = proto.Clone(resp)
			}
			am.sendReply(resp, err)
			// Mail dealloc in AtomCore.pushMessageMail.
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

			err := a.holder.OnWormhole(am.from, am.wormhole)
			am.sendReply(nil, err)
			// Mail dealloc in AtomCore.pushWormholeMail.
		}
	case MailScale:
		{
			name := "Scale-" + am.name
			a.setBusy(name)
			defer a.setWaiting(name)

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
func (a *BaseAtomos) onStop(killMail, remainMail *mail, num uint32) {
	a.task.stopLock()
	defer a.task.stopUnlock()

	state := a.GetState()
	if state == AtomosHalt {
		if a.mailbox.running {
			SharedLogging().pushFrameworkErrorLog("Atomos: onStop meets halted but mailbox running status. atomos=(%v)", a)
		}
		return
	}
	if state != AtomosWaiting {
		SharedLogging().pushFrameworkErrorLog("Atomos: onStop meets non-waiting status. atomos=(%v)", a)
	}

	a.setStopping()
	defer a.setHalt()

	defer func() {
		if r := recover(); r != nil {
			err := NewErrorf(ErrFrameworkPanic, "Atomos: Stopping recovers from panic.").AddPanicStack(nil, 2, r)
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

	//defer deallocAtomosMail(killAtomMail)
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
				t, err := a.task.cancelTask(remainMail.id, nil)
				if err == nil {
					cancels[remainMail.id] = t
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
	err := a.holder.OnStopping(killMail.mail.from, cancels)
	killMail.mail.sendReply(nil, err)
}
