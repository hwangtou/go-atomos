package atomos

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

func admissionPendingCounts(p *CosmosProcess, e *ElementLocal) (int, int) {
	p.mutex.RLock()
	processPending := p.atomSpawnPending
	p.mutex.RUnlock()
	e.lock.RLock()
	elementPending := e.atomSpawnPending
	e.lock.RUnlock()
	return processPending, elementPending
}

func waitAdmissionCondition(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func gateClearAtomSpawn(elem *ElementLocal) (chan struct{}, chan struct{}) {
	started := make(chan struct{})
	release := make(chan struct{})
	elem.elemImpl.Interface.AtomSpawner = func(AtomSelfID, Atomos, proto.Message, proto.Message, ...ArgsForBaseAtomos) error {
		close(started)
		<-release
		return nil
	}
	return started, release
}

func TestAtomSpawnAdmissionDrainGetDoesNotActivate(t *testing.T) {
	process, elem := startClearCosmos(t, &reproLogger{})
	existing, _, err := elem.SpawnAtom(process.local, "existing", nil, nil)
	if err != nil {
		t.Fatalf("Spawn existing atom: %v", err)
	}
	if err := process.Drain(time.Hour); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if err := process.Drain(time.Hour); err != nil {
		t.Fatalf("repeated Drain: %v", err)
	}

	got, _, err := elem.GetAtomID("existing", nil)
	if err != nil || got != existing {
		t.Fatalf("Get existing atom during Drain = (%v, %v), want (%v, nil)", got, err, existing)
	}
	missing, _, err := elem.GetAtomID("missing", nil)
	if !errors.Is(err, ErrCosmosNodeDraining) {
		t.Fatalf("Get missing atom during Drain error = %v, want ErrCosmosNodeDraining", err)
	}
	if missing != nil {
		t.Fatalf("Get missing atom returned ID %v", missing)
	}
	elem.lock.RLock()
	_, published := elem.atoms["missing"]
	operations := len(elem.spawnOperations)
	elem.lock.RUnlock()
	if published || operations != 0 {
		t.Fatalf("rejected Get left published=%v operations=%d", published, operations)
	}
	if processPending, elementPending := admissionPendingCounts(process, elem); processPending != 0 || elementPending != 0 {
		t.Fatalf("rejected Get leaked pending process=%d element=%d", processPending, elementPending)
	}
}

func TestAtomSpawnAdmissionAdmittedSpawnCompletesAcrossDrain(t *testing.T) {
	process, elem := startClearCosmos(t, &reproLogger{})
	started, release := gateClearAtomSpawn(elem)
	spawned := make(chan spawnLifecycleResult, 1)
	go func() {
		id, _, err := elem.SpawnAtom(process.local, "admitted", nil, nil)
		spawned <- spawnLifecycleResult{id: id, err: err}
	}()
	waitSpawnLifecycleSignal(t, started, "admitted Spawn did not enter user Spawn")
	if processPending, elementPending := admissionPendingCounts(process, elem); processPending != 1 || elementPending != 1 {
		t.Fatalf("pending before Drain process=%d element=%d, want 1/1", processPending, elementPending)
	}

	if err := process.Drain(time.Hour); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	close(release)
	result := waitSpawnLifecycleResult(t, spawned)
	if result.err != nil || result.id == nil {
		t.Fatalf("admitted Spawn across Drain = (%v, %v)", result.id, result.err)
	}
	waitAdmissionCondition(t, func() bool {
		processPending, elementPending := admissionPendingCounts(process, elem)
		return processPending == 0 && elementPending == 0
	}, "completed Spawn did not release pending")
	atom := result.id.(*AtomLocal)
	if got := elem.GetActiveAtomsNum(); got != 1 {
		t.Fatalf("active atoms after admitted Spawn = %d, want 1", got)
	}

	if err := process.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitMailboxGoroutineExit(atom.atomos.mailbox, 2*time.Second)
	if atom.State() != BaseAtomosHalt || atom.atomos.mailbox.isRunning() {
		t.Fatalf("admitted atom survived Stop: state=%v mailboxRunning=%v", atom.State(), atom.atomos.mailbox.isRunning())
	}
}

func TestAtomSpawnAdmissionElementStopWaitsAndResnapshots(t *testing.T) {
	process, elem := startClearCosmos(t, &reproLogger{})
	started, release := gateClearAtomSpawn(elem)
	spawned := make(chan spawnLifecycleResult, 1)
	go func() {
		id, _, err := elem.SpawnAtom(process.local, "stop_interleave", nil, nil)
		spawned <- spawnLifecycleResult{id: id, err: err}
	}()
	waitSpawnLifecycleSignal(t, started, "Spawn did not enter user Spawn before Stop")

	stopped := make(chan error, 1)
	go func() { stopped <- process.Stop() }()
	waitAdmissionCondition(t, func() bool {
		elem.lock.RLock()
		accepting := elem.atomSpawnAccepting
		elem.lock.RUnlock()
		return !accepting
	}, "Element did not close spawn admission")
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned while admitted Spawn was pending: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if _, _, err := elem.SpawnAtom(process.local, "after_stop", nil, nil); !errors.Is(err, ErrAtomosIsStopping) {
		t.Fatalf("Spawn after Stop error = %v, want ErrAtomosIsStopping", err)
	}

	close(release)
	result := waitSpawnLifecycleResult(t, spawned)
	if result.err != nil || result.id == nil {
		t.Fatalf("admitted Spawn during Stop = (%v, %v)", result.id, result.err)
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not finish after admitted Spawn completed")
	}
	atom := result.id.(*AtomLocal)
	waitMailboxGoroutineExit(atom.atomos.mailbox, 2*time.Second)
	if atom.State() != BaseAtomosHalt || atom.atomos.mailbox.isRunning() {
		t.Fatalf("orphan mailbox after Element stop: state=%v mailboxRunning=%v", atom.State(), atom.atomos.mailbox.isRunning())
	}
	elem.lock.RLock()
	atoms := len(elem.atoms)
	operations := len(elem.spawnOperations)
	elem.lock.RUnlock()
	if atoms != 0 || operations != 0 {
		t.Fatalf("Element final snapshot missed cleanup: atoms=%d operations=%d", atoms, operations)
	}
}

func TestAtomSpawnAdmissionDrainDeadlineAndRepeat(t *testing.T) {
	process, _ := startClearCosmos(t, &reproLogger{})
	if err := process.Drain(40 * time.Millisecond); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if err := process.Drain(time.Hour); err != nil {
		t.Fatalf("repeated Drain: %v", err)
	}
	waitAdmissionCondition(t, func() bool {
		process.mutex.RLock()
		state := process.state
		process.mutex.RUnlock()
		return state == CosmosProcessStateOff
	}, "Drain deadline did not stop the process")
	if err := process.Drain(time.Hour); CodeOf(err) != ErrCodeCosmosProcessInvalidTransition {
		t.Fatalf("Drain after deadline error = %v, want ErrCodeCosmosProcessInvalidTransition", err)
	}
}
