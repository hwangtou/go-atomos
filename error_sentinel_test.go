package atomos

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorIsByCode(t *testing.T) {
	err := NewError(ErrCodeAtomNotExists, "atom foo::bar::baz not found")
	if !errors.Is(err, ErrAtomNotExists) {
		t.Errorf("errors.Is(err, ErrAtomNotExists) = false, want true")
	}
	if errors.Is(err, ErrAtomAlreadyExists) {
		t.Errorf("errors.Is(err, ErrAtomAlreadyExists) = true, want false")
	}
	wrapped := fmt.Errorf("spawn: %w", err)
	if !errors.Is(wrapped, ErrAtomNotExists) {
		t.Errorf("errors.Is(wrapped, ErrAtomNotExists) = false, want true")
	}
	if !errors.Is(ErrAtomNotExists, ErrAtomNotExists) {
		t.Errorf("errors.Is(sentinel, itself) = false, want true")
	}
}

func TestErrorIsNilSafety(t *testing.T) {
	var nilErr *Error
	if nilErr.Is(ErrAtomNotExists) {
		t.Errorf("nil *Error matched ErrAtomNotExists")
	}
	// A typed-nil *Error inside an error interface must not panic errors.Is.
	var err error = nilErr
	if err == nil {
		t.Fatalf("typed-nil *Error produced a nil error interface")
	}
	if errors.Is(err, ErrAtomNotExists) {
		t.Errorf("errors.Is(typed-nil, ErrAtomNotExists) = true, want false")
	}
	if got := CodeOf(err); got != ErrCodeUnknown {
		t.Errorf("CodeOf(typed-nil) = %d, want %d (ErrCodeUnknown)", got, ErrCodeUnknown)
	}
	var nilTarget *Error
	if NewError(ErrCodeAtomNotExists, "x").Is(nilTarget) {
		t.Error("non-nil *Error matched a typed-nil target")
	}
	if errors.Is(NewError(ErrCodeAtomNotExists, "x"), nilTarget) {
		t.Error("errors.Is matched a typed-nil target")
	}
}

func TestCodeOf(t *testing.T) {
	if got := CodeOf(nil); got != OK {
		t.Errorf("CodeOf(nil) = %d, want %d (OK)", got, OK)
	}
	err := NewErrorf(ErrCodeCosmosNodeDraining, "node %s draining", "n1")
	if got := CodeOf(err); got != ErrCodeCosmosNodeDraining {
		t.Errorf("CodeOf(err) = %d, want %d", got, ErrCodeCosmosNodeDraining)
	}
	wrapped := fmt.Errorf("outer: %w", err)
	if got := CodeOf(wrapped); got != ErrCodeCosmosNodeDraining {
		t.Errorf("CodeOf(wrapped) = %d, want %d", got, ErrCodeCosmosNodeDraining)
	}
	if got := CodeOf(errors.New("plain")); got != ErrCodeUnknown {
		t.Errorf("CodeOf(plain) = %d, want %d (ErrCodeUnknown)", got, ErrCodeUnknown)
	}
	if got := CodeOf(ErrAtomAlreadyExists); got != ErrCodeAtomAlreadyExists {
		t.Errorf("CodeOf(sentinel) = %d, want %d", got, ErrCodeAtomAlreadyExists)
	}
}

func TestIsAtomExistDeprecated(t *testing.T) {
	if !NewError(ErrCodeAtomAlreadyExists, "x").IsAtomExist() {
		t.Errorf("IsAtomExist on ErrCodeAtomAlreadyExists = false, want true")
	}
	if NewError(ErrCodeAtomNotExists, "x").IsAtomExist() {
		t.Errorf("IsAtomExist on ErrCodeAtomNotExists = true, want false")
	}
}

func TestSentinelsMatchCodes(t *testing.T) {
	sentinels := map[string]error{
		"ErrFrameworkInternalError":       ErrFrameworkInternalError,
		"ErrFrameworkRecoverFromPanic":    ErrFrameworkRecoverFromPanic,
		"ErrFrameworkInvalidID":           ErrFrameworkInvalidID,
		"ErrElementNotFound":              ErrElementNotFound,
		"ErrAtomosInvalidArguments":       ErrAtomosInvalidArguments,
		"ErrAtomosIsStopping":             ErrAtomosIsStopping,
		"ErrAtomosIsNotRunning":           ErrAtomosIsNotRunning,
		"ErrOperationUnsupported":         ErrOperationUnsupported,
		"ErrAtomosPushTimeoutHandling":    ErrAtomosPushTimeoutHandling,
		"ErrAtomosPushTimeoutReject":      ErrAtomosPushTimeoutReject,
		"ErrAtomosTaskNotCancellable":     ErrAtomosTaskNotCancellable,
		"ErrIDFirstSyncCallDeadlock":      ErrIDFirstSyncCallDeadlock,
		"ErrAtomNotExists":                ErrAtomNotExists,
		"ErrAtomAlreadyExists":            ErrAtomAlreadyExists,
		"ErrAtomSpawning":                 ErrAtomSpawning,
		"ErrAtomIsStopping":               ErrAtomIsStopping,
		"ErrAtomNotRunning":               ErrAtomNotRunning,
		"ErrAtomInstanceMismatch":         ErrAtomInstanceMismatch,
		"ErrCosmosNodeDraining":           ErrCosmosNodeDraining,
		"ErrAsyncCallbackCancelledByHalt": ErrAsyncCallbackCancelledByHalt,
		"ErrAsyncRetryQueueFull":          ErrAsyncRetryQueueFull,
		"ErrAsyncDuplicateMessage":        ErrAsyncDuplicateMessage,
		"ErrAsyncRetryUnavailable":        ErrAsyncRetryUnavailable,
		"ErrOwnershipLost":                ErrOwnershipLost,
		"ErrOwnershipNotReady":            ErrOwnershipNotReady,
		"ErrStaleFence":                   ErrStaleFence,
		"ErrOperationRunning":             ErrOperationRunning,
		"ErrOperationNotFound":            ErrOperationNotFound,
		"ErrOperationConflict":            ErrOperationConflict,
		"ErrOperationContended":           ErrOperationContended,
		"ErrTakeoverRequired":             ErrTakeoverRequired,
		"ErrOperationOutcomeUnknown":      ErrOperationOutcomeUnknown,
		"ErrFencingRequired":              ErrFencingRequired,
	}
	for name, s := range sentinels {
		wantName := "ErrCode" + name[3:]
		want, ok := errorCodeSnapshot[wantName]
		if !ok {
			t.Errorf("no registered code found for sentinel %s (looked up %s)", name, wantName)
			continue
		}
		if got := CodeOf(s); got != want {
			t.Errorf("CodeOf(%s) = %d, want %d", name, got, want)
		}
	}
}

// TestPublicAPISuccessReturnsUntypedNil guards the v3 error-semantics
// contract: a successful public API call must return a genuine nil error,
// never a typed nil *Error boxed in an error interface (a typed nil would
// make `err == nil` false on the success path).
func TestPublicAPISuccessReturnsUntypedNil(t *testing.T) {
	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		atomID, err := SpawnForTestAtomosAtom(p.local, p.local, "nil_check_atom", &ForTestSpawnArg{})
		if err != nil {
			t.Fatalf("SpawnForTestAtomosAtom returned non-nil error on success: %#v", err)
		}
		defer atomID.Release()

		if _, err := atomID.Greeting(p.local, &ForTestGreetingI{Mode: 1}); err != nil {
			t.Fatalf("Greeting returned non-nil error on success: %#v", err)
		}

		gotID, err := GetForTestAtomosAtomID(p.local, "nil_check_atom")
		if err != nil {
			t.Fatalf("GetForTestAtomosAtomID returned non-nil error on success: %#v", err)
		}
		defer gotID.Release()

		if err := atomID.Kill(p.local, ArgBaseAtomosWaitKilled()); err != nil {
			t.Fatalf("Kill returned non-nil error on success: %#v", err)
		}
	})
}
