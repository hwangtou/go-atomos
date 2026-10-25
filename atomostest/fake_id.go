package atomostest

import (
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	atomos "github.com/hwangtou/go-atomos"
)

// FakeCall describes one observed call on a FakeID.
type FakeCall struct {
	// Method is the ID method that was invoked, e.g. "SyncMessagingByName".
	Method string
	// Name is the message name for messaging calls; empty otherwise.
	Name string
	// In is the input message for messaging calls; nil otherwise.
	In proto.Message
}

// FakeID implements atomos.ID for unit-testing business logic without
// starting a Cosmos process. Replies are configured per message name and
// every interaction is recorded for assertion.
//
// A FakeID is a stand-in for the ID of a collaborator actor. It is not a
// framework-internal ID: passing it into framework kernel paths (for example
// as the callerID of a real actor's message) fails explicitly with
// atomos.ErrFrameworkInvalidID, by design.
type FakeID struct {
	mu       sync.Mutex
	info     *atomos.IDInfo
	cosmos   atomos.CosmosNode
	state    atomos.BaseAtomosState
	idleTime time.Duration

	// replies maps message name to the canned reply for that message.
	replies map[string]fakeReply
	// syncHandler, when set, takes precedence over replies for
	// SyncMessagingByName and AsyncMessagingByName.
	syncHandler func(name string, in proto.Message) (proto.Message, error)

	killErr     error
	wormholeErr error

	calls []FakeCall
}

type fakeReply struct {
	out proto.Message
	err error
}

// NewFakeID returns a FakeID whose IDInfo describes an Atom named
// "fake-atom" of Element "fake-element" on node "fake-node" of Cosmos
// "fake-cosmos". Use the Set* methods to customize it.
func NewFakeID() *FakeID {
	return &FakeID{
		info: &atomos.IDInfo{
			Type:    atomos.IDType_Atom,
			Cosmos:  "fake-cosmos",
			Node:    "fake-node",
			Element: "fake-element",
			Atom:    "fake-atom",
		},
		replies: make(map[string]fakeReply),
	}
}

var _ atomos.ID = (*FakeID)(nil)

// SetIDInfo replaces the ID identity returned by GetIDInfo.
func (f *FakeID) SetIDInfo(info *atomos.IDInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.info = info
}

// SetCosmosNode sets the CosmosNode returned by Cosmos.
func (f *FakeID) SetCosmosNode(node atomos.CosmosNode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cosmos = node
}

// SetState sets the state returned by State.
func (f *FakeID) SetState(state atomos.BaseAtomosState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = state
}

// SetIdleTime sets the duration returned by IdleTime.
func (f *FakeID) SetIdleTime(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idleTime = d
}

// SetMessageReply configures the reply for the named message. Both
// SyncMessagingByName and AsyncMessagingByName use it. A nil out with a nil
// err is a valid reply.
func (f *FakeID) SetMessageReply(name string, out proto.Message, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies[name] = fakeReply{out: out, err: err}
}

// SetSyncHandler installs a dynamic reply handler for messaging calls. It
// takes precedence over SetMessageReply; return an error from the handler to
// make the call fail.
func (f *FakeID) SetSyncHandler(handler func(name string, in proto.Message) (proto.Message, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncHandler = handler
}

// SetKillError sets the error returned by Kill.
func (f *FakeID) SetKillError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.killErr = err
}

// SetWormholeError sets the error returned by SendWormhole.
func (f *FakeID) SetWormholeError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wormholeErr = err
}

func (f *FakeID) GetIDInfo() *atomos.IDInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.info
}

func (f *FakeID) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	info := f.info
	if info == nil {
		return "FakeID(nil)"
	}
	switch info.Type {
	case atomos.IDType_Cosmos:
		return info.Cosmos
	case atomos.IDType_Element:
		return fmt.Sprintf("%s.%s:%s", info.Cosmos, info.Node, info.Element)
	default:
		return fmt.Sprintf("%s.%s:%s.%s", info.Cosmos, info.Node, info.Element, info.Atom)
	}
}

func (f *FakeID) Cosmos() atomos.CosmosNode {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cosmos
}

func (f *FakeID) State() atomos.BaseAtomosState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *FakeID) IdleTime() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.idleTime
}

// SyncMessagingByName records the call and returns the reply configured via
// SetMessageReply or SetSyncHandler. If no reply is configured for the name,
// it returns an error — an unconfigured call in a test is almost always a
// mistake, so it is reported rather than silently returning nil, nil.
func (f *FakeID) SyncMessagingByName(callerID atomos.ID, name string, in proto.Message, ext ...atomos.ArgsForBaseAtomos) (proto.Message, error) {
	f.record("SyncMessagingByName", name, in)
	out, err, _ := f.resolve(name, in)
	return out, err
}

// AsyncMessagingByName records the call and, when a reply is configured,
// invokes callback synchronously with it (including a configured error)
// before returning nil. If no reply is configured for the name, the callback
// is not invoked and an error is returned as errBeforeExec.
func (f *FakeID) AsyncMessagingByName(callerID atomos.ID, name string, in proto.Message, callback func(out proto.Message, err error), ext ...atomos.ArgsForBaseAtomos) error {
	f.record("AsyncMessagingByName", name, in)
	out, err, ok := f.resolve(name, in)
	if !ok {
		return err
	}
	if callback != nil {
		callback(out, err)
	}
	return nil
}

// DecoderByName always returns nil decoders: message decoders are produced by
// generated code, which a fake does not have. Tests that need decoders should
// supply their own mock.
func (f *FakeID) DecoderByName(name string) (in, out atomos.MessageDecoder) {
	f.record("DecoderByName", name, nil)
	return nil, nil
}

// Kill records the call and returns the error set via SetKillError.
func (f *FakeID) Kill(callerID atomos.ID, ext ...atomos.ArgsForBaseAtomos) error {
	f.record("Kill", "", nil)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.killErr
}

// SendWormhole records the call and returns the error set via
// SetWormholeError.
func (f *FakeID) SendWormhole(callerID atomos.ID, wormhole atomos.BaseAtomosWormhole, ext ...atomos.ArgsForBaseAtomos) error {
	f.record("SendWormhole", "", nil)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.wormholeErr
}

// resolve looks up the reply for name. ok is false when nothing is
// configured, in which case err explains the miss.
func (f *FakeID) resolve(name string, in proto.Message) (out proto.Message, err error, ok bool) {
	f.mu.Lock()
	handler := f.syncHandler
	r, has := f.replies[name]
	f.mu.Unlock()
	if handler != nil {
		out, err = handler(name, in)
		return out, err, true
	}
	if has {
		return r.out, r.err, true
	}
	return nil, fmt.Errorf("FakeID: no reply configured for message %q", name), false
}

func (f *FakeID) record(method, name string, in proto.Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, FakeCall{Method: method, Name: name, In: in})
}

// Calls returns a copy of the recorded call history in invocation order.
func (f *FakeID) Calls() []FakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]FakeCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// CallCount returns how many times the named method was invoked.
func (f *FakeID) CallCount(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.Method == method {
			n++
		}
	}
	return n
}

// ResetCalls discards the recorded call history.
func (f *FakeID) ResetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}
