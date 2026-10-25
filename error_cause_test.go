package atomos

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
	"weak"

	"google.golang.org/protobuf/proto"
)

var errCauseTestSentinel = errors.New("cause test: business failure")

func TestAsError_PreservesCause(t *testing.T) {
	cause := fmt.Errorf("load config: %w", errCauseTestSentinel)
	e := asError(cause)

	if got := CodeOf(e); got != ErrCodeUnknown {
		t.Fatalf("code = %v, want ErrCodeUnknown", got)
	}
	if e.Unwrap() != cause {
		t.Fatalf("Unwrap() = %v, want the original error", e.Unwrap())
	}
	if !errors.Is(e, errCauseTestSentinel) {
		t.Fatalf("errors.Is lost the user sentinel through the wrapper: %v", e)
	}
	if !strings.Contains(e.Error(), "load config") {
		t.Fatalf("message no longer carries the cause text: %q", e.Error())
	}

	// errors.As must still reach the framework *Error view.
	var view *Error
	if !errors.As(e, &view) || view != e {
		t.Fatalf("errors.As(*Error) did not reach the wrapper: %v", e)
	}
}

func TestAsError_PlainFrameworkErrorHasNoCause(t *testing.T) {
	e := NewError(ErrCodeMainElementNotFound, "not found")
	if e.Unwrap() != nil {
		t.Fatalf("framework-built error should unwrap to nil, got %v", e.Unwrap())
	}
	if asError(nil) != nil {
		t.Fatal("asError(nil) must be nil")
	}
	// A direct *Error is copied so adding a framework hop cannot mutate an
	// object shared with its original owner.
	in := NewError(ErrCodeAtomNotExists, "no atom")
	out := asError(in)
	if out == in {
		t.Fatal("asError must return a fresh *Error")
	}
	out.AddStack(nil)
	if len(in.CallStacks) != 0 {
		t.Fatal("AddStack on the conversion mutated the original *Error")
	}
	if out.Unwrap() != nil {
		t.Fatalf("plain framework error gained a cause: %v", out.Unwrap())
	}
}

func TestAsError_FrameworkSentinelWrappedByUserStillMatches(t *testing.T) {
	// A user handler wrapping a framework sentinel with %w keeps both the
	// framework code and the complete user-facing wrapper as the cause.
	userErr := fmt.Errorf("greeting failed: %w", ErrAtomNotExists)
	e := asError(userErr)
	if got := CodeOf(e); got != ErrCodeAtomNotExists {
		t.Fatalf("code = %v, want ErrCodeAtomNotExists", got)
	}
	if e.Unwrap() != userErr {
		t.Fatalf("Unwrap() = %v, want the original user wrapper", e.Unwrap())
	}
	if !errors.Is(e, ErrAtomNotExists) {
		t.Fatalf("framework sentinel lost through user wrap: %v", e)
	}
	if !strings.Contains(e.Error(), "greeting failed") {
		t.Fatalf("user context lost from message: %q", e.Error())
	}
	// The wrapper is fresh, never the shared sentinel: AddStack hops must not
	// corrupt package state.
	e.AddStack(nil)
	if len(ErrAtomNotExists.(*Error).CallStacks) != 0 {
		t.Fatal("AddStack on the conversion must not touch the shared sentinel")
	}
}

func TestAsError_DirectSentinelAddStackDoesNotMutate(t *testing.T) {
	shared := ErrAtomNotExists.(*Error)
	e := asError(ErrAtomNotExists)
	if e == shared {
		t.Fatal("asError returned the shared sentinel itself")
	}
	e.AddStack(nil)
	if len(shared.CallStacks) != 0 {
		t.Fatal("AddStack polluted the shared sentinel")
	}
}

type errorCauseTestTyped struct{ message string }

func (e *errorCauseTestTyped) Error() string { return e.message }

func TestAsError_JoinPreservesCodeAndUserChain(t *testing.T) {
	typed := &errorCauseTestTyped{message: "typed user failure"}
	joined := errors.Join(
		fmt.Errorf("framework context: %w", ErrCosmosNodeDraining),
		fmt.Errorf("user context: %w", errCauseTestSentinel),
		typed,
	)
	e := asError(joined)
	if got := CodeOf(e); got != ErrCodeCosmosNodeDraining {
		t.Fatalf("code = %v, want ErrCodeCosmosNodeDraining", got)
	}
	if e.Unwrap() != joined {
		t.Fatal("joined user error was not retained as the cause")
	}
	if !errors.Is(e, ErrCosmosNodeDraining) || !errors.Is(e, errCauseTestSentinel) {
		t.Fatalf("errors.Is lost a joined branch: %v", e)
	}
	var gotTyped *errorCauseTestTyped
	if !errors.As(e, &gotTyped) || gotTyped != typed {
		t.Fatalf("errors.As lost the typed joined branch: %v", e)
	}
}

func TestErrorCause_NotSerialized(t *testing.T) {
	bare := NewError(ErrCodeUnknown, "boom")
	withCause := setCause(NewError(ErrCodeUnknown, "boom"), errors.New("inner"))

	b1, err := proto.Marshal(bare)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := proto.Marshal(withCause)
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Fatal("cause association must not change the serialized bytes")
	}

	// A deserialized copy carries no cause.
	cp := proto.Clone(bare).(*Error)
	if cp.Unwrap() != nil {
		t.Fatal("deserialized *Error must not inherit a process-local cause")
	}
}

func TestErrorCause_ReleasedOnceUnreachable(t *testing.T) {
	// Track only this test's own entry: the table is process-global and
	// other tests' cleanups shrink it concurrently, so global counting
	// would be flaky.
	e := asError(errors.New("transient"))
	key := weak.Make(e)
	if _, ok := errCauses.Load(key); !ok {
		t.Fatal("entry missing right after asError")
	}
	e = nil
	// Cleanups run asynchronously after GC observes the death; poll with a
	// generous ceiling so this stays deterministic on loaded machines.
	runtime.GC()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, ok := errCauses.Load(key); !ok {
			return // entry removed once the error became unreachable
		}
		if time.Now().After(deadline) {
			t.Fatal("entry leaked after the error became unreachable")
		}
		runtime.GC()
		time.Sleep(20 * time.Millisecond)
	}
}

func TestWithStack_CapturesGoStackAndHop(t *testing.T) {
	err := WithStack(fmt.Errorf("plain: %w", errCauseTestSentinel), nil)

	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("WithStack must return an *Error, got %T", err)
	}
	if !errors.Is(err, errCauseTestSentinel) {
		t.Fatalf("user sentinel lost: %v", err)
	}
	if len(e.CallStacks) != 1 {
		t.Fatalf("hops = %d, want 1", len(e.CallStacks))
	}
	hop := e.CallStacks[0]
	if hop.PanicStack == "" {
		t.Fatal("first hop should carry the Go stack")
	}
	if !strings.Contains(hop.PanicStack, "TestWithStack_CapturesGoStackAndHop") {
		t.Fatalf("Go stack should contain this test frame:\n%s", hop.PanicStack)
	}
	if !strings.Contains(hop.File, "error_cause_test.go") {
		t.Fatalf("hop file = %s, want this test file", hop.File)
	}
	// Display: a non-panic stack renders under the GoStack label.
	if !strings.Contains(e.Error(), "GoStack:") {
		t.Fatalf("Error() should render the GoStack label:\n%s", e.Error())
	}
	if strings.Contains(e.Error(), "Recover:") {
		t.Fatalf("Error() must not claim a panic:\n%s", e.Error())
	}
}

func TestWithStack_LaterHopsAreFileLineOnly(t *testing.T) {
	err := WithStack(errors.New("first"), nil)
	err = WithStack(err, nil)
	err = WithStack(err, nil)

	e := err.(*Error)
	if len(e.CallStacks) != 3 {
		t.Fatalf("hops = %d, want 3", len(e.CallStacks))
	}
	for i, hop := range e.CallStacks[1:] {
		if hop.PanicStack != "" {
			t.Fatalf("hop %d should be file:line only, got GoStack", i+1)
		}
	}
	// Existing framework hops also suppress the Go stack capture: a
	// kernel-built error entering WithStack only gains a hop.
	kerr := NewError(ErrCodeAtomNotExists, "no atom").AddStack(nil)
	traced := asError(WithStack(kerr, nil))
	if n := len(traced.CallStacks); n != 2 {
		t.Fatalf("hops = %d, want 2", n)
	}
	if len(kerr.CallStacks) != 1 {
		t.Fatal("WithStack mutated its input framework error")
	}
	if traced.CallStacks[1].PanicStack != "" {
		t.Fatal("hop on an already-traced error must not capture a Go stack")
	}
}

func TestWithStack_NilIsNil(t *testing.T) {
	if WithStack(nil, nil) != nil {
		t.Fatal("WithStack(nil) must be nil")
	}
}

func TestError_PanicEntryStillRendersRecover(t *testing.T) {
	e := NewError(ErrCodeFrameworkRecoverFromPanic, "boom").AddPanicStack(nil, 2, "kaboom")
	if !strings.Contains(e.Error(), "Recover: kaboom") {
		t.Fatalf("panic entries must keep the Recover label:\n%s", e.Error())
	}
}

// TestErrorCause_SyncCallPreservesUserSentinel is the end-to-end pin: a
// handler returns a plain wrapped error; the sync caller's error still
// matches the user sentinel while exposing the framework error view
// (code + trace hops).
func TestErrorCause_SyncCallPreservesUserSentinel(t *testing.T) {
	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const name = "cause_sync_atom"
		impl := spawnTestAtom(t, elem, name)
		impl.greetingPlainErr = fmt.Errorf("greeting: %w", errCauseTestSentinel)

		id, tracker, err := elem.GetAtomID(name, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}
		defer tracker.Release()

		_, err = id.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 6}, ArgBaseAtomosTimeout(2*time.Second))
		if err == nil {
			t.Fatal("expected the handler error, got nil")
		}
		if !errors.Is(err, errCauseTestSentinel) {
			t.Fatalf("user sentinel lost across the sync-call boundary: %v", err)
		}
		if got := CodeOf(err); got != ErrCodeUnknown {
			t.Fatalf("code = %v, want ErrCodeUnknown for a converted user error", got)
		}
		view := asError(err)
		if len(view.CallStacks) == 0 {
			t.Fatalf("kernel should have stamped its dispatch hop: %v", view)
		}
		if view.Unwrap() == nil {
			t.Fatal("cause missing after the mailbox round trip")
		}
	})
}
