package clock

import "time"

// Clock supplies time for lifecycle scheduling. Implementations must be safe
// for concurrent use.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
	AfterFunc(time.Duration, func()) Timer
	NewTimer(time.Duration) Timer
	NewTicker(time.Duration) Ticker
}

// Timer is a schedulable timer. AfterFunc timers have a nil channel.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(time.Duration) bool
}

// Ticker is a repeating timer.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}
