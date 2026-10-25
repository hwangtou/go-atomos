package atomostest

// Polling and leak-detection helpers for actor tests: actor state changes
// are asynchronous (everything happens on mailbox goroutines), so tests
// poll instead of sleeping, and shutdown assertions check that the mailbox
// goroutines actually left.

import (
	"fmt"
	"runtime"
	"time"
)

// TestReporter is the narrow slice of testing.TB the helpers need. Accepting
// an interface (instead of *testing.T) lets users exercise failure paths
// with their own recording double.
type TestReporter interface {
	Helper()
	Fatalf(format string, args ...any)
}

// Eventually polls condition until it returns true, failing t when timeout
// elapses first. interval <= 0 falls back to 10ms. Sleeps are capped at the
// remaining timeout. A condition call is synchronous and cannot be interrupted,
// so a slow or blocked condition may extend Eventually beyond the timeout.
// Actor state transitions happen on mailbox goroutines; prefer this over
// time.Sleep in tests.
func Eventually(t TestReporter, timeout, interval time.Duration, condition func() bool, msgAndArgs ...any) {
	t.Helper()
	if interval <= 0 {
		interval = 10 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for !condition() {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			if len(msgAndArgs) > 0 {
				if format, ok := msgAndArgs[0].(string); ok {
					t.Fatalf("atomostest: condition not met within %v: %s", timeout, fmt.Sprintf(format, msgAndArgs[1:]...))
					return
				}
				t.Fatalf("atomostest: condition not met within %v: %v", timeout, msgAndArgs)
				return
			}
			t.Fatalf("atomostest: condition not met within %v", timeout)
			return
		}
		if interval < remaining {
			remaining = interval
		}
		time.Sleep(remaining)
	}
}

// Goroutines is a point-in-time goroutine-count snapshot for leak checks.
type Goroutines int

// TakeGoroutines captures the current goroutine count. Take it before the
// actor activity under test, then call AssertNoLeak after shutdown.
func TakeGoroutines() Goroutines {
	return Goroutines(runtime.NumGoroutine())
}

// AssertNoLeak waits up to timeout for the goroutine count to fall back to
// the snapshot plus slack (background workers and in-flight test teardown
// justify a small slack; 2 is a sane default) and fails t otherwise.
// Mailbox goroutines exit shortly after their atom halts — a persistent
// surplus after Stop usually means a leaked mailbox or a blocked handler.
func (g Goroutines) AssertNoLeak(t TestReporter, slack int, timeout time.Duration) {
	t.Helper()
	Eventually(t, timeout, 20*time.Millisecond, func() bool {
		return runtime.NumGoroutine() <= int(g)+slack
	}, "goroutine leak: before=%d, now=%d, slack=%d", int(g), runtime.NumGoroutine(), slack)
}
