package atomos

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
)

//
// Atomos Task
//

type TaskFn func(taskID uint64)
type TaskFnWithCallback func(taskID uint64) (callback func())

type Task interface {
	TaskDeprecated
	TaskQueue
}

type TaskDeprecated interface {
	Add(fn TaskFn) (id uint64, err error)
	AddAfter(d time.Duration, fn TaskFn) (id uint64, err error)
	AddCrontab(spec string, fn TaskFn) (entryID uint64, err error)

	Cancel(id uint64) error
	CancelCrontab(entryID uint64) (err error)
}

type TaskQueue interface {
	// AddToAtomosQueue
	// 添加任务到Atomos的任务队列，并返回一个取消函数。和Task.Add类似，但不返回任务ID。
	// Append task to Atomos task queue, and return a cancel function.
	AddToAtomosQueue(task TaskFn, ext ...ArgsForTask) (cancel func(reason string) error)

	// AddToSerialQueue
	// 添加任务到指定名称的串行任务队列，并返回一个取消函数。如果该队列不存在，则创建一个新的队列；当队列为空时，删除该队列。
	// Append task to a named serial task queue, and return a cancel function. If the queue does not exist, create a new one; when the queue is empty, delete the queue.
	AddToSerialQueue(queueName string, workerFn TaskFnWithCallback, ext ...ArgsForTask) (cancel func(reason string) error)

	// AddToConcurrentQueue
	// 添加任务到并行任务队列，并返回一个取消函数。每一个并行任务都是独立的一个goroutine。
	// Append task to a concurrent task queue, and return a cancel function. Each concurrent task is an independent goroutine.
	AddToConcurrentQueue(workerFn TaskFnWithCallback, ext ...ArgsForTask) (cancel func(reason string) error)

	// HasMarking
	// 检查任务队列中是否存在指定标记的任务。
	// Check whether a task with the specified marking exists in the task queue.
	HasMarking(mark string) bool
}

// 测试思路：
// 1、如果执行的任务函数崩溃了，是否会影响到Atomos的运行。
// 2、临界状态，例如定时任务刚好被触发是，取消任务。
// 3、Add和AddAfter输入的参数如果不正确，会否影响应用运行。
// 4、Cancel的表现。
// 5、在Atomos的各种运行状态的操作表现是否正常，会否因为用户的误操作导致应用崩溃。

// TaskState
// 任务状态
// Atomos task state
type TaskState int

const (
	// TaskScheduling
	// 任务正在排程，还未加入到Atomos邮箱，目前仅定时任务会使用这种状态。
	// Task is scheduling, and has not been sent to Atomos mailbox yet, only timer task will use this state.
	TaskScheduling TaskState = 1

	// TaskCancelled
	// 任务被取消。
	// Task is cancelled.
	TaskCancelled TaskState = 2

	// TaskMailing
	// 任务已经被发送到Atomos邮箱，普通任务会被马上加到Atomos邮箱尾部，定时任务会在指定时间被加入到Atomos邮箱头部。
	// Task has been sent to Atomos Mailbox, common task will be sent to the tail of Atomos Mail immediately,
	// timer task will be sent to the head of Atomos Mail after the timer times up.
	TaskMailing TaskState = 3

	// TaskExecuting
	// 任务正在被执行。
	// Task is executing.
	TaskExecuting TaskState = 4

	// TaskDone
	// 任务已经被执行。
	// （暂时不会用到这种状态，因为这时atomosTask已经不再存在。）
	// Task has been done.
	// (Such a state has not been used yet.)
	TaskDone TaskState = 5
)

// 默认的任务队列长度
const defaultTasksSize = 10

// Atomos任务
// Atomos Task
type atomosTask struct {
	// 该Atomos的任务唯一id。
	// Unique id of this atomos.
	id      uint64
	manager *atomosTaskManager

	// 发去Atomos邮箱的Atomos邮件。
	// Atomos mail that send to Atomos mailbox.
	atomosMail *baseAtomosMail

	helper      *taskHelper
	queueHelper atomic.Pointer[taskHelper]

	// Atomos任务的定时器，决定了多久之后把邮件发送到Atomos的邮箱。
	// Timer of atomos task, which determines when to send Atomos mail to Atomos mailbox.
	timer   *time.Timer
	cronEID cron.EntryID

	// Atomos任务的状态。
	// State of Atomos task.
	timerState TaskState
}

// CancelledTask
// 被取消的任务。
// 在Atomos被停止的时候，会把所有未执行的AtomosTask包装成CancelledTask，并通过Atomos的Halt方法的参数回传给Atomos处理。
//
//// Cancelled Task.
//// When an atomos is being halted, it will pack all not processed task into CancelledTask instance, and pass
//type CancelledTask struct {
//	ID   uint64
//	Name string
//	Arg  proto.Message
//}

// Atomos任务管理器
// 负责管理任务ID计数器和任务容器。
//
// Atomos Task Manager
// In charge of the management of the increase task id and the task holder.
type atomosTaskManager struct {
	log loggingService

	// AtomosCore实例的引用。
	// Reference to Atomos instance.
	atomos *BaseAtomos

	// 被用于ID自增和任务增删的锁。
	// A mutex-lock uses for id increment and tasks management.
	mutex sync.Mutex

	// 自增id的计数器。
	// （有人能把所有uint64的数字用一遍吗？）
	//
	// Increase id counter.
	// (Is anyone able to use all the number of the unsigned int64?)
	curID uint64

	// 任务容器，记录排程、排队和执行中的任务。
	// Tasks holder for scheduling, queued and executing tasks.
	tasks map[uint64]*atomosTask

	// stopping 拒绝新任务，并由 mutex 保护。
	stopping bool

	cron *cron.Cron

	queueMap map[string]*taskMailbox
}

// 初始化atomosTasksManager的内容。
// 没有构造和释构函数，因为atomosTasksManager是Atomos内部使用的。
//
// Initialization of atomosTaskManager.
// No New and Delete function because atomosTaskManager is struct inner BaseAtomos.
func initAtomosTasksManager(log *loggingAtomos, at *atomosTaskManager, a *BaseAtomos) {
	at.log = log
	at.atomos = a
	at.curID = 0
	at.stopping = false
	at.tasks = make(map[uint64]*atomosTask, defaultTasksSize)
	at.queueMap = make(map[string]*taskMailbox)
}

func releaseAtomosTasksManager(at *atomosTaskManager) {
}

// beginStopping 关闭新任务准入，不在整个停机流程中持有 mutex。
func (at *atomosTaskManager) beginStopping() {
	at.mutex.Lock()
	at.stopping = true
	at.mutex.Unlock()
}

// Add
// 添加任务，并返回可以用于取消的任务id。
// Append task, and return a cancellable task id.
func (at *atomosTaskManager) Add(taskClosure TaskFn) (taskID uint64, err error) {
	if taskClosure == nil {
		return 0, NewErrorf(ErrCodeAtomosTaskInvalidFn, "AtomosTask: Task closure is nil.").AddStack(nil)
	}

	at.mutex.Lock()
	defer at.mutex.Unlock()

	if at.stopping || at.atomos == nil {
		return 0, NewErrorf(ErrCodeAtomosIsNotRunning,
			"AtomosTask: Atomos is not running. Add task failed.").AddStack(nil)
	}

	at.curID += 1
	curID := at.curID
	am := allocBaseAtomosMail()
	initTaskClosureMail(am, at.closureInfo(0), curID, taskClosure)
	t := &atomosTask{id: curID, manager: at, atomosMail: am, timerState: TaskMailing}
	am.atomosTask = t
	at.tasks[curID] = t

	if ok := at.atomos.mailbox.pushTail(am.mail); !ok {
		delete(at.tasks, curID)
		releaseMail(am.mail)
		return 0, NewErrorf(ErrCodeAtomosIsNotRunning,
			"AtomosTask: Atomos is not running. Add task failed.").AddStack(nil)
	}
	return curID, nil
}

// AddAfter
// 指定时间后添加任务，并返回可以用于取消的任务id。
// Append task after duration, and return a cancellable task id.
func (at *atomosTaskManager) AddAfter(after time.Duration, taskClosure TaskFn) (id uint64, err error) {
	if taskClosure == nil {
		return 0, NewErrorf(ErrCodeAtomosTaskInvalidFn, "AtomosTask: Task closure is nil.").AddStack(nil)
	}

	at.mutex.Lock()
	defer at.mutex.Unlock()
	if at.stopping || at.atomos == nil {
		return 0, NewErrorf(ErrCodeAtomosIsNotRunning,
			"AtomosTask: Atomos is not running. Add task failed.").AddStack(nil)
	}

	at.curID += 1
	curID := at.curID
	am := allocBaseAtomosMail()
	initTaskClosureMail(am, at.closureInfo(0), curID, taskClosure)
	t := &atomosTask{id: curID, manager: at, atomosMail: am, timerState: TaskScheduling}
	am.atomosTask = t
	at.tasks[curID] = t
	t.timer = time.AfterFunc(after, func() {
		at.mutex.Lock()
		it, has := at.tasks[t.id]
		if at.stopping || !has || it != t || it.timerState != TaskScheduling {
			at.mutex.Unlock()
			return
		}
		if ok := at.atomos.mailbox.pushHead(am.mail); ok {
			it.timerState = TaskMailing
			at.mutex.Unlock()
			return
		}
		// 邮箱已拒绝邮件，保留排程状态，由停机清理统一记录并释放。
		at.mutex.Unlock()
	})
	return curID, nil
}

func (at *atomosTaskManager) AddCrontab(spec string, taskClosure TaskFn) (entryID uint64, err error) {
	at.mutex.Lock()
	defer at.mutex.Unlock()
	if at.stopping || at.atomos == nil {
		return 0, NewErrorf(ErrCodeAtomosIsNotRunning, "AtomosTask: Atomos is not running. Add crontab failed.").AddStack(nil)
	}
	c := at.getCronLocked()
	eid, er := c.AddFunc(spec, func() {
		if _, er := at.Add(taskClosure); er != nil {
			// TODO
		}
	})
	if er != nil {
		return 0, NewErrorf(ErrCodeAtomosTaskAddCrontabFailed, "AtomosTask: Add crontab failed. spec=(%s),err=(%v)", spec, er).AddStack(nil)
	}
	return uint64(eid), nil
}

// Cancel
// 取消任务。
// Cancel task.
func (at *atomosTaskManager) Cancel(id uint64) error {
	err := at.cancelTask(id, false, nil)
	if err != nil {
		return err.AddStack(nil)
	}
	return nil
}

func (at *atomosTaskManager) CancelCrontab(entryID uint64) (err error) {
	at.mutex.Lock()
	defer at.mutex.Unlock()
	if at.cron == nil {
		return NewErrorf(ErrCodeAtomosTaskRemoveCrontabFailed, "AtomosTask: Cancel crontab failed, crontab not exist.").AddStack(nil)
	}

	at.cron.Remove(cron.EntryID(entryID))
	return nil
}

// 用于退出Atomos时的清理。主邮箱的剩余邮件由 mailboxOnStop 逐个取消。
func (at *atomosTaskManager) cancelAllSchedulingTasks() []uint64 {
	at.mutex.Lock()
	c := at.cron
	at.cron = nil
	ids := make([]uint64, 0, len(at.tasks))
	for id := range at.tasks {
		ids = append(ids, id)
	}
	at.mutex.Unlock()
	if c != nil {
		c.Stop()
	}

	cancels := make([]uint64, 0, len(ids))
	for _, id := range ids {
		at.mutex.Lock()
		t := at.tasks[id]
		skip := t != nil && t.timerState == TaskMailing &&
			(t.helper == nil || t.helper.mailbox == at.atomos.mailbox)
		at.mutex.Unlock()
		if skip {
			continue
		}
		if err := at.cancelTask(id, false, nil); err == nil {
			cancels = append(cancels, id)
		}
	}
	return cancels
}

// cancelTask 的 mailboxExit=true 形式供已持有 mutex 的内部调用方使用。
func (at *atomosTaskManager) cancelTask(id uint64, mailboxExit bool, t *atomosTask) (err *Error) {
	queueTask := false
	if !mailboxExit {
		at.mutex.Lock()
		if current := at.tasks[id]; current != nil {
			t = current
		}
		queueTask = t != nil && t.helper != nil
		at.mutex.Unlock()
	} else {
		if t == nil {
			t = at.tasks[id]
		}
		queueTask = t != nil && t.helper != nil
	}
	if queueTask {
		return at.cancelTaskQueue(t, mailboxExit, "Cancel due to exit")
	}

	if !mailboxExit {
		at.mutex.Lock()
		defer at.mutex.Unlock()
		if current := at.tasks[id]; current != t {
			t = current
		}
	}
	if t == nil {
		return NewErrorf(ErrCodeAtomosTaskNotExists, "AtomosTask: Task does not exist. id=(%d)", id).AddStack(nil)
	}

	switch t.timerState {
	case TaskScheduling:
		if t.timer != nil {
			t.timer.Stop()
		}
		t.timerState = TaskCancelled
		delete(at.tasks, id)
		if t.atomosMail != nil {
			releaseMail(t.atomosMail.mail)
			t.atomosMail = nil
		}
		return nil
	case TaskMailing:
		m := at.atomos.mailbox.popByID(id)
		if m != nil {
			releaseMail(m)
		}
		t.timerState = TaskCancelled
		delete(at.tasks, id)
		t.atomosMail = nil
		return nil
	case TaskExecuting:
		return NewErrorf(ErrCodeAtomosTaskCannotCancelRunningTask, "AtomosTask: Cannot cancel a running task. id=(%d)", id).AddStack(nil)
	case TaskDone:
		return NewErrorf(ErrCodeAtomosTaskCannotCancelDoneTask, "AtomosTask: Cannot cancel a done task. id=(%d)", id).AddStack(nil)
	case TaskCancelled:
		return NewErrorf(ErrCodeAtomosTaskCannotCancelCancelledTask, "AtomosTask: Task already cancelled. id=(%d)", id).AddStack(nil)
	default:
		return NewErrorf(ErrCodeAtomosTaskNotExists, "AtomosTask: Task does not exist. id=(%d)", id).AddStack(nil)
	}
}

// cancelRemainingTask 取消已经从主邮箱弹出、不会再进入 handleTask 的邮件。
// 返回 true 表示本次调用实际取消了任务；邮件本身仍由 mailboxOnStop 释放。
func (at *atomosTaskManager) cancelRemainingTask(am *baseAtomosMail) bool {
	id := am.mail.id
	if id == 0 {
		return false
	}
	manager := at
	if am.atomosTask != nil && am.atomosTask.manager != nil {
		manager = am.atomosTask.manager
	}
	manager.mutex.Lock()
	t := manager.tasks[id]
	if t == nil || t.timerState != TaskMailing || (am.atomosTask != nil && am.atomosTask != t) {
		manager.mutex.Unlock()
		return false
	}
	var callback func(string)
	if t.helper != nil {
		callback = t.helper.cancelCallback
	}
	t.timerState = TaskCancelled
	manager.releaseCancelledTask(t)
	manager.mutex.Unlock()
	manager.runCancelCallback(id, callback, "Cancel due to exit")
	return true
}

func (at *atomosTaskManager) runCancelCallback(id uint64, callback func(string), reason string) {
	if callback == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			at.log.pushFrameworkErrorLog("AtomosTask: Cancel callback panics. id=(%d),reason=(%v)", id, r)
		}
	}()
	callback(reason)
}

// Atomos正式开始处理任务。
// Atomos is beginning to handle a task.
func (at *atomosTaskManager) handleTask(am *baseAtomosMail) {
	var err *Error
	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					at.atomos.log.Fatal("AtomosTask: Task recovers from panic. err=(%v)", err)
				}
			}()
			if err == nil {
				err = NewErrorf(ErrCodeFrameworkRecoverFromPanic, "AtomosTask: Task recovers from panic.").AddPanicStack(nil, 2, r)
			} else {
				err = err.AddPanicStack(nil, 2, r)
			}
			// Hook or Log
			if ar, ok := at.atomos.instance.(AtomosRecover); ok {
				ar.TaskRecover(am.mail.id, am.name, am.arg, err)
			} else {
				at.atomos.log.Fatal("AtomosTask: Task recovers from panic. err=(%v)", err)
			}
			// Global hook
			at.atomos.process.onRecoverHook(at.atomos.id, err)
		}
	}()

	if am.taskClosure != nil {
		at.mutex.Lock()
		t := at.tasks[am.mail.id]
		if t == nil || t.timerState != TaskMailing {
			at.mutex.Unlock()
			return
		}
		t.timerState = TaskExecuting
		at.mutex.Unlock()
		defer func() {
			at.mutex.Lock()
			t.timerState = TaskDone
			if at.tasks[t.id] == t {
				delete(at.tasks, t.id)
			}
			at.mutex.Unlock()
		}()
		am.taskClosure(am.mail.id)
		return
	}
	at.handleTaskQueue(am)
}

func (at *atomosTaskManager) closureInfo(add int) string {
	_, file, line, ok := runtime.Caller(2 + add)
	if !ok {
		file, line = "???", 0
	}
	if at.atomos.process.local.runnable != nil {
		if buildPath := at.atomos.process.local.runnable.config.BuildPath; buildPath != "" {
			file = strings.TrimPrefix(file, buildPath)
		}
	}
	return fmt.Sprintf("%s:%d", file, line)
}
