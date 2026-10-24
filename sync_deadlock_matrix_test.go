package atomos

// Exhaustive deadlock-scenario matrix. Each test maps one row of the
// enumeration below. "Immediate 84" = the wait-graph rejects the closing push
// with ErrIDFirstSyncCallDeadlock before blocking; "timeout-bounded" = the
// scenario is invisible to cycle detection (documented blind spot) and the
// assertion is that timeouts unbind everyone instead of hanging.
//
// Ring shape (local):
//  L1  self-call A→A ...................... immediate 84 (TestDeadlock_SelfCall)
//  L2  nested two-ring A→B→A .............. immediate 84 (TestDeadlock_Cycle)
//  L3  simultaneous mutual A↔B ............ immediate 84, exactly one side
//                                          (TestDeadlockMatrix_SimultaneousMutualLocal)
//  L4  long ring A→B→C→A .................. immediate 84
//                                          (TestDeadlockMatrix_ThreeAtomRing)
//  L5  atom→element→atom ring ............. immediate 84 (element mailbox is
//                                          in the same wait-graph)
//                                          (TestDeadlockMatrix_AtomElementAtomRing)
// Ring shape (cross-node):
//  C1  A@n1→B@n2→A@n1 .................... immediate 84 (TestDeadlock_CrossNodeCycle)
//  C2  with local hop A@n1→B@n2→C@n2→A@n1  immediate 84, chain survives local
//                                          hops (TestDeadlockMatrix_CrossNodeWithLocalHop)
//  C3  simultaneous cross-node mutual ..... timeout-bounded: path-pushing only
//                                          knows its own ancestry, not concurrent
//                                          siblings (TestDeadlockMatrix_CrossNodeSimultaneous)
// Invisible to the wait-graph (timeout is the only protection):
//  I1  user-lock entanglement ............. timeout-bounded
//                                          (TestDeadlockMatrix_UserLockEntanglement)
//  I2  wormhole ring (PushWormholeMailAndWaitReply registers no wait edge)
//                                          timeout-bounded (TestDeadlockMatrix_WormholeCycle)
//  I3  spawn entanglement (startup waits are not wait-graph edges)
//                                          bounded (TestDeadlockMatrix_SpawnEntanglement)
// Negatives (must NOT deadlock / must NOT false-positive):
//  N1  async-then-sync-back ................ succeeds — async is the fix
//                                          (TestDeadlockMatrix_AsyncBreaksCycle)
//  N2  deep chain A→B→C→D .................. succeeds (TestDeadlockMatrix_DeepChain)
//  N3  queued waiters on kill .............. TestDeadlock_KillWakesQueuedSyncWaiter
//
// Analyzer-side scenarios live in cmd/atomos-vet/syncdeadlock/testdata.

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// matrixCallPeer returns a closure that sync-calls a local peer atom by name
// with the given Greeting mode. Accepts any SelfID (atom or element caller).
func matrixCallPeer(t *testing.T, peer string, mode ForTestGreetingI_TestMode, timeout time.Duration) func(self SelfID) *Error {
	return func(self SelfID) *Error {
		id, e := GetForTestAtomosAtomID(self.Cosmos(), peer)
		if e != nil {
			return asError(e).AddStack(self)
		}
		defer id.Release()
		_, e = id.Greeting(self, &ForTestGreetingI{Mode: mode}, ArgBaseAtomosTimeout(timeout))
		if e != nil {
			return asError(e).AddStack(self)
		}
		return nil
	}
}

// matrixCallAtom adapts matrixCallPeer to the remoteHook signature.
func matrixCallAtom(t *testing.T, peer string, mode ForTestGreetingI_TestMode, timeout time.Duration) func(self AtomSelfID) *Error {
	inner := matrixCallPeer(t, peer, mode, timeout)
	return func(self AtomSelfID) *Error { return inner(self) }
}

// L3: two handlers, gated so both are running before either calls out. The
// two outbound pushes race; the atomic check+register under waitMu must
// reject exactly one of them with the deadlock error, and both callers must
// complete quickly (no timeout wait).
func TestDeadlockMatrix_SimultaneousMutualLocal(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA = "simul_a"
		const atomB = "simul_b"
		implA := spawnTestAtom(t, elem, atomA)
		implB := spawnTestAtom(t, elem, atomB)

		gateA, gateB := make(chan struct{}), make(chan struct{})
		errA, errB := make(chan error, 1), make(chan error, 1)
		doneA, doneB := make(chan error, 1), make(chan error, 1)

		implA.greetingGate = gateA
		implB.greetingGate = gateB
		implA.greetingNotify = make(chan struct{}, 1)
		implB.greetingNotify = make(chan struct{}, 1)
		implA.remoteHook = func(self AtomSelfID) *Error {
			e := matrixCallPeer(t, atomB, 1, 3*time.Second)(self)
			errA <- errOrNil(e)
			return e
		}
		implB.remoteHook = func(self AtomSelfID) *Error {
			e := matrixCallPeer(t, atomA, 1, 3*time.Second)(self)
			errB <- errOrNil(e)
			return e
		}

		genA, err := GetForTestAtomosAtomID(p.local, atomA)
		if err != nil {
			t.Fatalf("GetForTestAtomosAtomID(A): %v", err)
		}
		defer genA.Release()
		genB, err := GetForTestAtomosAtomID(p.local, atomB)
		if err != nil {
			t.Fatalf("GetForTestAtomosAtomID(B): %v", err)
		}
		defer genB.Release()

		start := time.Now()
		genA.AsyncGreeting(p.local, &ForTestGreetingI{Mode: 5}, func(out *ForTestGreetingO, err error) { doneA <- err })
		genB.AsyncGreeting(p.local, &ForTestGreetingI{Mode: 5}, func(out *ForTestGreetingO, err error) { doneB <- err })

		// Both handlers are now inside Greeting, blocked on their gates.
		<-implA.greetingNotify
		<-implB.greetingNotify
		close(gateA)
		close(gateB)

		var innerA, innerB error
		select {
		case innerA = <-errA:
		case <-time.After(5 * time.Second):
			t.Fatal("A's outbound call did not finish")
		}
		select {
		case innerB = <-errB:
		case <-time.After(5 * time.Second):
			t.Fatal("B's outbound call did not finish")
		}
		for i := 0; i < 2; i++ {
			select {
			case <-doneA:
			case <-doneB:
			case <-time.After(5 * time.Second):
				t.Fatal("async caller callback did not finish")
			}
		}

		// Exactly one side's push is rejected as the deadlock; the other
		// side's error is that failure propagated back through the peer.
		c84 := 0
		for _, e := range []error{innerA, innerB} {
			if e != nil && CodeOf(e) == ErrCodeIDFirstSyncCallDeadlock {
				c84++
			}
		}
		if c84 != 1 {
			t.Fatalf("expected exactly one deadlock rejection, got %d (A=%v B=%v)", c84, innerA, innerB)
		}
		if elapsed := time.Since(start); elapsed > 4*time.Second {
			t.Fatalf("simultaneous mutual calls should resolve immediately, took %v", elapsed)
		}
	})
}

// L4: three-atom ring A→B→C→A. The closing push (C→A) walks gA→gB→gC and
// finds itself: immediate 84.
func TestDeadlockMatrix_ThreeAtomRing(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA, atomB, atomC = "ring3_a", "ring3_b", "ring3_c"
		spawnTestAtom(t, elem, atomA)
		spawnTestAtom(t, elem, atomB)
		spawnTestAtom(t, elem, atomC)
		implA := mustImpl(t, elem, atomA)
		implB := mustImpl(t, elem, atomB)
		implC := mustImpl(t, elem, atomC)
		implA.remoteHook = matrixCallAtom(t, atomB, 5, 3*time.Second)
		implB.remoteHook = matrixCallAtom(t, atomC, 5, 3*time.Second)
		implC.remoteHook = matrixCallAtom(t, atomA, 1, 3*time.Second)

		atomAID, _, err := elem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID(A): %v", err)
		}
		start := time.Now()
		_, err = atomAID.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(3*time.Second))
		if CodeOf(err) != ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("expected ErrIDFirstSyncCallDeadlock, got: %v", err)
		}
		if !strings.Contains(err.Error(), "(cycle)") {
			t.Fatalf("error should carry the readable chain: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("ring should be detected immediately, took %v", elapsed)
		}
	})
}

// L5: atom→element→atom. The element is a mailbox in the same wait-graph:
// A pushes to the element (edge gA→gElem), the element handler pushes back to
// A and closes the ring: immediate 84.
func TestDeadlockMatrix_AtomElementAtomRing(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA = "elemring_a"
		spawnTestAtom(t, elem, atomA)
		implA := mustImpl(t, elem, atomA)

		elemImpl, ok := elem.atomos.instance.(*testRunnableElement)
		if !ok {
			t.Fatalf("element impl is %T", elem.atomos.instance)
		}
		elemID, err := p.local.CosmosGetElementID(ForTestAtomosName)
		if err != nil {
			t.Fatalf("CosmosGetElementID: %v", err)
		}

		// The element handler calls back into A — the closing edge.
		elemImpl.sayHelloHook = func(from ID) *Error {
			return matrixCallPeer(t, atomA, 1, 3*time.Second)(elemImpl.self)
		}
		// A calls the element.
		implA.remoteHook = func(self AtomSelfID) *Error {
			_, e := elemID.SyncMessagingByName(self, "SayHello", &ForTestHelloI{}, ArgBaseAtomosTimeout(3*time.Second))
			if e != nil {
				return asError(e).AddStack(self)
			}
			return nil
		}

		atomAID, _, err2 := elem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err2 != nil {
			t.Fatalf("GetAtomID(A): %v", err2)
		}
		start := time.Now()
		_, err = atomAID.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(3*time.Second))
		if CodeOf(err) != ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("expected ErrIDFirstSyncCallDeadlock, got: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("atom→element→atom ring should be detected immediately, took %v", elapsed)
		}
	})
}

// C2: cross-node ring with a local hop on the far node —
// A@src →(remote) B@tgt →(local) C@tgt →(remote) A@src. The wait chain rides
// the local hop through the mail's ambient chain: immediate 84.
func TestDeadlockMatrix_CrossNodeWithLocalHop(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 50800, "hopx_cosmos", "hopx_node")
	defer cluster.close()

	const (
		nodeSource = "hopx_node_source"
		nodeTarget = "hopx_node_target"
		atomA      = "hop_a"
		atomB      = "hop_b"
		atomC      = "hop_c"
	)

	srcElem, gerr := cluster.sourceProcess.local.getLocalElement(ForTestAtomosName)
	if gerr != nil {
		t.Fatalf("getLocalElement(source): %v", gerr)
	}
	tgtElem, gerr := cluster.targetProcess.local.getLocalElement(ForTestAtomosName)
	if gerr != nil {
		t.Fatalf("getLocalElement(target): %v", gerr)
	}
	spawnTestAtom(t, srcElem, atomA)
	spawnTestAtom(t, tgtElem, atomB)
	spawnTestAtom(t, tgtElem, atomC)
	implA := mustImpl(t, srcElem, atomA)
	implB := mustImpl(t, tgtElem, atomB)
	implC := mustImpl(t, tgtElem, atomC)

	srcToTarget := cluster.sourceProcess.cluster.remoteCosmos[nodeTarget].elements[ForTestAtomosName]
	tgtToSource := cluster.targetProcess.cluster.remoteCosmos[nodeSource].elements[ForTestAtomosName]

	implA.remoteHook = func(self AtomSelfID) *Error {
		id, tracker, e := srcToTarget.GetAtomID(atomB, nil)
		if e != nil {
			return asError(e).AddStack(self)
		}
		defer tracker.Release()
		_, e = id.SyncMessagingByName(self, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(5*time.Second))
		if e != nil {
			return asError(e).AddStack(self)
		}
		return nil
	}
	// Local hop on the target node: the ambient chain of B's executing mail
	// is inherited by this local push to C.
	implB.remoteHook = matrixCallAtom(t, atomC, 5, 5*time.Second)
	implC.remoteHook = func(self AtomSelfID) *Error {
		id, tracker, e := tgtToSource.GetAtomID(atomA, nil)
		if e != nil {
			return asError(e).AddStack(self)
		}
		defer tracker.Release()
		_, e = id.SyncMessagingByName(self, "Greeting", &ForTestGreetingI{Mode: 1}, ArgBaseAtomosTimeout(5*time.Second))
		if e != nil {
			return asError(e).AddStack(self)
		}
		return nil
	}

	atomAID, _, err := srcElem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
	if err != nil {
		t.Fatalf("GetAtomID(A): %v", err)
	}
	start := time.Now()
	_, err = atomAID.SyncMessagingByName(cluster.sourceProcess.local, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(10*time.Second))
	if CodeOf(err) != ErrCodeIDFirstSyncCallDeadlock {
		t.Fatalf("expected ErrIDFirstSyncCallDeadlock, got: %v", err)
	}
	if !strings.Contains(err.Error(), "cross-node cycle") {
		t.Fatalf("error should mention cross-node cycle: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 9*time.Second {
		t.Fatalf("ring with local hop should be detected immediately, took %v", elapsed)
	}
}

// C3: simultaneous cross-node mutual calls (A@src→B@tgt while B@tgt→A@src,
// both from running handlers). Path-pushing only knows each call's own
// ancestry — neither chain contains the other's target — so this cannot be
// detected as a cycle; the assertion is that client deadlines unbind both
// sides. This is the documented limit of per-request chain propagation
// (a global coordinator would be needed for these).
func TestDeadlockMatrix_CrossNodeSimultaneous(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 50900, "xsim_cosmos", "xsim_node")
	defer cluster.close()

	const (
		nodeSource = "xsim_node_source"
		nodeTarget = "xsim_node_target"
		atomA      = "xsim_a"
		atomB      = "xsim_b"
	)

	srcElem, gerr := cluster.sourceProcess.local.getLocalElement(ForTestAtomosName)
	if gerr != nil {
		t.Fatalf("getLocalElement(source): %v", gerr)
	}
	tgtElem, gerr := cluster.targetProcess.local.getLocalElement(ForTestAtomosName)
	if gerr != nil {
		t.Fatalf("getLocalElement(target): %v", gerr)
	}
	implA := spawnTestAtom(t, srcElem, atomA)
	implB := spawnTestAtom(t, tgtElem, atomB)

	srcToTarget := cluster.sourceProcess.cluster.remoteCosmos[nodeTarget].elements[ForTestAtomosName]
	tgtToSource := cluster.targetProcess.cluster.remoteCosmos[nodeSource].elements[ForTestAtomosName]

	const callTimeout = 700 * time.Millisecond
	remoteCall := func(elem *elementRemoteFromSource, peer string, self AtomSelfID) *Error {
		id, tracker, e := elem.GetAtomID(peer, nil)
		if e != nil {
			return asError(e).AddStack(self)
		}
		defer tracker.Release()
		_, e = id.SyncMessagingByName(self, "Greeting", &ForTestGreetingI{Mode: 1}, ArgBaseAtomosTimeout(callTimeout))
		if e != nil {
			return asError(e).AddStack(self)
		}
		return nil
	}

	gateA, gateB := make(chan struct{}), make(chan struct{})
	errA, errB := make(chan error, 1), make(chan error, 1)
	implA.greetingGate = gateA
		implB.greetingGate = gateB
		implA.greetingNotify = make(chan struct{}, 1)
		implB.greetingNotify = make(chan struct{}, 1)
		implA.remoteHook = func(self AtomSelfID) *Error {
			e := remoteCall(srcToTarget, atomB, self)
			errA <- errOrNil(e)
			return e
		}
		implB.remoteHook = func(self AtomSelfID) *Error {
			e := remoteCall(tgtToSource, atomA, self)
			errB <- errOrNil(e)
			return e
		}

		genA, err := GetForTestAtomosAtomID(cluster.sourceProcess.local, atomA)
		if err != nil {
			t.Fatalf("GetForTestAtomosAtomID(A): %v", err)
		}
		defer genA.Release()
		genB, err := GetForTestAtomosAtomID(cluster.targetProcess.local, atomB)
		if err != nil {
			t.Fatalf("GetForTestAtomosAtomID(B): %v", err)
		}
		defer genB.Release()

		start := time.Now()
		genA.AsyncGreeting(cluster.sourceProcess.local, &ForTestGreetingI{Mode: 5}, func(*ForTestGreetingO, error) {})
		genB.AsyncGreeting(cluster.targetProcess.local, &ForTestGreetingI{Mode: 5}, func(*ForTestGreetingO, error) {})
	<-implA.greetingNotify
	<-implB.greetingNotify
	close(gateA)
	close(gateB)

	var innerA, innerB error
	select {
	case innerA = <-errA:
	case <-time.After(4 * time.Second):
		t.Fatal("A's remote call did not finish — cross-node simultaneous calls must be deadline-bounded")
	}
	select {
	case innerB = <-errB:
	case <-time.After(4 * time.Second):
		t.Fatal("B's remote call did not finish — cross-node simultaneous calls must be deadline-bounded")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("both sides should unbind via deadline quickly, took %v", elapsed)
	}
	for name, e := range map[string]error{"A": innerA, "B": innerB} {
		if e == nil {
			t.Fatalf("side %s: expected deadline error, got nil", name)
		}
		if CodeOf(e) == ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("side %s: simultaneous cross-node calls are NOT detectable as cycles; unexpected 84: %v", name, e)
		}
	}
}

// I1: user-lock entanglement — A holds a mutex and calls B; B's handler needs
// the same mutex. The wait-graph cannot see user locks, so this degrades to
// A's timeout (TimeoutHandling: B's handler is executing, blocked on the
// mutex). Assert bounded, and explicitly NOT a false deadlock report.
func TestDeadlockMatrix_UserLockEntanglement(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA, atomB = "lock_a", "lock_b"
		spawnTestAtom(t, elem, atomA)
		spawnTestAtom(t, elem, atomB)
		implA := mustImpl(t, elem, atomA)
		implB := mustImpl(t, elem, atomB)

		var mu sync.Mutex
		implA.remoteHook = func(self AtomSelfID) *Error {
			mu.Lock()
			defer mu.Unlock()
			// Mode 5 so B's handler actually runs its lock branch.
			return matrixCallAtom(t, atomB, 5, 300*time.Millisecond)(self)
		}
		implB.remoteHook = func(self AtomSelfID) *Error {
			mu.Lock()
			defer mu.Unlock()
			return nil
		}

		atomAID, _, err := elem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID(A): %v", err)
		}
		start := time.Now()
		_, err = atomAID.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(3*time.Second))
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("expected timeout error from user-lock entanglement")
		}
		if CodeOf(err) == ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("user locks are invisible to the wait-graph; deadlock report would be a false positive: %v", err)
		}
		if CodeOf(err) != ErrCodeAtomosPushTimeoutHandling {
			t.Fatalf("expected TimeoutHandling (B's handler is executing), got: %v", err)
		}
		if elapsed > 2*time.Second {
			t.Fatalf("entanglement should unbind at the 300ms timeout, took %v", elapsed)
		}
	})
}

// matrixWormhole is a minimal wormhole payload.
type matrixWormhole struct{}

func (matrixWormhole) IsWormhole() {}

// I2: wormhole ring — A sends a wormhole to B (A's mailbox blocks in
// PushWormholeMailAndWaitReply, which registers no wait-graph edge), and B's
// AcceptWormhole sync-calls back A. Invisible to cycle detection by design
// today; the assertion is that the wormhole timeout unbinds both sides (and
// notably: NOT a deadlock report).
func TestDeadlockMatrix_WormholeCycle(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA, atomB = "worm_a", "worm_b"
		spawnTestAtom(t, elem, atomA)
		spawnTestAtom(t, elem, atomB)
		implA := mustImpl(t, elem, atomA)
		implB := mustImpl(t, elem, atomB)

		var bInnerErr error
		done := make(chan struct{})
		implB.wormholeHook = func(from ID) error {
			e := matrixCallPeer(t, atomA, 1, 300*time.Millisecond)(implB.self)
			bInnerErr = errOrNil(e)
			close(done)
			return e
		}
		implA.remoteHook = func(self AtomSelfID) *Error {
			idB, e := GetForTestAtomosAtomID(self.Cosmos(), atomB)
			if e != nil {
				return asError(e).AddStack(self)
			}
			defer idB.Release()
			if e := idB.SendWormhole(self, matrixWormhole{}, ArgBaseAtomosTimeout(300*time.Millisecond)); e != nil {
				return asError(e).AddStack(self)
			}
			return nil
		}

		atomAID, _, err := elem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID(A): %v", err)
		}
		start := time.Now()
		_, err = atomAID.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(3*time.Second))
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("wormhole ring must produce an error (timeout-bounded)")
		}
		if CodeOf(err) == ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("wormhole waits register no wait edges; deadlock report would be unexpected: %v", err)
		}
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("B's AcceptWormhole did not unbind")
		}
		if bInnerErr == nil || CodeOf(bInnerErr) == ErrCodeIDFirstSyncCallDeadlock {
			t.Fatalf("B's inner call should be a timeout error, got: %v", bInnerErr)
		}
		if elapsed > 2*time.Second {
			t.Fatalf("wormhole ring should unbind at the 300ms timeout, took %v", elapsed)
		}
	})
}

// I3: spawn entanglement — A's handler spawns a fresh atom whose Spawn
// startup sync-calls back A. Atom startup waits are not wait-graph edges, so
// this cannot close a detected cycle; the assertion is bounded completion on
// both sides (no permanent wedge, no hang past the timeouts).
func TestDeadlockMatrix_SpawnEntanglement(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA = "spawn_a"
		const spawnB = "spawn_ent_b"
		spawnTestAtom(t, elem, atomA)
		implA := mustImpl(t, elem, atomA)

		// Inject a spawn hook into the atom that Spawn will create. Its
		// callback to A uses a short timeout so the test never relies on the
		// spawn path's own (longer) timeouts.
		saved := testAtomConstructorHook
		testAtomConstructorHook = func(name string) *testRunnableAtom {
			a := &testRunnableAtom{}
			if name == spawnB {
				a.spawnHook = func(self AtomSelfID) error {
					return errOrNil(matrixCallPeer(t, atomA, 1, 300*time.Millisecond)(self))
				}
			}
			return a
		}
		defer func() { testAtomConstructorHook = saved }()

		implA.remoteHook = func(self AtomSelfID) *Error {
			id, e := SpawnForTestAtomosAtom(self, self.Cosmos(), spawnB, nil)
			if e != nil {
				return asError(e).AddStack(self)
			}
			id.Release()
			return nil
		}

		atomAID, _, err := elem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID(A): %v", err)
		}
		start := time.Now()
		_, err = atomAID.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(3*time.Second))
		elapsed := time.Since(start)
		// Observed behavior (mechanism): B's startup callback to A queues
		// behind A's busy mailbox and times out with ErrAtomosPushTimeoutReject
		// (code 80, mail still queued); that failure propagates up through the
		// spawn chain and unbinds A — the whole ring resolves in ~300ms.
		// Spawn startup waits register no wait-graph edges, so this is
		// timeout-bounded, not a detected cycle.
		if err == nil {
			t.Fatal("expected the spawn-entanglement error to propagate back")
		}
		if CodeOf(err) != ErrCodeAtomosPushTimeoutReject {
			t.Fatalf("expected ErrCodeAtomosPushTimeoutReject from the startup callback, got: %v", err)
		}
		if elapsed > 2*time.Second {
			t.Fatalf("spawn entanglement must unbind at the callback timeout, took %v", elapsed)
		}
		t.Logf("spawn entanglement resolved in %v: err=%v", elapsed, err)
	})
}

// N1: async-then-sync-back must NOT deadlock — B's handler sync-calls A
// while A is free (async never blocked A's mailbox). This is the documented
// remedy for would-be rings.
func TestDeadlockMatrix_AsyncBreaksCycle(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA, atomB = "async_a", "async_b"
		spawnTestAtom(t, elem, atomA)
		spawnTestAtom(t, elem, atomB)
		implB := mustImpl(t, elem, atomB)
		implB.remoteHook = matrixCallAtom(t, atomA, 1, 3*time.Second)

		done := make(chan error, 1)
		genB, err := GetForTestAtomosAtomID(p.local, atomB)
		if err != nil {
			t.Fatalf("GetForTestAtomosAtomID(B): %v", err)
		}
		defer genB.Release()
		genB.AsyncGreeting(p.local, &ForTestGreetingI{Mode: 5}, func(out *ForTestGreetingO, err error) { done <- err })
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("async→sync-back must succeed, got: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("async→sync-back did not complete")
		}
	})
}

// N2: deep sync chain A→B→C→D must complete with no deadlock and no
// false-positive cycle report.
func TestDeadlockMatrix_DeepChain(t *testing.T) {
	savedDebug := allocMailDebug.Load()
	allocMailDebug.Store(false)
	defer func() { allocMailDebug.Store(savedDebug) }()

	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const atomA, atomB, atomC, atomD = "deep_a", "deep_b", "deep_c", "deep_d"
		for _, n := range []string{atomA, atomB, atomC, atomD} {
			spawnTestAtom(t, elem, n)
		}
		mustImpl(t, elem, atomA).remoteHook = matrixCallAtom(t, atomB, 5, 3*time.Second)
		mustImpl(t, elem, atomB).remoteHook = matrixCallAtom(t, atomC, 5, 3*time.Second)
		mustImpl(t, elem, atomC).remoteHook = matrixCallAtom(t, atomD, 1, 3*time.Second)

		atomAID, _, err := elem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID(A): %v", err)
		}
		done := make(chan error, 1)
		go func() {
			_, err := atomAID.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(3*time.Second))
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("deep chain must succeed, got: %v", err)
			}
		case <-time.After(4 * time.Second):
			t.Fatal("deep chain did not complete")
		}
	})
}

// mustImpl fetches a spawned test atom's implementation by name.
func mustImpl(t *testing.T, elem *ElementLocal, name string) *testRunnableAtom {
	t.Helper()
	atom, tracker, err := elem.GetAtomID(name, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
	if err != nil {
		t.Fatalf("GetAtomID(%s): %v", name, err)
	}
	defer tracker.Release()
	atomLocal, ok := atom.(*AtomLocal)
	if !ok {
		t.Fatalf("GetAtomID(%s) returned %T", name, atom)
	}
	impl, ok := atomLocal.atomos.instance.(*testRunnableAtom)
	if !ok {
		t.Fatalf("atom %s impl is %T", name, atomLocal.atomos.instance)
	}
	return impl
}
