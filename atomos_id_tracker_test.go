package atomos

import (
	"testing"
)

// TestIDTracker_BasicRefCount covers the core IDTracker counting semantics:
// addIDTracker increments refCount, Release decrements it, and refCount
// reflects the number of outstanding trackers.
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
		if got := atom.atomos.it.refCount(); got != 1 {
			t.Fatalf("Expected refCount 1 after spawn, got %d", got)
		}

		// Acquire two more trackers.
		t2 := atom.atomos.it.addIDTracker(NewIDTrackerInfoFromLocalGoroutine(2))
		t3 := atom.atomos.it.addIDTracker(NewIDTrackerInfoFromLocalGoroutine(2))
		if got := atom.atomos.it.refCount(); got != 3 {
			t.Fatalf("Expected refCount 3 after two more addIDTracker, got %d", got)
		}

		// Release one; count drops to 2.
		t2.Release()
		if got := atom.atomos.it.refCount(); got != 2 {
			t.Fatalf("Expected refCount 2 after one Release, got %d", got)
		}

		// Release the remaining two; count drops to 0.
		t3.Release()
		tracker.Release()
		if got := atom.atomos.it.refCount(); got != 0 {
			t.Fatalf("Expected refCount 0 after all Release, got %d", got)
		}

		// ToString should report nil-safely and a reasonable id format.
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
// tracker is a safe no-op (the target repo's Release() detaches the manager
// on first Release). This guards against double-counting that could spuriously
// drive the idMap to 0 and re-trigger onIDReleased.
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
		if got := atom.atomos.it.refCount(); got != 0 {
			t.Fatalf("Expected refCount 0 after first Release, got %d", got)
		}

		// Second Release must be a no-op and must NOT drive the count negative
		// or re-trigger onIDReleased.
		tracker.Release()
		if got := atom.atomos.it.refCount(); got != 0 {
			t.Fatalf("Expected refCount still 0 after double Release, got %d", got)
		}
	})
}
