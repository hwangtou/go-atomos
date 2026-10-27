package atomostest

import (
	"time"

	"github.com/hwangtou/go-atomos/internal/testclock"
)

// VirtualClock is a deterministic Clock. Timers fire only when Advance or
// RunUntilBlocked is called.
type VirtualClock = testclock.VirtualClock

// NewVirtualClock creates a clock at the Unix epoch.
func NewVirtualClock() *VirtualClock { return testclock.NewVirtualClock() }

// NewVirtualClockAt creates a clock at a chosen initial time.
func NewVirtualClockAt(now time.Time) *VirtualClock { return testclock.NewVirtualClockAt(now) }
