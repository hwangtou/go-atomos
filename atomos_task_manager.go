package go_atomos

import (
	"fmt"
	"github.com/robfig/cron/v3"
	"runtime"
	"sync"
	"time"
)

//
// Atomos Task
//

type TaskFn func(taskID uint64)

type Task interface {
	Add(fn TaskFn, ext ...interface{}) (id uint64, err *Error)
	AddAfter(d time.Duration, fn TaskFn, ext ...interface{}) (id uint64, err *Error)
	AddCrontab(spec string, fn TaskFn, ext ...interface{}) (entryID uint64, err *Error)

	Cancel(id uint64) *Error
	CancelCrontab(entryID uint64) (err *Error)
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
	TaskScheduling TaskState = 0

	// TaskCancelled
	// 任务被取消。
	// Task is cancelled.
	TaskCancelled TaskState = 1

	// TaskMailing
	// 任务已经被发送到Atomos邮箱，普通任务会被马上加到Atomos邮箱尾部，定时任务会在指定时间被加入到Atomos邮箱头部。
	// Task has been sent to Atomos Mailbox, common task will be sent to the tail of Atomos Mail immediately,
	// timer task will be sent to the head of Atomos Mail after the timer times up.
	TaskMailing TaskState = 2

	// TaskExecuting
	// 任务正在被执行。
	// （暂时不会用到这种状态，因为这时atomosTask已经不再存在。）
	// Task is executing.
	// (Such a state has not been used yet.)
	TaskExecuting TaskState = 3

	// TaskDone
	// 任务已经被执行。
	// （暂时不会用到这种状态，因为这时atomosTask已经不再存在。）
	// Task has been done.
	// (Such a state has not been used yet.)
	TaskDone TaskState = 4
)

// 默认的任务队列长度
const defaultTasksSize = 10

// Atomos任务
// Atomos Task
type atomosTask struct {
	// 该Atomos的任务唯一id。
	// Unique id of this atomos.
	id uint64

	// 发去Atomos邮箱的Atomos邮件。
	// Atomos mail that send to Atomos mailbox.
	mail *atomosMail

	// Atomos任务的定时器，决定了多久之后把邮件发送到Atomos的邮箱。
	// Timer of atomos task, which determines when to send Atomos mail to Atomos mailbox.
	timer *time.Timer

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
	log *loggingAtomos

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

	// 任务容器，目前仅定时任务使用。
	// Tasks holder, only timer task will use.
	tasks map[uint64]*atomosTask

	cron *cron.Cron
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
	at.tasks = make(map[uint64]*atomosTask, defaultTasksSize)
}

func releaseAtomosTasksManager(_ *atomosTaskManager) {
}

// 在Atomos开始退出的时候上锁，以避免新的任务请求。
// Lock at Atomos is going to stop, to prevent new incoming tasks.
func (at *atomosTaskManager) stopLock() {
	at.mutex.Lock()
}

// 在Atomos退出执行完毕时解锁。
// Unlock after Atomos has already stopped.
func (at *atomosTaskManager) stopUnlock() {
	at.mutex.Unlock()
}

// Add
// 添加任务，并返回可以用于取消的任务id。
// Append task, and return a cancellable task id.
func (at *atomosTaskManager) Add(taskClosure TaskFn, ext ...interface{}) (taskID uint64, err *Error) {
	if taskClosure == nil {
		return 0, NewErrorf(ErrAtomosTaskInvalidFn, "AtomosTask: Task closure is nil.").AddStack(nil)
	}

	at.mutex.Lock()
	defer at.mutex.Unlock()

	// If BaseAtomos is nil, Atomos has been stopped, add failed.
	if at.atomos == nil {
		return 0, NewErrorf(ErrAtomosIsNotRunning,
			"AtomosTask: Atomos is not running. Add task failed.").AddStack(nil)
	}

	// ID increment.
	at.curID += 1

	// Load the Atomos mail.
	am := allocAtomosMail()
	initTaskClosureMail(am, at.closureInfo(), at.curID, taskClosure)

	// Append to the tail of Atomos mailbox immediately.
	if ok := at.atomos.mailbox.pushTail(am.mail); !ok {
		return 0, NewErrorf(ErrAtomosIsNotRunning,
			"AtomosTask: Atomos is not running. Add task failed.").AddStack(nil)
	}
	return at.curID, nil
}

// AddAfter
// 指定时间后添加任务，并返回可以用于取消的任务id。
// Append task after duration, and return a cancellable task id.
func (at *atomosTaskManager) AddAfter(after time.Duration, taskClosure TaskFn, ext ...interface{}) (id uint64, err *Error) {
	if taskClosure == nil {
		return 0, NewErrorf(ErrAtomosTaskInvalidFn, "AtomosTask: Task closure is nil.").AddStack(nil)
	}

	at.mutex.Lock()
	defer at.mutex.Unlock()

	// If BaseAtomos is nil, Atomos has been stopped, add failed.
	if at.atomos == nil {
		return 0, NewErrorf(ErrAtomosIsNotRunning,
			"AtomosTask: Atomos is not running. Add task failed.").AddStack(nil)
	}

	// Increment
	at.curID += 1
	curID := at.curID

	// Load the Atomos mail.
	am := allocAtomosMail()
	initTaskClosureMail(am, at.closureInfo(), at.curID, taskClosure)

	// But not append to the mailbox, now is to create a timer task.
	t := &atomosTask{}
	at.tasks[curID] = t
	t.id = curID
	t.mail = am
	// Set the task to state TaskScheduling, and try to add mail to mailbox after duration.
	t.timerState = TaskScheduling
	t.timer = time.AfterFunc(after, func() {
		at.mutex.Lock()
		defer at.mutex.Unlock()
		it, has := at.tasks[t.id]
		if !has {
			// 当且仅当发生在边缘的情况，任务计时器还未成功被取消，但任务已经被移出容器的情况。
			// It should only happen at the edge of the timer has been triggered,
			// but the TimerTask has been cancelled.
			return
		}
		switch it.timerState {
		// 任务已被取消。
		// 正常来说这段代码不会触发到，除非框架逻辑有问题。
		// Task has been cancelled.
		// This switch-case is unreachable unless framework has a bug.
		case TaskCancelled:
			if ta, has := at.tasks[it.id]; has {
				delete(at.tasks, it.id)
				deallocAtomosMail(ta.mail)
				// FRAMEWORK LEVEL ERROR
				// Because it should not happen, once a TimerTask has been cancelled,
				// it will be removed, thread-safely, immediately.
				at.log.pushFrameworkErrorLog("AtomosTask: AddAfter, FRAMEWORK ERROR, timer cancel")
			}
			return

		// 任务正在排程。
		// 这段代码符合正常功能的期待。
		// Task is scheduling.
		// This code is expected.
		case TaskScheduling:
			it.timerState = TaskMailing
			delete(at.tasks, it.id)
			if ok := at.atomos.mailbox.pushHead(am.mail); !ok {
				at.atomos.log.Fatal("AtomosTask: AddAfter, atomos is not running")
			}

		// FRAMEWORK LEVEL ERROR
		// Because it should not happen, once a TimerTask has been executed,
		// it will be removed, thread-safely, immediately.
		default:
			at.log.pushFrameworkErrorLog("AtomosTask: AddAfter, FRAMEWORK ERROR, timer executing")
		}
	})
	return curID, nil
}

func (at *atomosTaskManager) AddCrontab(spec string, taskClosure TaskFn, ext ...interface{}) (entryID uint64, err *Error) {
	c := func() *cron.Cron {
		at.mutex.Lock()
		defer at.mutex.Unlock()
		if at.cron == nil {
			at.cron = cron.New()
			at.cron.Start()
		}
		return at.cron
	}()

	eid, er := c.AddFunc(spec, func() {
		if _, er := at.Add(taskClosure); er != nil {
			// TODO
		}
	})
	if er != nil {
		return 0, NewErrorf(ErrAtomosTaskAddCrontabFailed, "AtomosTask: Add crontab failed. spec=(%s),err=(%v)", spec, er).AddStack(nil)
	}
	return uint64(eid), nil
}

// Cancel
// 取消任务。
// Cancel task.
func (at *atomosTaskManager) Cancel(id uint64) *Error {
	at.mutex.Lock()
	defer at.mutex.Unlock()

	err := at.cancelTask(id, nil)
	if err != nil {
		return err.AddStack(nil)
	}
	return nil
}

func (at *atomosTaskManager) CancelCrontab(entryID uint64) (err *Error) {
	at.mutex.Lock()
	defer at.mutex.Unlock()
	if at.cron == nil {
		return NewErrorf(ErrAtomosTaskRemoveCrontabFailed, "AtomosTask: Cancel crontab failed, crontab not exist.").AddStack(nil)
	}

	at.cron.Remove(cron.EntryID(entryID))
	return nil
}

// 用于退出Atomos时的清理。
// For cleaning an stopping atomos.
func (at *atomosTaskManager) cancelAllSchedulingTasks() []uint64 {
	// 外部已经锁过一次，这里不需要再锁。
	if at.cron != nil {
		at.cron.Stop()
		at.cron = nil
	}
	cancels := make([]uint64, len(at.tasks))
	for id, t := range at.tasks {
		if err := at.cancelTask(id, t); err == nil {
			cancels = append(cancels, id)
		}
	}
	return cancels
}

// 用于取消指定任务。
// 仅供Cancel和cancelAllSchedulingTasks这两个线程安全的函数调用。
// 删除两种任务：
// 1、 删除添加到Atomos mailbox的atomosTask。
// 2、 删除还在容器的atomosTask，并删除定时器。
//
// Use to cancel a task.
// Only for Cancel and cancelAllSchedulingTasks method, they are thread-safe.
// Delete two kinds of tasks:
// 1. delete append atomosTask
// 2. delete timer atomosTask
func (at *atomosTaskManager) cancelTask(id uint64, t *atomosTask) (err *Error) {
	if t == nil {
		ta := at.tasks[id]
		t = ta
	}
	// If it has a timer, it's a timer task.
	if t != nil && t.timer != nil {
		switch t.timerState {
		// 任务已被取消。
		// 正常来说这段代码不会触发到，除非框架逻辑有问题。
		// Task has been cancelled.
		// This switch-case is unreachable unless framework has bug.
		case TaskCancelled:
			delete(at.tasks, id)
			deallocAtomosMail(t.mail)
			// FRAMEWORK LEVEL ERROR
			// Because it shouldn't happen, we won't find Canceled timer.
			err = NewErrorf(ErrFrameworkRecoverFromPanic, "AtomosTask: Delete a not exists task timer. task=(%+v)", t).AddStack(nil)
			at.log.pushFrameworkErrorLog("AtomosTask: CancelTask, FRAMEWORK ERROR. err=(%v)", err)
			return err

		// 排程的任务准备执行。
		// 这段代码符合正常功能的期待。
		// Scheduled task is going to execute.
		// This code is expected.
		case TaskScheduling:
			ok := t.timer.Stop()
			t.timerState = TaskCancelled
			delete(at.tasks, id)
			deallocAtomosMail(t.mail)
			if !ok {
				// Might only happen on the edge of scheduled time has reached,
				// the period between time.AfterFunc has executed the function,
				// and the function still have acquired the mutex.
				at.atomos.log.Info("AtomosTask: Atomos has halted")
			}
			return nil

		// 定时任务已经在Atomos mailbox中。
		// Timer task is already in Atomos mailbox.
		case TaskMailing:
			m := at.atomos.mailbox.popByID(id)
			if m == nil {
				err = NewErrorf(ErrAtomosTaskNotExists, "AtomosTask: Delete a not exists task timer, task=(%+v)", t).AddStack(nil)
				return err
			}
			return nil
		default:
			// FRAMEWORK LEVEL ERROR
			err = NewErrorf(ErrFrameworkRecoverFromPanic, "AtomosTask: Unknown timer state, task=(%+v)", t).AddStack(nil)
			at.log.pushFrameworkErrorLog("AtomosTask: CancelTask, FRAMEWORK ERROR. err=(%v)", err)
			return err
		}
	}
	m := at.atomos.mailbox.popByID(id)
	if m == nil {
		err = NewErrorf(ErrAtomosTaskNotExists, "AtomosTask: Delete a not exists task timer, task=(%+v)", t).AddStack(nil)
		return err
	}
	return nil
}

// Atomos正式开始处理任务。
// Atomos is beginning to handle a task.
func (at *atomosTaskManager) handleTask(am *atomosMail) {
	var err *Error
	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					at.atomos.log.Fatal("AtomosTask: Task recovers from panic. err=(%v)", err)
				}
			}()
			if err == nil {
				err = NewErrorf(ErrFrameworkRecoverFromPanic, "AtomosTask: Task recovers from panic.").AddPanicStack(nil, 2, r)
			} else {
				err = err.AddPanicStack(nil, 2, r)
			}
			// Hook or Log
			if ar, ok := at.atomos.instance.(AtomosRecover); ok {
				ar.TaskRecover(am.id, am.name, am.arg, err)
			} else {
				at.atomos.log.Fatal("AtomosTask: Task recovers from panic. err=(%v)", err)
			}
			// Global hook
			at.atomos.process.onRecoverHook(at.atomos.id, err)
		}
	}()

	am.taskClosure(am.mail.id)
}

func (at *atomosTaskManager) closureInfo() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file, line = "???", 0
	}
	return fmt.Sprintf("%s:%d", file, line)
}
