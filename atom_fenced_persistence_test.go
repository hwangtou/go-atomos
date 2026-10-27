package atomos

import (
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

type fakeFencedAtomStore struct {
	mutex       sync.Mutex
	fence       uint64
	incarnation uint64
	calls       int
}

func (s *fakeFencedAtomStore) write(_ string, _ proto.Message, incarnation, fence uint64) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.calls++
	if fence <= s.fence {
		return NewErrorf(ErrCodeStaleFence, "fake store: fence %d is not newer than %d", fence, s.fence)
	}
	s.fence = fence
	s.incarnation = incarnation
	return nil
}

func (d *lifecycleDev) SetAtomDataAtFence(name string, data proto.Message, incarnation, fence uint64) error {
	if d.fencedStore == nil {
		return errors.New("missing fenced store")
	}
	return d.fencedStore.write(name, data, incarnation, fence)
}

func TestFencedAtomAutoDataRejectsStaleWrites(t *testing.T) {
	store := &fakeFencedAtomStore{}
	if err := store.write("atom", nil, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.write("atom", nil, 1, 1); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("equal fence error = %v", err)
	}
	if err := store.write("atom", nil, 2, 2); err != nil {
		t.Fatal(err)
	}
	if err := store.write("atom", nil, 1, 1); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("old incarnation write error = %v", err)
	}
}

func TestAtomHaltUsesFencedPersistenceForOwnedID(t *testing.T) {
	const elementName = "FencedLifecycleElem"
	store := &fakeFencedAtomStore{}
	atom := &lifecycleAtom{name: "owned", haltStarted: make(chan struct{}), haltRelease: make(chan struct{})}
	dev := &lifecycleDev{
		atom: atom, setDataErr: errors.New("legacy SetAtomData must not be called"), fencedStore: store,
	}
	impl := NewImplementationFromDeveloper(dev)
	impl.Interface = NewInterfaceFromDeveloper(elementName, dev)
	impl.Interface.ElementSpawner = func(ElementSelfID, Atomos, proto.Message, ...ArgsForBaseAtomos) error {
		return nil
	}
	impl.Interface.AtomSpawner = func(AtomSelfID, Atomos, proto.Message, proto.Message, ...ArgsForBaseAtomos) error {
		return nil
	}
	impl.ElementHandlers = map[string]MessageHandler{}
	impl.AtomHandlers = map[string]MessageHandler{}

	process, cerr := newCosmosProcess("fencedCosmos", "fencedNode", &reproLogger{})
	if cerr != nil {
		t.Fatal(cerr)
	}
	runnable := (&CosmosRunnable{}).
		SetConfig(&Config{Cosmos: "fencedCosmos", Node: "fencedNode", LogLevel: LogLevel_Debug, EnableElements: []string{elementName}}).
		SetMainScript(&lifecycleMainScript{}).
		AddElementImplementationWithSpawn(impl)
	if err := process.Start(runnable); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Stop() })
	elem, err := process.local.getLocalElement(elementName)
	if err != nil {
		t.Fatal(err)
	}
	id, _, spawnErr := elem.SpawnAtom(process.local, "owned", &Config{Cosmos: "arg"}, nil)
	if spawnErr != nil {
		t.Fatal(spawnErr)
	}
	local := id.(*AtomLocal)
	local.atomos.id.Incarnation = 7
	local.atomos.id.StartupId = 9
	local.atomos.id.Fence = 1

	killDone := make(chan error, 1)
	go func() { killDone <- local.Kill(process.local, ArgBaseAtomosWaitKilled()) }()
	select {
	case <-atom.haltStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("Halt did not start")
	}
	close(atom.haltRelease)
	select {
	case err := <-killDone:
		if err != nil {
			t.Fatalf("controlled fenced Halt: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Halt did not finish")
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.fence != 1 || store.incarnation != 7 || store.calls != 1 {
		t.Fatalf("fenced store state = fence %d incarnation %d calls %d", store.fence, store.incarnation, store.calls)
	}
}
