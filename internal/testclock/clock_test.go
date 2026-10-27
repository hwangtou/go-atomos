package testclock

import (
	"testing"
	"time"
)

func TestVirtualClockNestedAdvanceDoesNotRegress(t *testing.T) {
	clock := NewVirtualClock()
	var events []string
	clock.AfterFunc(time.Second, func() {
		events = append(events, "outer")
		clock.Advance(2 * time.Second)
		events = append(events, clock.Now().String())
	})
	clock.AfterFunc(2*time.Second, func() { events = append(events, "inner timer") })

	clock.Advance(time.Second)

	wantNow := time.Unix(3, 0).UTC()
	if got := clock.Now(); !got.Equal(wantNow) {
		t.Fatalf("Now after nested Advance = %v, want %v", got, wantNow)
	}
	wantEvents := []string{"outer", "inner timer", wantNow.String()}
	if len(events) != len(wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	for i := range wantEvents {
		if events[i] != wantEvents[i] {
			t.Fatalf("events = %v, want %v", events, wantEvents)
		}
	}
}
