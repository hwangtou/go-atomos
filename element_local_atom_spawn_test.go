package atomos

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestElementLocal_AtomSpawn_SpawnLockMap(t *testing.T) {
	e := &ElementLocal{
		spawnLockMap: NewMapGoWithRefCount[string, *sync.Mutex](),
	}

	f := func(name string, exe func()) {
		lock, _ := e.spawnLockMap.GetOrPut(name, &sync.Mutex{})
		defer e.spawnLockMap.Remove(name)

		lock.Lock()
		defer lock.Unlock()

		exe()
	}

	var counter int64
	var wg sync.WaitGroup
	wg.Add(2)
	go f("test1", func() {
		if !atomic.CompareAndSwapInt64(&counter, 0, 1) {
			t.Error("Expected counter to be 0, got", counter)
		}
		defer func() {
			if !atomic.CompareAndSwapInt64(&counter, 1, 0) {
				t.Error("Expected counter to be 1, got", counter)
			}
		}()

		t.Log("Executing test1")
		if l := e.spawnLockMap.Len(); l != 1 {
			t.Error("Expected spawnLockMap length to be 1, got", l)
		}
		wg.Done()
	})
	go f("test1", func() {
		if !atomic.CompareAndSwapInt64(&counter, 0, 2) {
			t.Error("Expected counter to be 0, got", counter)
		}
		defer func() {
			if !atomic.CompareAndSwapInt64(&counter, 2, 0) {
				t.Error("Expected counter to be 2, got", counter)
			}
		}()

		t.Log("Executing test1 again")
		if l := e.spawnLockMap.Len(); l != 1 {
			t.Error("Expected spawnLockMap length to be 1, got", l)
		}
		wg.Done()
	})
	wg.Wait()
	if l := e.spawnLockMap.Len(); l != 0 {
		t.Error("Expected spawnLockMap length to be 0 after execution, got", l)
	}
}

func TestElementLocal_AtomSpawn_SpawnTwice(t *testing.T) {
	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatalf("Failed to get local element: %v", err)
		}

		// Spawn the atom for the first time
		atom, tracker, err := elem.elementAtomSpawn(nil, "test_atom_1", nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), true, true)
		if err != nil {
			t.Fatalf("Failed to spawn atom: %v", err)
		}
		if atom == nil {
			t.Fatal("Expected atom to be non-nil")
		}
		if tracker == nil {
			t.Fatal("Expected tracker to be non-nil")
		}
		atom.atomos.asyncCallbackID = 10
		atom.atomos.asyncCallbackMap[10] = asyncCallbackWrap{}

		oldGoID := atom.atomos.GetGoID()
		oldNameElement := atom.nameElement
		oldImpl := atom.atomos.impl
		oldID := atom.atomos.id
		oldMailbox := atom.atomos.mailbox
		oldHolder := atom.atomos.holder
		oldInstance := atom.atomos.instance
		oldLogging := atom.atomos.log
		oldTask := &atom.atomos.task
		oldMt := &atom.atomos.mt
		oldStopping := atom.atomos.stoppingChan
		oldAsyncCallbackID := atom.atomos.asyncCallbackID
		oldInstanceID := atom.atomos.instanceID

		oldAtomInstance := atom.atomos.instance.(*testRunnableAtom)
		oldAtomInstance.haltWait = 10 * time.Millisecond
		oldAtomInstance.haltNotify = make(chan struct{}, 1)

		// Spawn the atom for the second time with the same name, which should fail
		_, _, err = elem.elementAtomSpawn(nil, "test_atom_1", nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), true, true)
		if err == nil {
			t.Fatal("Expected error when spawning atom with duplicate name, got nil")
		}
		if err.Code != ErrAtomSpawningAnExistedAtom {
			t.Fatalf("Expected error code %d, got %d", ErrAtomSpawningAnExistedAtom, err.Code)
		}

		// Stop and spawn again
		atom.KillSelf()
		<-oldAtomInstance.haltNotify
		if atom.State() != BaseAtomosStopping {
			t.Fatalf("Expected old atom to be in stopping state after kill, got %d", atom.State())
		}
		newAtom, newTracker, err := elem.elementAtomSpawn(nil, "test_atom_1", nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), true, true)
		if err != nil {
			t.Fatalf("Failed to spawn atom after killing previous one: %v", err)
		}
		if newAtom == nil {
			t.Fatal("Expected new atom to be non-nil")
		}
		if newTracker == nil {
			t.Fatal("Expected new tracker to be non-nil")
		}
		// After M2, a respawn allocates a brand-new *AtomLocal; the old pointer
		// is NOT overlaid in place. The new atom is the live entry under the name.
		if atom == newAtom {
			t.Fatal("Expected new atom to be a distinct pointer after respawn (no struct overlay)")
		}
		if newAtom.State() != BaseAtomosWaiting {
			t.Fatalf("Expected NEW atom to be waiting after respawn, got %d", newAtom.State())
		}
		if oldGoID == newAtom.atomos.GetGoID() {
			t.Fatal("Expected new atom to have different GoID after killing previous one")
		}
		if oldNameElement != newAtom.nameElement {
			t.Fatal("Expected new atom to have same name element after killing previous one")
		}
		if oldImpl == newAtom.atomos.impl {
			t.Fatal("Expected new atom to have different impl after killing previous one")
		}
		if oldID == newAtom.atomos.id {
			t.Fatal("Expected new atom to have different ID after killing previous one")
		}
		if oldMailbox == newAtom.atomos.mailbox {
			t.Fatal("Expected new atom to have different mailbox after killing previous one")
		}
		if oldHolder == newAtom.atomos.holder {
			t.Fatal("Expected new atom to have different holder after killing previous one")
		}
		if oldInstance == newAtom.atomos.instance {
			t.Fatal("Expected new atom to have different instance after killing previous one")
		}
		if oldLogging == newAtom.atomos.log {
			t.Fatal("Expected new atom to have different logging after killing previous one")
		}
		// Task/mt are value fields of the new struct, so they live at a new
		// address (the old *AtomLocal is not overlaid anymore).
		if oldTask == &newAtom.atomos.task {
			t.Fatal("Expected new atom to have different task after killing previous one")
		}
		if oldMt == &newAtom.atomos.mt {
			t.Fatal("Expected new atom to have different message tracker after killing previous one")
		}
		// instanceID must differ between the old and respawned instances; this is
		// the cornerstone of per-instance reference counting (atomRefState).
		if oldInstanceID == newAtom.atomos.instanceID {
			t.Fatalf("Expected new atom to have a different instanceID after respawn; old=%d new=%d", oldInstanceID, newAtom.atomos.instanceID)
		}

		if oldAsyncCallbackID != newAtom.atomos.asyncCallbackID {
			t.Fatal("Expected new atom to have same async callback ID after killing previous one")
		}
		// The new atom must NOT reuse the old async callback map: it gets a fresh
		// empty map. The old map's pending callbacks are failed and cleared during
		// halt (see mailboxOnStop), so we only assert that the new map is empty.
		if len(newAtom.atomos.asyncCallbackMap) != 0 {
			t.Fatal("Expected new atom to have an empty async callback map after killing previous one")
		}
		if oldStopping == newAtom.atomos.stoppingChan {
			t.Fatal("Expected new atom to have different stopping channel after killing previous one")
		}
	})
}

func testElementLocalAtomSpawnHelper(t *testing.T, exe func(*CosmosProcess)) {
	id := &IDInfo{
		Type:    IDType_Atom,
		Cosmos:  "test_cosmos",
		Node:    "test_node",
		Element: "test_element",
		Atom:    "test_atomos",
		Version: 0,
	}
	p, err := newCosmosProcess(id.Cosmos, id.Node, newTestLogging(t))
	if err != nil {
		t.Fatalf("Failed to create CosmosProcess: %v", err)
	}
	r := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: id.Cosmos, Node: id.Node})
	if err := p.Start(r); err != nil {
		t.Fatalf("Failed to start CosmosProcess: %v", err)
	}
	defer func() {
		if err := p.Stop(); err != nil {
			t.Fatalf("Failed to stop CosmosProcess: %v", err)
		}
	}()

	exe(p)
}
