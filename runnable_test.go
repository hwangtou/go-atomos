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
		AddElementImplementation(GetForTestAtomosImplement(&testRunnableDev{}), true).
		SetMainScript(&testMainScript{})
	return r
}

type testRunnableDev struct{}

func (d *testRunnableDev) AtomConstructor(name string) Atomos {
	return &testRunnableAtom{}
}

func (d *testRunnableDev) ElementConstructor() Atomos {
	return &testRunnableElement{}
}

type testRunnableElement struct {
	self ElementSelfID
}

func (t *testRunnableElement) Spawn(self ElementSelfID, data *ForTestData) *Error {
	t.self = self
	return nil
}

func (t *testRunnableElement) String() string {
	return t.self.String()
}

func (t *testRunnableElement) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

func (t *testRunnableElement) SayHello(from ID, in *ForTestHelloI) (out *ForTestHelloO, err *Error) {
	//TODO implement me
	panic("implement me")
}

type testRunnableAtom struct {
	self AtomSelfID

	greetingNotify chan struct{}
	greetingWait   time.Duration

	// cyclePeer is the atom name to call in Greeting modes 3/4 (cycle tests).
	cyclePeer string

	haltNotify chan struct{}
	haltWait   time.Duration
}

func (t *testRunnableAtom) Spawn(self AtomSelfID, arg *ForTestSpawnArg, data *ForTestData) *Error {
	t.self = self
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

func (t *testRunnableAtom) Greeting(from ID, in *ForTestGreetingI) (out *ForTestGreetingO, err *Error) {
	if t.greetingNotify != nil {
		t.greetingNotify <- struct{}{}
	}
	if t.greetingWait > 0 {
		<-time.After(t.greetingWait)
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
			return nil, e.AddStack(t.self)
		}
		defer selfID.Release()
		_, e = selfID.Greeting(t.self, &ForTestGreetingI{Mode: 1})
		if e != nil {
			return nil, e.AddStack(t.self)
		}
		out = &ForTestGreetingO{}
	case 3:
		// Cycle: this atom sync-calls a peer atom, which sync-calls back here.
		// The peer name is carried in the in-message (reuse the existing field
		// via a convention: we read t.cyclePeer set by the test).
		if t.cyclePeer == "" {
			return nil, NewErrorf(ErrFrameworkInternalError, "cycle test: cyclePeer not set").AddStack(t.self)
		}
		peerID, e := GetForTestAtomosAtomID(t.self.Cosmos(), t.cyclePeer)
		if e != nil {
			return nil, e.AddStack(t.self)
		}
		defer peerID.Release()
		// Call the peer; the peer's Greeting(mode=4) calls back here.
		_, e = peerID.Greeting(t.self, &ForTestGreetingI{Mode: 4})
		if e != nil {
			return nil, e.AddStack(t.self)
		}
		out = &ForTestGreetingO{}
	case 4:
		// Cycle callback: the peer calls back to its own peer (the originator).
		// This is the second hop that closes the cycle → must deadlock.
		if t.cyclePeer == "" {
			return nil, NewErrorf(ErrFrameworkInternalError, "cycle test: cyclePeer not set").AddStack(t.self)
		}
		peerID, e := GetForTestAtomosAtomID(t.self.Cosmos(), t.cyclePeer)
		if e != nil {
			return nil, e.AddStack(t.self)
		}
		defer peerID.Release()
		_, e = peerID.Greeting(t.self, &ForTestGreetingI{Mode: 1})
		if e != nil {
			return nil, e.AddStack(t.self)
		}
		out = &ForTestGreetingO{}
	default:
		panic("unknown mode")
	}
	return out, nil
}

type testMainScript struct{}

func (t *testMainScript) OnBoot(local *CosmosProcess) *Error {
	local.local.Log().Info("OnBoot")
	return nil
}

func (t *testMainScript) OnStartUp(local *CosmosProcess) *Error {
	local.local.Log().Info("OnStartUp")
	return nil
}

func (t *testMainScript) OnShutdown() *Error {
	return nil
}
