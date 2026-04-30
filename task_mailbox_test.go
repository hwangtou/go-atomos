package atomos

import (
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// TestTaskMailbox_SerialLifeCycle is a test for the life cycle of a task mailbox.
// Check for starting, processing mails, and stopping.
// Check for processing order.
func TestTaskMailbox_SerialLifeCycle(t *testing.T) {
	id := &IDInfo{Type: IDType_Atom, Cosmos: "test_task_mailbox_lifecycle", Node: "test_node", Element: "test_element", Atom: "test_atomos"}
	p := newTestCosmosProcessWithoutCluster(t, id.Cosmos, id.Node)
	tba := newTestTaskMailboxBaseAtomos(t, p, id)

	taskMailboxName := "test_task_mailbox_serial_life_cycle"
	mb := createTaskMailbox(taskMailboxName, true, newTestLoggingAtomos(t))
	taskMailboxGoID := mb.mailbox.goID
	t.Logf("task mailbox info: %s on goID(%d)", mb.mailbox.name, taskMailboxGoID)

	goMap := dumpAllGoID()
	if _, exists := goMap[taskMailboxGoID]; !exists {
		t.Fatal("goID not found in goMap")
	}

	count := 5

	var wg sync.WaitGroup
	wg.Add(count)
	expectedIdx := uint64(1)
	tba.atomos.task.queueMap[mb.mailbox.name] = mb

	for i := 0; i < count; i++ {
		helper := &taskHelper{
			task: func(taskID uint64) {
				if expectedIdx != taskID {
					t.Fatal("expected task id ", expectedIdx, " got ", taskID)
				}
				if getGoID() != taskMailboxGoID {
					t.Fatal("task is not running in mailbox goID:", taskMailboxGoID)
				}
				atomic.AddUint64(&expectedIdx, 1)
				wg.Done()
				t.Log("OK, task executed:", taskID)
			},
			taskWithCallback: nil,
			invalidArgs:      nil,
			conflictArgs:     nil,
			mark:             "",
			delay:            0,
			cronSchedule:     nil,
			appendToHead:     false,
			cancelCallback:   nil,
			recoverFn:        nil,
			manager:          &tba.atomos.task,
			mailbox:          mb.mailbox,
			atomosTask:       nil,
		}
		helper.buildAtomosTask()

		am := allocBaseAtomosMail()
		initTaskQueueMail(am, "", helper)

		mb.mailbox.pushTail(am.mail)
	}

	wg.Wait()

	if mb.mailbox.isRunning() {
		t.Fatal("Mailbox should be stopped after processing all mails.")
	}

	<-time.After(1 * time.Millisecond)
	goMap = dumpAllGoID()
	if _, exists := goMap[taskMailboxGoID]; exists {
		t.Fatal("Mailbox should be stopped after processing all mails.")
	}

	<-time.After(time.Millisecond)
}

// TestTaskMailbox_SerialLifeCycleWithCallback is a test for the life cycle of a task mailbox with callback.
// Check for starting, processing mails, and stopping.
// Check for processing order.
func TestTaskMailbox_SerialLifeCycleWithCallback(t *testing.T) {
	id := &IDInfo{Type: IDType_Atom, Cosmos: "test_task_mailbox_lifecycle_with_callback", Node: "test_node", Element: "test_element", Atom: "test_atomos"}
	p := newTestCosmosProcessWithoutCluster(t, id.Cosmos, id.Node)
	tba := newTestTaskMailboxBaseAtomos(t, p, id)
	atomosGoID := tba.atomos.mailbox.goID
	t.Logf("task mailbox base atomos goID(%d)", atomosGoID)

	taskMailboxName := "test_task_mailbox_serial_life_cycle_with_callback"
	mb := createTaskMailbox(taskMailboxName, true, newTestLoggingAtomos(t))
	taskMailboxGoID := mb.mailbox.goID
	t.Logf("task mailbox info: %s on goID(%d)", mb.mailbox.name, taskMailboxGoID)

	goMap := dumpAllGoID()
	if _, exists := goMap[taskMailboxGoID]; !exists {
		t.Fatal("goID not found in goMap")
	}

	count := 5

	var wg sync.WaitGroup
	wg.Add(count)
	expectedIdx := uint64(1)
	tba.expectedCallback = 1
	tba.atomos.task.queueMap[mb.mailbox.name] = mb

	for i := 0; i < count; i++ {
		helper := &taskHelper{
			task: nil,
			taskWithCallback: func(taskID uint64) func() {
				if getGoID() != taskMailboxGoID {
					t.Fatal("task is not running in mailbox goID:", taskMailboxGoID)
				}
				return func() {
					if expectedIdx != taskID {
						t.Fatal("expected task id ", expectedIdx, " got ", taskID)
					}
					atomic.AddUint64(&expectedIdx, 1)
					if tba.expectedCallback != taskID {
						t.Fatal("expected callback id ", tba.expectedCallback, " got ", taskID)
					}
					if getGoID() != atomosGoID {
						t.Fatal("callback is not running in atomos goID:", atomosGoID)
					}
					atomic.AddUint64(&tba.expectedCallback, 1)
					wg.Done()
					t.Log("OK, task executed:", taskID)
				}
			},
			invalidArgs:    nil,
			conflictArgs:   nil,
			mark:           "",
			delay:          0,
			cronSchedule:   nil,
			appendToHead:   false,
			cancelCallback: nil,
			recoverFn:      nil,
			manager:        &tba.atomos.task,
			mailbox:        mb.mailbox,
			atomosTask:     nil,
		}
		helper.buildAtomosTask()

		am := allocBaseAtomosMail()
		initTaskQueueMail(am, "", helper)
		am.atomosTask.timerState = TaskMailing
		am.atomosTask.atomosMail = am

		mb.mailbox.pushTail(am.mail)
	}

	wg.Wait()

	if mb.mailbox.isRunning() {
		t.Fatal("Mailbox should be stopped after processing all mails.")
	}

	<-time.After(1 * time.Millisecond)
	goMap = dumpAllGoID()
	if _, exists := goMap[taskMailboxGoID]; exists {
		t.Fatal("Mailbox should be stopped after processing all mails.")
	}

	<-time.After(time.Millisecond)
}

func TestTaskMailbox_SerialCancelTask(t *testing.T) {
	id := &IDInfo{Type: IDType_Atom, Cosmos: "test_task_mailbox_cancel_task", Node: "test_node", Element: "test_element", Atom: "test_atomos"}
	p := newTestCosmosProcessWithoutCluster(t, id.Cosmos, id.Node)
	tba := newTestTaskMailboxBaseAtomos(t, p, id)
	atomosGoID := tba.atomos.mailbox.goID
	t.Logf("task mailbox base atomos goID(%d)", atomosGoID)

	taskMailboxName := "test_task_mailbox_serial_cancel_task"
	mb := createTaskMailbox(taskMailboxName, true, newTestLoggingAtomos(t))
	taskMailboxGoID := mb.mailbox.goID
	t.Logf("task mailbox info: %s on goID(%d)", mb.mailbox.name, taskMailboxGoID)

	cancelGoID := getGoID()

	goMap := dumpAllGoID()
	if _, exists := goMap[taskMailboxGoID]; !exists {
		t.Fatal("goID not found in goMap")
	}

	// Simulate Tasks:
	// 0. Add a task with 100ms delay, let those later task cancellation happen before execution.
	// 1. Cancel the task before it gets executed, check cancel callback is called.
	// 2. Add another task, let it execute normally.

	var wg, callbackWg sync.WaitGroup
	wg.Add(3)
	callbackWg.Add(3)

	tba.expectedCallback = 1
	tba.atomos.task.queueMap[mb.mailbox.name] = mb

	var cancelTask *atomosTask
	for i := 0; i < 3; i++ {
		tID := uint64(i + 1)
		helper := &taskHelper{
			task: nil,
			taskWithCallback: func(taskID uint64) (callback func()) {
				tba.executedMaps[taskID] = true
				if tID != taskID {
					t.Fatal("Unexpected task ID:", taskID)
				}
				if getGoID() != taskMailboxGoID {
					t.Fatal("task is not running in mailbox goID:", taskMailboxGoID)
				}
				switch taskID {
				case 1:
					time.After(100 * time.Millisecond)
					wg.Done()
				case 2:
					t.Fatal("Task 1 should have been cancelled")
				case 3:
					t.Logf("Task 2 ok")
					wg.Done()
				default:
					t.Fatal("Unexpected task ID:", taskID)
				}
				return func() {
					t.Logf("Task %d callback ok", taskID)
					if getGoID() != atomosGoID {
						t.Fatal("callback is not running in atomos goID:", atomosGoID)
					}
					tba.callbackMaps[taskID] = true
					callbackWg.Done()
				}
			},
			invalidArgs:  nil,
			conflictArgs: nil,
			mark:         "",
			delay:        0,
			cronSchedule: nil,
			appendToHead: false,
			cancelCallback: func(reason string) {
				t.Logf("Task %d cancelled because: %s", tID, reason)
				if getGoID() != cancelGoID {
					t.Fatal("cancel callback is not running in atomos goID:", atomosGoID)
				}
				tba.cancelledMaps[tID] = true
				wg.Done()
				callbackWg.Done()
			},
			recoverFn:  nil,
			manager:    &tba.atomos.task,
			mailbox:    mb.mailbox,
			atomosTask: nil,
		}
		helper.buildAtomosTask()

		am := allocBaseAtomosMail()
		initTaskQueueMail(am, "", helper)
		am.atomosTask.timerState = TaskMailing
		am.atomosTask.atomosMail = am

		mb.mailbox.pushTail(am.mail)
		if i == 1 {
			cancelTask = am.atomosTask
		}
	}

	// Cancel the task before it gets executed.
	if cancelTask == nil {
		t.Fatal("Cancel task is nil")
	}
	if err := tba.atomos.task.cancelTaskQueue(cancelTask, false, "test cancellation"); err != nil {
		t.Fatalf("Failed to cancel task: %v", err)
	}

	wg.Wait()
	callbackWg.Wait()

	if len(tba.cancelledMaps) == 1 && tba.cancelledMaps[2] {
		t.Log("Cancel callback executed as expected.")
	} else {
		t.Fatal("Cancel callback was not executed as expected.")
	}

	if len(tba.callbackMaps) == 2 && tba.callbackMaps[1] && tba.callbackMaps[3] {
		t.Log("Task callbacks executed as expected.")
	} else {
		t.Fatal("Task callbacks were not executed as expected.")
	}

	if len(tba.executedMaps) == 2 && tba.executedMaps[1] && tba.executedMaps[3] {
		t.Log("All tasks executed as expected.")
	} else {
		t.Fatal("Not all tasks were executed as expected.")
	}

	if mb.mailbox.isRunning() {
		t.Fatal("Mailbox should be stopped after processing all mails.")
	}

	<-time.After(1 * time.Millisecond)
	goMap = dumpAllGoID()
	if _, exists := goMap[mb.mailbox.goID]; exists {
		t.Fatal("Mailbox should be stopped after processing all mails.")
	}

	<-time.After(time.Millisecond)
}

// Utils for testing

type testTaskMailboxBaseAtomos struct {
	t      *testing.T
	atomos *BaseAtomos

	expectedCallback uint64

	cancelledMaps map[uint64]bool
	executedMaps  map[uint64]bool
	callbackMaps  map[uint64]bool
}

func newTestTaskMailboxBaseAtomos(t *testing.T, p *CosmosProcess, id *IDInfo) *testTaskMailboxBaseAtomos {
	tba := &testTaskMailboxBaseAtomos{}
	ba := NewBaseAtomos(nil, id, LogLevel_Debug, tba, tba, p)
	if err := ba.start(func() *Error {
		return nil
	}); err != nil {
		t.Fatalf("Failed to start BaseAtomos: %v", err)
	}
	tba.t = t
	tba.atomos = ba
	tba.cancelledMaps = make(map[uint64]bool)
	tba.executedMaps = make(map[uint64]bool)
	tba.callbackMaps = make(map[uint64]bool)
	return tba
}

// BaseAtomosHolder

func (tba *testTaskMailboxBaseAtomos) OnSyncMessaging(fromID ID, name string, in proto.Message) (out proto.Message, err *Error) {
	//TODO implement me
	panic("implement me")
}

func (tba *testTaskMailboxBaseAtomos) OnAsyncMessaging(fromID ID, name string, startupID, asyncID uint64, in proto.Message) {
	//TODO implement me
	panic("implement me")
}

func (tba *testTaskMailboxBaseAtomos) OnAsyncMessagingCallback(asyncID uint64, in proto.Message, err *Error) {
	//TODO implement me
	panic("implement me")
}

func (tba *testTaskMailboxBaseAtomos) OnFnCallback(callback *atomosCallback) {
	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					tba.t.Logf("Recovered in OnFnCallback recoverFn: %v\n%s", r2, string(debug.Stack()))
				}
			}()
			if f := callback.recoverFn; f != nil {
				f(r)
			} else {
				tba.t.Logf("Recovered in OnFnCallback: %v\n%s", r, string(debug.Stack()))
			}
		}
	}()
	callback.callback()
}

func (tba *testTaskMailboxBaseAtomos) OnWormhole(from ID, wormhole BaseAtomosWormhole) *Error {
	panic("implement me")
}

func (tba *testTaskMailboxBaseAtomos) OnStopping(from ID, cancelled []uint64) *Error {
	tba.t.Logf("OnStopping: from=(%v),cancelled=(%v)", from, cancelled)
	return nil
}

func (tba *testTaskMailboxBaseAtomos) OnIDsReleased() {
	panic("implement me")
}

// Atomos

func (tba *testTaskMailboxBaseAtomos) String() string {
	panic("implement me")
}

func (tba *testTaskMailboxBaseAtomos) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	panic("implement me")
}
