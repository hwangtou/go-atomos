package atomostest

import (
	"testing"
	"time"
)

func TestVirtualClockTimerOrderAndReset(t *testing.T) {
	clock := NewVirtualClockAt(time.Unix(100, 0))
	var events []string
	clock.AfterFunc(2*time.Second, func() { events = append(events, "second") })
	clock.AfterFunc(time.Second, func() { events = append(events, "first-a") })
	clock.AfterFunc(time.Second, func() { events = append(events, "first-b") })
	clock.Advance(time.Second)
	if len(events) != 2 || events[0] != "first-a" || events[1] != "first-b" {
		t.Fatalf("same-time events = %v", events)
	}
	if got := clock.Now(); !got.Equal(time.Unix(101, 0)) {
		t.Fatalf("Now = %v", got)
	}

	timer := clock.NewTimer(time.Hour)
	if !timer.Stop() || timer.Stop() {
		t.Fatal("Stop did not report active then inactive")
	}
	if timer.Reset(300 * time.Millisecond) {
		t.Fatal("Reset reported an active stopped timer")
	}
	clock.Advance(300 * time.Millisecond)
	select {
	case got := <-timer.C():
		if !got.Equal(time.Unix(101, 300000000)) {
			t.Fatalf("timer time = %v", got)
		}
	default:
		t.Fatal("reset timer did not fire")
	}
	clock.Advance(700 * time.Millisecond)
	if len(events) != 3 || events[2] != "second" {
		t.Fatalf("events after target = %v", events)
	}
	if clock.PendingTimers() != 0 {
		t.Fatalf("pending timers = %d", clock.PendingTimers())
	}
}

func TestVirtualClockTickerAndCallbackScheduling(t *testing.T) {
	clock := NewVirtualClock()
	ticker := clock.NewTicker(time.Second)
	clock.AfterFunc(1500*time.Millisecond, func() {
		clock.AfterFunc(0, func() {
			clock.AfterFunc(0, nil)
		})
	})
	for i, want := range []time.Time{time.Unix(1, 0).UTC(), time.Unix(2, 0).UTC(), time.Unix(3, 0).UTC()} {
		clock.Advance(time.Second)
		select {
		case got := <-ticker.C():
			if !got.Equal(want) {
				t.Fatalf("tick %d = %v, want %v", i, got, want)
			}
		default:
			t.Fatalf("missing tick %d", i)
		}
	}
	ticker.Stop()
	clock.Advance(time.Second)
	select {
	case got := <-ticker.C():
		t.Fatalf("stopped ticker fired at %v", got)
	default:
	}
	if clock.PendingTimers() != 0 {
		t.Fatalf("pending timers = %d", clock.PendingTimers())
	}
}

func TestVirtualClockDoesNotFireEarly(t *testing.T) {
	clock := NewVirtualClock()
	fired := false
	clock.AfterFunc(time.Second, func() { fired = true })
	clock.Advance(999 * time.Millisecond)
	if fired {
		t.Fatal("timer fired early")
	}
	clock.Advance(time.Millisecond)
	if !fired {
		t.Fatalf("timer did not fire at deadline: now=%v pending=%d", clock.Now(), clock.PendingTimers())
	}
}
