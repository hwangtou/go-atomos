package atomos

import (
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

func TestBaseAtomosStateStringAndMatching(t *testing.T) {
	tests := []struct {
		state BaseAtomosState
		want  string
	}{
		{state: BaseAtomosInvalidState, want: "Unknown"},
		{state: BaseAtomosSpawning, want: "Spawning"},
		{state: BaseAtomosWaiting, want: "Waiting"},
		{state: BaseAtomosBusy, want: "Busy"},
		{state: BaseAtomosStopping, want: "Stopping"},
		{state: BaseAtomosHalt, want: "Halted"},
		{state: BaseAtomosState(99), want: "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("state %d String() = %q, want %q", tt.state, got, tt.want)
		}
	}

	holder, _ := newBaseAtomosBehaviorForTest(t)
	ba := holder.atomos
	if !ba.IsInState(BaseAtomosWaiting) {
		t.Fatal("IsInState did not match the current state")
	}
	if !ba.IsInState(BaseAtomosBusy, BaseAtomosWaiting) {
		t.Fatal("IsInState did not match one of multiple states")
	}
	if ba.IsInState() || ba.IsInState(BaseAtomosBusy, BaseAtomosHalt) {
		t.Fatal("IsInState matched an absent state")
	}
}

func TestBaseAtomosPushValidation(t *testing.T) {
	holder, _ := newBaseAtomosBehaviorForTest(t)
	ba := holder.atomos
	var callbackCalls atomic.Int32
	callback := func(proto.Message, error) { callbackCalls.Add(1) }

	tests := []struct {
		name     string
		call     func() error
		wantCode int64
	}{
		{
			name: "sync nil caller",
			call: func() error {
				_, err := ba.PushSyncMessage(nil, "sync", &Strings{})
				return err
			},
			wantCode: ErrCodeFrameworkIncorrectUsage,
		},
		{
			name: "async nil caller",
			call: func() error {
				return ba.PushAsyncMessage(nil, "async", &Strings{}, callback)
			},
			wantCode: ErrCodeFrameworkIncorrectUsage,
		},
		{
			name: "kill nil caller",
			call: func() error {
				return ba.PushKillMail(nil)
			},
			wantCode: ErrCodeFrameworkIncorrectUsage,
		},
		{
			name: "wormhole nil caller",
			call: func() error {
				return ba.PushWormholeMailAndWaitReply(nil, &baseAtomosTestWormhole{})
			},
			wantCode: ErrCodeFrameworkIncorrectUsage,
		},
		{
			name: "sync invalid argument",
			call: func() error {
				_, err := ba.PushSyncMessage(holder.process.local, "sync", &Strings{}, ArgBaseAtomosWaitKilled())
				return err
			},
			wantCode: ErrCodeAtomosInvalidArguments,
		},
		{
			name: "async invalid argument",
			call: func() error {
				return ba.PushAsyncMessage(holder.process.local, "async", &Strings{}, callback, ArgBaseAtomosTimeout(time.Second))
			},
			wantCode: ErrCodeAtomosInvalidArguments,
		},
		{
			name: "kill invalid argument",
			call: func() error {
				return ba.PushKillMail(holder.process.local, nil)
			},
			wantCode: ErrCodeAtomosInvalidArguments,
		},
		{
			name: "wormhole invalid argument",
			call: func() error {
				return ba.PushWormholeMailAndWaitReply(holder.process.local, &baseAtomosTestWormhole{}, ArgBaseAtomosWaitKilled())
			},
			wantCode: ErrCodeAtomosInvalidArguments,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queuedBefore := ba.mailbox.mailNum()
			if err := tt.call(); CodeOf(err) != tt.wantCode {
				t.Fatalf("error = %v, want code %v", err, tt.wantCode)
			}
			if queuedAfter := ba.mailbox.mailNum(); queuedAfter != queuedBefore {
				t.Fatalf("queued mails = %d, want %d", queuedAfter, queuedBefore)
			}
		})
	}
	if got := callbackCalls.Load(); got != 0 {
		t.Fatalf("callback called %d times for rejected messages", got)
	}
	if pending := holder.process.local.atomos.AsyncCallbackPending(); pending != 0 {
		t.Fatalf("pending callbacks = %d, want 0", pending)
	}
	if ba.GetState() != BaseAtomosWaiting {
		t.Fatalf("state = %v, want Waiting", ba.GetState())
	}
}

func TestBaseAtomosSyncResponseIsCloned(t *testing.T) {
	holder, _ := newBaseAtomosBehaviorForTest(t)
	response := &Strings{Ss: []string{"response"}}
	holder.syncHandler = func(ID, Atomos, proto.Message) (proto.Message, error) {
		return response, nil
	}

	out, err := holder.SyncMessagingByName(holder.process.local, "sync", &Strings{})
	if err != nil {
		t.Fatalf("SyncMessagingByName failed: %v", err)
	}
	got, ok := out.(*Strings)
	if !ok {
		t.Fatalf("response type = %T, want *Strings", out)
	}
	if got == response {
		t.Fatal("response aliases the handler response instead of being cloned")
	}
	if !proto.Equal(got, response) {
		t.Fatalf("response = %v, want %v", got, response)
	}
}

func TestBaseAtomosSyncHandlerError(t *testing.T) {
	holder, _ := newBaseAtomosBehaviorForTest(t)
	holder.syncHandler = func(ID, Atomos, proto.Message) (proto.Message, error) {
		return nil, NewError(ErrCodeAtomNotImplemented, "handler failed")
	}

	out, err := holder.SyncMessagingByName(holder.process.local, "sync", &Strings{})
	if out != nil {
		t.Fatalf("response = %v, want nil", out)
	}
	if CodeOf(err) != ErrCodeAtomNotImplemented {
		t.Fatalf("error = %v, want ErrCodeAtomNotImplemented", err)
	}
	if holder.atomos.GetState() != BaseAtomosWaiting {
		t.Fatalf("state = %v, want Waiting", holder.atomos.GetState())
	}
}

func TestBaseAtomosSyncHandlerPanicRecovers(t *testing.T) {
	holder, instance := newBaseAtomosBehaviorForTest(t)
	holder.syncHandler = func(ID, Atomos, proto.Message) (proto.Message, error) {
		panic("sync panic")
	}

	out, err := holder.SyncMessagingByName(holder.process.local, "sync-panic", &Strings{Ss: []string{"input"}})
	if out != nil {
		t.Fatalf("response = %v, want nil", out)
	}
	if CodeOf(err) != ErrCodeFrameworkRecoverFromPanic {
		t.Fatalf("error = %v, want ErrCodeFrameworkRecoverFromPanic", err)
	}
	select {
	case recovered := <-instance.messageRecover:
		if recovered.name != "sync-panic" || CodeOf(recovered.err) != ErrCodeFrameworkRecoverFromPanic {
			t.Fatalf("MessageRecover = %+v", recovered)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("MessageRecover was not called")
	}

	holder.syncHandler = func(ID, Atomos, proto.Message) (proto.Message, error) {
		return &Strings{Ss: []string{"alive"}}, nil
	}
	out, err = holder.SyncMessagingByName(holder.process.local, "sync-after-panic", &Strings{})
	if err != nil || out.(*Strings).Ss[0] != "alive" {
		t.Fatalf("mailbox did not survive panic: out=%v err=%v", out, err)
	}
}

func TestBaseAtomosAsyncHandlerError(t *testing.T) {
	holder, _ := newBaseAtomosBehaviorForTest(t)
	holder.asyncHandler = func(ID, Atomos, proto.Message) (proto.Message, error) {
		return &Strings{Ss: []string{"partial response"}}, NewError(ErrCodeAtomNotImplemented, "async failed")
	}
	results := make(chan baseAtomosAsyncResult, 2)
	if err := holder.AsyncMessagingByName(holder.process.local, "async", &Strings{}, func(out proto.Message, err error) {
		results <- baseAtomosAsyncResult{out: out, err: err}
	}); err != nil {
		t.Fatalf("AsyncMessagingByName failed: %v", err)
	}

	result := waitBaseAtomosAsyncResult(t, results)
	if CodeOf(result.err) != ErrCodeAtomNotImplemented {
		t.Fatalf("callback error = %v, want ErrCodeAtomNotImplemented", result.err)
	}
	if !proto.Equal(result.out, &Strings{Ss: []string{"partial response"}}) {
		t.Fatalf("callback response = %v", result.out)
	}
	assertNoSecondBaseAtomosAsyncResult(t, results)
	if pending := holder.process.local.atomos.AsyncCallbackPending(); pending != 0 {
		t.Fatalf("pending callbacks = %d, want 0", pending)
	}
}

func TestBaseAtomosAsyncHandlerPanicRecovers(t *testing.T) {
	holder, instance := newBaseAtomosBehaviorForTest(t)
	holder.asyncHandler = func(ID, Atomos, proto.Message) (proto.Message, error) {
		panic("async panic")
	}
	results := make(chan baseAtomosAsyncResult, 2)
	if err := holder.AsyncMessagingByName(holder.process.local, "async-panic", &Strings{Ss: []string{"input"}}, func(out proto.Message, err error) {
		results <- baseAtomosAsyncResult{out: out, err: err}
	}); err != nil {
		t.Fatalf("AsyncMessagingByName failed: %v", err)
	}

	result := waitBaseAtomosAsyncResult(t, results)
	if result.out != nil || CodeOf(result.err) != ErrCodeFrameworkRecoverFromPanic {
		t.Fatalf("callback = %+v, want nil response and panic recovery error", result)
	}
	select {
	case recovered := <-instance.messageRecover:
		if recovered.name != "async-panic" || CodeOf(recovered.err) != ErrCodeFrameworkRecoverFromPanic {
			t.Fatalf("MessageRecover = %+v", recovered)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("MessageRecover was not called")
	}
	assertNoSecondBaseAtomosAsyncResult(t, results)
	if holder.atomos.GetState() != BaseAtomosWaiting {
		t.Fatalf("state = %v, want Waiting", holder.atomos.GetState())
	}
}

func TestBaseAtomosWormhole(t *testing.T) {
	holder, _ := newBaseAtomosBehaviorForTest(t)
	payload := &baseAtomosTestWormhole{value: "payload"}
	holder.wormholeHandler = func(from ID, wormhole BaseAtomosWormhole) *Error {
		if from != holder.process.local {
			return NewErrorf(ErrCodeFrameworkInternalError, "unexpected caller: %v", from)
		}
		if wormhole != payload {
			return NewErrorf(ErrCodeFrameworkInternalError, "unexpected payload: %v", wormhole)
		}
		return nil
	}
	if err := holder.SendWormhole(holder.process.local, payload); err != nil {
		t.Fatalf("SendWormhole failed: %v", err)
	}

	holder.wormholeHandler = func(ID, BaseAtomosWormhole) *Error {
		return NewError(ErrCodeAtomNotImplemented, "wormhole rejected")
	}
	if err := holder.SendWormhole(holder.process.local, payload); CodeOf(err) != ErrCodeAtomNotImplemented {
		t.Fatalf("SendWormhole error = %v, want ErrCodeAtomNotImplemented", err)
	}
}

func TestBaseAtomosTaskCallback(t *testing.T) {
	holder, _ := newBaseAtomosBehaviorForTest(t)
	holder.taskCallbackHandler = func(callback *atomosCallback) {
		defer func() {
			if recovered := recover(); recovered != nil && callback.recoverFn != nil {
				callback.recoverFn(recovered)
			}
		}()
		callback.callback()
	}

	called := make(chan struct{}, 1)
	holder.atomos.PushTaskCallbackMail("ordinary", func() { called <- struct{}{} }, nil)
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("ordinary task callback was not executed")
	}

	recovered := make(chan any, 1)
	holder.atomos.PushTaskCallbackMail("panic", func() { panic("callback panic") }, func(reason any) {
		recovered <- reason
	})
	select {
	case reason := <-recovered:
		if reason != "callback panic" {
			t.Fatalf("recover reason = %v, want callback panic", reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("task callback recovery function was not executed")
	}
	if holder.atomos.GetState() != BaseAtomosWaiting {
		t.Fatalf("state = %v, want Waiting", holder.atomos.GetState())
	}
}

type baseAtomosTestWormhole struct {
	value string
}

func (*baseAtomosTestWormhole) IsWormhole() {}

type baseAtomosAsyncResult struct {
	out proto.Message
	err error
}

func waitBaseAtomosAsyncResult(t *testing.T, results <-chan baseAtomosAsyncResult) baseAtomosAsyncResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("async callback was not called")
		return baseAtomosAsyncResult{}
	}
}

func assertNoSecondBaseAtomosAsyncResult(t *testing.T, results <-chan baseAtomosAsyncResult) {
	t.Helper()
	select {
	case result := <-results:
		t.Fatalf("async callback called more than once: %+v", result)
	case <-time.After(20 * time.Millisecond):
	}
}

type baseAtomosRecoverRecord struct {
	name string
	arg  proto.Message
	err  error
}

type baseAtomosBehaviorInstance struct {
	messageRecover chan baseAtomosRecoverRecord
}

func (i *baseAtomosBehaviorInstance) String() string { return "baseAtomosBehaviorInstance" }
func (i *baseAtomosBehaviorInstance) Halt(ID, []uint64) (bool, proto.Message) {
	return false, nil
}
func (i *baseAtomosBehaviorInstance) ParallelRecover(error)             {}
func (i *baseAtomosBehaviorInstance) SpawnRecover(proto.Message, error) {}
func (i *baseAtomosBehaviorInstance) TaskRecover(uint64, string, proto.Message, error) {
}
func (i *baseAtomosBehaviorInstance) StopRecover(error) {}
func (i *baseAtomosBehaviorInstance) MessageRecover(name string, arg proto.Message, err error) {
	i.messageRecover <- baseAtomosRecoverRecord{name: name, arg: arg, err: err}
}

type baseAtomosBehaviorHolder struct {
	*testBaseAtomos
	syncHandler         MessageHandler
	asyncHandler        MessageHandler
	wormholeHandler     func(ID, BaseAtomosWormhole) *Error
	taskCallbackHandler func(*atomosCallback)
}

func (h *baseAtomosBehaviorHolder) onSyncMessaging(from ID, name string, in proto.Message) (proto.Message, *Error) {
	return h.atomos.onSyncMessaging(from, name, h.syncHandler, in)
}

func (h *baseAtomosBehaviorHolder) onAsyncMessaging(from ID, name string, startupID, asyncID uint64, in proto.Message) {
	h.atomos.onAsyncMessaging(from, h, name, h.asyncHandler, startupID, asyncID, in)
}

func (h *baseAtomosBehaviorHolder) onWormhole(from ID, wormhole BaseAtomosWormhole) *Error {
	return h.wormholeHandler(from, wormhole)
}

func (h *baseAtomosBehaviorHolder) onFnCallback(callback *atomosCallback) {
	h.taskCallbackHandler(callback)
}

func newBaseAtomosBehaviorForTest(t *testing.T) (*baseAtomosBehaviorHolder, *baseAtomosBehaviorInstance) {
	t.Helper()
	process := newTestCosmosProcessWithoutCluster(t, "base_atomos_behavior", "node")
	base := &testBaseAtomos{t: t, process: process}
	holder := &baseAtomosBehaviorHolder{testBaseAtomos: base}
	instance := &baseAtomosBehaviorInstance{messageRecover: make(chan baseAtomosRecoverRecord, 4)}
	id := &IDInfo{Type: IDType_Atom, Cosmos: "base_atomos_behavior", Node: "node", Element: "element", Atom: "atom"}
	ba := newBaseAtomos(holder, id, LogLevel_Debug, holder, instance, process)
	base.atomos = ba
	if err := ba.start(nil); err != nil {
		t.Fatalf("failed to start BaseAtomos: %v", err)
	}
	t.Cleanup(func() {
		if ba.mailbox.isRunning() {
			if err := ba.PushKillMail(process.local, ArgBaseAtomosWaitKilled(), ArgBaseAtomosTimeout(5*time.Second)); err != nil {
				t.Errorf("failed to stop BaseAtomos: %v", err)
			}
		}
		waitMailboxGoroutineExit(ba.mailbox, 5*time.Second)
	})
	return holder, instance
}
