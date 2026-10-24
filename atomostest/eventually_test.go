package atomostest

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// recordingReporter captures Fatalf instead of exiting, for exercising the
// failure paths of Eventually / AssertNoLeak.
type recordingReporter struct {
	fatal string
}

func (r *recordingReporter) Helper() {}
func (r *recordingReporter) Fatalf(format string, args ...any) {
	r.fatal = fmt.Sprintf(format, args...)
}

func TestEventually_Success(t *testing.T) {
	var n atomic.Int32
	go func() {
		time.Sleep(30 * time.Millisecond)
		n.Store(1)
	}()
	Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool { return n.Load() == 1 },
		"counter should reach 1")
}

func TestEventually_Timeout(t *testing.T) {
	r := &recordingReporter{}
	Eventually(r, 50*time.Millisecond, 5*time.Millisecond, func() bool { return false },
		"flag=(%v)", true)
	if r.fatal == "" {
		t.Fatal("expected Fatalf on timeout")
	}
	if !strings.Contains(r.fatal, "condition not met within 50ms") || !strings.Contains(r.fatal, "flag=(true)") {
		t.Fatalf("unexpected Fatalf message: %q", r.fatal)
	}
}

func TestEventually_DefaultInterval(t *testing.T) {
	var n atomic.Int32
	go func() {
		time.Sleep(20 * time.Millisecond)
		n.Store(1)
	}()
	// interval 0 falls back to 10ms internally; must still succeed.
	Eventually(t, 2*time.Second, 0, func() bool { return n.Load() == 1 })
}

func TestGoroutines_NoLeak(t *testing.T) {
	before := TakeGoroutines()
	done := make(chan struct{})
	for i := 0; i < 5; i++ {
		go func() { <-done; time.Sleep(10 * time.Millisecond) }()
	}
	close(done)
	before.AssertNoLeak(t, 2, 3*time.Second)
}

func TestGoroutines_LeakDetected(t *testing.T) {
	r := &recordingReporter{}
	before := TakeGoroutines()
	block := make(chan struct{})
	defer close(block)
	for i := 0; i < 3; i++ {
		go func() { <-block }()
	}
	before.AssertNoLeak(r, 0, 50*time.Millisecond)
	if r.fatal == "" {
		t.Fatal("expected Fatalf on goroutine leak")
	}
	if !strings.Contains(r.fatal, "goroutine leak") {
		t.Fatalf("unexpected Fatalf message: %q", r.fatal)
	}
}
