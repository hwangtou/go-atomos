package atomos

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type retryTestCall struct {
	delay time.Duration
	fn    func()
}

type retryTestClock struct {
	calls []retryTestCall
}

func (c *retryTestClock) Now() time.Time                       { return time.Time{} }
func (c *retryTestClock) After(time.Duration) <-chan time.Time { return nil }
func (c *retryTestClock) AfterFunc(d time.Duration, fn func()) Timer {
	c.calls = append(c.calls, retryTestCall{delay: d, fn: fn})
	return &retryTestTimer{}
}
func (c *retryTestClock) NewTimer(time.Duration) Timer { return nil }
func (c *retryTestClock) NewTicker(time.Duration) Ticker {
	return nil
}
func (c *retryTestClock) next() retryTestCall {
	call := c.calls[0]
	c.calls = c.calls[1:]
	return call
}

type retryTestTimer struct{}

func (t *retryTestTimer) C() <-chan time.Time      { return nil }
func (t *retryTestTimer) Stop() bool               { return true }
func (t *retryTestTimer) Reset(time.Duration) bool { return true }

func TestAsyncRetryBackoffFIFOAndIndependentTargets(t *testing.T) {
	clock := &retryTestClock{}
	retryable := errors.New("retryable")
	manager := newAsyncRetryManager(clock, func(err error) bool { return errors.Is(err, retryable) }, nil)
	var mutex sync.Mutex
	var attempts []string
	terminal := make(chan error, 3)
	record := func(name string, failures int) func(attempt int) error {
		return func(attempt int) error {
			mutex.Lock()
			attempts = append(attempts, name)
			mutex.Unlock()
			if attempt <= failures {
				return retryable
			}
			return nil
		}
	}
	for _, request := range []struct {
		target, id string
		failures   int
	}{
		{target: "a", id: "a1", failures: 1},
		{target: "a", id: "a2"},
		{target: "b", id: "b1"},
	} {
		request := request
		if err := manager.submit(request.target, request.id, record(request.id, request.failures), func(err error) {
			terminal <- err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.submit("a", "a1", record("duplicate", 0), nil); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("duplicate pending message error = %v", err)
	}
	clock.next().fn()
	if got := len(clock.calls); got != 2 {
		t.Fatalf("scheduled calls = %d, want other target and retry", got)
	}
	clock.next().fn()
	mutex.Lock()
	got := append([]string(nil), attempts...)
	mutex.Unlock()
	if len(got) != 2 || got[0] != "a1" || got[1] != "b1" {
		t.Fatalf("attempt order before retry = %v", got)
	}
	clock.next().fn()
	clock.next().fn()
	mutex.Lock()
	got = append([]string(nil), attempts...)
	mutex.Unlock()
	if len(got) != 4 || got[2] != "a1" || got[3] != "a2" {
		t.Fatalf("attempt order after retry = %v", got)
	}
	for i := 0; i < 3; i++ {
		select {
		case err := <-terminal:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("missing terminal result")
		}
	}
}

func TestAsyncRetryPermanentFailureAndClose(t *testing.T) {
	clock := &retryTestClock{}
	permanent := errors.New("permanent")
	manager := newAsyncRetryManager(clock, func(error) bool { return false }, nil)
	results := make(chan error, 2)
	if err := manager.submit("a", "failed", func(int) error { return permanent }, func(err error) {
		results <- err
	}); err != nil {
		t.Fatal(err)
	}
	clock.next().fn()
	if err := <-results; !errors.Is(err, permanent) {
		t.Fatalf("permanent result = %v", err)
	}
	if len(clock.calls) != 0 {
		t.Fatal("permanent error was retried")
	}
	if err := manager.submit("a", "pending", func(int) error { return nil }, func(err error) {
		results <- err
	}); err != nil {
		t.Fatal(err)
	}
	manager.close()
	if err := <-results; !errors.Is(err, ErrAtomosIsNotRunning) {
		t.Fatalf("close result = %v", err)
	}
	if err := manager.submit("a", "late", func(int) error { return nil }, nil); !errors.Is(err, ErrAtomosIsNotRunning) {
		t.Fatalf("submit after close = %v", err)
	}
}

func TestAsyncRetryCloseDuringDelivery(t *testing.T) {
	manager := newAsyncRetryManager(systemClock{}, func(error) bool { return true }, nil)
	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	if err := manager.submit("a", "running", func(int) error {
		close(started)
		<-release
		return nil
	}, func(err error) { firstDone <- err }); err != nil {
		t.Fatal(err)
	}
	if err := manager.submit("a", "queued", func(int) error { return nil }, func(err error) {
		secondDone <- err
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("delivery did not start")
	}
	manager.close()
	select {
	case err := <-secondDone:
		if !errors.Is(err, ErrAtomosIsNotRunning) {
			t.Fatalf("queued result = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued request was not terminated")
	}
	close(release)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("running request result = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("running request did not finish")
	}
}
