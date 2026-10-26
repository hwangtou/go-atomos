package atomos

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBaseAtomosStopTaskCallbacksAndCancelledIDs(t *testing.T) {
	id := &IDInfo{Type: IDType_Atom, Cosmos: "test_base_atomos_stop_task", Node: "test_node", Element: "test_element", Atom: "test_atomos"}
	p, cerr := newCosmosProcess(id.Cosmos, id.Node, &reproLogger{})
	if cerr != nil {
		t.Fatalf("newCosmosProcess failed: %v", cerr)
	}
	t.Cleanup(func() {
		if p.local.atomos.mailbox.isRunning() {
			_ = p.local.atomos.PushKillMail(p.local)
		}
		waitMailboxGoroutineExit(p.local.atomos.mailbox, 5*time.Second)
		if p.logging.logBox != nil && p.logging.logBox.isRunning() {
			p.logging.stop()
		}
		waitMailboxGoroutineExit(p.logging.logBox, 5*time.Second)
	})
	tba := newTestTaskMailboxBaseAtomos(t, p, id)
	ba := tba.atomos
	at := &ba.task

	runningStarted := make(chan struct{})
	runningRelease := make(chan struct{})
	runningReturned := make(chan struct{})
	var runningReleaseOnce sync.Once
	releaseRunning := func() { runningReleaseOnce.Do(func() { close(runningRelease) }) }
	defer releaseRunning()
	var runningCancelCalls atomic.Int32
	cancelRunning := at.AddToConcurrentQueue(func(taskID uint64) func() {
		close(runningStarted)
		<-runningRelease
		close(runningReturned)
		return nil
	}, ArgTaskCancelCallback(func(reason string) {
		runningCancelCalls.Add(1)
	}))
	<-runningStarted
	if err := cancelRunning("still running"); CodeOf(err) != ErrCodeAtomosTaskCannotCancelRunningTask {
		t.Fatalf("Cancel running task error = %v", err)
	}

	mainStarted := make(chan struct{})
	mainRelease := make(chan struct{})
	var mainReleaseOnce sync.Once
	releaseMain := func() { mainReleaseOnce.Do(func() { close(mainRelease) }) }
	defer releaseMain()
	mainID, err := at.Add(func(taskID uint64) {
		close(mainStarted)
		<-mainRelease
	})
	if err != nil {
		t.Fatalf("Add main blocker failed: %v", err)
	}
	<-mainStarted

	ordinaryID, err := at.Add(func(taskID uint64) {
		t.Errorf("Pending ordinary task %d executed", taskID)
	})
	if err != nil {
		t.Fatalf("Add ordinary task failed: %v", err)
	}
	afterID, err := at.AddAfter(time.Hour, func(taskID uint64) {
		t.Errorf("Scheduling task %d executed", taskID)
	})
	if err != nil {
		t.Fatalf("AddAfter failed: %v", err)
	}

	at.AddToAtomosQueue(func(taskID uint64) {
		t.Errorf("Pending queue task %d executed", taskID)
	}, ArgTaskCancelCallback(func(reason string) {
		panic("cancel callback panic")
	}))
	cancelCallbackReturned := make(chan struct{})
	at.AddToAtomosQueue(func(taskID uint64) {
		t.Errorf("Pending marked task %d executed", taskID)
	}, ArgTaskMark("pending"), ArgTaskCancelCallback(func(reason string) {
		_ = at.HasMarking("pending")
		_ = at.Cancel(afterID)
		close(cancelCallbackReturned)
	}))

	var cancelled []uint64
	var haltAPIErr error
	recordHaltError := func(err error) {
		if haltAPIErr == nil {
			haltAPIErr = err
		}
	}
	var rejectedQueueCallbacks atomic.Int32
	var rejectedTaskExecuted atomic.Bool
	rejectedWorker := func(taskID uint64) { rejectedTaskExecuted.Store(true) }
	rejectedWorkerWithCallback := func(taskID uint64) func() {
		rejectedTaskExecuted.Store(true)
		return nil
	}
	rejectedArg := func() ArgsForTask {
		return ArgTaskCancelCallback(func(reason string) { rejectedQueueCallbacks.Add(1) })
	}
	tba.stopHook = func(from ID, ids []uint64) *Error {
		cancelled = append(cancelled, ids...)
		if taskID, err := at.Add(rejectedWorker); taskID != 0 || CodeOf(err) != ErrCodeAtomosIsNotRunning {
			recordHaltError(fmt.Errorf("Add in Halt returned id=%d, err=%v", taskID, err))
		}
		if taskID, err := at.AddAfter(time.Second, rejectedWorker); taskID != 0 || CodeOf(err) != ErrCodeAtomosIsNotRunning {
			recordHaltError(fmt.Errorf("AddAfter in Halt returned id=%d, err=%v", taskID, err))
		}
		_ = at.Cancel(afterID)
		if at.HasMarking("pending") {
			recordHaltError(fmt.Errorf("cancelled marking remains visible in Halt"))
		}

		cancels := []func(string) error{
			at.AddToAtomosQueue(rejectedWorker, rejectedArg()),
			at.AddToSerialQueue("halt", rejectedWorkerWithCallback, rejectedArg()),
			at.AddToConcurrentQueue(rejectedWorkerWithCallback, rejectedArg()),
		}
		for i, cancel := range cancels {
			if cancel == nil {
				recordHaltError(fmt.Errorf("rejected queue %d returned nil cancel", i))
				continue
			}
			if err := cancel("late cancel"); err != nil {
				recordHaltError(fmt.Errorf("rejected queue %d cancel returned %v", i, err))
			}
		}
		return nil
	}

	killDone := make(chan error, 1)
	go func() {
		killDone <- ba.PushKillMail(p.local, ArgBaseAtomosAppendToHead(), ArgBaseAtomosWaitKilled(), ArgBaseAtomosTimeout(5*time.Second))
	}()
	deadline := time.Now().Add(5 * time.Second)
	for ba.mailbox.getByID(defaultMailID) == nil {
		if time.Now().After(deadline) {
			t.Fatal("Kill mail was not queued")
		}
		time.Sleep(time.Millisecond)
	}
	releaseMain()

	select {
	case err := <-killDone:
		if err != nil {
			t.Fatalf("WaitKilled kill failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitKilled kill blocked in stop callbacks or Halt")
	}
	if haltAPIErr != nil {
		t.Fatal(haltAPIErr)
	}
	if got := rejectedQueueCallbacks.Load(); got != 3 {
		t.Fatalf("Rejected queue cancel callbacks = %d, want 3", got)
	}
	if rejectedTaskExecuted.Load() {
		t.Fatal("Task admitted during Halt executed")
	}
	select {
	case <-cancelCallbackReturned:
	default:
		t.Fatal("Cancel callbacks after a panic did not all run")
	}
	if got := runningCancelCalls.Load(); got != 0 {
		t.Fatalf("Executing task received %d cancel callbacks", got)
	}

	want := map[uint64]bool{ordinaryID: true, afterID: true, 5: true, 6: true}
	if len(cancelled) != len(want) {
		t.Fatalf("Cancelled IDs = %v, want %v (mainID=%d, runningID=1)", cancelled, want, mainID)
	}
	seen := make(map[uint64]bool, len(cancelled))
	for _, taskID := range cancelled {
		if taskID == 0 || seen[taskID] || !want[taskID] {
			t.Fatalf("Invalid cancelled IDs %v, want %v", cancelled, want)
		}
		seen[taskID] = true
	}
	if ba.GetState() != BaseAtomosHalt || ba.mailbox.isRunning() {
		t.Fatalf("WaitKilled returned before complete halt: state=%s running=%v", ba.GetState(), ba.mailbox.isRunning())
	}

	releaseRunning()
	select {
	case <-runningReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("Executing concurrent task did not return")
	}
	waitMailboxGoroutineExit(ba.mailbox, 5*time.Second)
}
