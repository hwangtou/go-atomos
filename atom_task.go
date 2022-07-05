package go_atomos

// CHECKED!

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"google.golang.org/protobuf/proto"
)

//
// Atom Task
//

type TaskFn func(taskId uint64, data proto.Message)

type AtomosTasking interface {
	Append(fn TaskFn, msg proto.Message) (id uint64, err error)
	AddAfter(d time.Duration, fn TaskFn, msg proto.Message) (id uint64, err error)
	Cancel(id uint64) (CancelledTask, error)
}

// 测试思路：
// 1、如果执行的任务函数崩溃了，是否会影响到Atom的运行。
// 2、临界状态，例如定时任务刚好被触发是，取消任务。
// 3、Add和AddAfter输入的参数如果不正确，会否影响应用运行。
// 4、Cancel的表现。
// 5、在Atom的各种运行状态的操作表现是否正常，会否因为用户的误操作导致应用崩溃。

// TaskState
// 任务状态
// Atom task state
type TaskState int

const (
	// TaskScheduling
	// 任务正在排程，还未加入到Atom邮箱，目前仅定时任务会使用这种状态。
	// Task is scheduling, and has not been sent to Atom mailbox yet, only timer task will use this state.
	TaskScheduling TaskState = 0

	// TaskCancelled
	// 任务被取消。
	// Task is cancelled.
	TaskCancelled TaskState = 1

	// TaskMailing
	// 任务已经被发送到Atom邮箱，普通任务会被马上加到Atom邮箱尾部，定时任务会在指定时间被加入到Atom邮箱头部。
	// Task has been sent to Atom Mailbox, common task will be sent to the tail of Atom Mail immediately,
	// timer task will be sent to the head of Atom Mail after the timer times up.
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

// Atom任务
// Atom Task
type atomTask struct {
	// 该Atom的任务唯一id。
	// Unique id of this atom.
	id uint64

	// 发去Atom邮箱的Atom邮件。
	// Atom mail that send to Atom mailbox.
	mail *atomMail

	// Atom任务的定时器，决定了多久之后把邮件发送到Atom的邮箱。
	// Timer of atom task, which determines when to send Atom mail to Atom mailbox.
	timer *time.Timer

	// Atom任务的状态。
	// State of Atom task.
	timerState TaskState
}

// CancelledTask
// 被取消的任务。
// 在Atom被停止的时候，会把所有未执行的AtomTask包装成CancelledTask，并通过Atom的Halt方法的参数回传给Atom处理。
//
// Cancelled Task.
// When an atom is being halted, it will pack all not processed task into CancelledTask instance, and pass
type CancelledTask struct {
	Id   uint64
	Name string
	Arg  proto.Message
}

// Atom任务管理器
// 负责管理任务ID计数器和任务容器。
//
// Atom Tasks Manager
// In charge of the management of the increase task id and the task container.
type atomTasksManager struct {
	// AtomCore实例的引用。
	// Reference to baseAtomos instance.
	atom *baseAtomos

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
	// Tasks container, only timer task will use.
	tasks map[uint64]*atomTask
}

// 初始化atomTasksManager的内容。
// 没有构造和释构函数，因为atomTasksManager是AtomCore内部使用的。
//
// Initialization of atomTasksManager.
// No New and Delete function because atomTasksManager is struct inner baseAtomos.
func initAtomTasksManager(at *atomTasksManager, a *baseAtomos) {
	at.atom = a
	at.curId = 0
	at.tasks = make(map[uint64]*atomTask, defaultTasksSize)
}

// 释放atomTasksManager对象的内容。
// 因为atomTasksManager是thread-safe的，所以可以借助tasks和AtomCore是否为空来判断atom是否执行中。
//
// Releasing atomTasksManager.
// Because atomTasksManager is thread-safe, so we can judge atom is running though whether the task and baseAtomos is nil
// or not.
func releaseAtomTask(at *atomTasksManager) {
	at.tasks = nil
	at.atom = nil
}

// 在Atom开始退出的时候上锁，以避免新的任务请求。
// Lock at Atom is going to stop, to prevent new incoming tasks.
func (at *atomTasksManager) stopLock() {
	at.mutex.Lock()
}

// 在Atom退出执行完毕时解锁。
// Unlock after Atom has already stopped.
func (at *atomTasksManager) stopUnlock() {
	at.mutex.Unlock()
}

// Append
// 添加任务，并返回可以用于取消的任务id。
// Append task, and return an cancellable task id.
func (at *atomTasksManager) Append(fn TaskFn, msg proto.Message) (id uint64, err error) {
	// Check if illegal before scheduling.
	fnName, err := checkTaskFn(at.atom, fn, msg)
	if err != nil {
		return 0, err
	}

	at.mutex.Lock()
	defer at.mutex.Unlock()

	// If baseAtomos is nil, Atom has been stopped, add failed.
	if at.atom == nil {
		return 0, ErrAtomIsNotRunning
	}

	// Id increment.
	at.curId += 1

	// Load the Atom mail.
	am := allocAtomMail()
	initTaskMail(am, at.curId, fnName, msg)
	// Append to the tail of Atom mailbox immediately.
	if ok := at.atom.mailbox.pushTail(am.mail); !ok {
		return 0, ErrAtomIsNotRunning
	}
	return at.curId, nil
}

// AddAfter
// 指定时间后添加任务，并返回可以用于取消的任务id。
// Append task after duration, and return an cancellable task id.
func (at *atomTasksManager) AddAfter(after time.Duration, fn TaskFn, msg proto.Message) (id uint64, err error) {
	// Check if illegal before scheduling.
	fnName, err := checkTaskFn(at.atom, fn, msg)
	if err != nil {
		return 0, err
	}

	at.mutex.Lock()
	defer at.mutex.Unlock()

	// If baseAtomos is nil, Atom has been stopped, add failed.
	if at.atom == nil {
		return 0, ErrAtomIsNotRunning
	}

	// Increment
	at.curId += 1
	curId := at.curId

	// Load the Atom mail.
	am := allocAtomMail()
	initTaskMail(am, at.curId, fnName, msg)
	// But not append to the mailbox, now is to create a timer task.
	t := &atomTask{}
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
				deallocAtomMail(ta.mail)
				// FRAMEWORK LEVEL ERROR
				// Because it should not happen, once a TimerTask has been cancelled,
				// it will be removed, thread-safely, immediately.
				at.atom.log.Fatal("AtomosTask: AddAfter, FRAMEWORK ERROR, timer cancel")
			}
			return

		// 任务正在排程。
		// 这段代码符合正常功能的期待。
		// Task is scheduling.
		// This code is expected.
		case TaskScheduling:
			it.timerState = TaskMailing
			delete(at.tasks, it.id)
			if ok := at.atom.mailbox.pushHead(am.mail); !ok {
				at.atom.log.Fatal("AtomosTask: AddAfter, atom is not running")
			}

		// FRAMEWORK LEVEL ERROR
		// Because it should not happen, once a TimerTask has been executed,
		// it will be removed, thread-safely, immediately.
		default:
			at.atom.log.Fatal("AtomosTask: AddAfter, FRAMEWORK ERROR, timer executing")
		}
	})
	return curId, nil
}

// 用于Add和AddAfter的检查任务合法性逻辑。
// Uses in Append and AddAfter for checking task legal.
func checkTaskFn(a *baseAtomos, fn TaskFn, msg proto.Message) (string, error) {
	// Check func type.
	fnValue := reflect.ValueOf(fn)
	fnType := reflect.TypeOf(fn)
	if fnValue.Kind() != reflect.Func {
		return "", fmt.Errorf("AtomosTask: Append invalid function, type=%T,fn=%+v", fn, fn)
	}
	fnRuntime := runtime.FuncForPC(fnValue.Pointer())
	if fnRuntime == nil {
		return "", fmt.Errorf("AtomosTask: Append invalid function runtime, type=%T,fn=%+v", fn, fn)
	}
	// Get func name.
	fnRawName := fnRuntime.Name()
	fnName := getTaskFnName(fnRawName)
	fnRunes := []rune(fnName)
	if len(fnRunes) == 0 || unicode.IsLower(fnRunes[0]) {
		return "", fmt.Errorf("AtomosTask: Append invalid function name, type=%T,fn=%+v", fn, fn)
	}
	// 用反射来执行任务函数。
	// Executing task method using reflect.
	instValue := reflect.ValueOf(a.instance)
	method := instValue.MethodByName(fnName)
	if !method.IsValid() {
		return "", fmt.Errorf("AtomosTask: Method invalid, type=%T,fn=%+v", fn, fn)
	}
	_, err := checkFnArgs(fnType, fnName, msg)
	return fnName, err
}

func checkFnArgs(fnType reflect.Type, fnName string, msg proto.Message) (int, error) {
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
		if msg == nil {
			return 0, fmt.Errorf("AtomosTask: Append nil message, fn=%s", fnName)
		}
		fn0Type, fn1Type := fnType.In(0), fnType.In(1)
		// Check task id.
		if fn0Type.Kind() != reflect.Uint64 {
			return 0, fmt.Errorf("AtomosTask: Append illegal task id receiver, fn=%s,msg=%+v", fnName, msg)
		}
		// Check message.
		msgType := reflect.TypeOf(msg)
		if fn1Type.String() != msgType.String() || !msgType.AssignableTo(fn1Type) {
			return 0, fmt.Errorf("AtomosTask: Append illegal message receiver, fn=%s,msg=%+v", fnName, msg)
		}
		return 2, nil
	}
	return 0, fmt.Errorf("AtomosTask: Append illegal function, fn=%v", fnType)
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
func (at *atomTasksManager) Cancel(id uint64) (CancelledTask, error) {
	at.mutex.Lock()
	defer at.mutex.Unlock()

	return at.cancelTask(id, nil)
}

// 用于退出Atom时的清理。
// For cleaning an stopping atom.
func (at *atomTasksManager) cancelAllSchedulingTasks() map[uint64]CancelledTask {
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
// 1、 删除添加到Atom mailbox的atomTask。
// 2、 删除还在容器的atomTask，并删除定时器。
//
// Use to cancel a task.
// Only for Cancel and cancelAllSchedulingTasks method, they are thread-safe.
// Delete two kinds of tasks:
// 1. delete append atomTask
// 2. delete timer atomTask
func (at *atomTasksManager) cancelTask(id uint64, t *atomTask) (cancel CancelledTask, err error) {
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
			deallocAtomMail(t.mail)
			// FRAMEWORK LEVEL ERROR
			// Because it shouldn't happen, we won't find Canceled timer.
			err = fmt.Errorf("cannot delete timer that not exists")
			at.atom.log.Fatal("AtomosTask: Cancel, FRAMEWORK ERROR, err=%v", err)
			return cancel, err

		// 排程的任务准备执行。
		// 这段代码符合正常功能的期待。
		// Scheduled task is going to execute.
		// This code is expected.
		case TaskScheduling:
			ok := t.timer.Stop()
			t.timerState = TaskCancelled
			delete(at.tasks, id)
			deallocAtomMail(t.mail)
			if !ok {
				// Might only happen on the edge of scheduled time has reached,
				// the period between time.AfterFunc has execute the function,
				// and the function still have acquired the mutex.
				at.atom.log.Info("AtomosTask: Cancel on the edge")
			}
			cancel.Id = id
			cancel.Name = t.mail.name
			cancel.Arg = t.mail.arg
			return cancel, nil

		// 定时任务已经在Atom mailbox中。
		// Timer task is already in Atom mailbox.
		case TaskMailing:
			m := at.atom.mailbox.popById(id)
			if m == nil {
				err = fmt.Errorf("cannot delete timer that not exists")
				return cancel, err
			}
			cancel.Id = id
			cancel.Name = t.mail.name
			cancel.Arg = t.mail.arg
			return cancel, nil
		default:
			// FRAMEWORK LEVEL ERROR
			at.atom.log.Fatal("AtomosTask: Cancel, FRAMEWORK ERROR, unknown timer state, state=%v",
				t.timerState)
			return cancel, fmt.Errorf("unknown timer state")
		}
	}
	m := at.atom.mailbox.popById(id)
	if m == nil {
		err = fmt.Errorf("cannot delete timer that not exists")
		return cancel, err
	}
	am, ok := m.Content.(*atomMail)
	if !ok {
		err = fmt.Errorf("cannot delete timer that atom mail is invalid")
		return cancel, err
	}
	cancel.Id = id
	cancel.Name = am.name
	cancel.Arg = am.arg
	return cancel, nil
}

// Atom正式开始处理任务。
// Atom is beginning to handle a task.
func (at *atomTasksManager) handleTask(am *atomMail) {
	at.atom.setBusy()
	defer func() {
		// 只有任务执行完毕，Atom状态仍然为AtomBusy时，才会把状态设置为AtomWaiting，因为执行的任务可能会把Atom终止。
		// Only after the task executed and atom state is still AtomBusy, will this "setWaiting" method call,
		// because the task may stop the Atom.
		if at.atom.getState() == AtomBusy {
			at.atom.setWaiting()
		}
	}()
	defer deallocAtomMail(am)
	// 用反射来执行任务函数。
	// Executing task method using reflect.
	instValue := reflect.ValueOf(at.atom.instance)
	method := instValue.MethodByName(am.name)
	if !method.IsValid() {
		at.atom.log.Error("AtomosTask: Method invalid, id=%d,name=%s,arg=%+v",
			am.mail.id, am.name, am.arg)
		return
	}
	inNum, err := checkFnArgs(method.Type(), am.name, am.arg)
	if err != nil {
		at.atom.log.Error("AtomosTask: Argument invalid, id=%d,name=%s,arg=%+v,err=%v",
			am.mail.id, am.name, am.arg, err)
		return
	}
	var val []reflect.Value
	var id = reflect.ValueOf(am.id)
	var arg reflect.Value
	if am.arg != nil {
		arg = reflect.ValueOf(am.arg)
	} else {
		arg = reflect.ValueOf((proto.Message)(nil))
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
func (at *atomTasksManager) getTasksNum() int {
	at.mutex.Lock()
	num := len(at.tasks)
	at.mutex.Unlock()
	return num
}
