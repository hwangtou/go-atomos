package atomos

import (
	"time"

	internalclock "github.com/hwangtou/go-atomos/internal/clock"
)

// Clock supplies time for lifecycle scheduling. Implementations must be safe
// for concurrent use.
type Clock = internalclock.Clock

// Timer is a schedulable timer. AfterFunc timers have a nil channel.
type Timer = internalclock.Timer

// Ticker is a repeating timer.
type Ticker = internalclock.Ticker

type systemClock struct{}

func (p *CosmosProcess) getClock() Clock {
	if p == nil || p.clock == nil {
		return systemClock{}
	}
	return p.clock
}

func (systemClock) Now() time.Time { return time.Now() }
func (systemClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}
func (systemClock) AfterFunc(d time.Duration, fn func()) Timer {
	return &systemTimer{timer: time.AfterFunc(d, fn)}
}
func (systemClock) NewTimer(d time.Duration) Timer {
	timer := time.NewTimer(d)
	return &systemTimer{timer: timer, ch: timer.C}
}
func (systemClock) NewTicker(d time.Duration) Ticker {
	return &systemTicker{ticker: time.NewTicker(d)}
}

type systemTicker struct{ ticker *time.Ticker }

func (t *systemTicker) C() <-chan time.Time { return t.ticker.C }
func (t *systemTicker) Stop()               { t.ticker.Stop() }

type systemTimer struct {
	timer *time.Timer
	ch    <-chan time.Time
}

func (t *systemTimer) C() <-chan time.Time        { return t.ch }
func (t *systemTimer) Stop() bool                 { return t.timer.Stop() }
func (t *systemTimer) Reset(d time.Duration) bool { return t.timer.Reset(d) }
