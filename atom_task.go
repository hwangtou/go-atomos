package go_atomos

import (
	"github.com/golang/protobuf/proto"
	"log"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
)

// AtomTask

const defaultTimerSize = 10

// Atom Task
type atomTask struct {
	id         uint64
	mail       *atomMail
	timer      *time.Timer
	timerState TaskState
}

// Cancelled Task
type CancelledTask struct {
	Id   uint64
	Name string
	Arg  proto.Message
}

// Atom Tasks Manager
// We don't have to think about concurrency of atomTasksManager struct,
// because it's used by its own AtomCore, which is atomic.
type atomTasksManager struct {
	*AtomCore
	mutex sync.Mutex
	curId uint64
	tasks map[uint64]*atomTask
}

// No New and Delete because atomTasksManager is struct in AtomCore.

func initAtomTasksManager(at *atomTasksManager, a *AtomCore) {
	at.AtomCore = a
	at.curId = 0
	at.tasks = make(map[uint64]*atomTask, defaultTimerSize)
}

func releaseAtomTask(at *atomTasksManager) {
	at.tasks = nil
	at.AtomCore = nil
}

func (at *atomTasksManager) Add(fn interface{}, msg proto.Message) error {
	// Because new atomTask must call from inner AtomCore, so this is thread-safe
	if at.state == Halt {
		return ErrAtomIsNotSpawning
	}
	fnName, err := checkTaskFn(fn, msg)
	if err != nil {
		return err
	}

	at.mutex.Lock()
	defer at.mutex.Unlock()

	// Increment
	at.curId += 1

	// Add mail
	am := allocAtomMail()
	initTaskMail(am, at.curId, fnName, msg)
	if ok := at.mailbox.PushTail(am.Mail); !ok {
		return ErrAtomIsNotSpawning
	}
	return nil
}

func (at *atomTasksManager) AddAfter(d time.Duration, fn interface{}, msg proto.Message) error {
	// Because new atomTask must call from inner AtomCore, so this is thread-safe
	if at.state == Halt {
		return ErrAtomIsNotSpawning
	}
	fnName, err := checkTaskFn(fn, msg)
	if err != nil {
		return err
	}

	at.mutex.Lock()
	defer at.mutex.Unlock()

	// Increment
	at.curId += 1
	curId := at.curId

	// Add mail
	am := allocAtomMail()
	initTaskMail(am, at.curId, fnName, msg)

	// Create task
	t := &atomTask{}
	at.tasks[curId] = t
	t.id = curId
	t.mail = am
	t.timerState = TaskScheduling
	t.timer = time.AfterFunc(d, func() {
		at.mutex.Lock()
		defer at.mutex.Unlock()
		it, has := at.tasks[t.id]
		if !has {
			// It should only happen, at the edge of the timer has been triggered,
			// but the TimerTask has been cancelled.
			return
		}
		switch it.timerState {
		case TaskCancelled:
			if ta, has := at.tasks[it.id]; has {
				delete(at.tasks, it.id)
				deallocAtomMail(ta.mail)
				// FRAMEWORK LEVEL ERROR
				// Because it should not happen, once a TimerTask has been cancelled,
				// it will be removed, thread-safely, immediately.
				log.Println("Panic ErrTimerCancelled") // todo
			}
			return
		case TaskScheduling:
			it.timerState = TaskMailing
			delete(at.tasks, it.id)
			if ok := at.mailbox.PushHead(am.Mail); !ok {
				log.Println(ErrAtomIsNotSpawning)
			}
		default:
			// FRAMEWORK LEVEL ERROR
			// Because it should not happen, once a TimerTask has been executed,
			// it will be removed, thread-safely, immediately.
			log.Println("ErrTimerExecuting") // todo
		}
	})
	return nil
}

func (at *atomTasksManager) Cancel(id uint64) (CancelledTask, error) {
	at.mutex.Lock()
	defer at.mutex.Unlock()

	return at.cancelTask(id, nil)
}

func (at *atomTasksManager) cancelAllSchedulingTasks() map[uint64]CancelledTask {
	at.mutex.Lock()
	defer at.mutex.Unlock()

	cancels := make(map[uint64]CancelledTask, len(at.tasks))
	for id, t := range at.tasks {
		cancel, err := at.cancelTask(id, t)
		if err != nil {
			cancels[id] = cancel
		}
	}
	return cancels
}

// Two kinds of tasks:
// 1. delete append atomTask
// 2. delete timer atomTask
func (at *atomTasksManager) cancelTask(id uint64, t *atomTask) (cancel CancelledTask, err error) {
	if t == nil {
		ta, has := at.tasks[id]
		if !has {
			return cancel, ErrAtomTaskNotFound
		}
		t = ta
	}
	if t.timer != nil {
		switch t.timerState {
		case TaskCancelled:
			delete(at.tasks, t.id)
			deallocAtomMail(t.mail)
			// FRAMEWORK LEVEL ERROR
			// Because it shouldn't happen, we won't find Canceled timer.
			log.Println("Panic ErrTimerCancelled")
			return cancel, ErrAtomTaskCannotDelete
		case TaskScheduling:
			ok := t.timer.Stop()
			t.timerState = TaskCancelled
			delete(at.tasks, t.id)
			deallocAtomMail(t.mail)
			if !ok {
				log.Println("Edge")
				// Might only happen on the edge of scheduled time reached,
				// the period between time.AfterFunc has execute the function,
				// and the function still have acquired the mutex.
			}
			cancel.Id = t.mail.id
			cancel.Name = t.mail.name
			cancel.Arg = t.mail.arg
			return cancel, nil
		case TaskMailing:
			m := at.mailbox.PopById(t.id)
			log.Println("Cancelling Executing Task")
			if m == nil {
				return cancel, ErrAtomTaskCannotDelete
			}
			cancel.Id = t.mail.id
			cancel.Name = t.mail.name
			cancel.Arg = t.mail.arg
			return cancel, nil
		default:
			panic("FRAMEWORK LEVEL ERROR")
		}
	} else {
		m := at.mailbox.PopById(t.id)
		log.Println("Cancelling Executing Task")
		if m == nil {
			return cancel, ErrAtomTaskCannotDelete
		}
		cancel.Id = t.mail.id
		cancel.Name = t.mail.name
		cancel.Arg = t.mail.arg
		return cancel, nil
	}
}

func (at *atomTasksManager) handleTask(am *atomMail) {
	at.setBusy()
	defer at.setWaiting()
	defer deallocAtomMail(am)
	instType := reflect.ValueOf(at.instance).Type()
	for i := 0; i < instType.NumMethod(); i++ {
	}
	method := reflect.ValueOf(at.instance).MethodByName(am.name)
	if !method.IsValid() {
		log.Println("ErrHandleTask Func")
		return
	}
	method.Call([]reflect.Value{
		reflect.ValueOf(am.arg),
	})
}

func checkTaskFn(fn interface{}, msg proto.Message) (string, error) {
	// Check func type
	fnValue := reflect.ValueOf(fn)
	fnType := reflect.TypeOf(fn)
	if fnValue.Kind() != reflect.Func {
		return "", ErrAtomAddTaskNotFunc
	}
	if fnType.NumIn() != 1 {
		return "", ErrAtomAddTaskIllegalArg
	}
	argType := fnType.In(0)
	msgType := reflect.TypeOf(msg)
	fnRuntime := runtime.FuncForPC(fnValue.Pointer())
	if fnRuntime == nil {
		return "", ErrAtomAddTaskNotFunc
	}
	// Get func name
	fnRawName := fnRuntime.Name()
	fnName := getTaskFnName(fnRawName)
	fnRunes := []rune(fnName)
	if len(fnRunes) == 0 || unicode.IsLower(fnRunes[0]) {
		return "", ErrAtomAddTaskNotFunc
	}
	if argType.String() == msgType.String() {
		return fnName, nil
	}
	if msgType.AssignableTo(argType) {
		return fnName, nil
	}
	return "", ErrAtomAddTaskIllegalMsg
}

func getTaskFnName(fnRawName string) string {
	ss := strings.Split(fnRawName, ".")
	s := ss[len(ss)-1]
	ss = strings.Split(s, "-")
	return ss[0]
}
