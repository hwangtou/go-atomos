package atomos

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// atomRefState tracks all outstanding IDTracker references for a single atom
// name within an Element. Its lifetime is longer than any individual Atom
// instance: it survives a halt+respawn cycle so that references acquired
// against a now-halted instance can still be Release()d correctly afterwards.
//
// References are accounted per instanceID (assigned at BaseAtomos
// construction). A respawn produces a new instanceID, so the old instance's
// references and the new instance's references are counted in separate cells
// and never interfere. When a cell drains to zero and that instance has
// halted, the instance becomes eligible for GC.
type atomRefState struct {
	element *ElementLocal
	name    string

	mu sync.Mutex
	// cells maps instanceID -> outstanding reference count for that instance.
	// A cell is created on first addRef for an instance and removed when its
	// count returns to zero. A halted-but-still-referenced instance keeps its
	// cell alive until the last Release.
	cells map[uint64]int64
	// counter assigns a diagnostic id per tracker (for ToString/debug).
	counter uint64
	// debug optionally records per-instance tracker metadata (file:line) so
	// leaks can be enumerated via String(). Populated only when the owning
	// process has idTrackerDebug enabled.
	debug map[uint64][]*IDTracker
}

// newAtomRefState constructs an empty ref state for the given name.
func newAtomRefState(element *ElementLocal, name string) *atomRefState {
	return &atomRefState{
		element: element,
		name:    name,
		cells:   map[uint64]int64{},
	}
}

// addRef increments the reference count for the given instance and returns a
// new IDTracker bound to (state, instanceID). The tracker's Release will
// decrement the same cell regardless of later respawns.
func (s *atomRefState) addRef(instID uint64, info *IDTrackerInfo) *IDTracker {
	s.mu.Lock()
	s.cells[instID]++
	s.counter++
	tr := &IDTracker{
		state:  s,
		instID: instID,
		id:     s.counter,
	}
	if info != nil {
		tr.file = info.File
		tr.line = int(info.Line)
		tr.name = info.Name
	}
	if s.element.cosmosLocal != nil && s.element.cosmosLocal.process != nil &&
		s.element.cosmosLocal.process.idTrackerDebug {
		if s.debug == nil {
			s.debug = map[uint64][]*IDTracker{}
		}
		s.debug[instID] = append(s.debug[instID], tr)
		// Register a GC finalizer so a tracker that is never Release()d reports
		// itself (with its allocation site) instead of leaking silently. Only
		// done in debug mode to avoid the per-GC cost in production; Release()
		// detaches the finalizer on the happy path.
		runtime.SetFinalizer(tr, finalizeIDTracker)
	}
	s.mu.Unlock()
	return tr
}

// release decrements the reference count for the tracker's instance. When the
// cell reaches zero it is removed and, if that instance has halted, the GC
// hook is invoked. It is idempotent (guarded by IDTracker.released).
//
// Returns true if this call drained the instance's count to zero (so the
// caller, the IDTracker, can fire the GC hook after dropping the lock).
func (s *atomRefState) release(instID uint64) (drained bool) {
	s.mu.Lock()
	cnt, ok := s.cells[instID]
	if !ok {
		s.mu.Unlock()
		return false
	}
	cnt--
	if cnt <= 0 {
		delete(s.cells, instID)
		// Clean up the per-instance debug slice if present.
		if s.debug != nil {
			if _, has := s.debug[instID]; has {
				delete(s.debug, instID)
			}
		}
		drained = true
	} else {
		s.cells[instID] = cnt
	}
	s.mu.Unlock()
	return drained
}

// refCount returns the number of outstanding references for an instance.
func (s *atomRefState) refCount(instID uint64) int64 {
	s.mu.Lock()
	cnt := s.cells[instID]
	s.mu.Unlock()
	return cnt
}

// isEmpty reports whether there are no outstanding references for any instance
// of this name. Used by ElementLocal to retire a refState whose atom is gone.
func (s *atomRefState) isEmpty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cells) == 0
}

// String returns a human-readable dump of outstanding references, for leak
// diagnostics (GetAllInactiveAtomsIDTrackerInfo).
func (s *atomRefState) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cells) == 0 {
		return "No IDTracker remain"
	}
	b := strings.Builder{}
	b.WriteString("IDTracker Info:")
	if s.debug == nil {
		// No debug metadata recorded; report per-instance counts only.
		for instID, cnt := range s.cells {
			b.WriteString(fmt.Sprintf("\n\tinstance=%d count=%d", instID, cnt))
		}
		return b.String()
	}
	for instID, trackers := range s.debug {
		for _, tr := range trackers {
			b.WriteString("\n\t")
			b.WriteString(tr.ToString())
		}
		// Include instances that have a cell but no debug trackers recorded.
		if len(trackers) == 0 {
			if cnt, ok := s.cells[instID]; ok {
				b.WriteString(fmt.Sprintf("\n\tinstance=%d count=%d", instID, cnt))
			}
		}
	}
	// Also include any cell instances not present in the debug map.
	for instID, cnt := range s.cells {
		if _, has := s.debug[instID]; !has {
			b.WriteString(fmt.Sprintf("\n\tinstance=%d count=%d", instID, cnt))
		}
	}
	return b.String()
}

// IDTracker is used to track the lifecycle of an ID (a reference to an Atom).
// Release must be called when the holder is done with the ID; the framework's
// generated code emits `defer id.Release()` for this purpose. A tracker is
// bound to the instance that was live when it was acquired, so a respawn under
// the same name does not change which instance's count it decrements.
type IDTracker struct {
	state    *atomRefState
	instID   uint64
	id       uint64
	released atomic.Bool

	file string
	line int
	name string
}

// ToString returns a diagnostic string "<instID>:<id>-<file>:<line>".
func (i *IDTracker) ToString() string {
	if i == nil {
		return "nil"
	}
	return fmt.Sprintf("%d:%d-%s:%d", i.instID, i.id, i.file, i.line)
}

// Release decrements the reference count for the bound instance. It is
// idempotent: a second call is a safe no-op. When the instance's count reaches
// zero and the instance has halted, the Element is notified so it may collect
// the halted instance.
func (i *IDTracker) Release() {
	if i == nil || i.state == nil {
		return
	}
	if !i.released.CompareAndSwap(false, true) {
		return
	}
	// Detach the leak-detection finalizer on the happy path so a properly
	// released tracker does no extra work at GC time.
	runtime.SetFinalizer(i, nil)
	if i.state.release(i.instID) {
		// This instance's references just drained to zero. Notify the Element
		// so it can collect the instance if it has halted.
		i.state.element.onInstanceRefsDrained(i.state, i.instID)
	}
}

// logger returns the process logger reachable from the tracker, nil-safe at
// every hop so it can be used from a GC finalizer (where parts of the chain
// may already be torn down during process shutdown).
func (i *IDTracker) logger() *loggingAtomos {
	if i == nil || i.state == nil {
		return nil
	}
	e := i.state.element
	if e == nil || e.cosmosLocal == nil || e.cosmosLocal.process == nil {
		return nil
	}
	return e.cosmosLocal.process.logging
}

// finalizeIDTracker is the GC backstop for leak detection (debug mode only).
// If a tracker is collected without ever being Release()d, this reports it with
// its allocation site. It only reports — it does NOT mutate the refState
// (acquiring atomRefState.mu from a finalizer risks racing process teardown);
// the leaked count dies with the process.
func finalizeIDTracker(tr *IDTracker) {
	if tr == nil || tr.released.Load() {
		return
	}
	if logging := tr.logger(); logging != nil {
		logging.pushFrameworkErrorLog(
			"IDTracker: leaked (never Release()d). inst=%d id=%d alloc=%s:%d caller=%s",
			tr.instID, tr.id, tr.file, tr.line, tr.name)
	}
}

// IDTrackerInfo is used to create IDTracker in local.

func NewIDTrackerInfoFromLocalGoroutine(skip int) *IDTrackerInfo {
	tracker := &IDTrackerInfo{}
	caller, file, line, ok := runtime.Caller(skip)
	if ok {
		tracker.File = file
		tracker.Line = int32(line)
		if pc := runtime.FuncForPC(caller); pc != nil {
			tracker.Name = pc.Name()
		}
	}
	return tracker
}

// WithID is a block-scope RAII helper for an IDTracker: it guarantees Release
// is called on the given tracker when fn returns — whether by normal return,
// early return, or panic. Use it when a bare `defer id.Release()` at function
// scope would keep the reference alive longer than necessary, or to make the
// acquire/release pair visually scoped.
//
//	tr, _ := GetXxxAtomID(...)
//	return atomos.WithID(tr, func() (*Out, *Error) {
//	    // use the ID captured above; tr is released on return
//	})
//
// The tracker is constrained directly (rather than the full ID type) to avoid
// the unexported-method trap on the ID interface; callers capture their ID in
// the enclosing scope or pass it through the closure.
func WithID[R any](tr *IDTracker, fn func() (R, *Error)) (R, *Error) {
	defer tr.Release()
	return fn()
}
