package atomos

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// gRPC status interop for call-class RPCs (SyncMessagingByName,
// AsyncMessagingByName, SpawnAtom, KillAtom, GetAtomID, GetIDState,
// GetIDIdleTime).
//
// Double-write strategy (see docs/DESIGN_V3_API.md §8): when a call-class
// handler fails, the response body still carries the embedded *Error (v2
// peers read only that field), and the RPC additionally returns a non-OK
// gRPC status whose details contain the same *Error proto, so monitoring and
// interceptors can recognize failures. On the client, a recovered status
// detail wins over the body because it is equally complete.
//
// Management-plane RPCs (TryKilling, DrainNode, GetElementInfo,
// ElementBroadcast, AsyncOnMessageCallback) deliberately keep body-embedded
// errors only: their toolchain consumers prefer a always-OK transport, so no
// status mapping is applied there.

// grpcCodeOfErrCode maps a framework error code to a gRPC status code. The
// mapping is many-to-one and only meaningful in the forward direction; exact
// framework codes survive the round trip through the status details, not
// through the status code.
func grpcCodeOfErrCode(code int64) codes.Code {
	switch code {
	// Not-exist class.
	case ErrCodeMainElementNotFound,
		ErrCodeCosmosRemoteElementNotFound,
		ErrCodeAtomosTaskNotExists,
		ErrCodeAtomDataNotFound,
		ErrCodeAtomNotExists:
		return codes.NotFound

	// State-rejection class: draining, stopping, not running, already
	// running, spawn conflicts and stale-instance mismatches.
	case ErrCodeCosmosIsClosed,
		ErrCodeAtomosIsStopping,
		ErrCodeAtomosIsNotRunning,
		ErrCodeElementLoaded,
		ErrCodeAtomIsRunning,
		ErrCodeAtomIsStopping,
		ErrCodeAtomSpawningAnExistedAtom,
		ErrCodeAtomInstanceMismatch,
		ErrCodeCosmosNodeDraining,
		ErrCodeAsyncCallbackCancelledByHalt:
		return codes.FailedPrecondition

	// Invalid-argument class, including type mismatches and programming
	// errors surfaced through the ID interface.
	case ErrCodeFrameworkIncorrectUsage,
		ErrCodeFrameworkInvalidID,
		ErrCodeCosmosRemoteRequestInvalid,
		ErrCodeCosmosRemoteServerInvalidArgs,
		ErrCodeCosmosRemoteInfoInvalid,
		ErrCodeAtomosInvalidArguments,
		ErrCodeAtomosTaskInvalidFn,
		ErrCodeElementMessageArgType,
		ErrCodeElementFromIDInvalid,
		ErrCodeElementToIDInvalid,
		ErrCodeAtomFromIDInvalid,
		ErrCodeAtomToIDInvalid,
		ErrCodeAtomMessageAtomType,
		ErrCodeAtomMessageArgType,
		ErrCodeAtomMessageReplyType:
		return codes.InvalidArgument

	// Timeout class.
	case ErrCodeAtomosPushTimeoutHandling,
		ErrCodeAtomosPushTimeoutReject:
		return codes.DeadlineExceeded

	// Panic / internal-error class.
	case ErrCodeFrameworkInternalError,
		ErrCodeFrameworkRecoverFromPanic,
		ErrCodeCosmosProcessOnStartupPanic,
		ErrCodeCosmosProcessOnShutdownPanic,
		ErrCodeMainStartRunningPanic:
		return codes.Internal

	default:
		return codes.Unknown
	}
}

// errCodeOfGRPCCode is the best-effort reverse of grpcCodeOfErrCode, used
// when a gRPC error carries no *Error detail (transport-level failures,
// local context deadlines, or non-Atomos servers). It maps to a documented
// representative code per status code; the original status message is kept
// in the constructed error.
func errCodeOfGRPCCode(c codes.Code) int64 {
	switch c {
	case codes.NotFound:
		return ErrCodeAtomNotExists
	case codes.FailedPrecondition:
		return ErrCodeAtomosIsNotRunning
	case codes.InvalidArgument:
		return ErrCodeAtomosInvalidArguments
	case codes.DeadlineExceeded:
		return ErrCodeAtomosPushTimeoutHandling
	case codes.Unavailable:
		return ErrCodeCosmosRemoteConnectFailed
	case codes.Internal:
		return ErrCodeFrameworkInternalError
	default:
		return ErrCodeCosmosRemoteResponseFailed
	}
}

// grpcStatusFromError converts a body-embedded framework error into a gRPC
// status error whose details carry the full *Error proto (code, message and
// call stacks). It returns nil for nil, so handlers can unconditionally
// return grpcStatusFromError(rsp.Error) alongside the response.
func grpcStatusFromError(err *Error) error {
	if err == nil {
		return nil
	}
	st, derr := status.New(grpcCodeOfErrCode(err.Code), err.Message).WithDetails(err)
	if derr != nil {
		// Details are best-effort: fall back to the bare status.
		return status.New(grpcCodeOfErrCode(err.Code), err.Message).Err()
	}
	return st.Err()
}

// grpcErrorToError recovers a framework error from a failed call-class RPC.
// A *Error carried in the status details (v3 servers) is returned as-is,
// preserving its code and call stacks. Otherwise the status code is
// reverse-mapped to a representative framework code with the status message
// attached. Non-status errors (should not happen with gRPC) become
// ErrCodeCosmosRemoteResponseInvalid.
func grpcErrorToError(er error) *Error {
	if er == nil {
		return nil
	}
	st, ok := status.FromError(er)
	if !ok {
		return NewErrorf(ErrCodeCosmosRemoteResponseInvalid, "CosmosRemote: response error. err=(%v)", er)
	}
	for _, d := range st.Details() {
		if e, isErr := d.(*Error); isErr && e != nil {
			return e
		}
	}
	return NewErrorf(errCodeOfGRPCCode(st.Code()), "CosmosRemote: remote status %s. message=(%s)", st.Code(), st.Message())
}

// callRspWithStatus implements the double-write for call-class handlers: the
// response keeps its body-embedded *Error, and a non-nil *Error additionally
// surfaces as a non-OK gRPC status with the *Error proto in the details.
func callRspWithStatus[Rsp interface{ GetError() *Error }](rsp Rsp, err error) (Rsp, error) {
	if err != nil {
		return rsp, err
	}
	return rsp, grpcStatusFromError(rsp.GetError())
}
