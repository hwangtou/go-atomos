package atomos_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/atomostest"
	"google.golang.org/protobuf/proto"
)

type dstScript struct{}

func (*dstScript) OnBoot(*atomos.CosmosProcess) error    { return nil }
func (*dstScript) OnStartUp(*atomos.CosmosProcess) error { return nil }
func (*dstScript) OnShutdown() error                     { return nil }

type dstActor struct{ name string }

func (a *dstActor) String() string { return a.name }
func (*dstActor) Halt(atomos.ID, []uint64) (bool, proto.Message) {
	return false, nil
}

type dstFixture struct {
	clock   *atomostest.VirtualClock
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	self    chan atomos.ElementSelfID
}

func newDSTFixture() *dstFixture {
	return &dstFixture{
		clock:   atomostest.NewVirtualClock(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
		self:    make(chan atomos.ElementSelfID, 1),
	}
}

func (f *dstFixture) ElementConstructor() atomos.Atomos { return &dstActor{name: "element"} }
func (f *dstFixture) AtomConstructor(name string) atomos.Atomos {
	return &dstActor{name: name}
}
func (*dstFixture) AtomAutoData() atomos.AtomAutoData       { return nil }
func (*dstFixture) ElementAutoData() atomos.ElementAutoData { return nil }

func (f *dstFixture) runnable() *atomos.CosmosRunnable {
	impl := atomos.NewImplementationFromDeveloper(f)
	impl.Interface = atomos.NewInterfaceFromDeveloper("dstElement", f)
	impl.Interface.ElementSpawner = func(self atomos.ElementSelfID, _ atomos.Atomos, _ proto.Message, _ ...atomos.ArgsForBaseAtomos) error {
		f.self <- self
		return nil
	}
	impl.Interface.AtomSpawner = func(atomos.AtomSelfID, atomos.Atomos, proto.Message, proto.Message, ...atomos.ArgsForBaseAtomos) error {
		return nil
	}
	decoder := (&atomos.Messenger[*atomostest.FakeID, *atomostest.FakeID, *dstActor, *atomos.Config, *atomos.Config]{Name: "Block"}).Decoder(&atomos.Config{}, &atomos.Config{})
	impl.Interface.ElementDecoders = map[string]*atomos.IOMessageDecoder{"Block": decoder, "Respond": decoder}
	impl.ElementHandlers = map[string]atomos.MessageHandler{
		"Block": func(atomos.ID, atomos.Atomos, proto.Message) (proto.Message, error) {
			f.once.Do(func() { close(f.entered) })
			<-f.release
			return &atomos.Config{}, nil
		},
		"Respond": func(atomos.ID, atomos.Atomos, proto.Message) (proto.Message, error) {
			return &atomos.Config{}, nil
		},
	}
	return (&atomos.CosmosRunnable{}).
		SetConfig(&atomos.Config{Cosmos: "dstCosmos", Node: "dstNode", LogLevel: atomos.LogLevel_Debug}).
		SetMainScript(&dstScript{}).
		SetClock(f.clock).
		SetMessageTimeoutTracer(false, 0).
		AddElementImplementationWithSpawn(impl)
}

func (f *dstFixture) run(t *testing.T) (*atomos.CosmosProcess, atomos.ID, atomos.ElementSelfID) {
	t.Helper()
	process := atomostest.Run(t, f.runnable())
	t.Cleanup(func() { f.once.Do(func() { close(f.entered) }); close(f.release) })
	element, err := process.Self().CosmosGetElementID("dstElement")
	if err != nil {
		t.Fatalf("get element: %v", err)
	}
	return process, element, <-f.self
}

func waitDST(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(time.Millisecond)
	}
}

func assertNoDSTResult(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("operation completed without clock advancement: %v", err)
	default:
	}
}

func receiveDSTError(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not complete after clock advancement")
		return nil
	}
}

func TestDST_MailboxTimeoutReject(t *testing.T) {
	fixture := newDSTFixture()
	process, element, _ := fixture.run(t)
	if err := element.AsyncMessagingByName(process.Self(), "Block", &atomos.Config{}, nil); err != nil {
		t.Fatalf("start blocking handler: %v", err)
	}
	<-fixture.entered

	result := make(chan error, 1)
	go func() {
		_, err := element.SyncMessagingByName(process.Self(), "Respond", &atomos.Config{}, atomos.ArgBaseAtomosTimeout(5*time.Second))
		result <- err
	}()
	waitDST(t, func() bool { return fixture.clock.PendingTimers() == 1 }, "sync wait timer was not registered")
	assertNoDSTResult(t, result)
	fixture.clock.Advance(5*time.Second - time.Nanosecond)
	assertNoDSTResult(t, result)
	fixture.clock.Advance(time.Nanosecond)
	if err := receiveDSTError(t, result); !errors.Is(err, atomos.ErrAtomosPushTimeoutReject) {
		t.Fatalf("timeout error = %v, want reject", err)
	}
}

func TestDST_MailboxTimeoutHandling(t *testing.T) {
	fixture := newDSTFixture()
	process, element, _ := fixture.run(t)
	result := make(chan error, 1)
	go func() {
		_, err := element.SyncMessagingByName(process.Self(), "Block", &atomos.Config{}, atomos.ArgBaseAtomosTimeout(5*time.Second))
		result <- err
	}()
	<-fixture.entered
	waitDST(t, func() bool { return fixture.clock.PendingTimers() == 1 }, "sync wait timer was not registered")
	assertNoDSTResult(t, result)
	fixture.clock.Advance(5*time.Second - time.Nanosecond)
	assertNoDSTResult(t, result)
	fixture.clock.Advance(time.Nanosecond)
	if err := receiveDSTError(t, result); !errors.Is(err, atomos.ErrAtomosPushTimeoutHandling) {
		t.Fatalf("timeout error = %v, want handling", err)
	}
}

func TestDST_DelayedTasks(t *testing.T) {
	fixture := newDSTFixture()
	_, _, self := fixture.run(t)
	after := make(chan uint64, 1)
	queued := make(chan uint64, 1)
	if _, err := self.Task().AddAfter(2*time.Second, func(id uint64) { after <- id }); err != nil {
		t.Fatalf("AddAfter: %v", err)
	}
	self.Task().AddToAtomosQueue(func(id uint64) { queued <- id }, atomos.ArgTaskDelay(2*time.Second))
	if fixture.clock.PendingTimers() != 2 {
		t.Fatalf("pending timers = %d, want 2", fixture.clock.PendingTimers())
	}
	select {
	case <-after:
		t.Fatal("AddAfter fired without Advance")
	default:
	}
	select {
	case <-queued:
		t.Fatal("delayed queue task fired without Advance")
	default:
	}
	fixture.clock.Advance(2*time.Second - time.Nanosecond)
	select {
	case <-after:
		t.Fatal("AddAfter fired before its deadline")
	default:
	}
	select {
	case <-queued:
		t.Fatal("delayed queue task fired before its deadline")
	default:
	}
	fixture.clock.Advance(time.Nanosecond)
	for name, result := range map[string]<-chan uint64{"AddAfter": after, "queue": queued} {
		select {
		case <-result:
		case <-time.After(3 * time.Second):
			t.Fatalf("%s did not fire at its deadline", name)
		}
	}
}
