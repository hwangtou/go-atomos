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

// grpcCodeOfErrCode gets the transport classification from the same registry
// used by recovery policies. Exact framework and application codes survive in
// status details; the gRPC code is intentionally many-to-one.
func grpcCodeOfErrCode(code int64) codes.Code {
	if spec, ok := errorSpecOf(code); ok {
		return spec.grpc
	}
	return codes.Unknown
}

// errCodeOfGRPCCode classifies a status that has no Atomos detail. It returns
// only remote-domain codes: a bare NotFound, for example, carries no evidence
// that an atom rather than another resource was missing.
func errCodeOfGRPCCode(c codes.Code) int64 {
	switch c {
	case codes.NotFound:
		return ErrCodeCosmosRemoteStatusNotFound
	case codes.FailedPrecondition:
		return ErrCodeCosmosRemoteStatusFailedPrecondition
	case codes.InvalidArgument:
		return ErrCodeCosmosRemoteStatusInvalidArgument
	case codes.DeadlineExceeded:
		return ErrCodeCosmosRemoteStatusDeadlineExceeded
	case codes.Unavailable:
		return ErrCodeCosmosRemoteStatusUnavailable
	case codes.Internal:
		return ErrCodeCosmosRemoteStatusInternal
	case codes.PermissionDenied:
		return ErrCodeCosmosRemoteStatusPermissionDenied
	case codes.Canceled:
		return ErrCodeCosmosRemoteStatusCanceled
	case codes.ResourceExhausted:
		return ErrCodeCosmosRemoteStatusResourceExhausted
	case codes.Unauthenticated:
		return ErrCodeCosmosRemoteStatusUnauthenticated
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
// A *Error carried in the status details is returned as-is, preserving any
// framework or application code and all call stacks. Without details, only
// remote-domain status errors are synthesized. Non-status errors become
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
