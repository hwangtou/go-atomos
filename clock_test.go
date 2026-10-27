package atomos

import (
	"testing"
	"time"
)

func TestSystemClockTimers(t *testing.T) {
	clock := systemClock{}
	timer := clock.NewTimer(5 * time.Millisecond)
	select {
	case <-timer.C():
	case <-time.After(time.Second):
		t.Fatal("system timer did not fire")
	}
	if timer.Stop() {
		t.Fatal("fired timer reported active")
	}

	ticker := clock.NewTicker(time.Millisecond)
	defer ticker.Stop()
	select {
	case <-ticker.C():
	case <-time.After(time.Second):
		t.Fatal("system ticker did not fire")
	}

	fired := make(chan struct{})
	callback := clock.AfterFunc(0, func() { close(fired) })
	if callback.C() != nil {
		t.Fatal("AfterFunc timer must not expose a channel")
	}
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("system AfterFunc did not run")
	}
}
