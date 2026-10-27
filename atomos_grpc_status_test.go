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
		{ErrCodeElementNotFound, codes.NotFound},
		{ErrCodeAtomosTaskNotExists, codes.NotFound},
		{ErrCodeAtomDataNotFound, codes.NotFound},
		{ErrCodeAtomNotExists, codes.NotFound},
		{ErrCodeOperationNotFound, codes.NotFound},
		{ErrCodeAtomAlreadyExists, codes.AlreadyExists},
		{ErrCodeOperationConflict, codes.AlreadyExists},
		{ErrCodeOwnershipLost, codes.Aborted},
		{ErrCodeOperationContended, codes.Aborted},
		{ErrCodeStaleFence, codes.Aborted},
		{ErrCodeAtomInstanceMismatch, codes.Aborted},
		{ErrCodeAtomosIsStopping, codes.FailedPrecondition},
		{ErrCodeAtomosIsNotRunning, codes.FailedPrecondition},
		{ErrCodeAtomIsStopping, codes.FailedPrecondition},
		{ErrCodeOperationRunning, codes.FailedPrecondition},
		{ErrCodeTakeoverRequired, codes.FailedPrecondition},
		{ErrCodeFencingRequired, codes.FailedPrecondition},
		{ErrCodeOperationUnsupported, codes.Unimplemented},
		{ErrCodeFrameworkIncorrectUsage, codes.InvalidArgument},
		{ErrCodeFrameworkInvalidID, codes.InvalidArgument},
		{ErrCodeCosmosRemoteRequestInvalid, codes.InvalidArgument},
		{ErrCodeAtomosInvalidArguments, codes.InvalidArgument},
		{ErrCodeAtomosTaskInvalidFn, codes.InvalidArgument},
		{ErrCodeMessageFromIDInvalid, codes.InvalidArgument},
		{ErrCodeMessageArgTypeInvalid, codes.InvalidArgument},
		{ErrCodeAtomosPushTimeoutHandling, codes.DeadlineExceeded},
		{ErrCodeAtomosPushTimeoutReject, codes.DeadlineExceeded},
		{ErrCodeAtomSpawning, codes.Unavailable},
		{ErrCodeCosmosNodeDraining, codes.Unavailable},
		{ErrCodeCosmosRemoteConnectFailed, codes.Unavailable},
		{ErrCodeCosmosRemoteResponseInvalid, codes.DataLoss},
		{ErrCodeStorageDataCorrupted, codes.DataLoss},
		{ErrCodeAsyncRetryQueueFull, codes.ResourceExhausted},
		{ErrCodeFrameworkInternalError, codes.Internal},
		{ErrCodeFrameworkRecoverFromPanic, codes.Internal},
		{ErrCodeCosmosProcessOnStartupPanic, codes.Internal},
		{ErrCodeCosmosProcessOnShutdownPanic, codes.Internal},
		{ErrCodeMainStartRunningPanic, codes.Internal},
		{ErrCodeOperationOutcomeUnknown, codes.Unknown},
		{int64(1000001), codes.Unknown},
	}
	for _, c := range cases {
		if got := grpcCodeOfErrCode(c.code); got != c.want {
			t.Errorf("grpcCodeOfErrCode(%d) = %v, want %v", c.code, got, c.want)
		}
	}
}

func TestGRPCStatusRoundTrip(t *testing.T) {
	for _, code := range []int64{ErrCodeAtomNotExists, 1000001} {
		src := NewErrorf(code, "error %d", code).AddStack(nil)
		er := grpcStatusFromError(src)
		if er == nil {
			t.Fatal("grpcStatusFromError returned nil")
		}
		st, ok := status.FromError(er)
		if !ok {
			t.Fatalf("not a status error: %v", er)
		}
		if st.Code() != grpcCodeOfErrCode(code) {
			t.Fatalf("status code = %v, want %v", st.Code(), grpcCodeOfErrCode(code))
		}
		if st.Message() != src.Message {
			t.Fatalf("status message = %q, want %q", st.Message(), src.Message)
		}
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

func TestGRPCErrorToErrorWithoutDetailsUsesRemoteCodes(t *testing.T) {
	cases := []struct {
		code codes.Code
		want int64
	}{
		{codes.NotFound, ErrCodeCosmosRemoteStatusNotFound},
		{codes.FailedPrecondition, ErrCodeCosmosRemoteStatusFailedPrecondition},
		{codes.InvalidArgument, ErrCodeCosmosRemoteStatusInvalidArgument},
		{codes.DeadlineExceeded, ErrCodeCosmosRemoteStatusDeadlineExceeded},
		{codes.Unavailable, ErrCodeCosmosRemoteStatusUnavailable},
		{codes.Internal, ErrCodeCosmosRemoteStatusInternal},
		{codes.PermissionDenied, ErrCodeCosmosRemoteStatusPermissionDenied},
		{codes.Canceled, ErrCodeCosmosRemoteStatusCanceled},
		{codes.ResourceExhausted, ErrCodeCosmosRemoteStatusResourceExhausted},
		{codes.Unauthenticated, ErrCodeCosmosRemoteStatusUnauthenticated},
		{codes.Unknown, ErrCodeCosmosRemoteResponseFailed},
	}
	for _, c := range cases {
		got := grpcErrorToError(status.Error(c.code, "boom"))
		if got == nil || got.Code != c.want {
			t.Errorf("grpcErrorToError(status %v) code = %v, want %d", c.code, got, c.want)
		}
		if got != nil && got.Code >= 4000 && got.Code < 7000 {
			t.Errorf("bare status %v was misidentified as an actor/lifecycle error: %d", c.code, got.Code)
		}
		if got != nil && got.Message == "" {
			t.Errorf("grpcErrorToError(status %v) lost the status message", c.code)
		}
	}
	if got := grpcErrorToError(errors.New("plain")); got == nil || got.Code != ErrCodeCosmosRemoteResponseInvalid {
		t.Fatalf("grpcErrorToError(plain) = %v", got)
	}
}

func TestCallRspWithStatus(t *testing.T) {
	okRsp := &CosmosRemoteGetAtomIDRsp{Id: &IDInfo{Atom: "a"}}
	rsp, err := callRspWithStatus(okRsp, nil)
	if err != nil {
		t.Fatalf("success case err=(%v)", err)
	}
	if rsp != okRsp {
		t.Fatal("response must be passed through")
	}

	body := NewErrorf(ErrCodeAtomIsStopping, "atom is stopping").AddStack(nil)
	badRsp := &CosmosRemoteGetAtomIDRsp{Error: body}
	rsp, err = callRspWithStatus(badRsp, nil)
	if err == nil {
		t.Fatal("failure case should return a status error")
	}
	if rsp.Error != body {
		t.Fatal("body-embedded error must be preserved")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.FailedPrecondition {
		t.Fatalf("status code = %v, want FailedPrecondition", st.Code())
	}
	restored := grpcErrorToError(err)
	if restored.Code != ErrCodeAtomIsStopping || len(restored.CallStacks) == 0 {
		t.Fatalf("restored = (%d, stacks=%d)", restored.Code, len(restored.CallStacks))
	}

	in := status.Error(codes.Unavailable, "conn refused")
	if _, err = callRspWithStatus(okRsp, in); err != in {
		t.Fatal("non-nil err must pass through untouched")
	}
}
