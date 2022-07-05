package go_atomos

type baseAtomos struct {
	cosmosLogMailbox *mailBox

	// 邮箱，也是实现Atom无锁队列的关键。
	// Mailbox, the key of lockless queue of Atom.
	mailbox *mailBox

	// 状态
	// State
	state    AtomState
	instance Atomos

	id *IDInfo

	// 任务管理器，用于处理来自Atom内部的任务调派。
	// Task Manager, uses to handle Task from inner Atom.
	task atomTasksManager

	// 日志管理器，用于处理来自Atom内部的日志。
	// Logs Manager, uses to handle Log from inner Atom.
	log atomosLogsManager
}

func initBaseAtomos(a *baseAtomos, cl *mailBox, lv LogLevel) {
	a.cosmosLogMailbox = cl
	//a.mailbox =
	initAtomosLog(&a.log, a, lv)
	initAtomTasksManager(&a.task, a)
}

func releaseAtomos(a *baseAtomos) {
	releaseAtomTask(&a.task)
	releaseAtomosLog(&a.log)
}

func deallocAtomos(a *baseAtomos) {
	atomsPool.Put(a)
}

func (a *baseAtomos) Log() *atomosLogsManager {
	return &a.log
}

func (a *baseAtomos) Task() TaskManager {
	return &a.task
}

// Atom状态
// AtomState

type AtomState int

const (
	// AtomHalt
	// 停止
	// Atom is stopped.
	AtomHalt AtomState = 0

	// AtomSpawning
	// 启动中
	// Atom is starting up.
	AtomSpawning AtomState = 1

	// AtomWaiting
	// 启动成功，等待消息
	// Atom is started and waiting for message.
	AtomWaiting AtomState = 2

	// AtomBusy
	// 启动成功，正在处理消息
	// Atom is started and busy processing message.
	AtomBusy AtomState = 3

	// AtomStopping
	// 停止中
	// Atom is stopping.
	AtomStopping AtomState = 4
)

func (as AtomState) String() string {
	switch as {
	case AtomHalt:
		return "Halt"
	case AtomSpawning:
		return "Spawning"
	case AtomWaiting:
		return "Waiting"
	case AtomBusy:
		return "Busy"
	case AtomStopping:
		return "Stopping"
	}
	return "Unknown"
}

// State
// 各种状态
// State of Atom

func (a *baseAtomos) getState() AtomState {
	return a.state
}

func (a *baseAtomos) setBusy() {
	a.state = AtomBusy
}

func (a *baseAtomos) setWaiting() {
	a.state = AtomWaiting
}

func (a *baseAtomos) setStopping() {
	a.state = AtomStopping
}

func (a *baseAtomos) setHalt() {
	a.state = AtomHalt
}
