package atomos

import (
	"time"

	"google.golang.org/protobuf/proto"
)

func newTestCosmosRunnable(id *IDInfo) *CosmosRunnable {
	r := &CosmosRunnable{
		config: &Config{
			Cosmos:         id.Cosmos,
			Node:           id.Node,
			LogLevel:       LogLevel_Debug,
			LogPath:        "",
			LogMaxSize:     0,
			BuildPath:      "",
			BinPath:        "",
			RunPath:        "",
			EtcPath:        "",
			EnableCluster:  nil,
			EnableElements: nil,
			Customize:      nil,
		},
		implements:   map[string]*ElementImplementation{},
		spawnElement: map[string]bool{},
		spawnOrder:   nil,
		mainScript:   nil,
		mainRouter:   nil,

		// hooks
		spawningHook:       nil,
		spawnHook:          nil,
		stoppingHook:       nil,
		haltedHook:         nil,
		messageTimeoutHook: nil,
		recoverHook:        nil,
		newErrorHook:       nil,
	}
	r.
		AddElementImplementationWithSpawn(GetForTestAtomosImplement(&testRunnableDev{})).
		SetMainScript(&testMainScript{})
	return r
}

type testRunnableDev struct{}

// testAtomConstructorHook lets a test customize atoms created by
// testRunnableDev.AtomConstructor — e.g. install a spawnHook on the new
// instance before its Spawn startup runs (spawn-entanglement tests). The
// returning function restores the default constructor behavior.
var testAtomConstructorHook func(name string) *testRunnableAtom

func (d *testRunnableDev) AtomConstructor(name string) Atomos {
	if testAtomConstructorHook != nil {
		if a := testAtomConstructorHook(name); a != nil {
			return a
		}
	}
	return &testRunnableAtom{}
}

func (d *testRunnableDev) ElementConstructor() Atomos {
	return &testRunnableElement{}
}

type testRunnableElement struct {
	self ElementSelfID

	// sayHelloHook, when set, is invoked by the element-level ElementSayHello
	// handler (atom→element ring tests). Returning a non-nil error fails the
	// element message with that error.
	sayHelloHook func(from ID) *Error
}

func (t *testRunnableElement) Spawn(self ElementSelfID, data *ForTestData, _ ...ArgsForBaseAtomos) error {
	t.self = self
	return nil
}

func (t *testRunnableElement) String() string {
	return t.self.String()
}

func (t *testRunnableElement) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

func (t *testRunnableElement) ElementSayHello(from ID, in *ForTestHelloI) (out *ForTestHelloO, err error) {
	if t.sayHelloHook != nil {
		if e := t.sayHelloHook(from); e != nil {
			return nil, e
		}
	}
	return &ForTestHelloO{}, nil
}

type testRunnableAtom struct {
	self AtomSelfID

	greetingNotify chan struct{}
	greetingWait   time.Duration

	// cyclePeer is the atom name to call in Greeting modes 3/4 (cycle tests).
	cyclePeer string

	// remoteHook is invoked by Greeting mode 5 (cross-node cycle tests): the
	// closure makes an outbound sync call — remote or local — on behalf of
	// the handler.
	remoteHook func(self AtomSelfID) *Error

	// greetingGate, when non-nil, blocks Greeting at entry until the channel
	// is closed/_signaled (simultaneous-call tests need both handlers running
	// before either calls out).
	greetingGate chan struct{}

	// greetingPlainErr, when set, is returned by Greeting mode 6 as a plain
	// (non-*Error) error, exercising cause preservation at the kernel
	// boundary (see error_cause.go).
	greetingPlainErr error

	// wormholeHook is invoked by AcceptWormhole when set; implementing
	// AcceptWormhole enables wormhole reception for this atom.
	wormholeHook func(from ID) error

	// spawnHook, when set, runs inside the atom's Spawn startup.
	spawnHook func(self AtomSelfID) error

	haltNotify chan struct{}
	haltWait   time.Duration
}

func (t *testRunnableAtom) Spawn(self AtomSelfID, arg *ForTestSpawnArg, data *ForTestData, _ ...ArgsForBaseAtomos) error {
	t.self = self
	if t.spawnHook != nil {
		if e := t.spawnHook(self); e != nil {
			return e
		}
	}
	return nil
}

// AcceptWormhole enables wormhole reception for testRunnableAtom.
func (t *testRunnableAtom) AcceptWormhole(from ID, wormhole BaseAtomosWormhole) error {
	if t.wormholeHook != nil {
		return t.wormholeHook(from)
	}
	return nil
}

func (t *testRunnableAtom) String() string {
	return t.self.String()
}

func (t *testRunnableAtom) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	t.self.Log().Info("halting")
	if t.haltNotify != nil {
		t.haltNotify <- struct{}{}
	}
	if t.haltWait > 0 {
		<-time.After(t.haltWait)
	}
	return false, nil
}

func (t *testRunnableAtom) Greeting(from ID, in *ForTestGreetingI) (out *ForTestGreetingO, err error) {
	if t.greetingNotify != nil {
		t.greetingNotify <- struct{}{}
	}
	if t.greetingWait > 0 {
		<-time.After(t.greetingWait)
	}
	if t.greetingGate != nil {
		<-t.greetingGate
	}
	switch in.Mode {
	case 1:
		out = &ForTestGreetingO{}
	case 2:
		// Self-sync-call: this atom's mailbox goroutine sync-calls itself.
		// Must be detected as deadlock (ErrIDFirstSyncCallDeadlock) by the
		// wait-graph before blocking. Without detection this would hang 10s.
		selfID, e := GetForTestAtomosAtomID(t.self.Cosmos(), t.self.GetIDInfo().Atom)
		if e != nil {
			return nil, asError(e).AddStack(t.self)
		}
		defer selfID.Release()
		_, e = selfID.Greeting(t.self, &ForTestGreetingI{Mode: 1})
		if e != nil {
			return nil, asError(e).AddStack(t.self)
		}
		out = &ForTestGreetingO{}
	case 3:
		// Cycle: this atom sync-calls a peer atom, which sync-calls back here.
		// The peer name is carried in the in-message (reuse the existing field
		// via a convention: we read t.cyclePeer set by the test).
		if t.cyclePeer == "" {
			return nil, NewErrorf(ErrCodeFrameworkInternalError, "cycle test: cyclePeer not set").AddStack(t.self)
		}
		peerID, e := GetForTestAtomosAtomID(t.self.Cosmos(), t.cyclePeer)
		if e != nil {
			return nil, asError(e).AddStack(t.self)
		}
		defer peerID.Release()
		// Call the peer; the peer's Greeting(mode=4) calls back here.
		_, e = peerID.Greeting(t.self, &ForTestGreetingI{Mode: 4})
		if e != nil {
			return nil, asError(e).AddStack(t.self)
		}
		out = &ForTestGreetingO{}
	case 4:
		// Cycle callback: the peer calls back to its own peer (the originator).
		// This is the second hop that closes the cycle → must deadlock.
		if t.cyclePeer == "" {
			return nil, NewErrorf(ErrCodeFrameworkInternalError, "cycle test: cyclePeer not set").AddStack(t.self)
		}
		peerID, e := GetForTestAtomosAtomID(t.self.Cosmos(), t.cyclePeer)
		if e != nil {
			return nil, asError(e).AddStack(t.self)
		}
		defer peerID.Release()
		_, e = peerID.Greeting(t.self, &ForTestGreetingI{Mode: 1})
		if e != nil {
			return nil, asError(e).AddStack(t.self)
		}
		out = &ForTestGreetingO{}
	case 5:
		// Remote hook (cross-node tests): call out through the configured
		// closure and propagate its error.
		if t.remoteHook == nil {
			return nil, NewErrorf(ErrCodeFrameworkInternalError, "remote hook not set").AddStack(t.self)
		}
		if e := t.remoteHook(t.self); e != nil {
			return nil, e
		}
		out = &ForTestGreetingO{}
	case 6:
		// Plain user error (not *Error): the kernel boundary must preserve it
		// as the unwrap cause of the *Error the caller receives.
		if t.greetingPlainErr == nil {
			panic("mode 6: greetingPlainErr not set")
		}
		return nil, t.greetingPlainErr
	default:
		panic("unknown mode")
	}
	return out, nil
}

type testMainScript struct{}

func (t *testMainScript) OnBoot(local *CosmosProcess) error {
	local.local.Log().Info("OnBoot")
	return nil
}

func (t *testMainScript) OnStartUp(local *CosmosProcess) error {
	local.local.Log().Info("OnStartUp")
	return nil
}

func (t *testMainScript) OnShutdown() error {
	return nil
}
