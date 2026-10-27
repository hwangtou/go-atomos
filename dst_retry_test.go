package atomos

import (
	"errors"
	"testing"
	"time"

	"github.com/hwangtou/go-atomos/internal/testclock"
)

func TestDST_RetryBackoff(t *testing.T) {
	clock := testclock.NewVirtualClock()
	retryable := errors.New("retryable")
	manager := newAsyncRetryManager(clock, func(err error) bool { return errors.Is(err, retryable) }, nil)
	defer manager.close()
	attempts := make(chan int, 2)
	terminal := make(chan error, 1)
	if err := manager.submit("target", "message", func(attempt int) error {
		attempts <- attempt
		if attempt == 1 {
			return retryable
		}
		return nil
	}, func(err error) { terminal <- err }); err != nil {
		t.Fatalf("submit: %v", err)
	}
	clock.RunUntilBlocked()
	if attempt := receiveDSTValue(t, attempts, "first retry attempt did not run"); attempt != 1 {
		t.Fatalf("first attempt = %d, want 1", attempt)
	}
	if clock.PendingTimers() != 1 {
		t.Fatalf("pending retry timers = %d, want 1", clock.PendingTimers())
	}
	select {
	case attempt := <-attempts:
		t.Fatalf("retry attempt %d ran without Advance", attempt)
	default:
	}
	clock.Advance(100*time.Millisecond - time.Nanosecond)
	select {
	case attempt := <-attempts:
		t.Fatalf("retry attempt %d ran before backoff elapsed", attempt)
	default:
	}
	clock.Advance(time.Nanosecond)
	if attempt := receiveDSTValue(t, attempts, "second retry attempt did not run"); attempt != 2 {
		t.Fatalf("retry attempt = %d, want 2", attempt)
	}
	if err := receiveDSTValue(t, terminal, "retry did not report its terminal result"); err != nil {
		t.Fatalf("terminal result: %v", err)
	}
}

func TestDST_DrainDeadline(t *testing.T) {
	clock := testclock.NewVirtualClock()
	process, err := newCosmosProcess("dstDrain", "dstNode", newTestLogging(t))
	if err != nil {
		t.Fatalf("new process: %v", err)
	}
	runnable := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: "dstDrain", Node: "dstNode"}).SetClock(clock)
	if err := process.Start(runnable); err != nil {
		t.Fatalf("start process: %v", err)
	}
	t.Cleanup(func() { _ = process.Stop() })
	if process.getClock() != clock {
		t.Fatal("process did not retain the injected clock")
	}
	process.mutex.Lock()
	process.atomSpawnPending = 1
	process.atomSpawnDrained = make(chan struct{})
	process.mutex.Unlock()
	if err := process.Drain(3 * time.Second); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	waitForDST(t, func() bool { return clock.PendingTimers() == 2 }, "drain timers were not registered")
	if !process.IsHealthy() {
		t.Fatal("process exited without Advance")
	}
	clock.Advance(3*time.Second - time.Nanosecond)
	if !process.IsHealthy() {
		t.Fatal("process exited before drain deadline")
	}
	clock.Advance(time.Nanosecond)
	waitForDST(t, func() bool { return !process.IsHealthy() }, "process did not exit at drain deadline")
	waitDSTMailboxExit(t, process.local.atomos.mailbox)
	waitDSTMailboxExit(t, process.logging.logBox)
	process.mutex.Lock()
	state := process.state
	process.mutex.Unlock()
	if state != CosmosProcessStateOff {
		t.Fatalf("process state after Stop = %v, want Off", state)
	}
}

func receiveDSTValue[T any](t *testing.T, ch <-chan T, message string) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal(message)
		var zero T
		return zero
	}
}

func waitDSTMailboxExit(t *testing.T, mailbox *mailBox) {
	t.Helper()
	waitMailboxGoroutineExit(mailbox, 3*time.Second)
	if mailbox.goID != 0 && dumpAllGoID()[mailbox.goID] {
		t.Fatalf("%s goroutine did not exit", mailbox.name)
	}
}

func waitForDST(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(time.Millisecond)
	}
}
