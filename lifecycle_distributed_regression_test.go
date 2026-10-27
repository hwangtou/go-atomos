package atomos

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

func TestElementLocal_AtomSpawn_StoppingTimeoutDoesNotReplace(t *testing.T) {
	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatalf("get local element: %v", err)
		}

		const name = "stopping_timeout_no_replace"
		oldAtom, oldTracker, err := elem.elementAtomSpawn(nil, name, nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), true, true)
		if err != nil {
			t.Fatalf("spawn old atom: %v", err)
		}
		defer oldTracker.Release()
		oldInstanceID := oldAtom.atomos.instanceID
		oldImpl := oldAtom.atomos.instance.(*testRunnableAtom)
		oldImpl.haltNotify = make(chan struct{}, 1)
		oldImpl.haltWait = 200 * time.Millisecond

		oldAtom.KillSelf()
		select {
		case <-oldImpl.haltNotify:
		case <-time.After(2 * time.Second):
			t.Fatal("old atom did not enter Halt")
		}

		newAtom, newTracker, spawnErr := elem.elementAtomSpawnInternalFoundStopping(
			name, nil, oldAtom, nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), true, true, 20*time.Millisecond,
		)
		if !errors.Is(spawnErr, ErrAtomosIsStopping) {
			t.Fatalf("spawn while stopping error = %v, want ErrAtomosIsStopping", spawnErr)
		}
		if newAtom != nil || newTracker != nil {
			t.Fatalf("timed-out spawn returned atom=%v tracker=%v", newAtom, newTracker)
		}
		elem.lock.RLock()
		current := elem.atoms[name]
		elem.lock.RUnlock()
		if current != oldAtom {
			t.Fatal("timed-out spawn replaced the container entry")
		}

		select {
		case <-oldAtom.atomos.stoppingChan:
		case <-time.After(2 * time.Second):
			t.Fatal("old atom did not finish Halt")
		}
		newAtom, newTracker, err = elem.elementAtomSpawn(nil, name, nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), true, true)
		if err != nil {
			t.Fatalf("spawn after old atom halted: %v", err)
		}
		defer newTracker.Release()
		if newAtom == oldAtom || newAtom.atomos.instanceID == oldInstanceID {
			t.Fatal("respawn did not create a new incarnation")
		}
	})
}

type lifecycleAtom struct {
	name        string
	haltStarted chan struct{}
	haltRelease chan struct{}
}

func (a *lifecycleAtom) String() string { return "lifecycleAtom:" + a.name }

func (a *lifecycleAtom) Halt(ID, []uint64) (bool, proto.Message) {
	close(a.haltStarted)
	<-a.haltRelease
	return true, &Config{Cosmos: a.name}
}

type lifecycleDev struct {
	atom        *lifecycleAtom
	setDataErr  error
	fencedStore *fakeFencedAtomStore
}

func (d *lifecycleDev) ElementConstructor() Atomos       { return &lifecycleAtom{name: "element"} }
func (d *lifecycleDev) AtomConstructor(string) Atomos    { return d.atom }
func (d *lifecycleDev) AtomAutoData() AtomAutoData       { return d }
func (d *lifecycleDev) ElementAutoData() ElementAutoData { return nil }
func (d *lifecycleDev) GetAtomData(string) (proto.Message, error) {
	return &Config{Cosmos: "data"}, nil
}
func (d *lifecycleDev) SetAtomData(string, proto.Message) error { return d.setDataErr }

type lifecycleMainScript struct{}

func (s *lifecycleMainScript) OnBoot(*CosmosProcess) error    { return nil }
func (s *lifecycleMainScript) OnStartUp(*CosmosProcess) error { return nil }
func (s *lifecycleMainScript) OnShutdown() error              { return nil }

func TestLifecycleConcurrentWaitKilledReturnsAfterHalt(t *testing.T) {
	for _, tc := range []struct {
		name       string
		setDataErr error
	}{
		{name: "success"},
		{name: "persistence_error", setDataErr: errors.New("injected persistence failure")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const (
				elementName = "LifecycleElem"
				atomName    = "victim"
			)
			atom := &lifecycleAtom{
				name:        atomName,
				haltStarted: make(chan struct{}),
				haltRelease: make(chan struct{}),
			}
			dev := &lifecycleDev{atom: atom, setDataErr: tc.setDataErr}
			impl := NewImplementationFromDeveloper(dev)
			impl.Interface = NewInterfaceFromDeveloper(elementName, dev)
			impl.Interface.ElementSpawner = func(ElementSelfID, Atomos, proto.Message, ...ArgsForBaseAtomos) error {
				return nil
			}
			impl.Interface.AtomSpawner = func(AtomSelfID, Atomos, proto.Message, proto.Message, ...ArgsForBaseAtomos) error {
				return nil
			}

			blockStarted := make(chan struct{}, 1)
			blockRelease := make(chan struct{})
			impl.ElementHandlers = map[string]MessageHandler{}
			impl.AtomHandlers = map[string]MessageHandler{
				"block": func(ID, Atomos, proto.Message) (proto.Message, error) {
					blockStarted <- struct{}{}
					<-blockRelease
					return &String{S: "ok"}, nil
				},
			}

			logger := &reproLogger{}
			process, cerr := newCosmosProcess("lifecycleCosmos", "lifecycleNode", logger)
			if cerr != nil {
				t.Fatalf("new process: %v", cerr)
			}
			halted := make(chan error, 1)
			runnable := (&CosmosRunnable{}).
				SetConfig(&Config{
					Cosmos:         "lifecycleCosmos",
					Node:           "lifecycleNode",
					LogLevel:       LogLevel_Debug,
					EnableElements: []string{elementName},
				}).
				SetMainScript(&lifecycleMainScript{}).
				SetHaltedHook(func(id *IDInfo, err error, _ *AtomosMessageTrackerExporter) {
					if id.Type == IDType_Atom && id.Atom == atomName {
						halted <- err
					}
				}).
				AddElementImplementationWithSpawn(impl)
			if err := process.Start(runnable); err != nil {
				t.Fatalf("start process: %v", err)
			}
			t.Cleanup(func() {
				_ = process.Stop()
			})

			elem, err := process.local.getLocalElement(elementName)
			if err != nil {
				t.Fatalf("get element: %v", err)
			}
			id, _, spawnErr := elem.SpawnAtom(process.local, atomName, &Config{Cosmos: "arg"}, nil)
			if spawnErr != nil {
				t.Fatalf("spawn atom: %v", spawnErr)
			}
			victim := id.(*AtomLocal)

			blockResult := make(chan error, 1)
			go func() {
				_, err := victim.SyncMessagingByName(process.local, "block", nil)
				blockResult <- err
			}()
			select {
			case <-blockStarted:
			case <-time.After(2 * time.Second):
				t.Fatal("blocking handler did not start")
			}

			killResults := []chan error{make(chan error, 1), make(chan error, 1)}
			for _, result := range killResults {
				result := result
				go func() {
					result <- victim.Kill(process.local, ArgBaseAtomosWaitKilled())
				}()
			}
			accepted := make(chan error, 1)
			go func() {
				accepted <- victim.Kill(process.local)
			}()
			select {
			case err := <-accepted:
				if err != nil {
					t.Fatalf("non-waiting Kill: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("non-waiting Kill blocked behind Halt")
			}
			waitLifecycleMailboxNum(t, victim.atomos.mailbox, 3)
			close(blockRelease)
			if err := <-blockResult; err != nil {
				t.Fatalf("blocking handler: %v", err)
			}

			select {
			case <-atom.haltStarted:
			case <-time.After(2 * time.Second):
				t.Fatal("atom did not enter Halt")
			}
			for i, result := range killResults {
				select {
				case err := <-result:
					t.Fatalf("WaitKilled %d returned before Halt completed: %v", i, err)
				default:
				}
			}
			close(atom.haltRelease)

			for i, result := range killResults {
				select {
				case err := <-result:
					if tc.setDataErr == nil && err != nil {
						t.Fatalf("WaitKilled %d: %v", i, err)
					}
					if tc.setDataErr != nil && (err == nil || CodeOf(err) != CodeOf(asError(tc.setDataErr))) {
						t.Fatalf("WaitKilled %d error = %v, want persistence failure", i, err)
					}
				case <-time.After(2 * time.Second):
					t.Fatalf("WaitKilled %d did not return", i)
				}
				if victim.State() != BaseAtomosHalt {
					t.Fatalf("WaitKilled %d returned in state %v", i, victim.State())
				}
			}

			select {
			case hookErr := <-halted:
				if tc.setDataErr == nil && hookErr != nil {
					t.Fatalf("halted hook error = %v", hookErr)
				}
				if tc.setDataErr != nil && (hookErr == nil || CodeOf(hookErr) != CodeOf(asError(tc.setDataErr))) {
					t.Fatalf("halted hook error = %v, want persistence failure", hookErr)
				}
			default:
				t.Fatal("WaitKilled returned before halted hook")
			}
			select {
			case <-victim.atomos.stoppingChan:
			default:
				t.Fatal("WaitKilled returned before stopping notification")
			}
		})
	}
}

func waitLifecycleMailboxNum(t *testing.T, mailbox *mailBox, want uint32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for mailbox.mailNum() != want {
		if time.Now().After(deadline) {
			t.Fatalf("mailbox queue length = %d, want %d", mailbox.mailNum(), want)
		}
		time.Sleep(time.Millisecond)
	}
}
