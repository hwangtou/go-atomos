package atomos

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGRPCCodeOfErrCode(t *testing.T) {
	cases := []struct {
		code int64
		want codes.Code
	}{
		// Not-exist class.
		{ErrCodeMainElementNotFound, codes.NotFound},
		{ErrCodeCosmosRemoteElementNotFound, codes.NotFound},
		{ErrCodeAtomosTaskNotExists, codes.NotFound},
		{ErrCodeAtomDataNotFound, codes.NotFound},
		{ErrCodeAtomNotExists, codes.NotFound},

		// State-rejection class.
		{ErrCodeCosmosIsClosed, codes.FailedPrecondition},
		{ErrCodeAtomosIsStopping, codes.FailedPrecondition},
		{ErrCodeAtomosIsNotRunning, codes.FailedPrecondition},
		{ErrCodeElementLoaded, codes.FailedPrecondition},
		{ErrCodeAtomIsRunning, codes.FailedPrecondition},
		{ErrCodeAtomIsStopping, codes.FailedPrecondition},
		{ErrCodeAtomSpawningAnExistedAtom, codes.FailedPrecondition},
		{ErrCodeAtomInstanceMismatch, codes.FailedPrecondition},
		{ErrCodeCosmosNodeDraining, codes.FailedPrecondition},
		{ErrCodeAsyncCallbackCancelledByHalt, codes.FailedPrecondition},

		// Invalid-argument class.
		{ErrCodeFrameworkIncorrectUsage, codes.InvalidArgument},
		{ErrCodeFrameworkInvalidID, codes.InvalidArgument},
		{ErrCodeCosmosRemoteRequestInvalid, codes.InvalidArgument},
		{ErrCodeCosmosRemoteServerInvalidArgs, codes.InvalidArgument},
		{ErrCodeCosmosRemoteInfoInvalid, codes.InvalidArgument},
		{ErrCodeAtomosInvalidArguments, codes.InvalidArgument},
		{ErrCodeAtomosTaskInvalidFn, codes.InvalidArgument},
		{ErrCodeElementFromIDInvalid, codes.InvalidArgument},
		{ErrCodeAtomMessageArgType, codes.InvalidArgument},

		// Timeout class.
		{ErrCodeAtomosPushTimeoutHandling, codes.DeadlineExceeded},
		{ErrCodeAtomosPushTimeoutReject, codes.DeadlineExceeded},

		// Panic / internal class.
		{ErrCodeFrameworkInternalError, codes.Internal},
		{ErrCodeFrameworkRecoverFromPanic, codes.Internal},
		{ErrCodeCosmosProcessOnStartupPanic, codes.Internal},
		{ErrCodeCosmosProcessOnShutdownPanic, codes.Internal},
		{ErrCodeMainStartRunningPanic, codes.Internal},

		// Everything else.
		{ErrCodeCosmosRemoteConnectFailed, codes.Unknown},
		{ErrCodeCosmosRemoteResponseInvalid, codes.Unknown},
		{ErrCodeElementMessageHandlerNotExists, codes.Unknown},
		{999999, codes.Unknown},
	}
	for _, c := range cases {
		if got := grpcCodeOfErrCode(c.code); got != c.want {
			t.Errorf("grpcCodeOfErrCode(%d) = %v, want %v", c.code, got, c.want)
		}
	}
}

func TestGRPCStatusRoundTrip(t *testing.T) {
	src := NewErrorf(ErrCodeAtomNotExists, "atom %q does not exist", "foo").AddStack(nil)

	// Server side: double-write produces a non-OK status carrying the *Error.
	er := grpcStatusFromError(src)
	if er == nil {
		t.Fatal("grpcStatusFromError returned nil")
	}
	st, ok := status.FromError(er)
	if !ok {
		t.Fatalf("not a status error: %v", er)
	}
	if st.Code() != codes.NotFound {
		t.Fatalf("status code = %v, want NotFound", st.Code())
	}
	if st.Message() != src.Message {
		t.Fatalf("status message = %q, want %q", st.Message(), src.Message)
	}

	// Client side: the detail restores code, message and call stacks.
	got := grpcErrorToError(er)
	if got == nil {
		t.Fatal("grpcErrorToError returned nil")
	}
	if got.Code != src.Code {
		t.Errorf("Code = %d, want %d", got.Code, src.Code)
	}
	if got.Message != src.Message {
		t.Errorf("Message = %q, want %q", got.Message, src.Message)
	}
	if len(got.CallStacks) == 0 {
		t.Error("CallStacks lost in status round trip")
	}
	if !errors.Is(got, ErrAtomNotExists) {
		t.Error("errors.Is(got, ErrAtomNotExists) = false")
	}
}

func TestGRPCStatusFromErrorNil(t *testing.T) {
	if grpcStatusFromError(nil) != nil {
		t.Fatal("grpcStatusFromError(nil) should be nil")
	}
	if grpcErrorToError(nil) != nil {
		t.Fatal("grpcErrorToError(nil) should be nil")
	}
}

func TestGRPCErrorToErrorReverseMapping(t *testing.T) {
	cases := []struct {
		code codes.Code
		want int64
	}{
		{codes.NotFound, ErrCodeAtomNotExists},
		{codes.FailedPrecondition, ErrCodeAtomosIsNotRunning},
		{codes.InvalidArgument, ErrCodeAtomosInvalidArguments},
		{codes.DeadlineExceeded, ErrCodeAtomosPushTimeoutHandling},
		{codes.Unavailable, ErrCodeCosmosRemoteConnectFailed},
		{codes.Internal, ErrCodeFrameworkInternalError},
		{codes.PermissionDenied, ErrCodeCosmosRemoteResponseFailed},
		{codes.Unknown, ErrCodeCosmosRemoteResponseFailed},
	}
	for _, c := range cases {
		er := status.Error(c.code, "boom")
		got := grpcErrorToError(er)
		if got == nil || got.Code != c.want {
			t.Errorf("grpcErrorToError(status %v) code = %v, want %d", c.code, got, c.want)
		}
		if got != nil && len(got.Message) == 0 {
			t.Errorf("grpcErrorToError(status %v) lost the status message", c.code)
		}
	}

	// A non-status error becomes ErrCodeCosmosRemoteResponseInvalid.
	if got := grpcErrorToError(errors.New("plain")); got == nil || got.Code != ErrCodeCosmosRemoteResponseInvalid {
		t.Fatalf("grpcErrorToError(plain) = %v", got)
	}
}

func TestCallRspWithStatus(t *testing.T) {
	// Success: no status error, response untouched.
	okRsp := &CosmosRemoteGetAtomIDRsp{Id: &IDInfo{Atom: "a"}}
	rsp, err := callRspWithStatus(okRsp, nil)
	if err != nil {
		t.Fatalf("success case err=(%v)", err)
	}
	if rsp != okRsp {
		t.Fatal("response must be passed through")
	}

	// Failure: body keeps the embedded *Error AND a status error is returned.
	body := NewErrorf(ErrCodeAtomIsStopping, "atom is stopping").AddStack(nil)
	badRsp := &CosmosRemoteGetAtomIDRsp{Error: body}
	rsp, err = callRspWithStatus(badRsp, nil)
	if err == nil {
		t.Fatal("failure case should return a status error")
	}
	if rsp.Error != body {
		t.Fatal("body-embedded error must remain for v2 readers")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.FailedPrecondition {
		t.Fatalf("status code = %v, want FailedPrecondition", st.Code())
	}
	restored := grpcErrorToError(err)
	if restored.Code != ErrCodeAtomIsStopping || len(restored.CallStacks) == 0 {
		t.Fatalf("restored = (%d, stacks=%d)", restored.Code, len(restored.CallStacks))
	}

	// A transport-level error passes through unchanged.
	in := status.Error(codes.Unavailable, "conn refused")
	if _, err = callRspWithStatus(okRsp, in); err != in {
		t.Fatal("non-nil err must pass through untouched")
	}
}
