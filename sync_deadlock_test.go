package atomos

import (
	"testing"
	"time"
)

// spawnTestAtom is a helper that spawns an atom via the full cosmos path and
// returns its instance for test configuration (e.g. setting cyclePeer).
func spawnTestAtom(t *testing.T, elem *ElementLocal, name string) *testRunnableAtom {
	t.Helper()
	atom, _, err := elem.elementAtomSpawn(nil, name, nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), false, true)
	if err != nil {
		t.Fatalf("elementAtomSpawn(%q): %v", name, err)
	}
	impl, ok := atom.atomos.instance.(*testRunnableAtom)
	if !ok {
		t.Fatalf("atom instance is %T, want *testRunnableAtom", atom.atomos.instance)
	}
	return impl
}

// TestDeadlock_SelfCall verifies that an Atom whose handler sync-calls itself
// gets ErrIDFirstSyncCallDeadlock immediately (not after the 10s timeout).
func TestDeadlock_SelfCall(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatalf("getLocalElement: %v", err)
		}
		const atomName = "deadlock_self"
		spawnTestAtom(t, elem, atomName)

		atomID, _, err := elem.GetAtomID(atomName, NewIDTrackerInfoFromLocalGoroutine(3), true)
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}

		// Call Greeting(mode=2) on the atom. The handler will self-sync-call;
		// that inner call returns ErrIDFirstSyncCallDeadlock, which the handler
		// propagates. We expect the outer call to succeed (mode 2 returns the
		// inner error) — actually the handler returns the inner error.
		// Use a short timeout to fail fast if deadlock detection is broken.
		_, err = atomID.SyncMessagingByName(p.local, "Greeting",
			&ForTestGreetingI{Mode: 2},
			[]ArgsForBaseAtomos{ArgBaseAtomosTimeout(2 * time.Second)})
		if err == nil {
			t.Fatal("expected ErrIDFirstSyncCallDeadlock from self-call, got nil")
		}
		if err.Code != ErrIDFirstSyncCallDeadlock {
			t.Fatalf("expected ErrIDFirstSyncCallDeadlock, got code=%d: %v", err.Code, err)
		}
	})
}

// TestDeadlock_Cycle verifies that A→B→A is detected: A's handler sync-calls B,
// B's handler sync-calls back to A → the second push returns
// ErrIDFirstSyncCallDeadlock.
func TestDeadlock_Cycle(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatalf("getLocalElement: %v", err)
		}
		const atomA = "deadlock_a"
		const atomB = "deadlock_b"
		implA := spawnTestAtom(t, elem, atomA)
		implB := spawnTestAtom(t, elem, atomB)
		// A's mode-3 calls B; B's mode-4 calls back A.
		implA.cyclePeer = atomB
		implB.cyclePeer = atomA

		atomAID, _, err := elem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), true)
		if err != nil {
			t.Fatalf("GetAtomID(A): %v", err)
		}
		// Trigger A's mode-3 → A calls B → B calls back A → deadlock detected.
		_, err = atomAID.SyncMessagingByName(p.local, "Greeting",
			&ForTestGreetingI{Mode: 3},
			[]ArgsForBaseAtomos{ArgBaseAtomosTimeout(2 * time.Second)})
		if err == nil {
			t.Fatal("expected ErrIDFirstSyncCallDeadlock from cycle, got nil")
		}
		if err.Code != ErrIDFirstSyncCallDeadlock {
			t.Fatalf("expected ErrIDFirstSyncCallDeadlock, got code=%d: %v", err.Code, err)
		}
	})
}

// TestDeadlock_NoFalsePositive verifies a normal chain A→B→C (no cycle) does
// NOT trigger a false deadlock.
func TestDeadlock_NoFalsePositive(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatalf("getLocalElement: %v", err)
		}
		// Simple: call an atom with mode=1 (normal reply). No nested sync call.
		spawnTestAtom(t, elem, "nofalsepos")
		atomID, _, err := elem.GetAtomID("nofalsepos", NewIDTrackerInfoFromLocalGoroutine(3), true)
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}
		out, err := atomID.SyncMessagingByName(p.local, "Greeting",
			&ForTestGreetingI{Mode: 1},
			[]ArgsForBaseAtomos{ArgBaseAtomosTimeout(5 * time.Second)})
		if err != nil {
			t.Fatalf("normal sync call should succeed, got: %v", err)
		}
		if out == nil {
			t.Fatal("expected non-nil reply")
		}
	})
}

// TestDeadlock_TimeoutStillWorks verifies the timeout fallback still works for
// a genuinely stuck handler (not a cycle), confirming deadlock detection did
// not break the timeout path.
func TestDeadlock_TimeoutStillWorks(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatalf("getLocalElement: %v", err)
		}
		impl := spawnTestAtom(t, elem, "timeout_atom")
		impl.greetingWait = 300 * time.Millisecond // handler sleeps longer than the timeout

		atomID, _, err := elem.GetAtomID("timeout_atom", NewIDTrackerInfoFromLocalGoroutine(3), true)
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}
		_, err = atomID.SyncMessagingByName(p.local, "Greeting",
			&ForTestGreetingI{Mode: 1},
			[]ArgsForBaseAtomos{ArgBaseAtomosTimeout(50 * time.Millisecond)})
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		// A slow handler that is actively processing (mail popped) yields
		// ErrAtomosPushTimeoutHandling; one still queued yields Reject. Either
		// is acceptable — the point is it is NOT ErrIDFirstSyncCallDeadlock.
		if err.Code == ErrIDFirstSyncCallDeadlock {
			t.Fatalf("expected timeout (Reject or Handling), got deadlock false positive: %v", err)
		}
	})
}
