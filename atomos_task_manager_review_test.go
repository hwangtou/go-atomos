package atomos

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestAtomosTaskManager_HasMarkingNilHelper(t *testing.T) {
	at := newTestAtomosTaskManager(t)
	at.mutex.Lock()
	at.tasks[1] = &atomosTask{id: 1, timerState: TaskMailing}
	at.mutex.Unlock()

	if at.HasMarking("missing") {
		t.Fatal("Task without helper must not have a marking")
	}

	at.mutex.Lock()
	delete(at.tasks, 1)
	at.mutex.Unlock()
}

func TestAtomosTaskManager_CronRunsIndependentTasks(t *testing.T) {
	at := newTestAtomosTaskManager(t)
	runs := make(chan uint64, 8)
	cancel := at.AddToAtomosQueue(func(taskID uint64) {
		select {
		case runs <- taskID:
		default:
		}
	}, ArgTaskLikeCrontab("@every 5ms"))

	seen := make(map[uint64]bool)
	for len(seen) < 3 {
		select {
		case id := <-runs:
			if seen[id] {
				t.Fatalf("Cron run reused task ID %d", id)
			}
			seen[id] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("Cron executed only %d rounds", len(seen))
		}
	}
	cancelTaskWithTimeout(t, cancel, "test complete")

	deadline := time.Now().Add(5 * time.Second)
	for {
		at.mutex.Lock()
		remaining := 0
		for id := range seen {
			if _, ok := at.tasks[id]; ok {
				remaining++
			}
		}
		at.mutex.Unlock()
		if remaining == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d completed cron runs remain registered", remaining)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAtomosTaskManager_CronCancelDoesNotDeadlock(t *testing.T) {
	for _, mailboxExit := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "mailboxExit"}[mailboxExit], func(t *testing.T) {
			at := newTestAtomosTaskManager(t)
			cancel := at.AddToAtomosQueue(func(taskID uint64) {
				t.Error("Cancelled cron task executed")
			}, ArgTaskLikeCrontab("@every 1h"))

			at.mutex.Lock()
			if len(at.tasks) != 1 {
				at.mutex.Unlock()
				t.Fatalf("Expected one cron template, got %d tasks", len(at.tasks))
			}
			var task *atomosTask
			for _, task = range at.tasks {
			}
			at.mutex.Unlock()

			if mailboxExit {
				done := make(chan *Error, 1)
				go func() {
					at.mutex.Lock()
					defer at.mutex.Unlock()
					done <- at.cancelTaskQueue(task, true, "mailbox exit")
				}()
				select {
				case err := <-done:
					if err != nil {
						t.Fatalf("Cancel cron during mailbox exit failed: %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("Cron cancellation deadlocked during mailbox exit")
				}
			} else {
				cancelTaskWithTimeout(t, cancel, "test cancel")
			}

			at.mutex.Lock()
			_, registered := at.tasks[task.id]
			state := task.timerState
			at.mutex.Unlock()
			if registered || state != TaskCancelled {
				t.Fatalf("Cancelled cron template remains registered=%v with state=%d", registered, state)
			}
		})
	}
}

func TestAtomosTaskManager_CancelMailingQueuedReleasesTask(t *testing.T) {
	at := newTestAtomosTaskManager(t)
	tmb := at.getTaskSerialMailbox("cancel-queued")
	blockerStarted := make(chan struct{})
	blocker := make(chan struct{})
	blockHelper := createTaskHelper(at, tmb.mailbox, func(taskID uint64) {
		close(blockerStarted)
		<-blocker
	}, nil, nil)
	blockHelper.buildAtomosTask()
	blockMail := allocBaseAtomosMail()
	initTaskQueueMail(blockMail, "blocker", blockHelper)
	at.addToQueue(tmb.mailbox, blockHelper.atomosTask)
	<-blockerStarted

	var executed atomic.Bool
	cancelled := make(chan string, 1)
	helper := createTaskHelper(at, tmb.mailbox, func(taskID uint64) {
		executed.Store(true)
	}, nil, []ArgsForTask{ArgTaskCancelCallback(func(reason string) {
		cancelled <- reason
	})})
	helper.buildAtomosTask()
	task := helper.atomosTask
	am := allocBaseAtomosMail()
	initTaskQueueMail(am, "target", helper)
	cancel := at.addToQueue(tmb.mailbox, task)

	if err := cancel("queued"); err != nil {
		t.Fatalf("Cancel queued task failed: %v", err)
	}
	if executed.Load() {
		t.Fatal("Cancelled queued task executed")
	}
	assertCancelledTaskReleased(t, at, task, helper)
	select {
	case reason := <-cancelled:
		if reason != "queued" {
			t.Fatalf("Unexpected cancel reason %q", reason)
		}
	default:
		t.Fatal("Cancel callback was not invoked")
	}

	close(blocker)
	waitMailboxStopped(t, tmb.mailbox, 5*time.Second)
}

func TestAtomosTaskManager_CancelMailingPoppedReleasesTask(t *testing.T) {
	at := newTestAtomosTaskManager(t)
	tmb := at.getTaskSerialMailbox("cancel-popped")
	var executed atomic.Bool
	cancelled := make(chan string, 1)
	helper := createTaskHelper(at, tmb.mailbox, func(taskID uint64) {
		executed.Store(true)
	}, nil, []ArgsForTask{ArgTaskCancelCallback(func(reason string) {
		cancelled <- reason
	})})
	helper.buildAtomosTask()
	task := helper.atomosTask
	am := allocBaseAtomosMail()
	initTaskQueueMail(am, "target", helper)

	at.mutex.Lock()
	if !tmb.mailbox.pushTail(am.mail) {
		at.mutex.Unlock()
		t.Fatal("Failed to queue target task")
	}
	deadline := time.Now().Add(5 * time.Second)
	for tmb.mailbox.getByID(task.id) != nil {
		if time.Now().After(deadline) {
			at.mutex.Unlock()
			t.Fatal("Mailbox did not pop target task")
		}
		time.Sleep(time.Millisecond)
	}
	if err := at.cancelTaskQueue(task, true, "popped"); err != nil {
		at.mutex.Unlock()
		t.Fatalf("Cancel popped task failed: %v", err)
	}
	at.mutex.Unlock()

	waitCancelledTaskReleased(t, at, task, helper)
	if executed.Load() {
		t.Fatal("Cancelled popped task executed")
	}
	select {
	case reason := <-cancelled:
		if reason != "popped" {
			t.Fatalf("Unexpected cancel reason %q", reason)
		}
	default:
		t.Fatal("Cancel callback was not invoked")
	}
	waitMailboxStopped(t, tmb.mailbox, 5*time.Second)
}

func cancelTaskWithTimeout(t *testing.T, cancel func(string) error, reason string) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cancel(reason) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Cancel task failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Task cancellation deadlocked")
	}
}

func assertCancelledTaskReleased(t *testing.T, at *atomosTaskManager, task *atomosTask, helper *taskHelper) {
	t.Helper()
	at.mutex.Lock()
	defer at.mutex.Unlock()
	if _, ok := at.tasks[task.id]; ok {
		t.Fatal("Cancelled task remains registered")
	}
	if task.helper != nil || task.atomosMail != nil || helper.atomosTask != nil {
		t.Fatal("Cancelled task retains helper or mail references")
	}
	if helper.task != nil || helper.taskWithCallback != nil || helper.cancelCallback != nil || helper.recoverFn != nil {
		t.Fatal("Cancelled task retains closures")
	}
}

func waitCancelledTaskReleased(t *testing.T, at *atomosTaskManager, task *atomosTask, helper *taskHelper) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		at.mutex.Lock()
		_, registered := at.tasks[task.id]
		released := task.helper == nil && task.atomosMail == nil && helper.atomosTask == nil &&
			helper.task == nil && helper.taskWithCallback == nil && helper.cancelCallback == nil && helper.recoverFn == nil
		at.mutex.Unlock()
		if !registered && released {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Cancelled popped task was not released: registered=%v released=%v", registered, released)
		}
		time.Sleep(time.Millisecond)
	}
}
