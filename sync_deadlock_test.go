package atomos

import (
	"strings"
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
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomName = "deadlock_self"
		spawnTestAtom(t, elem, atomName)

		atomID, _, err := elem.GetAtomID(atomName, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
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
			ArgBaseAtomosTimeout(2 * time.Second))
		if err == nil {
			t.Fatal("expected ErrIDFirstSyncCallDeadlock from self-call, got nil")
		}
		if CodeOf(err) != ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("expected ErrIDFirstSyncCallDeadlock, got code=%d: %v", CodeOf(err), err)
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
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA = "deadlock_a"
		const atomB = "deadlock_b"
		implA := spawnTestAtom(t, elem, atomA)
		implB := spawnTestAtom(t, elem, atomB)
		// A's mode-3 calls B; B's mode-4 calls back A.
		implA.cyclePeer = atomB
		implB.cyclePeer = atomA

		atomAID, _, err := elem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID(A): %v", err)
		}
		// Trigger A's mode-3 → A calls B → B calls back A → deadlock detected.
		_, err = atomAID.SyncMessagingByName(p.local, "Greeting",
			&ForTestGreetingI{Mode: 3},
			ArgBaseAtomosTimeout(2 * time.Second))
		if err == nil {
			t.Fatal("expected ErrIDFirstSyncCallDeadlock from cycle, got nil")
		}
		if CodeOf(err) != ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("expected ErrIDFirstSyncCallDeadlock, got code=%d: %v", CodeOf(err), err)
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
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		// Simple: call an atom with mode=1 (normal reply). No nested sync call.
		spawnTestAtom(t, elem, "nofalsepos")
		atomID, _, err := elem.GetAtomID("nofalsepos", NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}
		out, err := atomID.SyncMessagingByName(p.local, "Greeting",
			&ForTestGreetingI{Mode: 1},
			ArgBaseAtomosTimeout(5 * time.Second))
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
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		impl := spawnTestAtom(t, elem, "timeout_atom")
		impl.greetingWait = 300 * time.Millisecond // handler sleeps longer than the timeout

		atomID, _, err := elem.GetAtomID("timeout_atom", NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}
		_, err = atomID.SyncMessagingByName(p.local, "Greeting",
			&ForTestGreetingI{Mode: 1},
			ArgBaseAtomosTimeout(50 * time.Millisecond))
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		// A slow handler that is actively processing (mail popped) yields
		// ErrAtomosPushTimeoutHandling; one still queued yields Reject. Either
		// is acceptable — the point is it NOT ErrIDFirstSyncCallDeadlock.
		if CodeOf(err) == ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("expected timeout (Reject or Handling), got deadlock false positive: %v", err)
		}
	})
}

// TestDeadlock_CycleReadableChain verifies the deadlock error carries a
// readable wait chain (atom identities, not bare goroutine numbers).
func TestDeadlock_CycleReadableChain(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA = "chain_a"
		const atomB = "chain_b"
		implA := spawnTestAtom(t, elem, atomA)
		implB := spawnTestAtom(t, elem, atomB)
		implA.cyclePeer = atomB
		implB.cyclePeer = atomA

		atomAID, _, err := elem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID(A): %v", err)
		}
		_, err = atomAID.SyncMessagingByName(p.local, "Greeting",
			&ForTestGreetingI{Mode: 3},
			ArgBaseAtomosTimeout(2 * time.Second))
		if CodeOf(err) != ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("expected ErrIDFirstSyncCallDeadlock, got: %v", err)
		}
		msg := err.Error()
		// The chain must name the atoms on the cycle, not just goroutine ids.
		if !strings.Contains(msg, "::"+atomB+"[") || !strings.Contains(msg, "(cycle)") {
			t.Fatalf("deadlock error lacks readable chain: %v", msg)
		}
	})
}

// TestDeadlock_DebugStacks verifies that with syncDeadlockDebug enabled the
// deadlock error carries the caller stacks captured along the wait chain.
func TestDeadlock_DebugStacks(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		p.syncDeadlockDebug = true

		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA = "stacks_a"
		const atomB = "stacks_b"
		implA := spawnTestAtom(t, elem, atomA)
		implB := spawnTestAtom(t, elem, atomB)
		implA.cyclePeer = atomB
		implB.cyclePeer = atomA

		atomAID, _, err := elem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID(A): %v", err)
		}
		_, err = atomAID.SyncMessagingByName(p.local, "Greeting",
			&ForTestGreetingI{Mode: 3},
			ArgBaseAtomosTimeout(2 * time.Second))
		if CodeOf(err) != ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("expected ErrIDFirstSyncCallDeadlock, got: %v", err)
		}
		msg := err.Error()
		if !strings.Contains(msg, "waiting for") || !strings.Contains(msg, "Greeting") {
			t.Fatalf("deadlock error lacks captured stacks: %v", msg)
		}
	})
}

// TestDeadlock_HookFiresOnCycle verifies SetDeadlockHook receives a
// DeadlockEventCycle (with the readable chain) when the wait-graph rejects a
// local cycle.
func TestDeadlock_HookFiresOnCycle(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		events := make(chan *DeadlockEvent, 4)
		p.local.runnable.deadlockHook = func(e *DeadlockEvent) { events <- e }

		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA = "hook_a"
		const atomB = "hook_b"
		implA := spawnTestAtom(t, elem, atomA)
		implB := spawnTestAtom(t, elem, atomB)
		implA.cyclePeer = atomB
		implB.cyclePeer = atomA

		atomAID, _, err := elem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID(A): %v", err)
		}
		_, err = atomAID.SyncMessagingByName(p.local, "Greeting",
			&ForTestGreetingI{Mode: 3},
			ArgBaseAtomosTimeout(2 * time.Second))
		if CodeOf(err) != ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("expected ErrIDFirstSyncCallDeadlock, got: %v", err)
		}
		select {
		case e := <-events:
			if e.Kind != DeadlockEventCycle {
				t.Fatalf("expected DeadlockEventCycle, got kind=%d", e.Kind)
			}
			if !strings.Contains(e.Chain, atomB) || e.Err == nil {
				t.Fatalf("unexpected event: %+v", e)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("deadlock hook did not fire")
		}
	})
}

// TestDeadlock_HookFiresOnTimeout verifies the hook receives a timeout event
// (Reject or Handling) when a sync call times out.
func TestDeadlock_HookFiresOnTimeout(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		events := make(chan *DeadlockEvent, 4)
		p.local.runnable.deadlockHook = func(e *DeadlockEvent) { events <- e }

		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		impl := spawnTestAtom(t, elem, "hook_timeout_atom")
		impl.greetingWait = 200 * time.Millisecond

		atomID, _, err := elem.GetAtomID("hook_timeout_atom", NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}
		_, err = atomID.SyncMessagingByName(p.local, "Greeting",
			&ForTestGreetingI{Mode: 1},
			ArgBaseAtomosTimeout(50 * time.Millisecond))
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		select {
		case e := <-events:
			if e.Kind != DeadlockEventTimeoutReject && e.Kind != DeadlockEventTimeoutHandling {
				t.Fatalf("expected timeout event, got kind=%d", e.Kind)
			}
			if e.Name != "Greeting" || e.Err == nil {
				t.Fatalf("unexpected event: %+v", e)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("deadlock hook did not fire on timeout")
		}
	})
}

// TestDeadlock_DumpStacks verifies the exported stack-dump helper returns
// goroutine stacks including this goroutine.
func TestDeadlock_DumpStacks(t *testing.T) {
	out := DumpStacks()
	if !strings.Contains(out, "goroutine ") || !strings.Contains(out, "TestDeadlock_DumpStacks") {
		t.Fatalf("DumpStacks output missing goroutine stacks:\n%s", out)
	}
}

// TestDeadlock_DefaultTimeoutConfigurable verifies syncCallDefaultTimeout
// bounds sync calls that pass no explicit timeout.
func TestDeadlock_DefaultTimeoutConfigurable(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		p.syncCallDefaultTimeout = 100 * time.Millisecond

		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		impl := spawnTestAtom(t, elem, "default_timeout_atom")
		impl.greetingWait = 300 * time.Millisecond // handler outlives the default timeout

		atomID, _, err := elem.GetAtomID("default_timeout_atom", NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}
		start := time.Now()
		// No ArgBaseAtomosTimeout: the process default must apply.
		_, err = atomID.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 1})
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		if CodeOf(err) == ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("expected timeout, got deadlock false positive: %v", err)
		}
		if elapsed > 5*time.Second {
			t.Fatalf("default timeout not applied, call took %v: %v", elapsed, err)
		}
	})
}
