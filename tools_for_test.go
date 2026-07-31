package atomos

import (
	"strings"
	"testing"
	"time"
)

type testLogging struct {
	t *testing.T
}

func newTestLogging(t *testing.T) *testLogging {
	return &testLogging{t: t}
}

func newTestLoggingAtomos(t *testing.T) *loggingAtomos {
	la := &loggingAtomos{}
	if err := la.init(&testLogging{t: t}); err != nil {
		t.Fatalf("newTestLogging: Init logging failed. err=(%v)", err.AddStack(nil))
	}
	// Stop the logging mailbox when the test finishes; otherwise its goroutine
	// keeps calling t.Log after the test has completed (a data race) and keeps
	// allocating debug mails that break other tests' allocation assertions.
	// Guard with isRunning because loggingAtomos.stop() is not idempotent.
	t.Cleanup(func() {
		if la.logBox != nil && la.logBox.isRunning() {
			la.stop()
		}
		waitMailboxGoroutineExit(la.logBox, 5*time.Second)
	})
	return la
}

func (t testLogging) WriteAccessLog(msg string) {
	if strings.HasSuffix(msg, "\n") {
		msg = msg[:len(msg)-1]
	}
	t.t.Log(msg)
}

func (t testLogging) WriteErrorLog(msg string) {
	if strings.HasSuffix(msg, "\n") {
		msg = msg[:len(msg)-1]
	}
	t.t.Error(msg)
}

func (t testLogging) Close() {}

// waitMailboxGoroutineExit waits until the mailbox loop goroutine has fully
// exited. It delegates to the production mailBox.waitExit; see its comment for
// why isRunning() alone is insufficient.
func waitMailboxGoroutineExit(mb *mailBox, timeout time.Duration) {
	mb.waitExit(timeout)
}

// waitMailboxStopped polls until the mailbox has stopped and its loop
// goroutine exited. Serial task mailboxes stop themselves asynchronously after
// their queue drains, so an immediate isRunning() check right after the last
// task callback is inherently racy.
func waitMailboxStopped(t *testing.T, mb *mailBox, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for mb.isRunning() {
		if time.Now().After(deadline) {
			t.Fatal("Mailbox should be stopped after processing all mails.")
		}
		time.Sleep(time.Millisecond)
	}
	mb.waitExit(timeout)
}

type benchLogging struct {
	b *testing.B
}

func newBenchLogging(b *testing.B) *benchLogging {
	return &benchLogging{b: b}
}

func newBenchLoggingAtomos(b *testing.B) *loggingAtomos {
	la := &loggingAtomos{}
	if err := la.init(&benchLogging{b: b}); err != nil {
		b.Fatalf("newBenchLogging: Init logging failed. err=(%v)", err.AddStack(nil))
	}
	return la
}

func (b benchLogging) WriteAccessLog(msg string) {
	if strings.HasSuffix(msg, "\n") {
		msg = msg[:len(msg)-1]
	}
	b.b.Log(msg)
}

func (b benchLogging) WriteErrorLog(msg string) {
	if strings.HasSuffix(msg, "\n") {
		msg = msg[:len(msg)-1]
	}
	b.b.Error(msg)
}

func (b benchLogging) Close() {}
