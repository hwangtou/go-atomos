package atomostest

// Probe: productized five-injection-point test probe. It wires the
// CosmosRunnable lifecycle/error hooks (SetSpawningHook, SetSpawnHook,
// SetStoppingHook, SetHaltedHook, SetRecoverHook) into a recording,
// pollable event log — the pattern the framework's own tests hand-roll.

import (
	"strings"
	"sync"
	"time"

	atomos "github.com/hwangtou/go-atomos"
)

// ProbeEventKind enumerates the wired hook points.
type ProbeEventKind string

const (
	ProbeSpawning  ProbeEventKind = "spawning"
	ProbeSpawned   ProbeEventKind = "spawned"
	ProbeStopping  ProbeEventKind = "stopping"
	ProbeHalted    ProbeEventKind = "halted"
	ProbeRecovered ProbeEventKind = "recovered"
)

// ProbeEvent is one recorded hook firing.
type ProbeEvent struct {
	Kind ProbeEventKind
	ID   *atomos.IDInfo
	Err  error
	At   time.Time
}

// Probe records lifecycle events from the hooks it installs. Create it
// BEFORE starting the process: NewProbe(runnable) wires the hooks, then
// atomostest.Run(t, runnable) starts the process with them in place.
type Probe struct {
	mu     sync.Mutex
	events []ProbeEvent
}

// NewProbe installs the five hooks on runnable and returns the recorder.
func NewProbe(r *atomos.CosmosRunnable) *Probe {
	p := &Probe{}
	r.SetSpawningHook(func(id *atomos.IDInfo) { p.add(ProbeSpawning, id, nil) }).
		SetSpawnHook(func(id *atomos.IDInfo) { p.add(ProbeSpawned, id, nil) }).
		SetStoppingHook(func(id *atomos.IDInfo) { p.add(ProbeStopping, id, nil) }).
		SetHaltedHook(func(id *atomos.IDInfo, err error, _ *atomos.AtomosMessageTrackerExporter) { p.add(ProbeHalted, id, err) }).
		SetRecoverHook(func(id *atomos.IDInfo, err error) { p.add(ProbeRecovered, id, err) })
	return p
}

func (p *Probe) add(kind ProbeEventKind, id *atomos.IDInfo, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ProbeEvent{Kind: kind, ID: id, Err: err, At: time.Now()})
}

// Events returns a copy of the recorded events in order.
func (p *Probe) Events() []ProbeEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]ProbeEvent(nil), p.events...)
}

// Reset drops all recorded events.
func (p *Probe) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = nil
}

// Has reports whether an event of kind whose ID string contains idContains
// has been recorded.
func (p *Probe) Has(kind ProbeEventKind, idContains string) bool {
	for _, e := range p.Events() {
		if e.Kind == kind && strings.Contains(e.ID.String(), idContains) {
			return true
		}
	}
	return false
}

// Require waits up to timeout for a matching event and fails t otherwise.
func (p *Probe) Require(t TestReporter, kind ProbeEventKind, idContains string, timeout time.Duration) {
	t.Helper()
	Eventually(t, timeout, 10*time.Millisecond, func() bool { return p.Has(kind, idContains) },
		"probe: no %s event for %q within %v; events=%v", kind, idContains, timeout, p.Events())
}
