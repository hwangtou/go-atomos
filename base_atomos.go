package go_atomos

import (
	"container/list"
	"fmt"
	"google.golang.org/protobuf/proto"
	"sync"
)

type AtomosHolder interface {
	atomosHalt(a *baseAtomos)
	atomosRelease(a *baseAtomos)
}

type baseAtomos struct {
	// 句柄信息
	id *IDInfo

	// 状态
	// State
	state AtomosState

	// Cosmos日志邮箱
	// 整个进程共享的，用于日志输出的邮箱。
	logging *loggingMailBox

	// Atomos邮箱，也是实现Atom无锁队列的关键。
	// Mailbox, the key of lockless queue of Atom.
	mailbox *mailBox

	// 容器
	holder AtomosHolder
	// 实际上运行的对象
	instance Atomos

	// 任务管理器，用于处理来自Atom内部的任务调派。
	// Task Manager, uses to handle Task from inner Atom.
	task atomosTasksManager

	// 日志管理器，用于处理来自Atom内部的日志。
	// Logs Manager, uses to handle Log from inner Atom.
	log atomosLogsManager

	refCount int

	// Element中名称列表的元素
	nameElement *list.Element
}

// Atom对象的内存池
// Atom instance pools.
var atomosPool = sync.Pool{
	New: func() interface{} {
		return &baseAtomos{}
	},
}

func allocBaseAtomos() *baseAtomos {
	return atomosPool.Get().(*baseAtomos)
}

func initBaseAtomos(a *baseAtomos, id *IDInfo, log *loggingMailBox, lv LogLevel, holder AtomosHolder, inst Atomos) {
	a.id = id
	a.state = AtomosHalt
	a.logging = log
	initMailBox(a)
	a.holder = holder
	a.instance = inst
	initAtomosLog(&a.log, a, lv)
	initAtomosTasksManager(&a.task, a)
	a.refCount = 1
}

func releaseAtomos(a *baseAtomos) {
	releaseAtomosTask(&a.task)
	releaseAtomosLog(&a.log)
}

func deallocAtomos(a *baseAtomos) {
	atomosPool.Put(a)
}

func (a *baseAtomos) Log() AtomosLogging {
	return &a.log
}

func (a *baseAtomos) Task() AtomosTasking {
	return &a.task
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
		return "Halt"
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

func (a *baseAtomos) getState() AtomosState {
	return a.state
}

func (a *baseAtomos) setBusy() {
	a.state = AtomosBusy
}

func (a *baseAtomos) setWaiting() {
	a.state = AtomosWaiting
}

func (a *baseAtomos) setStopping() {
	a.state = AtomosStopping
}

func (a *baseAtomos) setHalt() {
	a.state = AtomosHalt
}

// Mailbox

// 处理邮箱消息。
// Handle mailbox messages.
func (a *baseAtomos) onReceive(mail *mail) {
	am := mail.Content.(*atomosMail)
	switch am.mailType {
	case AtomosMailMessage:
		resp, err := a.instance.handleMessage(am.from, am.name, am.arg)
		if resp != nil {
			resp = proto.Clone(resp)
		}
		am.sendReply(resp, err)
		// Mail dealloc in AtomCore.pushMessageMail.
	case AtomosMailTask:
		a.task.handleTask(am)
		// Mail dealloc in atomosTasksManager.handleTask and cancels.
	case AtomosMailReload:
		err := a.instance.handleReload(am)
		am.sendReply(nil, err)
		// Mail dealloc in AtomCore.pushReloadMail.
	case AtomosMailWormhole:
		err := a.instance.handleWormhole(am.wormholeAction, am.wormhole)
		am.sendReply(nil, err)
		// Mail dealloc in AtomCore.pushWormholeMail.
	default:
		a.log.Fatal("Atomos: Received unknown message type, type=(%v),mail=(%+v)", am.mailType, am)
	}
}

// 处理邮箱消息时发生的异常。
// Handle mailbox panic while it is processing Mail.
func (a *baseAtomos) onPanic(mail *mail, trace string) {
	am := mail.Content.(*atomosMail)
	// Try to reply here, to prevent mail non-reply, and stub.
	err := NewErrorWithStack(ErrAtomosPanic, fmt.Sprintf("PANIC, mail=(%+v)", am), trace)
	switch am.mailType {
	case AtomosMailMessage:
		am.sendReply(nil, err)
		// Mail then will be dealloc in AtomCore.pushMessageMail.
	case AtomosMailHalt:
		am.sendReply(nil, err)
		// Mail then will be dealloc in AtomCore.pushKillMail.
	case AtomosMailReload:
		am.sendReply(nil, err)
		// Mail then will be dealloc in AtomCore.pushReloadMail.
	case AtomosMailWormhole:
		am.sendReply(nil, err)
		// Mail then will be dealloc in AtomCore.pushWormholeMail.
	case AtomosMailTask:
		a.log.Error("Atomos: PANIC when atomos is running task, id=(%s),type=(%v),mail=(%+v)", a.id.str(), am.mailType, am)
	default:
		a.log.Fatal("Atomos: PANIC unknown message type, id=(%s),type=(%v),mail=(%+v)", a.id.str(), am.mailType, am)
	}
}

// 处理邮箱退出。
// Handle mailbox stops.
func (a *baseAtomos) onStop(killMail, remainMails *mail, num uint32) {
	a.task.stopLock()
	defer a.task.stopUnlock()

	a.setStopping()
	defer a.holder.atomosHalt(a)
	defer a.setHalt()

	killAtomMail := killMail.Content.(*atomosMail)
	cancels := a.task.cancelAllSchedulingTasks()
	for ; remainMails != nil; remainMails = remainMails.next {
		err := NewError(ErrAtomosIsNotRunning, fmt.Sprintf("Atomos is stopping, mail=(%+v)", remainMails))
		remainAtomMail := remainMails.Content.(*atomosMail)
		switch remainAtomMail.mailType {
		case AtomosMailHalt:
			remainAtomMail.sendReply(nil, err)
			// Mail dealloc in AtomCore.pushKillMail.
		case AtomosMailMessage:
			remainAtomMail.sendReply(nil, err)
			// Mail dealloc in AtomCore.pushMessageMail.
		case AtomosMailTask:
			// 正常，因为可能因为断点等原因阻塞，导致在执行关闭atomos的过程中，有任务的计时器到达时间，从而导致此逻辑。
			// Is it needed? It just for preventing new mails receive after cancelAllSchedulingTasks,
			// but it's impossible to add task after locking.
			a.log.Fatal("Atomos: STOPPING task mails have been sent after start closing, id=(%s),mail=(%+v)", a.id.str(), remainMails)
			t, err := a.task.cancelTask(remainMails.id, nil)
			if err == nil {
				cancels[remainMails.id] = t
			}
			// Mail dealloc in atomosTasksManager.cancelTask.
		case AtomosMailReload:
			remainAtomMail.sendReply(nil, err)
			// Mail dealloc in AtomCore.pushReloadMail.
		case AtomosMailWormhole:
			remainAtomMail.sendReply(nil, err)
		// Mail dealloc in AtomCore.pushWormholeMail.
		default:
			a.log.Fatal("Atom.Mail: Stopped, unknown message type, type=%v,mail=%+v",
				remainAtomMail.mailType, remainAtomMail)
		}
	}

	// Handle Kill and Reply Kill.
	err := a.instance.handleKill(killAtomMail, cancels)
	killAtomMail.sendReply(nil, err)
}
