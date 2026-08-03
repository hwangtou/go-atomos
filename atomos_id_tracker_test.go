package atomos

import (
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
