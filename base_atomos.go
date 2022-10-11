package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"sync"
	"time"
)

type AtomosHolder interface {
	// OnMessaging
	// 收到消息
	OnMessaging(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo)

	// OnScaling
	// 负载均衡决策
	OnScaling(from ID, name string, args proto.Message) (id ID, err *ErrorInfo)

	// OnReloading
	// 通知新的Holder正在更新中
	OnReloading(oldInstance Atomos, reloadObject AtomosReloadable) (newInstance Atomos)

	// OnWormhole
	// 收到Wormhole
	OnWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo

	// OnStopping
	// 停止中
	OnStopping(from ID, cancelled map[uint64]CancelledTask) *ErrorInfo
}

type AtomosAcceptWormhole interface {
	AcceptWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo
}

type AtomosReloadable interface{}

type BaseAtomos struct {
	// 句柄信息
	id *IDInfo

	// 状态
	// State
	state AtomosState

	// Cosmos日志邮箱
	// 整个进程共享的，用于日志输出的邮箱。
	logging *LoggingAtomos

	// Atomos邮箱，也是实现Atom无锁队列的关键。
	// Mailbox, the key of lockless queue of Atom.
	mailbox *mailBox

	// 持有者
	holder AtomosHolder
	// 实际上运行的对象
	instance Atomos
	// 升级次数版本
	reloads int

	// 任务管理器，用于处理来自Atom内部的任务调派。
	// Task Manager, uses to handle Task from inner Atom.
	task atomosTaskManager

	// 日志管理器，用于处理来自Atom内部的日志。
	// Logs Manager, uses to handle Log from inner Atom.
	log atomosLoggingManager

	//refCount int

	//// Element中名称列表的元素
	//nameElement *list.Element

	lastBusy time.Time
	lastWait time.Time
	lastStop time.Time
}

// Atom对象的内存池
// Atom instance pools.
var atomosPool = sync.Pool{
	New: func() interface{} {
		return &BaseAtomos{}
	},
}

func NewBaseAtomos(id *IDInfo, log *LoggingAtomos, lv LogLevel, holder AtomosHolder, inst Atomos, reloads int) *BaseAtomos {
	a := atomosPool.Get().(*BaseAtomos)
	a.id = id
	a.state = AtomosHalt
	a.logging = log
	newMailBoxWithHandler(a)
	a.mailbox.start()
	a.holder = holder
	a.instance = inst
	a.reloads = reloads
	initAtomosLog(&a.log, log, a, lv)
	initAtomosTasksManager(&a.task, a)
	//a.refCount = 1
	a.state = AtomosWaiting
	return a
}

func (a *BaseAtomos) DeleteAtomos(wait bool) {
	if wait {
		a.mailbox.waitStop()
	} else {
		a.mailbox.stop()
	}
	releaseAtomosTask(&a.task)
	releaseAtomosLog(&a.log)
	atomosPool.Put(a)
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

//func (a *BaseAtomos) ReloadInstance(newInstance Atomos) (oldInstance Atomos) {
//	oldInstance = a.instance
//	a.instance = newInstance
//	return oldInstance
//}

func (a *BaseAtomos) Description() string {
	return a.instance.Description()
}

func (a *BaseAtomos) Log() Logging {
	return &a.log
}

func (a *BaseAtomos) Task() Task {
	return &a.task
}

func (a *BaseAtomos) PushMessageMailAndWaitReply(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	am := allocAtomosMail()
	initMessageMail(am, from, name, args)
	if ok := a.mailbox.pushTail(am.mail); !ok {
		return reply, NewErrorf(ErrAtomosIsNotRunning,
			"Atomos is not running, from=(%s),name=(%s),args=(%v)", from, name, args)
	}
	replyInterface, err := am.waitReply()
	deallocAtomosMail(am)
	if err != nil && err.Code == ErrAtomosIsNotRunning {
		return nil, err
	}
	reply, ok := replyInterface.(proto.Message)
	if !ok {
		//return reply, fmt.Errorf("Atom.Mail: Reply type error, name=%s,args=%+v,reply=%+v",
		//	name, args, replyInterface)
		return nil, err
	}
	return reply, err
}

func (a *BaseAtomos) PushScaleMailAndWaitReply(from ID, message string, args proto.Message) (ID, *ErrorInfo) {
	am := allocAtomosMail()
	initScaleMail(am, from, message, args)
	if ok := a.mailbox.pushTail(am.mail); !ok {
		return nil, NewErrorf(ErrAtomosIsNotRunning,
			"Atomos is not running, from=(%s),message=(%s),args=(%v)", from, message, args)
	}
	id, err := am.waitReplyID()
	deallocAtomosMail(am)
	if err != nil && err.Code == ErrAtomosIsNotRunning {
		return nil, err
	}
	return id, err
}

func (a *BaseAtomos) PushKillMailAndWaitReply(from ID, wait bool) (err *ErrorInfo) {
	am := allocAtomosMail()
	initKillMail(am, from)
	if ok := a.mailbox.pushHead(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos is not running, from=(%s),wait=(%v)", from, wait)
	}
	if wait {
		_, err := am.waitReply()
		//deallocAtomosMail(am)
		return err
	}
	//deallocAtomosMail(am)
	return nil
}

func (a *BaseAtomos) PushReloadMailAndWaitReply(from ID, reload AtomosReloadable, reloads int) (err *ErrorInfo) {
	am := allocAtomosMail()
	initReloadMail(am, reload, reloads)
	if ok := a.mailbox.pushHead(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos is not running, from=(%s),reloads=(%d)", from, reloads)
	}
	_, err = am.waitReply()
	deallocAtomosMail(am)
	return err
}

func (a *BaseAtomos) PushWormholeMailAndWaitReply(from ID, wormhole AtomosWormhole) (err *ErrorInfo) {
	am := allocAtomosMail()
	initWormholeMail(am, from, wormhole)
	if ok := a.mailbox.pushTail(am.mail); !ok {
		return NewErrorf(ErrAtomosIsNotRunning, "Atomos is not running, from=(%s),wormhole=(%v)", from, wormhole)
	}
	_, err = am.waitReply()
	deallocAtomosMail(am)
	return err
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

func (a *BaseAtomos) isNotHalt() bool {
	return a.state > AtomosHalt
}

func (a *BaseAtomos) isSpawnIdleAtomos() bool {
	switch a.state {
	case AtomosHalt:
		return false
	case AtomosSpawning:
		return true
	case AtomosWaiting:
		now := time.Now()
		a.lastBusy, a.lastWait = now, now
		return true
	case AtomosBusy:
		return true
	case AtomosStopping:
		fallthrough
	default:
		return false
	}
}

func (a *BaseAtomos) GetState() AtomosState {
	return a.state
}
func (a *BaseAtomos) setSpawning() {
	a.state = AtomosSpawning
	a.lastBusy = time.Now()
	a.lastWait = time.Now()
	a.lastStop = time.Time{}
}

func (a *BaseAtomos) setBusy() {
	a.state = AtomosBusy
	a.lastWait = time.Now()
}

func (a *BaseAtomos) setWaiting() {
	a.state = AtomosWaiting
	a.lastBusy = time.Now()
}

func (a *BaseAtomos) setStopping() {
	a.state = AtomosStopping
	//a.lastBusy = time.Now()
}

func (a *BaseAtomos) setHalt() {
	a.state = AtomosHalt
	a.lastStop = time.Now()
}

// Mailbox

// 处理邮箱消息。
// Handle mailbox messages.
func (a *BaseAtomos) onReceive(mail *mail) {
	am := mail.Content.(*atomosMail)
	// TODO: Debug Only.
	if a.state != AtomosWaiting {
		// TODO: 检查这种状态
		//panic("")
	}
	a.setBusy()
	defer a.setWaiting()
	switch am.mailType {
	case MailMessage:
		resp, err := a.holder.OnMessaging(am.from, am.name, am.arg)
		if resp != nil {
			resp = proto.Clone(resp)
		}
		am.sendReply(resp, err)
		// Mail dealloc in AtomCore.pushMessageMail.
	case MailTask:
		a.task.handleTask(am)
		// Mail dealloc in atomosTaskManager.handleTask and cancels.
	case MailReload:
		if am.reloads == a.reloads {
			break
		}
		a.reloads = am.reloads
		a.instance = a.holder.OnReloading(a.instance, am.reload)
		am.sendReply(nil, nil)
		// Mail dealloc in AtomCore.pushReloadMail.
	case MailWormhole:
		err := a.holder.OnWormhole(am.from, am.wormhole)
		am.sendReply(nil, err)
		// Mail dealloc in AtomCore.pushWormholeMail.
	case MailScale:
		id, err := a.holder.OnScaling(am.from, am.name, am.arg)
		am.sendReplyID(id, err)
		// Mail dealloc in AtomCore.pushScaleMail.
	default:
		a.log.Fatal("Atomos: Received unknown message type, type=(%v),mail=(%+v)", am.mailType, am)
	}
	//deallocAtomosMail(am)
}

// 处理邮箱消息时发生的异常。
// Handle mailbox panic while it is processing Mail.
func (a *BaseAtomos) onPanic(mail *mail, err *ErrorInfo) {
	am := mail.Content.(*atomosMail)
	//defer deallocAtomosMail(am)

	if !a.isNotHalt() {
		return
	}
	a.setBusy()
	defer a.setWaiting()
	// Try to reply here, to prevent mail non-reply, and stub.
	//err := NewErrorfWithStack(ErrAtomosPanic, trace, "PANIC, mail=(%+v)", am)
	switch am.mailType {
	case MailMessage:
		am.sendReply(nil, err)
		// Mail then will be dealloc in AtomCore.pushMessageMail.
	case MailHalt:
		am.sendReply(nil, err)
		// Mail then will be dealloc in AtomCore.pushKillMail.
	case MailReload:
		am.sendReply(nil, err)
		// Mail then will be dealloc in AtomCore.pushReloadMail.
	case MailWormhole:
		am.sendReply(nil, err)
		// Mail then will be dealloc in AtomCore.pushWormholeMail.
	case MailTask:
		a.log.Error("Atomos: PANIC when atomos is running task, id=(%s),type=(%v),mail=(%+v)", a.id.Info(), am.mailType, am)
	default:
		a.log.Fatal("Atomos: PANIC unknown message type, id=(%s),type=(%v),mail=(%+v)", a.id.Info(), am.mailType, am)
	}
}

// 处理邮箱退出。
// Handle mailbox stops.
func (a *BaseAtomos) onStop(killMail, remainMail *mail, num uint32) {
	a.task.stopLock()
	defer a.task.stopUnlock()

	if !a.isNotHalt() {
		return
	}

	a.setStopping()
	defer a.setHalt()

	killAtomMail, ok := killMail.Content.(*atomosMail)
	//defer deallocAtomosMail(killAtomMail)
	cancels := a.task.cancelAllSchedulingTasks()
	for ; remainMail != nil; remainMail = remainMail.next {
		func(remainMail *mail) {
			err := NewErrorf(ErrAtomosIsStopping, "Atomos is stopping, mail=(%+v)", remainMail)
			remainAtomMail := remainMail.Content.(*atomosMail)
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
				a.log.Fatal("Atomos: STOPPING task mails have been sent after start closing, id=(%s),mail=(%+v)", a.String(), remainMail)
				t, err := a.task.cancelTask(remainMail.id, nil)
				if err == nil {
					cancels[remainMail.id] = t
				}
				// Mail dealloc in atomosTaskManager.cancelTask.
			case MailReload:
				remainAtomMail.sendReply(nil, err)
				// Mail dealloc in AtomCore.pushReloadMail.
			case MailWormhole:
				remainAtomMail.sendReply(nil, err)
				// Mail dealloc in AtomCore.pushWormholeMail.
			default:
				a.log.Fatal("Atom.Mail: Stopped, unknown message type, type=%v,mail=%+v",
					remainAtomMail.mailType, remainAtomMail)
			}
		}(remainMail)
	}

	// Handle Kill and Reply Kill.
	if ok {
		err := a.holder.OnStopping(killAtomMail.from, cancels)
		killAtomMail.sendReply(nil, err)
	}
}
