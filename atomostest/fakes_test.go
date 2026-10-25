package atomostest

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	atomos "github.com/hwangtou/go-atomos"
)

// userMockID is a compile check: atomos.ID contains only exported methods,
// so user code outside the framework can implement it directly.
type userMockID struct{}

func (userMockID) GetIDInfo() *atomos.IDInfo     { return nil }
func (userMockID) String() string                { return "userMockID" }
func (userMockID) Cosmos() atomos.CosmosNode     { return nil }
func (userMockID) State() atomos.BaseAtomosState { return atomos.BaseAtomosInvalidState }
func (userMockID) IdleTime() time.Duration       { return 0 }
func (userMockID) SyncMessagingByName(callerID atomos.ID, name string, in proto.Message, ext ...atomos.ArgsForBaseAtomos) (proto.Message, error) {
	return nil, nil
}
func (userMockID) AsyncMessagingByName(callerID atomos.ID, name string, in proto.Message, callback func(out proto.Message, err error), ext ...atomos.ArgsForBaseAtomos) error {
	return nil
}
func (userMockID) DecoderByName(name string) (in, out atomos.MessageDecoder) { return nil, nil }
func (userMockID) Kill(callerID atomos.ID, ext ...atomos.ArgsForBaseAtomos) error {
	return nil
}
func (userMockID) SendWormhole(callerID atomos.ID, wormhole atomos.BaseAtomosWormhole, ext ...atomos.ArgsForBaseAtomos) error {
	return nil
}

var (
	_ atomos.ID      = userMockID{}
	_ atomos.Logging = (*FakeLogging)(nil)
	_ atomos.SelfID  = (*FakeSelfID)(nil)
	_ atomos.ID      = (*FakeID)(nil)
	_ atomos.Task    = (*FakeTask)(nil)
)

func TestFakeIDSyncReply(t *testing.T) {
	id := NewFakeID()
	want := wrapperspb.String("pong")
	id.SetMessageReply("Ping", want, nil)

	out, err := id.SyncMessagingByName(nil, "Ping", wrapperspb.String("ping"))
	if err != nil {
		t.Fatalf("SyncMessagingByName err=(%v)", err)
	}
	if !proto.Equal(out, want) {
		t.Fatalf("SyncMessagingByName out=(%v), want (%v)", out, want)
	}

	calls := id.Calls()
	if len(calls) != 1 || calls[0].Method != "SyncMessagingByName" || calls[0].Name != "Ping" {
		t.Fatalf("Calls()=(%+v)", calls)
	}
	if id.CallCount("SyncMessagingByName") != 1 {
		t.Fatalf("CallCount=(%d)", id.CallCount("SyncMessagingByName"))
	}
	id.ResetCalls()
	if len(id.Calls()) != 0 {
		t.Fatalf("Calls() after ResetCalls=(%+v)", id.Calls())
	}
}

func TestFakeIDSyncNoReplyConfigured(t *testing.T) {
	id := NewFakeID()
	if _, err := id.SyncMessagingByName(nil, "Missing", nil); err == nil {
		t.Fatal("SyncMessagingByName with no configured reply should fail")
	}
}

func TestFakeIDSyncHandler(t *testing.T) {
	id := NewFakeID()
	id.SetSyncHandler(func(name string, in proto.Message) (proto.Message, error) {
		return wrapperspb.String("echo:" + name), nil
	})
	out, err := id.SyncMessagingByName(nil, "Any", nil)
	if err != nil {
		t.Fatalf("SyncMessagingByName err=(%v)", err)
	}
	if got := out.(*wrapperspb.StringValue).Value; got != "echo:Any" {
		t.Fatalf("out=(%q)", got)
	}
}

func TestFakeIDSyncHandlerCanReenterFakeID(t *testing.T) {
	id := NewFakeID()
	id.SetSyncHandler(func(name string, in proto.Message) (proto.Message, error) {
		if got := id.CallCount("SyncMessagingByName") + id.CallCount("AsyncMessagingByName"); got == 0 {
			t.Error("handler did not observe its recorded call")
		}
		return wrapperspb.String("ok"), nil
	})
	if _, err := id.SyncMessagingByName(nil, "Sync", nil); err != nil {
		t.Fatalf("SyncMessagingByName err=(%v)", err)
	}
	if err := id.AsyncMessagingByName(nil, "Async", nil, nil); err != nil {
		t.Fatalf("AsyncMessagingByName err=(%v)", err)
	}
}

func TestFakeIDAsyncReply(t *testing.T) {
	id := NewFakeID()
	replyErr := errors.New("boom")
	id.SetMessageReply("Fail", nil, replyErr)

	var gotOut proto.Message
	var gotErr error
	called := false
	err := id.AsyncMessagingByName(nil, "Fail", nil, func(out proto.Message, err error) {
		called, gotOut, gotErr = true, out, err
	})
	if err != nil {
		t.Fatalf("AsyncMessagingByName errBeforeExec=(%v)", err)
	}
	if !called || gotOut != nil || !errors.Is(gotErr, replyErr) {
		t.Fatalf("callback called=(%v) out=(%v) err=(%v)", called, gotOut, gotErr)
	}

	// Unconfigured message: errBeforeExec, callback not invoked.
	called = false
	if err := id.AsyncMessagingByName(nil, "Missing", nil, func(proto.Message, error) { called = true }); err == nil {
		t.Fatal("AsyncMessagingByName with no configured reply should fail before exec")
	}
	if called {
		t.Fatal("callback must not be invoked when no reply is configured")
	}
}

func TestFakeIDKillAndWormhole(t *testing.T) {
	id := NewFakeID()
	if err := id.Kill(nil); err != nil {
		t.Fatalf("Kill err=(%v)", err)
	}
	killErr := errors.New("denied")
	id.SetKillError(killErr)
	if err := id.Kill(nil); !errors.Is(err, killErr) {
		t.Fatalf("Kill err=(%v), want (%v)", err, killErr)
	}
	if got := id.CallCount("Kill"); got != 2 {
		t.Fatalf("CallCount(Kill)=(%d)", got)
	}
}

func TestFakeIDIdentity(t *testing.T) {
	id := NewFakeID()
	info := id.GetIDInfo()
	if info.Type != atomos.IDType_Atom || info.Atom != "fake-atom" {
		t.Fatalf("GetIDInfo=(%+v)", info)
	}
	if s := id.String(); s != "fake-cosmos.fake-node:fake-element.fake-atom" {
		t.Fatalf("String()=(%q)", s)
	}
	id.SetState(atomos.BaseAtomosWaiting)
	if id.State() != atomos.BaseAtomosWaiting {
		t.Fatalf("State()=(%v)", id.State())
	}
	id.SetIdleTime(time.Second)
	if id.IdleTime() != time.Second {
		t.Fatalf("IdleTime()=(%v)", id.IdleTime())
	}
}

func TestFakeLogging(t *testing.T) {
	l := NewFakeLogging()
	l.Debug("d %d", 1)
	l.Info("i %s", "x")
	l.Warn("w")
	l.Error("e")
	l.Fatal("f")
	if _, err := l.Write([]byte("written\n")); err != nil {
		t.Fatalf("Write err=(%v)", err)
	}

	entries := l.Entries()
	if len(entries) != 6 {
		t.Fatalf("len(Entries())=(%d)", len(entries))
	}
	if entries[0].Level != atomos.LogLevel_Debug || entries[0].Text != "d 1" {
		t.Fatalf("entries[0]=(%+v)", entries[0])
	}
	if entries[5].Level != atomos.LogLevel_CoreInfo || entries[5].Text != "written" {
		t.Fatalf("entries[5]=(%+v)", entries[5])
	}
	if !l.Contains("i x") {
		t.Fatal("Contains(i x)=false")
	}
	if l.Contains("nope") {
		t.Fatal("Contains(nope)=true")
	}

	// Level filtering mirrors the framework.
	l.SetLevel(atomos.LogLevel_Warn)
	l.Info("filtered")
	if l.Contains("filtered") {
		t.Fatal("Info entry should have been filtered at Warn level")
	}

	l.Reset()
	if len(l.Entries()) != 0 {
		t.Fatalf("Entries() after Reset=(%+v)", l.Entries())
	}
}

func TestFakeSelfID(t *testing.T) {
	self := NewFakeSelfID()

	self.Log().Info("hello %s", "log")
	if !self.LogRecorder().Contains("hello log") {
		t.Fatal("LogRecorder missing entry")
	}

	ran := false
	if _, err := self.Task().Add(func(id uint64) { ran = true }); err != nil {
		t.Fatalf("Task().Add err=(%v)", err)
	}
	if !ran {
		t.Fatal("FakeTask.Add did not run the closure synchronously")
	}

	if self.CosmosMain() != nil {
		t.Fatal("CosmosMain should default to nil")
	}
	if self.AsyncCallbackPending() != 0 {
		t.Fatal("AsyncCallbackPending should default to 0")
	}
	self.SetAsyncCallbackPending(2)
	if self.AsyncCallbackPending() != 2 {
		t.Fatal("SetAsyncCallbackPending not effective")
	}

	self.KillSelf()
	self.KillSelf()
	if self.KillSelfCount() != 2 {
		t.Fatalf("KillSelfCount=(%d)", self.KillSelfCount())
	}

	parallelRan := false
	self.Parallel(func() { parallelRan = true })
	if !parallelRan {
		t.Fatal("Parallel did not run the closure")
	}

	cfg := map[string][]byte{"k": []byte("v")}
	self.SetConfig(cfg)
	if string(self.Config()["k"]) != "v" {
		t.Fatalf("Config()=(%+v)", self.Config())
	}

	// The embedded FakeID behavior is available too.
	self.SetMessageReply("M", wrapperspb.Int32(1), nil)
	if _, err := self.SyncMessagingByName(nil, "M", nil); err != nil {
		t.Fatalf("SyncMessagingByName err=(%v)", err)
	}
}

func TestFakeTask(t *testing.T) {
	task := NewFakeTask()

	var order []int
	if _, err := task.Add(func(id uint64) { order = append(order, 1) }); err != nil {
		t.Fatalf("Add err=(%v)", err)
	}
	id2, err := task.AddAfter(time.Hour, func(id uint64) { order = append(order, 2) })
	if err != nil {
		t.Fatalf("AddAfter err=(%v)", err)
	}
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("executed order=(%v)", order)
	}

	if _, err := task.Add(nil); err == nil {
		t.Fatal("Add(nil) should fail")
	}
	if err := task.Cancel(id2); err != nil {
		t.Fatalf("Cancel err=(%v)", err)
	}
	if err := task.Cancel(999); err == nil {
		t.Fatal("Cancel of unknown ID should fail")
	}
	if got := task.Cancelled(); len(got) != 1 || got[0] != id2 {
		t.Fatalf("Cancelled()=(%v)", got)
	}

	queueRan := false
	cancel := task.AddToAtomosQueue(func(id uint64) { queueRan = true })
	if !queueRan {
		t.Fatal("AddToAtomosQueue did not run synchronously")
	}
	if err := cancel("done"); err != nil {
		t.Fatalf("cancel err=(%v)", err)
	}

	serialRan, callbackRan := false, false
	task.AddToSerialQueue("q", func(id uint64) func() {
		serialRan = true
		return func() { callbackRan = true }
	})
	if !serialRan || !callbackRan {
		t.Fatal("AddToSerialQueue did not run worker and callback")
	}

	if task.HasMarking("any") {
		t.Fatal("HasMarking should always be false")
	}
}
