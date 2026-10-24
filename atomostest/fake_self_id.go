package atomostest

import (
	"fmt"
	"sync"
	"time"

	atomos "github.com/hwangtou/go-atomos"
)

// FakeSelfID implements atomos.SelfID for unit-testing Atomos business logic
// without starting a Cosmos process. The ID behavior is inherited from the
// embedded FakeID; the SelfID extras (Log, Task, CosmosMain, KillSelf,
// Parallel, Config) are faked as well.
//
// Like FakeID, a FakeSelfID is not a framework-internal ID: passing it into
// framework kernel paths fails explicitly with atomos.ErrFrameworkInvalidID.
type FakeSelfID struct {
	*FakeID

	mu      sync.Mutex
	log     *FakeLogging
	task    *FakeTask
	main    atomos.CosmosNode
	config  map[string][]byte
	pending int
	killCnt int
}

// NewFakeSelfID returns a FakeSelfID backed by a fresh FakeID, FakeLogging
// and FakeTask.
func NewFakeSelfID() *FakeSelfID {
	return &FakeSelfID{
		FakeID: NewFakeID(),
		log:    NewFakeLogging(),
		task:   NewFakeTask(),
	}
}

var _ atomos.SelfID = (*FakeSelfID)(nil)

// Log returns the actor logger, a *FakeLogging behind the atomos.Logging
// interface. Use LogRecorder to access the recorded entries without a type
// assertion.
func (f *FakeSelfID) Log() atomos.Logging {
	return f.log
}

// LogRecorder returns the FakeLogging behind Log.
func (f *FakeSelfID) LogRecorder() *FakeLogging {
	return f.log
}

// Task returns the actor task scheduler, a *FakeTask behind the atomos.Task
// interface.
func (f *FakeSelfID) Task() atomos.Task {
	return f.task
}

// TaskScheduler returns the FakeTask behind Task.
func (f *FakeSelfID) TaskScheduler() *FakeTask {
	return f.task
}

// CosmosMain returns the CosmosNode set via SetCosmosMain, or nil.
func (f *FakeSelfID) CosmosMain() atomos.CosmosNode {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.main
}

// SetCosmosMain sets the CosmosNode returned by CosmosMain.
func (f *FakeSelfID) SetCosmosMain(node atomos.CosmosNode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.main = node
}

// KillSelf records a self-kill instead of stopping anything. Use
// KillSelfCount to assert how often the actor tried to kill itself.
func (f *FakeSelfID) KillSelf() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.killCnt++
}

// KillSelfCount returns how many times KillSelf was called.
func (f *FakeSelfID) KillSelfCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.killCnt
}

// AsyncCallbackPending returns the value set via SetAsyncCallbackPending.
func (f *FakeSelfID) AsyncCallbackPending() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pending
}

// SetAsyncCallbackPending sets the value returned by AsyncCallbackPending,
// for testing idle-cleanup logic that defers KillSelf while callbacks are
// in flight.
func (f *FakeSelfID) SetAsyncCallbackPending(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending = n
}

// Parallel runs fn synchronously in the caller's goroutine. Unlike the
// framework, panics are not recovered, so they fail the test directly.
func (f *FakeSelfID) Parallel(fn func()) {
	fn()
}

// Config returns the config map set via SetConfig, or nil.
func (f *FakeSelfID) Config() map[string][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.config
}

// SetConfig sets the config map returned by Config.
func (f *FakeSelfID) SetConfig(config map[string][]byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.config = config
}

// FakeTask implements atomos.Task with deterministic, synchronous execution:
// every submitted task function runs immediately on the caller's goroutine,
// delays and crontab specs are ignored. This keeps tests free of timing
// flakes; code that depends on actual scheduling must be tested against a
// real process via Run.
type FakeTask struct {
	mu        sync.Mutex
	nextID    uint64
	issued    map[uint64]bool
	cancelled []uint64
}

// NewFakeTask returns a FakeTask with no tasks issued.
func NewFakeTask() *FakeTask {
	return &FakeTask{issued: make(map[uint64]bool)}
}

var _ atomos.Task = (*FakeTask)(nil)

// Add runs fn synchronously and returns its task ID.
func (f *FakeTask) Add(fn atomos.TaskFn) (uint64, error) {
	if fn == nil {
		return 0, fmt.Errorf("FakeTask: task closure is nil")
	}
	id := f.issue()
	fn(id)
	return id, nil
}

// AddAfter ignores the delay and runs fn synchronously.
func (f *FakeTask) AddAfter(d time.Duration, fn atomos.TaskFn) (uint64, error) {
	return f.Add(fn)
}

// AddCrontab ignores the spec and runs fn synchronously, once.
func (f *FakeTask) AddCrontab(spec string, fn atomos.TaskFn) (uint64, error) {
	return f.Add(fn)
}

// Cancel records the cancellation. Because tasks have already executed
// synchronously by the time Cancel can be called, Cancel only validates that
// the ID was issued.
func (f *FakeTask) Cancel(id uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.issued[id] {
		return fmt.Errorf("FakeTask: task %d does not exist", id)
	}
	f.cancelled = append(f.cancelled, id)
	return nil
}

// CancelCrontab behaves like Cancel.
func (f *FakeTask) CancelCrontab(entryID uint64) error {
	return f.Cancel(entryID)
}

// AddToAtomosQueue runs task synchronously and returns a no-op cancel
// function.
func (f *FakeTask) AddToAtomosQueue(task atomos.TaskFn, ext ...atomos.ArgsForTask) func(reason string) error {
	if task != nil {
		task(f.issue())
	}
	return func(reason string) error { return nil }
}

// AddToSerialQueue runs workerFn synchronously, then runs the callback it
// returns, if any, and returns a no-op cancel function.
func (f *FakeTask) AddToSerialQueue(queueName string, workerFn atomos.TaskFnWithCallback, ext ...atomos.ArgsForTask) func(reason string) error {
	if workerFn != nil {
		if callback := workerFn(f.issue()); callback != nil {
			callback()
		}
	}
	return func(reason string) error { return nil }
}

// AddToConcurrentQueue behaves like AddToSerialQueue: the work runs
// synchronously on the caller's goroutine.
func (f *FakeTask) AddToConcurrentQueue(workerFn atomos.TaskFnWithCallback, ext ...atomos.ArgsForTask) func(reason string) error {
	return f.AddToSerialQueue("", workerFn, ext...)
}

// HasMarking always returns false. Task marks are carried by framework-internal
// ArgsForTask values that a fake cannot inspect, and tasks execute immediately
// so none are ever pending.
func (f *FakeTask) HasMarking(mark string) bool {
	return false
}

// Cancelled returns the IDs passed to Cancel and CancelCrontab, in order.
func (f *FakeTask) Cancelled() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]uint64, len(f.cancelled))
	copy(out, f.cancelled)
	return out
}

func (f *FakeTask) issue() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.issued[f.nextID] = true
	return f.nextID
}
