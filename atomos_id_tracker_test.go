package atomos

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// atomRefCountFor returns the outstanding reference count for the given atom's
// instance under its name. Test helper.
func atomRefCountFor(t *testing.T, e *ElementLocal, atom *AtomLocal) int64 {
	t.Helper()
	name := atom.GetIDInfo().Atom
	e.lock.RLock()
	st := e.refStates[name]
	e.lock.RUnlock()
	if st == nil {
		return 0
	}
	return st.refCount(atom.atomos.instanceID)
}

// TestIDTracker_BasicRefCount covers the core IDTracker counting semantics:
// addRef increments the instance's count, Release decrements it.
//
// This replaces the previously commented-out refCount assertions in
// atom_local_test.go / nodes_test.go, which targeted an older API and could
// not be revived as-is.
func TestIDTracker_BasicRefCount(t *testing.T) {
	// The full cosmos spawn below allocates mails. The global allocMailDebug
	// map (used by atomos_task_manager_queue_test.go) is order-sensitive and
	// stays enabled once any prior test flips it on. Save/restore it and force
	// it off here so this test never pollutes that global counter.
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatalf("Failed to get local element: %v", err)
		}

		atom, tracker, err := elem.elementAtomSpawn(nil, "test_id_tracker_atom", nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), true, true)
		if err != nil {
			t.Fatalf("Failed to spawn atom: %v", err)
		}
		if tracker == nil {
			t.Fatal("Expected tracker to be non-nil")
		}

		// One outstanding tracker from the spawn above.
		if got := atomRefCountFor(t, elem, atom); got != 1 {
			t.Fatalf("Expected refCount 1 after spawn, got %d", got)
		}

		// Acquire two more trackers against the same instance.
		t2 := elem.addRefAtom(atom, NewIDTrackerInfoFromLocalGoroutine(2))
		t3 := elem.addRefAtom(atom, NewIDTrackerInfoFromLocalGoroutine(2))
		if got := atomRefCountFor(t, elem, atom); got != 3 {
			t.Fatalf("Expected refCount 3 after two more addRef, got %d", got)
		}

		// Release one; count drops to 2.
		t2.Release()
		if got := atomRefCountFor(t, elem, atom); got != 2 {
			t.Fatalf("Expected refCount 2 after one Release, got %d", got)
		}

		// Release the remaining two; count drops to 0.
		t3.Release()
		tracker.Release()
		if got := atomRefCountFor(t, elem, atom); got != 0 {
			t.Fatalf("Expected refCount 0 after all Release, got %d", got)
		}

		// ToString should report a reasonable instID:id-file:line format.
		if got := t3.ToString(); got == "" {
			t.Fatal("Expected non-empty ToString for tracker")
		}
	})
}

// TestIDTracker_NilReleaseSafety ensures Release on a nil tracker is a no-op,
// which is the contract remote IDs rely on (they return nil *IDTracker).
func TestIDTracker_NilReleaseSafety(t *testing.T) {
	var nilTracker *IDTracker
	// Must not panic.
	nilTracker.Release()

	if got := nilTracker.ToString(); got != "nil" {
		t.Fatalf("Expected \"nil\" for nil tracker ToString, got %q", got)
	}
}

// TestIDTracker_DoubleReleaseSafety ensures a second Release() on the same
// tracker is a safe no-op (the target repo's Release() uses released CAS).
// This guards against double-counting that could spuriously drive the count
// below zero.
func TestIDTracker_DoubleReleaseSafety(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatalf("Failed to get local element: %v", err)
		}

		atom, tracker, err := elem.elementAtomSpawn(nil, "test_double_release_atom", nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), true, true)
		if err != nil {
			t.Fatalf("Failed to spawn atom: %v", err)
		}

		// First Release drops count to 0.
		tracker.Release()
		if got := atomRefCountFor(t, elem, atom); got != 0 {
			t.Fatalf("Expected refCount 0 after first Release, got %d", got)
		}

		// Second Release must be a no-op and must NOT drive the count negative
		// or re-trigger GC.
		tracker.Release()
		if got := atomRefCountFor(t, elem, atom); got != 0 {
			t.Fatalf("Expected refCount still 0 after double Release, got %d", got)
		}
	})
}

// TestIDTracker_RefSurvivesRespawn is the highest-value M2 test: it validates
// that a tracker acquired BEFORE a halt+respawn still Release()s correctly
// afterwards, decrementing its own (old) instance's cell — without affecting
// the new instance's count. This directly validates the design that replaced
// fromOld/*oldAtom=*atom.
func TestIDTracker_RefSurvivesRespawn(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatalf("Failed to get local element: %v", err)
		}

		// Spawn atom instance v1 and take a tracker T against v1.
		atomV1, T, err := elem.elementAtomSpawn(nil, "test_respawn_atom", nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), true, true)
		if err != nil {
			t.Fatalf("Failed to spawn atom: %v", err)
		}
		instV1 := atomV1.atomos.instanceID

		// Halt v1: control shutdown timing via the test impl.
		oldImpl := atomV1.atomos.instance.(*testRunnableAtom)
		oldImpl.haltWait = 10 * time.Millisecond
		oldImpl.haltNotify = make(chan struct{}, 1)
		atomV1.KillSelf()
		<-oldImpl.haltNotify

		// Respawn under the same name -> v2 (new instanceID).
		atomV2, newTracker, err := elem.elementAtomSpawn(nil, "test_respawn_atom", nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), true, true)
		if err != nil {
			t.Fatalf("Failed to respawn atom: %v", err)
		}
		instV2 := atomV2.atomos.instanceID
		if instV1 == instV2 {
			t.Fatalf("Expected new instanceID after respawn; v1=%d v2=%d", instV1, instV2)
		}

		// The new instance has exactly 1 ref (its own spawn tracker).
		if got := atomRefCountFor(t, elem, atomV2); got != 1 {
			t.Fatalf("Expected v2 refCount 1 after respawn, got %d", got)
		}

		// Releasing T (the v1-era tracker) must NOT panic and must NOT affect
		// v2's count. It drains v1's cell.
		T.Release()
		if got := atomRefCountFor(t, elem, atomV2); got != 1 {
			t.Fatalf("v2 refCount changed after releasing v1 tracker: got %d, want 1", got)
		}

		// New tracker release drains v2.
		newTracker.Release()
		if got := atomRefCountFor(t, elem, atomV2); got != 0 {
			t.Fatalf("Expected v2 refCount 0 after releasing new tracker, got %d", got)
		}
	})
}

// BenchmarkIDTracker_AddRefRelease measures the addRef+Release hot path,
// comparing idTrackerDebug on (runtime.Caller + IDTrackerInfo allocation per
// call) vs off (nil info, no caller capture).
//
// Run: go test -bench=AddRefRelease -benchmem
func BenchmarkIDTracker_AddRefRelease(b *testing.B) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	// Stand up a real cosmos process so the element/atom infrastructure is live.
	id := &IDInfo{Type: IDType_Atom, Cosmos: "bench_cosmos", Node: "bench_node", Element: "bench_elem", Atom: "bench_atom"}
	p, err := newCosmosProcess(id.Cosmos, id.Node, newTestLogging(b))
	if err != nil {
		b.Fatalf("newCosmosProcess: %v", err)
	}
	r := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: id.Cosmos, Node: id.Node})
	if err := p.Start(r); err != nil {
		b.Fatalf("Start: %v", err)
	}
	defer func() { _ = p.Stop() }()

	elem, err := p.local.getLocalElement(ForTestAtomosName)
	if err != nil {
		b.Fatalf("getLocalElement: %v", err)
	}
	atom, _, err := elem.elementAtomSpawn(nil, "bench_atom", nil, elem.elemImpl, nil, &IDTrackerInfo{}, false, true)
	if err != nil {
		b.Fatalf("elementAtomSpawn: %v", err)
	}

	// Sub-benchmark: idTrackerDebug off (production hot path).
	b.Run("DebugOff", func(b *testing.B) {
		p.idTrackerDebug = false
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			tr := elem.addRefAtom(atom, nil)
			tr.Release()
		}
	})

	// Sub-benchmark: idTrackerDebug on (captures runtime.Caller per call).
	b.Run("DebugOn", func(b *testing.B) {
		p.idTrackerDebug = true
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			tr := elem.addRefAtom(atom, NewIDTrackerInfoFromLocalGoroutine(2))
			tr.Release()
		}
	})

	p.idTrackerDebug = false
}

// captureLogging is an appLogging that records all messages into a slice, so
// tests can assert on framework log output (used by the finalizer leak tests).
type captureLogging struct {
	mu   sync.Mutex
	logs []string
}

func (c *captureLogging) WriteAccessLog(s string) {
	c.mu.Lock()
	c.logs = append(c.logs, s)
	c.mu.Unlock()
}
func (c *captureLogging) WriteErrorLog(s string) {
	c.mu.Lock()
	c.logs = append(c.logs, s)
	c.mu.Unlock()
}
func (c *captureLogging) Close() {}

func (c *captureLogging) hasLog(substr string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.logs {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

// forceGC runs the GC enough times to queue and run finalizers, with a bounded
// retry loop since finalizer execution is not instantaneous.
func forceGCAndRunFinalizers(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		runtime.GC()
		runtime.GC() // first GC queues finalizers, second runs them
		time.Sleep(5 * time.Millisecond)
	}
}

// TestIDTracker_LeakReportedOnGC verifies the finalizer backstop: a tracker
// that is never Release()d and gets collected reports a leak (debug mode only).
func TestIDTracker_LeakReportedOnGC(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	capture := &captureLogging{}
	id := &IDInfo{Type: IDType_Atom, Cosmos: "leak_cosmos", Node: "leak_node", Element: "leak_elem", Atom: "leak_atom"}
	p, err := newCosmosProcess(id.Cosmos, id.Node, capture)
	if err != nil {
		t.Fatalf("newCosmosProcess: %v", err)
	}
	r := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: id.Cosmos, Node: id.Node})
	if err := p.Start(r); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = p.Stop() }()

	p.idTrackerDebug = true // enable finalizer registration

	elem, err := p.local.getLocalElement(ForTestAtomosName)
	if err != nil {
		t.Fatalf("getLocalElement: %v", err)
	}
	atom, _, err := elem.elementAtomSpawn(nil, "leak_test_atom", nil, elem.elemImpl, nil, &IDTrackerInfo{}, false, true)
	if err != nil {
		t.Fatalf("elementAtomSpawn: %v", err)
	}
	// Acquire a tracker and DON'T release it — drop the reference so it becomes
	// eligible for GC. The debug map (atomRefState.debug) holds a pointer to
	// every tracker for enumeration, which would keep it alive; clear that slot
	// so the tracker is collectable while the cell (int64) remains to simulate
	// the leak.
	leaked := elem.addRefAtom(atom, &IDTrackerInfo{File: "leak_test.go", Line: 42})
	_ = leaked // mark used; we intentionally drop the reference below to leak it
	instID := atom.atomos.instanceID
	name := atom.GetIDInfo().Atom
	// Detach the tracker from the debug map so it can be GC'd (the leaked count
	// in cells is preserved — that's the actual leak the finalizer detects).
	elem.lock.Lock()
	if st := elem.refStates[name]; st != nil && st.debug != nil {
		delete(st.debug, instID)
	}
	elem.lock.Unlock()

	// Drop all references to the tracker.
	leaked = nil
	atom = nil
	elem = nil

	forceGCAndRunFinalizers(t, 2*time.Second)

	if !capture.hasLog("IDTracker: leaked") {
		t.Fatalf("Expected leak log after GC, got logs=%v", capture.logs)
	}
	if !capture.hasLog("leak_test.go:42") {
		t.Fatalf("Expected leak log to include alloc site leak_test.go:42, got logs=%v", capture.logs)
	}
}

// TestIDTracker_FinalizerNotFiredAfterRelease verifies the happy path: a
// properly Release()d tracker does NOT trigger the leak finalizer.
func TestIDTracker_FinalizerNotFiredAfterRelease(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	capture := &captureLogging{}
	id := &IDInfo{Type: IDType_Atom, Cosmos: "noleak_cosmos", Node: "noleak_node", Element: "noleak_elem", Atom: "noleak_atom"}
	p, err := newCosmosProcess(id.Cosmos, id.Node, capture)
	if err != nil {
		t.Fatalf("newCosmosProcess: %v", err)
	}
	r := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: id.Cosmos, Node: id.Node})
	if err := p.Start(r); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = p.Stop() }()

	p.idTrackerDebug = true

	elem, err := p.local.getLocalElement(ForTestAtomosName)
	if err != nil {
		t.Fatalf("getLocalElement: %v", err)
	}
	atom, _, err := elem.elementAtomSpawn(nil, "noleak_test_atom", nil, elem.elemImpl, nil, &IDTrackerInfo{}, false, true)
	if err != nil {
		t.Fatalf("elementAtomSpawn: %v", err)
	}
	tr := elem.addRefAtom(atom, &IDTrackerInfo{File: "noleak_test.go", Line: 1})
	tr.Release() // happy path: properly released
	tr = nil

	forceGCAndRunFinalizers(t, 2*time.Second)

	if capture.hasLog("IDTracker: leaked") {
		t.Fatalf("Did NOT expect leak log after proper Release, got logs=%v", capture.logs)
	}
}

// TestWithID_ReleasesOnPanic verifies the RAII helper releases even on panic.
func TestWithID_ReleasesOnPanic(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatalf("getLocalElement: %v", err)
		}
		atom, _, err := elem.elementAtomSpawn(nil, "withid_panic_atom", nil, elem.elemImpl, nil, &IDTrackerInfo{}, false, true)
		if err != nil {
			t.Fatalf("elementAtomSpawn: %v", err)
		}
		tr := elem.addRefAtom(atom, nil)
		if atomRefCountFor(t, elem, atom) != 2 { // spawn(1) + this addRef(1)
			t.Fatalf("expected refCount 2 before WithID, got %d", atomRefCountFor(t, elem, atom))
		}

		// WithID must release even if fn panics.
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic to propagate")
				}
			}()
			_, _ = WithID(tr, func() (struct{}, *Error) {
				panic("boom")
			})
		}()

		// tr was released by WithID's defer → count back to 1 (the spawn tracker).
		if got := atomRefCountFor(t, elem, atom); got != 1 {
			t.Fatalf("expected refCount 1 after WithID panic-Release, got %d", got)
		}
	})
}

// TestWithID_NormalReturn verifies the RAII helper releases on normal return.
func TestWithID_NormalReturn(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatalf("getLocalElement: %v", err)
		}
		atom, _, err := elem.elementAtomSpawn(nil, "withid_normal_atom", nil, elem.elemImpl, nil, &IDTrackerInfo{}, false, true)
		if err != nil {
			t.Fatalf("elementAtomSpawn: %v", err)
		}
		tr := elem.addRefAtom(atom, nil)

		out, e := WithID(tr, func() (string, *Error) {
			return "ok", nil
		})
		if e != nil || out != "ok" {
			t.Fatalf("WithID returned unexpected: out=%v err=%v", out, e)
		}
		if got := atomRefCountFor(t, elem, atom); got != 1 {
			t.Fatalf("expected refCount 1 after WithID normal return, got %d", atomRefCountFor(t, elem, atom))
		}
	})
}

// TestCosmosSpawnAtom_SucceedsWithDebugOff is a regression test for the M3
// hot-path optimization (commit 9a6290e). That commit gated construction of
// IDTrackerInfo behind idTrackerDebug inside CosmosLocal.CosmosSpawnAtom /
// CosmosGetAtomID to skip the runtime.Caller cost in production. The original
// implementation passed a nil *IDTrackerInfo when debug was off, which tripped
// the non-nil guard in ElementLocal.SpawnAtom/GetAtomID
// ("fromLocalOrRemote requires a tracker") and made every local spawn/get
// through the Cosmos layer fail silently in production (debug default false).
//
// This test exercises the FULL cosmos-layer path (CosmosSpawnAtom then
// CosmosGetAtomID), not the lower-level ElementLocal.elementAtomSpawn that the
// other tests in this file use directly, and asserts success with
// idTrackerDebug == false — the combination that regressed. All existing
// spawn tests bypassed CosmosSpawnAtom (calling elementAtomSpawn with a
// non-nil tracker by hand), which is why the regression escaped CI.
func TestCosmosSpawnAtom_SucceedsWithDebugOff(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		// Production default: debug diagnostics off. This is the exact
		// combination that regressed under M3.
		if p.idTrackerDebug {
			t.Fatalf("test precondition: expected idTrackerDebug=false, got true")
		}

		const atomName = "cosmos_spawn_debug_off_atom"

		// Spawn through the Cosmos layer — the path generated code
		// (SpawnForTestAtomosAtom) and all real callers use. Before the fix
		// this returned ErrFrameworkInternalError ("id tracker is nil").
		id, tracker, err := p.local.CosmosSpawnAtom(p.local, ForTestAtomosName, atomName, &ForTestSpawnArg{})
		if err != nil {
			t.Fatalf("CosmosSpawnAtom failed with debug off: %v", err)
		}
		if id == nil {
			t.Fatal("CosmosSpawnAtom returned nil ID with no error")
		}
		if tracker == nil {
			t.Fatal("CosmosSpawnAtom returned nil tracker — caller cannot Release the reference (leak)")
		}
		defer tracker.Release()

		// The same nil-tracker regression also affected CosmosGetAtomID (M3
		// touched both). Exercise it against the atom just spawned.
		id2, tracker2, err := p.local.CosmosGetAtomID(ForTestAtomosName, atomName)
		if err != nil {
			t.Fatalf("CosmosGetAtomID failed with debug off: %v", err)
		}
		if id2 == nil {
			t.Fatal("CosmosGetAtomID returned nil ID with no error")
		}
		if tracker2 == nil {
			t.Fatal("CosmosGetAtomID returned nil tracker")
		}
		tracker2.Release()
	})
}


