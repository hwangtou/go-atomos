package atomostest

import (
	"sync"
	"time"

	"github.com/hwangtou/go-atomos"
)

// VirtualClock is a deterministic Clock. Timers fire only when Advance or
// RunUntilBlocked is called.
type VirtualClock struct {
	mutex  sync.Mutex
	now    time.Time
	seq    uint64
	timers map[uint64]*virtualTimer
}

// NewVirtualClock creates a clock at the Unix epoch.
func NewVirtualClock() *VirtualClock {
	return &VirtualClock{now: time.Unix(0, 0).UTC(), timers: map[uint64]*virtualTimer{}}
}

// NewVirtualClockAt creates a clock at a chosen initial time.
func NewVirtualClockAt(now time.Time) *VirtualClock {
	clock := NewVirtualClock()
	clock.now = now
	return clock
}

// Now returns the current virtual time.
func (c *VirtualClock) Now() time.Time {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.now
}

// After returns a channel that receives the virtual time at d.
func (c *VirtualClock) After(d time.Duration) <-chan time.Time {
	return c.NewTimer(d).C()
}

// AfterFunc schedules fn to run synchronously during clock advancement.
func (c *VirtualClock) AfterFunc(d time.Duration, fn func()) atomos.Timer {
	return c.schedule(d, 0, fn, nil)
}

// NewTimer schedules a channel timer.
func (c *VirtualClock) NewTimer(d time.Duration) atomos.Timer {
	return c.schedule(d, 0, nil, make(chan time.Time, 1))
}

// NewTicker schedules a repeating channel timer.
func (c *VirtualClock) NewTicker(d time.Duration) atomos.Ticker {
	if d <= 0 {
		panic("non-positive interval for NewTicker")
	}
	return &virtualTicker{timer: c.schedule(d, d, nil, make(chan time.Time, 1))}
}

// Advance moves time forward and fires every timer due through the target in
// deadline and registration order.
func (c *VirtualClock) Advance(d time.Duration) {
	if d < 0 {
		panic("negative duration for Advance")
	}
	c.mutex.Lock()
	target := c.now.Add(d)
	c.mutex.Unlock()
	c.advance(target)
}

// RunUntilBlocked fires timers due at the current virtual time.
func (c *VirtualClock) RunUntilBlocked() { c.advance(c.Now()) }

// PendingTimers returns the number of active timers and tickers.
func (c *VirtualClock) PendingTimers() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.timers)
}

func (c *VirtualClock) schedule(d, period time.Duration, fn func(), ch chan time.Time) *virtualTimer {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.seq++
	timer := &virtualTimer{
		clock:    c,
		id:       c.seq,
		seq:      c.seq,
		deadline: c.now.Add(d),
		period:   period,
		fn:       fn,
		ch:       ch,
		active:   true,
	}
	c.timers[timer.id] = timer
	return timer
}

func (c *VirtualClock) advance(target time.Time) {
	for {
		c.mutex.Lock()
		timer := c.nextLocked(target)
		if timer == nil {
			c.now = target
			c.mutex.Unlock()
			return
		}
		c.now = timer.deadline
		firedAt := timer.deadline
		if timer.period > 0 {
			timer.seq = c.nextSeqLocked()
			timer.deadline = timer.deadline.Add(timer.period)
		} else {
			timer.active = false
			delete(c.timers, timer.id)
		}
		fn, ch := timer.fn, timer.ch
		c.mutex.Unlock()

		if fn != nil {
			fn()
			continue
		}
		select {
		case ch <- firedAt:
		default:
		}
	}
}

func (c *VirtualClock) nextLocked(target time.Time) *virtualTimer {
	var next *virtualTimer
	for _, timer := range c.timers {
		if !timer.active || timer.deadline.After(target) {
			continue
		}
		if next == nil || timer.deadline.Before(next.deadline) ||
			(timer.deadline.Equal(next.deadline) && timer.seq < next.seq) {
			next = timer
		}
	}
	return next
}

func (c *VirtualClock) nextSeqLocked() uint64 {
	c.seq++
	return c.seq
}

type virtualTimer struct {
	clock    *VirtualClock
	id       uint64
	seq      uint64
	deadline time.Time
	period   time.Duration
	fn       func()
	ch       chan time.Time
	active   bool
}

func (t *virtualTimer) C() <-chan time.Time { return t.ch }

func (t *virtualTimer) Stop() bool {
	t.clock.mutex.Lock()
	defer t.clock.mutex.Unlock()
	wasActive := t.active
	if t.active {
		t.active = false
		delete(t.clock.timers, t.id)
	}
	return wasActive
}

func (t *virtualTimer) Reset(d time.Duration) bool {
	t.clock.mutex.Lock()
	defer t.clock.mutex.Unlock()
	wasActive := t.active
	t.clock.seq++
	t.seq = t.clock.seq
	t.deadline = t.clock.now.Add(d)
	t.active = true
	t.clock.timers[t.id] = t
	return wasActive
}

type virtualTicker struct{ timer *virtualTimer }

func (t *virtualTicker) C() <-chan time.Time { return t.timer.C() }
func (t *virtualTicker) Stop()               { t.timer.Stop() }
