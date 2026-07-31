package atomos

import (
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func newTestAtomosTaskManager(t *testing.T) *atomosTaskManager {
	id := &IDInfo{Type: IDType_Atom, Cosmos: "test_atomos_task_manager", Node: "test_node", Element: "test_element", Atom: "test_atomos"}
	p := newTestCosmosProcessWithoutCluster(t, id.Cosmos, id.Node)
	tba := newTestTaskMailboxBaseAtomos(t, p, id)
	at := &atomosTaskManager{}
	initAtomosTasksManager(tba.atomos.log.logging, at, tba.atomos)

	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}
	return at
}

// waitTaskDone polls until the task reaches TaskDone and is removed from the
// tasks map. task.timerState and manager.tasks are guarded by the manager
// mutex in product code, so tests must read them under the same lock instead
// of racing the mailbox goroutine that finalizes the task after the user
// closure returns.
func waitTaskDone(t *testing.T, at *atomosTaskManager, task *atomosTask) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		at.mutex.Lock()
		state := task.timerState
		_, inMap := at.tasks[task.id]
		at.mutex.Unlock()
		if state == TaskDone && !inMap {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Task %d not done in time: state=(%d), inMap=(%v)", task.id, state, inMap)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAtomosTaskManager_LifeCycle(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	at := newTestAtomosTaskManager(t)
	if at.tasks == nil {
		t.Fatal("AtomosTaskManager tasks map is nil after creation.")
	}
	if at.queueMap == nil {
		t.Fatal("AtomosTaskManager queueMap is nil after creation.")
	}

	allGoID := dumpAllGoID()
	atomosGoID := at.atomos.mailbox.goID
	testFnGoID := getGoID()
	if !allGoID[testFnGoID] {
		t.Fatal("Test function GoID not found in dumpAllGoID.")
	}

	// Create Serial Task Mailbox
	stMbSuffix := "test_serial_queue"
	stMb := at.getTaskSerialMailbox(stMbSuffix)
	if stMb == nil {
		t.Fatal("Failed to create serial task mailbox.")
	}
	if stMb.mailbox.name != at.atomos.id.Info()+"::test_serial_queue" {
		t.Fatal("Serial task mailbox has incorrect name.")
	}
	if !stMb.serial {
		t.Fatal("Serial task mailbox is not marked as serial.")
	}
	if stMb.mailbox == nil {
		t.Fatal("Serial task mailbox's mailbox is nil.")
	}
	if len(at.queueMap) != 1 || at.queueMap[stMb.mailbox.name] == nil {
		t.Fatal("Serial task mailbox not stored in queueMap.")
	}
	if at.getTaskSerialMailbox(stMbSuffix) != stMb {
		t.Fatal("getTaskSerialMailbox did not return the existing mailbox.")
	}

	allGoID = dumpAllGoID()
	if !allGoID[stMb.mailbox.goID] {
		t.Fatal("Serial mailbox GoID not found in dumpAllGoID.")
	}
	t.Log("Serial Task GoID :", stMb.mailbox.goID)

	// Create Concurrent Task Mailbox
	ctMb := at.newTaskSerialMailboxForConcurrent()
	if ctMb == nil {
		t.Fatal("Failed to create concurrent task mailbox.")
	}
	if ctMb.serial {
		t.Fatal("Concurrent task mailbox is incorrectly marked as serial.")
	}
	if ctMb.mailbox == nil {
		t.Fatal("Concurrent task mailbox's mailbox is nil.")
	}
	if ctMb.mailbox.name != at.atomos.id.Info()+concurrentMailboxSuffix {
		t.Fatal("Concurrent task mailbox has incorrect name.")
	}
	if len(at.queueMap) != 1 || at.queueMap[ctMb.mailbox.name] != nil {
		t.Fatal("Concurrent task mailbox should not be stored in queueMap.")
	}

	allGoID = dumpAllGoID()
	if !allGoID[ctMb.mailbox.goID] {
		t.Fatal("Concurrent mailbox GoID not found in dumpAllGoID.")
	}
	t.Log("Concurrent Task GoID :", ctMb.mailbox.goID)

	// Test processing a task in the serial mailbox

	wait := make(chan struct{})

	helper := createTaskHelper(at, stMb.mailbox, nil, func(taskID uint64) (callback func()) {
		if taskID != 1 {
			t.Fatal("Task ID mismatch in serial mailbox task.")
		}
		if getGoID() != stMb.mailbox.goID {
			t.Fatal("Task is not running in the serial mailbox goroutine.", getGoID())
		}
		return func() {
			if getGoID() != atomosGoID {
				t.Fatal("Task callback is not running in the test function goroutine.")
			}
			t.Log("Serial mailbox task callback executed.")
			wait <- struct{}{}
		}
	}, nil)
	if helper.hasErrors() {
		t.Fatal("Error creating task helper for serial mailbox task.")
	}
	helper.buildAtomosTask()

	waitAllocMailDebugZero(t, time.Second)

	am := allocBaseAtomosMail()
	initTaskQueueMail(am, at.closureInfo(-1), helper)

	at.addToQueue(stMb.mailbox, helper.atomosTask)
	<-wait

	waitAllocMailDebugZero(t, time.Second)

	waitMailboxStopped(t, stMb.mailbox, 5*time.Second)
	allGoID = dumpAllGoID()
	if allGoID[stMb.mailbox.goID] {
		t.Fatal("Serial mailbox goroutine should have exited after processing all mails.")
	}

	// Test processing a task in the concurrent mailbox

	helper2 := createTaskHelper(at, ctMb.mailbox, nil, func(taskID uint64) (callback func()) {
		if taskID != 2 {
			t.Fatal("Task ID mismatch in concurrent mailbox task.")
		}
		if getGoID() != ctMb.mailbox.goID {
			t.Fatal("Task is not running in the concurrent mailbox goroutine.")
		}
		return func() {
			if getGoID() != atomosGoID {
				t.Fatal("Task callback is not running in the test function goroutine.", getGoID())
			}
			t.Log("Concurrent mailbox task callback executed.")
			wait <- struct{}{}
		}
	}, nil)
	helper2.buildAtomosTask()

	am2 := allocBaseAtomosMail()
	initTaskQueueMail(am2, at.closureInfo(-1), helper2)

	if getAllocMailDebugNum() != 1 {
		t.Fatal("Expected 1 allocated mail after initializing concurrent task queue mail, got ", getAllocMailDebugNum())
	}

	at.addToQueue(ctMb.mailbox, am2.atomosTask)
	<-wait

	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	if ctMb.mailbox.isRunning() {
		t.Fatal("Concurrent mailbox should be stopped after processing all mails.")
	}
	allGoID = dumpAllGoID()
	if allGoID[ctMb.mailbox.goID] {
		t.Fatal("Concurrent mailbox goroutine should have exited after processing all mails.")
	}

	<-time.After(time.Millisecond)
}

func TestAtomosTaskManager_HasMarking(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	at := newTestAtomosTaskManager(t)

	if at.HasMarking("") {
		t.Fatal("Expected HasMarking to return false for empty marking.")
	}

	wait := make(chan struct{})
	helper := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
		<-time.After(time.Millisecond)
		return func() {
			wait <- struct{}{}
		}
	}, []ArgsForTask{
		ArgTaskMark("test_marking"),
	})
	helper.buildAtomosTask()
	am := allocBaseAtomosMail()
	initTaskQueueMail(am, at.closureInfo(-1), helper)

	at.addToQueue(at.atomos.mailbox, helper.atomosTask)

	for {
		if getAllocMailDebugNum() != 1 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	if !at.HasMarking("test_marking") {
		t.Fatal("Expected HasMarking to return true for 'test_marking'.")
	}

	<-wait
	if at.HasMarking("test_marking") {
		t.Fatal("Expected HasMarking to return false for 'test_marking' after task completion.")
	}

	<-time.After(time.Millisecond)
}

func TestAtomosTaskManager_AddToAtomos_SmokeTest(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	at := newTestAtomosTaskManager(t)

	wait := make(chan struct{}, 1)
	mailbox := at.atomos.mailbox
	tCb := func(taskID uint64) func() {
		t.Logf("SmokeTest task executed with ID: %d", taskID)
		return func() {
			wait <- struct{}{}
		}
	}

	at.addToAtomos(mailbox, nil, tCb)

	if getAllocMailDebugNum() != 1 {
		t.Fatal("Expected 1 allocated mail after AddToAtomos, got ", getAllocMailDebugNum())
	}

	<-wait

	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	t.Log("SmokeTest task completed.")

	_, am, m := allocBaseAtomosKillMail()
	initAtomosKillMail(am, nil)
	at.atomos.mailbox.pushHead(m)

	if getAllocMailDebugNum() != 1 {
		t.Fatal("Expected 1 allocated mail after pushing kill mail, got ", getAllocMailDebugNum())
	}

	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	<-time.After(time.Millisecond)
}

func TestAtomosTaskManager_DelayAddToQueue(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	at := newTestAtomosTaskManager(t)

	wait := make(chan struct{})

	// Test normal delayed task
	helper := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
		t.Logf("Delayed task executed with ID: %d", taskID)
		wait <- struct{}{}
		return func() {}
	}, []ArgsForTask{ArgTaskDelay(1 * time.Nanosecond)})
	helper.buildAtomosTask()
	task1 := helper.atomosTask
	if task1.id != 1 {
		t.Fatalf("Expected first task ID to be 1, got %d", task1.id)
	}

	am := allocBaseAtomosMail()
	initTaskQueueMail(am, at.closureInfo(-1), helper)

	for {
		if getAllocMailDebugNum() != 1 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	at.delayAddToQueue(at.atomos.mailbox, task1)
	<-wait

	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	// Test canceled delayed task
	helper2 := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
		t.Fatalf("Canceled delayed task should not execute, but got ID: %d", taskID)
		return func() {}
	}, []ArgsForTask{ArgTaskDelay(1 * time.Nanosecond)})
	helper2.buildAtomosTask()
	task2 := helper2.atomosTask
	if task2.id != 2 {
		t.Fatalf("Expected second task ID to be 2, got %d", task2.id)
	}

	am2 := allocBaseAtomosMail()
	initTaskQueueMail(am2, at.closureInfo(-1), helper2)

	if getAllocMailDebugNum() != 1 {
		t.Fatal("Expected 1 allocated mail after initializing canceled delayed task queue mail, got ", getAllocMailDebugNum())
	}

	if am2.atomosTask.timerState != TaskScheduling {
		t.Fatal("Expected task timerState to be TaskScheduling before cancellation.")
	}
	if at.tasks[task2.id] != task2 {
		t.Fatal("Expected task2 to be registered in AtomosTaskManager tasks map.")
	}
	delete(at.tasks, task2.id)
	am2.atomosTask.timerState = TaskCancelled
	releaseMail(am2.mail)
	at.delayAddToQueue(at.atomos.mailbox, task2)

	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	<-time.After(time.Millisecond)
}

func TestAtomosTaskManager_AddToQueue(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	at := newTestAtomosTaskManager(t)

	i := 0
	var wait sync.WaitGroup
	doneList := make([]uint64, 0)

	// Test normal task
	for ; i < 5; i++ {
		i := i
		wait.Add(1)
		helper := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
			t.Logf("Task executed with ID: %d", taskID)
			doneList = append(doneList, taskID)
			return func() {
				wait.Done()
			}
		}, nil)
		helper.buildAtomosTask()
		task := helper.atomosTask
		if task.id != uint64(i+1) {
			t.Fatalf("Expected first task ID to be %d, got %d", uint64(i+1), task.id)
		}

		am := allocBaseAtomosMail()
		initTaskQueueMail(am, at.closureInfo(-1), helper)

		at.addToQueue(at.atomos.mailbox, task)

	}
	wait.Wait()
	if len(doneList) != 5 {
		t.Fatalf("Expected 5 tasks to be done, got %d", len(doneList))
	}
	for idx, taskID := range doneList {
		expectedID := uint64(idx + 1)
		if taskID != expectedID {
			t.Fatalf("Expected done task ID to be %d, got %d", expectedID, taskID)
		}
	}
	waitAllocMailDebugZero(t, time.Second)

	// Test pushToHead task.
	// Occupy the mailbox with a blocker task first, so all pushToHead tasks
	// queue up while the mailbox is blocked; after unblocking they execute in
	// strict LIFO order. Without the blocker the mailbox consumes tasks
	// concurrently with the pushes, making the execution order (and thus this
	// assertion) timing-dependent.
	blocker := make(chan struct{})
	blockerStarted := make(chan struct{})
	blockHelper := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {
		close(blockerStarted)
		<-blocker
	}, nil, nil)
	blockHelper.buildAtomosTask()
	blockTask := blockHelper.atomosTask
	if blockTask.id != 6 {
		t.Fatalf("Expected blocker task ID to be 6, got %d", blockTask.id)
	}
	amB := allocBaseAtomosMail()
	initTaskQueueMail(amB, at.closureInfo(-1), blockHelper)
	at.addToQueue(at.atomos.mailbox, blockTask)
	// Ensure the blocker is EXECUTING (occupying the mailbox) before any
	// pushToHead task is queued; otherwise the first head-push could land in
	// front of the blocker's mail and execute first, breaking the LIFO order.
	<-blockerStarted

	doneList = doneList[:0]
	for ; i < 10; i++ {
		i := i
		helper := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
			t.Logf("PushToHead Task executed with ID: %d", taskID)
			doneList = append(doneList, taskID)
			return func() {
				wait.Done()
			}
		}, []ArgsForTask{
			ArgTaskAppendToHead(),
		})
		helper.buildAtomosTask()
		task := helper.atomosTask
		if task.id != uint64(i+2) { // +1 for 1-based ids, +1 for the blocker task
			t.Fatalf("Expected pushToHead task ID to be %d, got %d", uint64(i+2), task.id)
		}

		am := allocBaseAtomosMail()
		initTaskQueueMail(am, at.closureInfo(-1), helper)

		wait.Add(1)
		at.addToQueue(at.atomos.mailbox, task)
	}
	close(blocker)
	wait.Wait()
	if len(doneList) != 5 {
		t.Fatalf("Expected 5 pushToHead tasks to be done, got %d", len(doneList))
	}
	for idx, taskID := range doneList {
		expectedID := uint64(11 - idx) // strict LIFO: 11, 10, 9, 8, 7
		if taskID != expectedID {
			t.Fatalf("Expected pushToHead tasks to execute in strict LIFO order [11 10 9 8 7], but got %v", doneList)
		}
	}
	waitAllocMailDebugZero(t, time.Second)
	t.Log("All pushToHead tasks completed successfully.")

	<-time.After(time.Millisecond)
}

func TestAtomosTaskManager_AddToQueue_KillInMid(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	at := newTestAtomosTaskManager(t)

	// Test adding tasks then killing the mailbox
	// send 11 normal task mails, send 1 kill mail during the 11th mail, send 2 normal task mails, send 2 delayed task mails
	var wait, timerWg sync.WaitGroup
	waitExit := make(chan struct{})
	for i := 0; i < 15; i++ {
		i := i
		wait.Add(1)
		args := []ArgsForTask{
			ArgTaskCancelCallback(func(reason string) {
				if i == 10 {
					t.Fatal("Task should not be canceled, but cancel callback executed. Reason:", i+1, reason)
				} else {
					t.Log("Task should be canceled, cancel callback executed. Reason:", i+1, reason)
					wait.Done()
				}
			}),
		}
		if i > 12 {
			// Last two tasks have delay to ensure they don't execute before kill mail is processed
			args = append(args, ArgTaskDelay((100+time.Duration(i))*time.Millisecond))
		}
		helper := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
			if i < 10 {
			} else if i == 10 {
				t.Log("Task triggering kill mail with ID before: ", taskID)
				waitExit <- struct{}{}
				<-time.After(100 * time.Millisecond)
				t.Log("Task triggering kill mail with ID after: ", taskID)
			} else {
				t.Fatalf("Task should not execute after mailbox is killed, but got ID: %d", taskID)
			}
			return func() {
				if i <= 10 {
					t.Log("Task completed callback with ID: ", taskID)
					wait.Done()
				} else {
					t.Fatalf("Task should not execute after mailbox is killed, but got ID: %d", taskID)
				}
			}
		}, args)
		helper.buildAtomosTask()
		task := helper.atomosTask
		if task.id != uint64(i+1) {
			t.Fatalf("Expected task ID to be %d, got %d", uint64(i+1), task.id)
		}

		am := allocBaseAtomosMail()
		initTaskQueueMail(am, at.closureInfo(-1), helper)

		if getAllocMailDebugNum() == 0 {
			t.Fatal("Expected 0 allocated mails, but got ", getAllocMailDebugNum())
		}

		// #11 is normal task that triggers kill mail
		// #12 #13 are normal task
		// #14 #15 tasks are delayed
		if i > 12 {
			timerWg.Add(1)
			time.AfterFunc(task.helper.delay, func() {
				t.Logf("Adding task with ID which is delayed task after killing mail with ID=(%d), i=(%d), delay=(%v)", task.id, i, task.helper.delay)
				at.delayAddToQueue(at.atomos.mailbox, task)
				timerWg.Done()
			})
		} else {
			t.Logf("Adding task with ID: %d", task.id)
			at.addToQueue(at.atomos.mailbox, task)
		}
	}

	<-waitExit

	_, am, m := allocBaseAtomosKillMail()
	initAtomosKillMail(am, nil)

	t.Log("Adding kill mail to mailbox.")
	if ok := at.atomos.mailbox.pushHead(m); !ok {
		log.Fatal("Failed to push kill mail to mailbox.")
	}

	wait.Wait()
	timerWg.Wait()

	waitAllocMailDebugZero(t, time.Second)

	<-time.After(time.Millisecond)
}

func TestAtomosTaskManager_HandleTaskQueue(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	at := newTestAtomosTaskManager(t)

	wait := make(chan struct{})

	// Task1 is a normal task with no callback that should execute successfully.
	helper1 := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {
		t.Logf("HandleTaskQueue executed task1 with ID: %d", taskID)
		wait <- struct{}{}
	}, nil, nil)
	helper1.buildAtomosTask()
	task1 := helper1.atomosTask
	if task1.id != 1 {
		t.Fatalf("Expected first task1 ID to be 1, got %d", task1.id)
	}

	am := allocBaseAtomosMail()
	initTaskQueueMail(am, at.closureInfo(-1), helper1)

	at.addToQueue(at.atomos.mailbox, task1)
	<-wait
	waitTaskDone(t, at, task1)
	waitAllocMailDebugZero(t, time.Second)

	// Task2 is a normal task with a callback that should execute successfully.
	helper2 := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
		t.Logf("HandleTaskQueue executed task2 with ID: %d", taskID)
		return func() {
			t.Log("task2 callback executed.")
			wait <- struct{}{}
		}
	}, nil)
	helper2.buildAtomosTask()
	task2 := helper2.atomosTask
	if task2.id != 2 {
		t.Fatalf("Expected second task2 ID to be 2, got %d", task2.id)
	}

	am2 := allocBaseAtomosMail()
	initTaskQueueMail(am2, at.closureInfo(-1), helper2)

	if getAllocMailDebugNum() != 1 {
		t.Fatal("Expected 1 allocated mail after initializing task2 queue mail, got ", getAllocMailDebugNum())
	}

	at.addToQueue(at.atomos.mailbox, task2)
	<-wait
	waitTaskDone(t, at, task2)
	waitAllocMailDebugZero(t, time.Second)

	// Task3 is a normal task with a nil callback that should execute successfully.
	helper3 := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
		t.Logf("HandleTaskQueue executed task2 with ID: %d", taskID)
		wait <- struct{}{}
		return nil
	}, nil)
	helper3.buildAtomosTask()
	task3 := helper3.atomosTask
	if task3.id != 3 {
		t.Fatalf("Expected third task3 ID to be 3, got %d", task3.id)
	}

	am3 := allocBaseAtomosMail()
	initTaskQueueMail(am3, at.closureInfo(-1), helper3)

	if getAllocMailDebugNum() != 1 {
		t.Fatal("Expected 1 allocated mail after initializing task3 queue mail, got ", getAllocMailDebugNum())
	}

	at.addToQueue(at.atomos.mailbox, task3)
	<-wait
	waitTaskDone(t, at, task3)
	waitAllocMailDebugZero(t, time.Second)

	// Task4 is a task that panics during execution.
	helper4 := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
		panic("test panic")
	}, []ArgsForTask{
		ArgTaskRecoverFunc(func(r any) {
			t.Log("recovered from panic", r)
			wait <- struct{}{}
		}),
	})
	helper4.buildAtomosTask()
	task4 := helper4.atomosTask
	if task4.id != 4 {
		t.Fatalf("Expected fourth task4 ID to be 4, got %d", task4.id)
	}

	am4 := allocBaseAtomosMail()
	initTaskQueueMail(am4, at.closureInfo(-1), helper4)

	if getAllocMailDebugNum() != 1 {
		t.Fatal("Expected 1 allocated mail after initializing task4 queue mail, got ", getAllocMailDebugNum())
	}

	at.addToQueue(at.atomos.mailbox, task4)
	<-wait

	waitAllocMailDebugZero(t, time.Second)

	// Task5 is a task that callback panics during execution.
	helper5 := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
		return func() {
			panic("test callback panic")
		}
	}, []ArgsForTask{
		ArgTaskRecoverFunc(func(r any) {
			t.Log("recovered from callback panic", r)
			wait <- struct{}{}
		}),
	})
	helper5.buildAtomosTask()
	task5 := helper5.atomosTask
	if task5.id != 5 {
		t.Fatalf("Expected fifth task5 ID to be 5, got %d", task5.id)
	}

	am5 := allocBaseAtomosMail()
	initTaskQueueMail(am5, at.closureInfo(-1), helper5)

	if getAllocMailDebugNum() != 1 {
		t.Fatal("Expected 1 allocated mail after initializing task5 queue mail, got ", getAllocMailDebugNum())
	}

	at.addToQueue(at.atomos.mailbox, task5)
	<-wait

	waitAllocMailDebugZero(t, time.Second)

	<-time.After(time.Millisecond)

	// Test canceled task does not execute
}

func TestAtomosTaskManager_CancelTaskQueue_CancelSchedulingTask(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	at := newTestAtomosTaskManager(t)

	// Test canceling a scheduled task
	state := 0
	wait := make(chan struct{}, 1) // buffered to avoid goroutine deadlock because callback runs in test goroutine
	helper := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
		t.Fatalf("Canceled task should not execute, but got ID: %d", taskID)
		return func() {
			t.Fatal("Canceled task should not execute, but got ID:", taskID)
		}
	}, []ArgsForTask{
		ArgTaskDelay(10 * time.Millisecond),
		ArgTaskCancelCallback(func(reason string) {
			t.Logf("Canceled task callback executed. Reason: %s", reason)
			state = 1
			wait <- struct{}{}
		}),
	})
	helper.buildAtomosTask()
	task := helper.atomosTask
	if task.id != 1 {
		t.Fatalf("Expected first task ID to be 1, got %d", task.id)
	}
	if at.tasks[task.id] == nil {
		t.Fatal("Expected task to be registered in AtomosTaskManager tasks map after building.")
	}

	am := allocBaseAtomosMail()
	initTaskQueueMail(am, at.closureInfo(-1), helper)

	if getAllocMailDebugNum() != 1 {
		t.Fatal("Expected 1 allocated mail after initializing scheduled task queue mail, got ", getAllocMailDebugNum())
	}

	if am.atomosTask.timerState != TaskScheduling {
		t.Fatal("Expected task timerState to be TaskScheduling before cancellation.")
	}
	if at.tasks[task.id] != task {
		t.Fatal("Expected task to be registered in AtomosTaskManager tasks map.")
	}

	task.timer = time.AfterFunc(helper.delay, func() {
		at.delayAddToQueue(at.atomos.mailbox, task)
	})
	if err := at.cancelTaskQueue(task, false, "test cancel"); err != nil {
		t.Fatalf("Failed to cancel task: %v", err)
	}

	if task.timer.Stop() {
		t.Fatal("Expected task timer to be stopped after cancellation.")
	}
	if at.tasks[task.id] != nil {
		t.Fatal("Expected task to be removed from AtomosTaskManager tasks map after cancellation.")
	}
	if task.timerState != TaskCancelled {
		t.Fatal("Expected task timerState to be TaskCancelled after cancellation.")
	}

	// canceling again should be no-op
	if err := at.cancelTaskQueue(task, false, "test cancel again"); err == nil {
		t.Fatalf("Canceling an already canceled task should be no-op, but got error: %v", err)
	} else if err.Code != ErrAtomosTaskCannotCancelCancelledTask {
		t.Fatalf("Expected ErrAtomosTaskCannotCancelNotScheduled when canceling already canceled task, got: %v", err)
	}

	<-wait
	if task.timerState != TaskCancelled {
		t.Fatal("Expected task timerState to remain TaskCancelled after cancel callback.")
	}
	if at.tasks[task.id] != nil {
		t.Fatal("Expected task to remain removed from AtomosTaskManager tasks map after cancel callback.")
	}
	if state != 1 {
		t.Fatalf("Expected state to be 1 after cancel callback, got %d", state)
	}
	waitAllocMailDebugZero(t, time.Second)

	<-time.After(time.Millisecond)
}

func TestAtomosTaskManager_CancelTaskQueue_CancelMailingTask(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	at := newTestAtomosTaskManager(t)

	// Occupy the mailbox with a blocking task so that delayed tasks queue up in
	// TaskMailing state instead of being consumed immediately. This makes the
	// "cancel a mailing (queued) task" scenario deterministic: without the
	// blocker, the mailbox may execute task2 before cancelTaskQueue runs.
	blocker := make(chan struct{})
	var unblockOnce sync.Once
	unblock := func() { unblockOnce.Do(func() { close(blocker) }) }
	defer unblock()
	helper0 := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {
		<-blocker
	}, nil, nil)
	helper0.buildAtomosTask()
	blockTask := helper0.atomosTask
	am0 := allocBaseAtomosMail()
	initTaskQueueMail(am0, at.closureInfo(-1), helper0)
	at.addToQueue(at.atomos.mailbox, blockTask)

	// Test canceling a mailing task.
	// First mail with 100ms delay, second mail to cancel it in 10ms.
	var state atomic.Int32
	wait1 := make(chan struct{})
	helper1 := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
		t.Log("Executing first mailing", taskID)
		state.Store(1)
		wait1 <- struct{}{}
		<-time.After(time.Millisecond * 500)
		return func() {
			t.Log("Executing first mail done", taskID)
			state.Store(2)
			wait1 <- struct{}{}
		}
	}, []ArgsForTask{
		ArgTaskDelay(100 * time.Millisecond),
		ArgTaskCancelCallback(func(reason string) {
			t.Error("First mailing task should not be canceled, but cancel callback executed. Reason:", reason)
		}),
	})
	helper1.buildAtomosTask()
	task1 := helper1.atomosTask
	if task1.id != 2 {
		t.Fatalf("Expected second task ID to be 2, got %d", task1.id)
	}
	if at.tasks[task1.id] == nil {
		t.Fatal("Expected task1 to be registered in AtomosTaskManager tasks map after building.")
	}

	am1 := allocBaseAtomosMail()
	initTaskQueueMail(am1, at.closureInfo(-1), helper1)

	if getAllocMailDebugNum() != 2 {
		t.Fatal("Expected 2 allocated mails after initializing first mailing task queue mail, got ", getAllocMailDebugNum())
	}

	task1.timer = time.AfterFunc(helper1.delay, func() {
		at.delayAddToQueue(at.atomos.mailbox, task1)
	})

	// Second mail
	var cancelState atomic.Int32
	helper2 := createTaskHelper(at, at.atomos.mailbox, nil, func(taskID uint64) func() {
		t.Error("Canceled task should not execute, but got ID:", taskID)
		return func() {
			t.Error("Canceled task should not execute, but got ID:", taskID)
		}
	}, []ArgsForTask{
		ArgTaskDelay(100 * time.Millisecond),
		ArgTaskCancelCallback(func(reason string) {
			cancelState.Store(1)
			t.Log("Canceled mailing task callback executed. Reason:", reason)
		}),
	})
	helper2.buildAtomosTask()
	task2 := helper2.atomosTask
	if task2.id != 3 {
		t.Fatalf("Expected third task ID to be 3, got %d", task2.id)
	}

	am2 := allocBaseAtomosMail()
	initTaskQueueMail(am2, at.closureInfo(-1), helper2)

	if getAllocMailDebugNum() != 3 {
		t.Fatal("Expected 3 allocated mails after initializing second mailing task queue mail, got ", getAllocMailDebugNum())
	}

	// Check the pre-timer state BEFORE starting the timer: after time.AfterFunc
	// starts, delayAddToQueue mutates timerState/tasks under the manager mutex
	// from another goroutine, so an unguarded read here would race with it.
	at.mutex.Lock()
	if task2.timerState != TaskScheduling {
		t.Fatal("Expected task2 timerState to be TaskScheduling before cancellation.")
	}
	if at.tasks[task2.id] != task2 {
		t.Fatal("Expected task2 to be registered in AtomosTaskManager tasks map.")
	}
	at.mutex.Unlock()

	wait2 := make(chan struct{})
	task2.timer = time.AfterFunc(time.Millisecond, func() {
		at.delayAddToQueue(at.atomos.mailbox, task2)
		wait2 <- struct{}{}
	})
	<-wait2
	at.mutex.Lock()
	if task2.timerState != TaskMailing {
		t.Fatal("Expected task2 timerState to be TaskMailing after timer fired.")
	}
	if at.tasks[task2.id] != task2 {
		t.Fatal("Expected task2 to be still registered in AtomosTaskManager tasks map.")
	}
	at.mutex.Unlock()

	if state.Load() != 0 {
		t.Fatalf("Expected state to be 0 before cancellation, got %d", state.Load())
	}
	if err := at.cancelTaskQueue(task2, false, "test cancel mailing"); err != nil {
		t.Fatalf("Failed to cancel mailing task: %v", err)
	}
	if state.Load() != 0 {
		t.Fatalf("Expected state to remain 0 after cancellation, got %d", state.Load())
	}
	if cancelState.Load() != 1 {
		t.Fatalf("Expected cancelState to be 1 after cancel callback, got %d", cancelState.Load())
	}

	// Release the mailbox so the first (delayed) task can execute.
	unblock()

	// Testing cancel executing mailing task
	<-wait1
	if state.Load() != 1 {
		t.Fatalf("Expected state to be 1 after first mailing started, got %d", state.Load())
	}
	if task1.timerState != TaskExecuting {
		t.Fatal("Expected task1 timerState to be TaskExecuting during execution.")
	}
	if at.tasks[task1.id] != task1 {
		t.Fatal("Expected task1 to be removed from AtomosTaskManager tasks map during execution.")
	}
	if err := at.cancelTaskQueue(task1, false, "test cancel executing mailing"); err == nil {
		t.Fatalf("Failed to cancel executing mailing task: %v", err)
	} else if err.Code != ErrAtomosTaskCannotCancelRunningTask {
		t.Fatalf("Expected ErrAtomosTaskCannotCancelRunningTask when canceling executing task, got: %v", err)
	}

	<-wait1
	if state.Load() != 2 {
		t.Fatalf("Expected state to be 2 after first mailing completed, got %d", state.Load())
	}
	waitTaskDone(t, at, task1)
	if err := at.cancelTaskQueue(task1, false, "test cancel done mailing"); err == nil {
		t.Fatalf("Canceling a done task should fail, but got no error.")
	} else if err.Code != ErrAtomosTaskCannotCancelDoneTask {
		t.Fatalf("Expected ErrAtomosTaskCannotCancelDoneTask when canceling done task, got: %v", err)
	}

	waitAllocMailDebugZero(t, time.Second)
}

func TestAtomosTaskManager_GetCron(t *testing.T) {
	at := newTestAtomosTaskManager(t)
	if at.cron != nil {
		t.Fatal("Expected nil cron scheduler before getCron call.")
	}

	cron1 := at.getCron()
	if cron1 == nil {
		t.Fatal("Expected non-nil cron scheduler on first getCron call.")
	}
	cron2 := at.getCron()
	if cron1 != cron2 {
		t.Fatal("Expected same cron scheduler instance on subsequent getCron calls.")
	}

	// Create an every-second cron schedule to verify functionality
	schedule, er := cron.ParseStandard("@every 1s")
	if er != nil {
		t.Fatalf("Failed to parse cron schedule: %v", er)
	}
	// Execute 3 times
	times := 3
	now := time.Now()
	wg := sync.WaitGroup{}
	wg.Add(times)
	eID := cron1.Schedule(schedule, cron.FuncJob(func() {
		t.Log("Cron job executed.")
		wg.Done()
	}))
	wg.Wait()
	gap := time.Since(now)
	if gap < time.Duration(times-1)*time.Second || gap > time.Duration(times)*time.Second {
		t.Fatalf("Cron job did not execute expected number of times in expected duration, took %v", gap)
	}
	t.Logf("Cron job executed %d times over %v.", times, gap)

	cron1.Remove(eID)
	<-time.After(2 * time.Second)
	// If the removed job still runs, the test will hang here.
	t.Log("Cron job removed successfully, no further executions.")

	<-time.After(time.Millisecond)
}

func TestAtomosTaskManager_GenTaskID(t *testing.T) {
	at := newTestAtomosTaskManager(t)
	task1 := at.genTaskID(true)
	if task1.id != 1 {
		t.Fatalf("Expected first task ID to be 1, got %d", task1.id)
	}
	if task1.timerState != TaskScheduling {
		t.Fatalf("Expected first task timerState to be TaskScheduling, got %d", task1.timerState)
	}
	if len(at.tasks) != 1 || at.tasks[1] != task1 {
		t.Fatalf("Expected concurrent mailbox to be created for first task.")
	}

	task2 := at.genTaskID(false)
	if task2.id != 2 {
		t.Fatalf("Expected second task ID to be 2, got %d", task2.id)
	}
	if task2.timerState != TaskMailing {
		t.Fatalf("Expected second task timerState to be TaskNotScheduled, got %d", task2.timerState)
	}
	if len(at.tasks) != 2 || at.tasks[2] != task2 {
		t.Fatalf("Expected no concurrent mailbox to be created for second task.")
	}

	task3 := at.genTaskID(true)
	if task3.id != 3 {
		t.Fatalf("Expected third task ID to be 3, got %d", task3.id)
	}
	if task3.timerState != TaskScheduling {
		t.Fatalf("Expected third task timerState to be TaskScheduling, got %d", task3.timerState)
	}
	if len(at.tasks) != 3 || at.tasks[3] != task3 {
		t.Fatalf("Expected concurrent mailbox to be created for third task.")
	}

	<-time.After(time.Millisecond)
}

func TestAtomosTaskManager_TaskPanicRecovery(t *testing.T) {
	// TODO
}

// taskHelper

func TestTaskHelper_CreateTaskHelper(t *testing.T) {
	at := newTestAtomosTaskManager(t)

	helper1 := createTaskHelper(at, at.atomos.mailbox, nil, nil, nil)
	if !helper1.hasErrors() {
		t.Fatal("Expected error when creating TaskHelper with nil task function.")
	} else if len(helper1.invalidArgs) != 1 || helper1.invalidArgs[0].idx != -1 || helper1.invalidArgs[0].reason != "Task is nil" {
		t.Fatal("Expected invalidArgs to contain one entry for nil task function.")
	}

	helper2 := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {}, func(taskID uint64) func() { return nil }, nil)
	if !helper2.hasErrors() {
		t.Fatal("Did not expect error when creating TaskHelper with valid task and callback functions.")
	} else if len(helper2.invalidArgs) != 1 || helper2.invalidArgs[0].idx != -1 || helper2.invalidArgs[0].reason != "Both TaskFn and TaskFnWithCallback are provided" {
		t.Fatal("Expected invalidArgs to contain one entry for nil callback function.")
	}

	helper3 := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {}, nil, []ArgsForTask{
		ArgTaskDelay(-1 * time.Second),
	})
	if !helper3.hasErrors() {
		t.Fatal("Expected error when creating TaskHelper with negative delay.")
	} else if len(helper3.invalidArgs) != 1 || helper3.invalidArgs[0].idx != 0 || helper3.invalidArgs[0].reason != "Invalid delay argument" {
		t.Fatal("Expected invalidArgs to contain one entry for negative delay.")
	}

	helper4 := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {}, nil, []ArgsForTask{
		ArgTaskLikeCrontab("invalid cron"),
	})
	if !helper4.hasErrors() {
		t.Fatal("Expected error when creating TaskHelper with invalid cron expression.")
	} else if len(helper4.invalidArgs) != 1 || helper4.invalidArgs[0].idx != 0 || !strings.HasPrefix(helper4.invalidArgs[0].reason, "Invalid crontab format:") {
		t.Fatal("Expected invalidArgs to contain one entry for invalid cron expression.")
	}

	helper5 := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {}, nil, []ArgsForTask{
		ArgTaskLikeCrontab("*/5 * * * *"),
	})
	if helper5.hasErrors() {
		t.Fatal("Did not expect error when creating TaskHelper with valid cron expression.")
	} else if helper5.cronSchedule == nil {
		t.Fatal("Expected cronSpec to be set in TaskHelper.")
	}

	helper6 := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {}, nil, []ArgsForTask{
		ArgTaskAppendToHead(),
	})
	if helper6.hasErrors() {
		t.Fatal("Did not expect error when creating TaskHelper with valid append to head argument.")
	} else if !helper6.appendToHead {
		t.Fatal("Expected appendToHead to be true in TaskHelper.")
	}

	helper7 := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {}, nil, []ArgsForTask{
		ArgTaskCancelCallback(nil),
	})
	if !helper7.hasErrors() {
		t.Fatal("Did not expect error when creating TaskHelper with nil cancel callback argument.")
	} else if len(helper7.invalidArgs) != 1 || helper7.invalidArgs[0].idx != 0 || helper7.invalidArgs[0].reason != "Nil TaskCancelCallback argument" {
		t.Fatal("Expected invalidArgs to contain one entry for nil cancel callback.")
	}
	helper7OK := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {}, nil, []ArgsForTask{
		ArgTaskCancelCallback(func(reason string) {}),
	})
	if helper7OK.hasErrors() {
		t.Fatal("Did not expect error when creating TaskHelper with valid cancel callback argument.")
	}

	helper8 := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {}, nil, []ArgsForTask{
		ArgTaskRecoverFunc(nil),
	})
	if !helper8.hasErrors() {
		t.Fatal("Did not expect error when creating TaskHelper with nil recover function argument.")
	} else if len(helper8.invalidArgs) != 1 || helper8.invalidArgs[0].idx != 0 || helper8.invalidArgs[0].reason != "Nil TaskRecoverFunc argument" {
		t.Fatal("Expected invalidArgs to contain one entry for nil recover function.")
	}
	helper8OK := createTaskHelper(at, at.atomos.mailbox, func(taskID uint64) {}, nil, []ArgsForTask{
		ArgTaskRecoverFunc(func(r any) {}),
	})
	if helper8OK.hasErrors() {
		t.Fatal("Did not expect error when creating TaskHelper with valid recover function argument.")
	}

	<-time.After(time.Millisecond)
}
