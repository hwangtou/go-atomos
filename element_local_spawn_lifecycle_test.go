package atomos

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

type spawnLifecycleResult struct {
	id  ID
	err error
}

func testSpawnLifecycleHelper(t *testing.T, exe func(*CosmosProcess)) {
	t.Helper()
	id := &IDInfo{Type: IDType_Cosmos, Cosmos: "test_cosmos", Node: "test_node"}
	p, err := newCosmosProcess(id.Cosmos, id.Node, &reproLogger{})
	if err != nil {
		t.Fatalf("new process: %v", err)
	}
	if err := p.Start(newTestCosmosRunnable(id)); err != nil {
		t.Fatalf("start process: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop() })
	exe(p)
}

func TestElementLocalSpawnLifecycleSingleConstructorAndGetReady(t *testing.T) {
	testSpawnLifecycleHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatal(err)
		}
		const name = "single_constructor"
		constructorStarted := make(chan struct{})
		releaseSpawn := make(chan struct{})
		var constructors atomic.Int32
		savedHook := testAtomConstructorHook
		testAtomConstructorHook = func(string) *testRunnableAtom {
			if constructors.Add(1) == 1 {
				close(constructorStarted)
			}
			return &testRunnableAtom{spawnHook: func(AtomSelfID) error {
				<-releaseSpawn
				return nil
			}}
		}
		defer func() { testAtomConstructorHook = savedHook }()

		first := make(chan spawnLifecycleResult, 1)
		go func() {
			id, _, err := elem.SpawnAtom(nil, name, nil, nil)
			first <- spawnLifecycleResult{id: id, err: err}
		}()
		waitSpawnLifecycleSignal(t, constructorStarted, "constructor did not start")

		for i := 0; i < 8; i++ {
			_, _, err := elem.SpawnAtom(nil, name, nil, nil)
			if !errors.Is(err, ErrAtomAlreadyExists) {
				t.Fatalf("duplicate spawn %d error = %v", i, err)
			}
		}
		getResult := make(chan spawnLifecycleResult, 1)
		go func() {
			id, _, err := elem.GetAtomID(name, nil)
			getResult <- spawnLifecycleResult{id: id, err: err}
		}()
		select {
		case result := <-getResult:
			t.Fatalf("Get returned before Ready: %+v", result)
		case <-time.After(30 * time.Millisecond):
		}
		if got := elem.GetAtomsNum(); got != 0 {
			t.Fatalf("unready atom was published, container size = %d", got)
		}

		close(releaseSpawn)
		spawned := waitSpawnLifecycleResult(t, first)
		if spawned.err != nil {
			t.Fatalf("first spawn: %v", spawned.err)
		}
		got := waitSpawnLifecycleResult(t, getResult)
		if got.err != nil || got.id != spawned.id {
			t.Fatalf("Get after Ready = (%v, %v), want (%v, nil)", got.id, got.err, spawned.id)
		}
		if constructors.Load() != 1 {
			t.Fatalf("constructor count = %d, want 1", constructors.Load())
		}
		if got.id.State() != BaseAtomosWaiting {
			t.Fatalf("state after Ready = %v", got.id.State())
		}
	})
}

func TestElementLocalSpawnLifecycleReentrantAndAB(t *testing.T) {
	t.Run("same_name", func(t *testing.T) {
		testSpawnLifecycleHelper(t, func(p *CosmosProcess) {
			elem, err := p.local.getLocalElement(ForTestAtomosName)
			if err != nil {
				t.Fatal(err)
			}
			const name = "reentrant"
			var spawnErr, getErr error
			savedHook := testAtomConstructorHook
			testAtomConstructorHook = func(string) *testRunnableAtom {
				return &testRunnableAtom{spawnHook: func(AtomSelfID) error {
					_, _, spawnErr = elem.SpawnAtom(nil, name, nil, nil)
					_, _, getErr = elem.GetAtomID(name, nil)
					return nil
				}}
			}
			defer func() { testAtomConstructorHook = savedHook }()

			done := make(chan error, 1)
			go func() {
				_, _, err := elem.SpawnAtom(nil, name, nil, nil)
				done <- err
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("outer spawn: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("same-name reentry deadlocked")
			}
			if !errors.Is(spawnErr, ErrAtomAlreadyExists) {
				t.Fatalf("reentrant Spawn error = %v", spawnErr)
			}
			if !errors.Is(getErr, ErrAtomosIsNotRunning) {
				t.Fatalf("reentrant pre-Ready Get error = %v", getErr)
			}
		})
	})

	t.Run("a_b", func(t *testing.T) {
		testSpawnLifecycleHelper(t, func(p *CosmosProcess) {
			elem, err := p.local.getLocalElement(ForTestAtomosName)
			if err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{}, 2)
			startCalls := make(chan struct{})
			callErrors := make(chan error, 2)
			savedHook := testAtomConstructorHook
			testAtomConstructorHook = func(name string) *testRunnableAtom {
				return &testRunnableAtom{spawnHook: func(self AtomSelfID) error {
					entered <- struct{}{}
					<-startCalls
					peer := "a"
					if name == peer {
						peer = "b"
					}
					_, _, err := elem.SpawnAtom(nil, peer, nil, nil)
					callErrors <- err
					return nil
				}}
			}
			defer func() { testAtomConstructorHook = savedHook }()

			results := make(chan spawnLifecycleResult, 2)
			for _, name := range []string{"a", "b"} {
				go func(name string) {
					id, _, err := elem.SpawnAtom(nil, name, nil, nil)
					results <- spawnLifecycleResult{id: id, err: err}
				}(name)
			}
			waitSpawnLifecycleSignal(t, entered, "A did not enter Spawn")
			waitSpawnLifecycleSignal(t, entered, "B did not enter Spawn")
			close(startCalls)
			for i := 0; i < 2; i++ {
				if err := waitSpawnLifecycleResult(t, results).err; err != nil {
					t.Fatalf("outer A/B spawn: %v", err)
				}
				if err := waitSpawnLifecycleError(t, callErrors); !errors.Is(err, ErrAtomAlreadyExists) {
					t.Fatalf("cross Spawn error = %v", err)
				}
			}
		})
	})
}

func TestElementLocalSpawnLifecycleSpawnHookCanGetPublishedAtom(t *testing.T) {
	testSpawnLifecycleHelper(t, func(p *CosmosProcess) {
		elem, err := p.local.getLocalElement(ForTestAtomosName)
		if err != nil {
			t.Fatal(err)
		}
		const name = "spawn_hook_get"
		hookResult := make(chan spawnLifecycleResult, 1)
		p.local.runnable.SetSpawnHook(func(id *IDInfo) {
			if id.Type != IDType_Atom || id.Atom != name {
				return
			}
			got, _, err := elem.GetAtomID(name, nil)
			hookResult <- spawnLifecycleResult{id: got, err: err}
		})
		defer p.local.runnable.SetSpawnHook(nil)

		id, _, spawnErr := elem.SpawnAtom(nil, name, nil, nil)
		if spawnErr != nil {
			t.Fatalf("Spawn: %v", spawnErr)
		}
		got := waitSpawnLifecycleResult(t, hookResult)
		if got.err != nil || got.id != id {
			t.Fatalf("hook Get = (%v, %v), want (%v, nil)", got.id, got.err, id)
		}
		if got.id.State() != BaseAtomosWaiting {
			t.Fatalf("hook observed state %v", got.id.State())
		}
	})
}

func TestElementLocalSpawnLifecycleFailureCleanup(t *testing.T) {
	for _, tc := range []struct {
		name        string
		constructor bool
		spawnError  bool
		spawnPanic  bool
		hookPanic   bool
	}{
		{name: "constructor_panic", constructor: true},
		{name: "spawn_error", spawnError: true},
		{name: "spawn_panic", spawnPanic: true},
		{name: "spawn_hook_panic", hookPanic: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testSpawnLifecycleHelper(t, func(p *CosmosProcess) {
				elem, err := p.local.getLocalElement(ForTestAtomosName)
				if err != nil {
					t.Fatal(err)
				}
				const name = "failed_spawn"
				haltNotify := make(chan struct{}, 1)
				var constructed *testRunnableAtom
				savedConstructor := testAtomConstructorHook
				testAtomConstructorHook = func(string) *testRunnableAtom {
					if tc.constructor {
						panic("constructor failure")
					}
					constructed = &testRunnableAtom{haltNotify: haltNotify}
					return constructed
				}
				defer func() { testAtomConstructorHook = savedConstructor }()

				savedSpawner := elem.elemImpl.Interface.AtomSpawner
				if tc.spawnError {
					elem.elemImpl.Interface.AtomSpawner = func(AtomSelfID, Atomos, proto.Message, proto.Message, ...ArgsForBaseAtomos) error {
						return NewError(ErrCodeAtomNotImplemented, "spawn failure")
					}
				} else if tc.spawnPanic {
					elem.elemImpl.Interface.AtomSpawner = func(AtomSelfID, Atomos, proto.Message, proto.Message, ...ArgsForBaseAtomos) error {
						panic("spawn failure")
					}
				}
				defer func() { elem.elemImpl.Interface.AtomSpawner = savedSpawner }()
				if tc.hookPanic {
					p.local.runnable.SetSpawnHook(func(id *IDInfo) {
						if id.Type == IDType_Atom && id.Atom == name {
							panic("spawn hook failure")
						}
					})
					defer p.local.runnable.SetSpawnHook(nil)
				}
				halted := make(chan struct{}, 1)
				p.local.runnable.SetHaltedHook(func(id *IDInfo, _ error, _ *AtomosMessageTrackerExporter) {
					if id.Type == IDType_Atom && id.Atom == name {
						halted <- struct{}{}
					}
				})
				defer p.local.runnable.SetHaltedHook(nil)

				_, _, spawnErr := elem.SpawnAtom(nil, name, nil, nil)
				if spawnErr == nil {
					t.Fatal("failed Spawn returned nil error")
				}
				elem.lock.RLock()
				_, published := elem.atoms[name]
				operations := len(elem.spawnOperations)
				elem.lock.RUnlock()
				if published || operations != 0 {
					t.Fatalf("failed Spawn left published=%v operations=%d", published, operations)
				}
				select {
				case <-haltNotify:
					t.Fatal("user Halt ran for unsuccessful Spawn")
				default:
				}
				if !tc.constructor {
					waitSpawnLifecycleSignal(t, halted, "missing Halt notification")
				}

				testAtomConstructorHook = func(string) *testRunnableAtom { return &testRunnableAtom{} }
				elem.elemImpl.Interface.AtomSpawner = savedSpawner
				p.local.runnable.SetSpawnHook(nil)
				id, _, retryErr := elem.SpawnAtom(nil, name, nil, nil)
				if retryErr != nil {
					t.Fatalf("retry after failed Spawn: %v", retryErr)
				}
				if err := id.Kill(p.local, ArgBaseAtomosWaitKilled()); err != nil {
					t.Fatalf("kill retry atom: %v", err)
				}
			})
		})
	}
}

type spawnAbortTestBaseAtomos struct {
	*testBaseAtomos
	stopping atomic.Int32
}

func (a *spawnAbortTestBaseAtomos) onStopping(ID, []uint64) *Error {
	a.stopping.Add(1)
	return nil
}

func TestBaseAtomosAbortSpawnRejectsQueuedWorkWithoutUserHalt(t *testing.T) {
	p := newTestCosmosProcessWithoutCluster(t, "abort_spawn", "node")
	p.local.runnable = &CosmosRunnable{config: &Config{}}
	defer func() { p.local.runnable = nil }()
	base := &testBaseAtomos{t: t, process: p}
	holder := &spawnAbortTestBaseAtomos{testBaseAtomos: base}
	id := &IDInfo{Type: IDType_Atom, Cosmos: "abort_spawn", Node: "node", Element: "element", Atom: "atom"}
	ba := newBaseAtomos(holder, id, LogLevel_Debug, holder, &testInstance{}, p)
	base.atomos = ba

	syncResult := make(chan error, 1)
	killResult := make(chan error, 1)
	asyncResult := make(chan error, 1)
	var taskRan atomic.Bool
	startErr := NewError(ErrCodeAtomNotImplemented, "abort startup")
	err := ba.start(func() *Error {
		if _, err := ba.task.Add(func(uint64) { taskRan.Store(true) }); err != nil {
			return asError(err)
		}
		if err := ba.PushAsyncMessage(p.local, "unused", &Strings{}, func(_ proto.Message, err error) {
			asyncResult <- err
		}); err != nil {
			return asError(err)
		}
		go func() {
			_, err := ba.PushSyncMessage(p.local, "unused", &Strings{})
			syncResult <- err
		}()
		go func() { killResult <- ba.PushKillMail(p.local, ArgBaseAtomosWaitKilled()) }()
		deadline := time.Now().Add(time.Second)
		for ba.mailbox.mailNum() != 4 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if ba.mailbox.mailNum() != 4 {
			return NewErrorf(ErrCodeFrameworkInternalError, "test mails not queued: %d", ba.mailbox.mailNum())
		}
		return startErr
	})
	if !errors.Is(err, startErr) {
		t.Fatalf("start error = %v, want %v", err, startErr)
	}
	for name, result := range map[string]<-chan error{"sync": syncResult, "kill": killResult, "async": asyncResult} {
		select {
		case err := <-result:
			if !errors.Is(err, ErrAtomosIsNotRunning) {
				t.Fatalf("%s waiter error = %v", name, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s waiter remained blocked", name)
		}
	}
	if ba.GetState() != BaseAtomosHalt || ba.mailbox.isRunning() || ba.mailbox.mailNum() != 0 {
		t.Fatalf("abort state=%v running=%v queued=%d", ba.GetState(), ba.mailbox.isRunning(), ba.mailbox.mailNum())
	}
	if holder.stopping.Load() != 0 {
		t.Fatal("user stopping/Halt path ran during abortSpawn")
	}
	if taskRan.Load() {
		t.Fatal("queued task ran during abortSpawn")
	}
	ba.task.mutex.Lock()
	remainingTasks := len(ba.task.tasks)
	stopping := ba.task.stopping
	ba.task.mutex.Unlock()
	if remainingTasks != 0 || !stopping {
		t.Fatalf("task cleanup remaining=%d stopping=%v", remainingTasks, stopping)
	}
	if _, err := ba.task.Add(func(uint64) {}); !errors.Is(err, ErrAtomosIsStopping) {
		t.Fatalf("task admission after abort = %v", err)
	}
}

func waitSpawnLifecycleSignal(t *testing.T, ch <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal(message)
	}
}

func waitSpawnLifecycleResult(t *testing.T, ch <-chan spawnLifecycleResult) spawnLifecycleResult {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("spawn lifecycle call did not return")
		return spawnLifecycleResult{}
	}
}

func waitSpawnLifecycleError(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("spawn lifecycle call did not return")
		return nil
	}
}
