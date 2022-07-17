package go_atomos

// CHECKED!

import (
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"google.golang.org/protobuf/proto"
)

//
// Atomos Task
//

type TaskFn func(taskId uint64, data proto.Message)

type Task interface {
	Append(fn TaskFn, msg proto.Message) (id uint64, err *ErrorInfo)
	AddAfter(d time.Duration, fn TaskFn, msg proto.Message) (id uint64, err *ErrorInfo)
	Cancel(id uint64) (CancelledTask, *ErrorInfo)
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
	// （暂时不会用到这种状态）
	// Task is executing.
	// (Such a state has not been used yet.)
	TaskExecuting TaskState = 3

	// TaskDone
	// 任务已经被执行。
	// （暂时不会用到这种状态）
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
// Cancelled Task.
// When an atomos is being halted, it will pack all not processed task into CancelledTask instance, and pass
type CancelledTask struct {
	Id   uint64
	Name string
	Arg  proto.Message
}

// Atomos任务管理器
// 负责管理任务ID计数器和任务容器。
//
// Atomos Task Manager
// In charge of the management of the increase task id and the task holder.
type atomosTaskManager struct {
	// AtomosCore实例的引用。
	// Reference to baseAtomosos instance.
	atomos *BaseAtomos

	// 被用于ID自增和任务增删的锁。
	// A mutex-lock uses for id increment and tasks management.
	mutex sync.Mutex

	// 自增id的计数器。
	// （有人能把所有uint64的数字用一遍吗？）
	//
	// Increase id counter.
	// (Is anyone able to use all the number of the unsigned int64?)
	curId uint64

	// 任务容器，目前仅定时任务使用。
	// Tasks holder, only timer task will use.
	tasks map[uint64]*atomosTask
}

// 初始化atomosTasksManager的内容。
// 没有构造和释构函数，因为atomosTasksManager是Atomos内部使用的。
//
// Initialization of atomosTaskManager.
// No New and Delete function because atomosTaskManager is struct inner BaseAtomos.
func initAtomosTasksManager(at *atomosTaskManager, a *BaseAtomos) {
	at.atomos = a
	at.curId = 0
	at.tasks = make(map[uint64]*atomosTask, defaultTasksSize)
}

// 释放atomosTasksManager对象的内容。
// 因为atomosTasksManager是thread-safe的，所以可以借助tasks和Atomos是否为空来判断atomos是否执行中。
//
// Releasing atomosTaskManager.
// Because atomosTaskManager is thread-safe, so we can judge atomos is running though whether the task and BaseAtomos is nil
// or not.
func releaseAtomosTask(at *atomosTaskManager) {
	at.tasks = nil
	at.atomos = nil
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

// Append
// 添加任务，并返回可以用于取消的任务id。
// Append task, and return a cancellable task id.
func (at *atomosTaskManager) Append(fn TaskFn, msg proto.Message) (id uint64, err *ErrorInfo) {
	// Check if illegal before scheduling.
	fnName, err := checkTaskFn(at.atomos, fn, msg)
	if err != nil {
		return 0, err
	}

	at.mutex.Lock()
	defer at.mutex.Unlock()

	// If BaseAtomos is nil, Atomos has been stopped, add failed.
	if at.atomos == nil {
		return 0, NewErrorf(ErrAtomosIsNotRunning,
			"STOPPED, Append atomos failed, fn=(%T),msg=(%+v)", fn, msg)
	}

	// Id increment.
	at.curId += 1

	// Load the Atomos mail.
	am := allocAtomosMail()
	initTaskMail(am, at.curId, fnName, msg)
	// Append to the tail of Atomos mailbox immediately.
	if ok := at.atomos.mailbox.pushTail(am.mail); !ok {
		return 0, NewErrorf(ErrAtomosIsNotRunning,
			"STOPPED, Append mailbox failed, fn=(%T),msg=(%+v)", fn, msg)
	}
	return at.curId, nil
}

// AddAfter
// 指定时间后添加任务，并返回可以用于取消的任务id。
// Append task after duration, and return an cancellable task id.
func (at *atomosTaskManager) AddAfter(after time.Duration, fn TaskFn, msg proto.Message) (id uint64, err *ErrorInfo) {
	// Check if illegal before scheduling.
	fnName, err := checkTaskFn(at.atomos, fn, msg)
	if err != nil {
		return 0, err
	}

	at.mutex.Lock()
	defer at.mutex.Unlock()

	// If BaseAtomos is nil, Atomos has been stopped, add failed.
	if at.atomos == nil {
		return 0, NewErrorf(ErrAtomosIsNotRunning,
			"STOPPED, AddAfter failed, after=(%v),fn=(%T),msg=(%+v)", after, fn, msg)
	}

	// Increment
	at.curId += 1
	curId := at.curId

	// Load the Atomos mail.
	am := allocAtomosMail()
	initTaskMail(am, at.curId, fnName, msg)
	// But not append to the mailbox, now is to create a timer task.
	t := &atomosTask{}
	at.tasks[curId] = t
	t.id = curId
	t.mail = am
	// Set the task to state TaskScheduling, and try to add mail to mailbox after duartion.
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
		// This switch-case is unreachable unless framework has bug.
		case TaskCancelled:
			if ta, has := at.tasks[it.id]; has {
				delete(at.tasks, it.id)
				deallocAtomosMail(ta.mail)
				// FRAMEWORK LEVEL ERROR
				// Because it should not happen, once a TimerTask has been cancelled,
				// it will be removed, thread-safely, immediately.
				at.atomos.log.Fatal("AtomosTask: AddAfter, FRAMEWORK ERROR, timer cancel")
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
			at.atomos.log.Fatal("AtomosTask: AddAfter, FRAMEWORK ERROR, timer executing")
		}
	})
	return curId, nil
}

// 用于Add和AddAfter的检查任务合法性逻辑。
// Uses in Append and AddAfter for checking task legal.
func checkTaskFn(a *BaseAtomos, fn TaskFn, msg proto.Message) (string, *ErrorInfo) {
	// Check func type.
	fnValue := reflect.ValueOf(fn)
	fnType := reflect.TypeOf(fn)
	if fnValue.Kind() != reflect.Func {
		return "", NewErrorf(ErrAtomosTaskInvalidFn, "Invalid task func type, fnType=(%T)", fn)
	}
	fnRuntime := runtime.FuncForPC(fnValue.Pointer())
	if fnRuntime == nil {
		return "", NewErrorf(ErrAtomosTaskInvalidFn, "Invalid task func runtime, fnType=(%T)", fn)
	}
	// Get func name.
	fnRawName := fnRuntime.Name()
	fnName := getTaskFnName(fnRawName)
	fnRunes := []rune(fnName)
	if len(fnRunes) == 0 || unicode.IsLower(fnRunes[0]) {
		return "", NewErrorf(ErrAtomosTaskInvalidFn, "Invalid task func name, fnType=(%T)", fn)
	}
	// 用反射来执行任务函数。
	// Executing task method using reflect.
	instValue := reflect.ValueOf(a.instance)
	method := instValue.MethodByName(fnName)
	if !method.IsValid() {
		return "", NewErrorf(ErrAtomosTaskInvalidFn, "Invalid task func value, fnType=(%T)", fn)
	}
	_, err := checkFnArgs(fnType, fnName, msg)
	return fnName, err
}

func checkFnArgs(fnType reflect.Type, fnName string, msg proto.Message) (int, *ErrorInfo) {
	fnNumIn := fnType.NumIn()
	switch fnNumIn {
	//case 0:
	//	// Call without argument.
	//	return 0, nil
	//case 1:
	//	// Call with only a task id argument.
	//	if msg != nil {
	//		return 0, fmt.Errorf("AtomosTask: Append func not support message, fn=%s", fnName)
	//	}
	//	fn0Type := fnType.In(0)
	//	// Check task id.
	//	if fn0Type.Kind() != reflect.Uint64 {
	//		return 0, fmt.Errorf("AtomosTask: Append illegal task id receiver, fn=%s,msg=%+v", fnName, msg)
	//	}
	//	return 1, nil
	case 2:
		// Call with task id as first argument and message as second argument.
		//if msg == nil {
		//	return 0, fmt.Errorf("AtomosTask: Append nil message, fn=%s", fnName)
		//}
		fn0Type, fn1Type := fnType.In(0), fnType.In(1)
		// Check task id.
		if fn0Type.Kind() != reflect.Uint64 {
			return 0, NewErrorf(ErrAtomosTaskInvalidFn, "Invalid task func task id receiver, fnName=(%s),fn0Type=(%s)", fnName, fn0Type.Name())
		}
		// Check message.
		if msg != nil {
			msgType := reflect.TypeOf(msg)
			if fn1Type.String() != msgType.String() || !msgType.AssignableTo(fn1Type) {
				return 0, NewErrorf(ErrAtomosTaskInvalidFn, "Invalid task func message receiver, fnName=(%s),fn1Type=(%T)", fnName, msg)
			}
		}
		return 2, nil
	}
	return 0, NewErrorf(ErrAtomosTaskInvalidFn, "Invalid task func signature, fnName=(%s)", fnName)
}

func getTaskFnName(fnRawName string) string {
	ss := strings.Split(fnRawName, ".")
	s := ss[len(ss)-1]
	ss = strings.Split(s, "-")
	return ss[0]
}

// Cancel
// 取消任务。
// Cancel task.
func (at *atomosTaskManager) Cancel(id uint64) (CancelledTask, *ErrorInfo) {
	at.mutex.Lock()
	defer at.mutex.Unlock()

	return at.cancelTask(id, nil)
}

// 用于退出Atomos时的清理。
// For cleaning an stopping atomos.
func (at *atomosTaskManager) cancelAllSchedulingTasks() map[uint64]CancelledTask {
	cancels := make(map[uint64]CancelledTask, len(at.tasks))
	for id, t := range at.tasks {
		cancel, err := at.cancelTask(id, t)
		if err == nil {
			cancels[id] = cancel
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
func (at *atomosTaskManager) cancelTask(id uint64, t *atomosTask) (cancel CancelledTask, err *ErrorInfo) {
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
			err = NewErrorf(ErrFrameworkPanic, "Delete a not exists task timer, task=(%+v)", t)
			at.atomos.log.Fatal("AtomosTask: Cancel, FRAMEWORK ERROR, err=(%v)", err)
			return cancel, err

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
			cancel.Id = id
			cancel.Name = t.mail.name
			cancel.Arg = t.mail.arg
			return cancel, nil

		// 定时任务已经在Atomos mailbox中。
		// Timer task is already in Atomos mailbox.
		case TaskMailing:
			m := at.atomos.mailbox.popById(id)
			if m == nil {
				err = NewErrorf(ErrAtomosTaskNotExists, "Delete a not exists task timer, task=(%+v)", t)
				return cancel, err
			}
			cancel.Id = id
			cancel.Name = t.mail.name
			cancel.Arg = t.mail.arg
			return cancel, nil
		default:
			// FRAMEWORK LEVEL ERROR
			err = NewErrorf(ErrFrameworkPanic, "Unknown timer state, task=(%+v)", t)
			at.atomos.log.Fatal("AtomosTask: Cancel, FRAMEWORK ERROR, err=(%v)", err)
			return cancel, err
		}
	}
	m := at.atomos.mailbox.popById(id)
	if m == nil {
		err = NewErrorf(ErrAtomosTaskNotExists, "Delete a not exists task timer, task=(%+v)", t)
		return cancel, err
	}
	am, ok := m.Content.(*atomosMail)
	if !ok {
		err = NewErrorf(ErrAtomosTaskNotExists, "Delete a task timer but its mail is invalid, task=(%+v)", t)
		return cancel, err
	}
	cancel.Id = id
	cancel.Name = am.name
	cancel.Arg = am.arg
	return cancel, nil
}

// Atomos正式开始处理任务。
// Atomos is beginning to handle a task.
func (at *atomosTaskManager) handleTask(am *atomosMail) {
	//at.atomos.SetBusy()
	//defer func() {
	//	// 只有任务执行完毕，Atomos状态仍然为AtomosBusy时，才会把状态设置为AtomosWaiting，因为执行的任务可能会把Atomos终止。
	//	// Only after the task executed and atomos state is still AtomosBusy, will this "SetWaiting" method call,
	//	// because the task may stop the Atomos.
	//	if at.atomos.GetState() == AtomosBusy {
	//		at.atomos.SetWaiting()
	//	}
	//}()
	//defer deallocAtomosMail(am)
	// 用反射来执行任务函数。
	// Executing task method using reflect.
	instValue := reflect.ValueOf(at.atomos.instance)
	method := instValue.MethodByName(am.name)
	if !method.IsValid() {
		at.atomos.log.Fatal("AtomosTask: Method invalid, id=(%d),name=(%s),arg=(%+v)", am.mail.id, am.name, am.arg)
		return
	}
	inNum, err := checkFnArgs(method.Type(), am.name, am.arg)
	if err != nil {
		at.atomos.log.Error("AtomosTask: Argument invalid, id=(%d),name=(%s),arg=(%+v),err=(%v)", am.mail.id, am.name, am.arg, err)
		return
	}
	var val []reflect.Value
	var id = reflect.ValueOf(am.mail.id)
	var arg reflect.Value
	if am.arg != nil {
		arg = reflect.ValueOf(am.arg)
	} else {
		arg = reflect.New(method.Type().In(1)).Elem() // reflect.ValueOf((proto.Message)(nil))
	}
	switch inNum {
	case 1:
		val = append(val, id)
	case 2:
		val = append(val, id, arg)
	}
	method.Call(val)
}

// 供给Telnet使用的任务数量统计。
func (at *atomosTaskManager) getTasksNum() int {
	at.mutex.Lock()
	num := len(at.tasks)
	at.mutex.Unlock()
	return num
}
