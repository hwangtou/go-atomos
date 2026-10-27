package atomos

import "errors"

// Sentinel errors for the codes callers most often compare against. Test an
// error with errors.Is(err, ErrXxx), or extract its code with CodeOf(err).
//
// Sentinels are package-shared: never mutate them through a type assertion
// (e.g. do not call AddStack on them). To build an error that carries
// stacks, construct a fresh one with NewError/NewErrorf and the matching
// ErrCode constant.
var (
	ErrFrameworkInternalError       error = &Error{Code: ErrCodeFrameworkInternalError, Message: "framework internal error"}
	ErrFrameworkRecoverFromPanic    error = &Error{Code: ErrCodeFrameworkRecoverFromPanic, Message: "framework recovered from panic"}
	ErrFrameworkInvalidID           error = &Error{Code: ErrCodeFrameworkInvalidID, Message: "ID does not implement the framework-internal extension"}
	ErrOperationUnsupported         error = &Error{Code: ErrCodeOperationUnsupported, Message: "operation is not supported"}
	ErrElementNotFound              error = &Error{Code: ErrCodeElementNotFound, Message: "element not found"}
	ErrAtomosInvalidArguments       error = &Error{Code: ErrCodeAtomosInvalidArguments, Message: "invalid arguments"}
	ErrAtomosIsStopping             error = &Error{Code: ErrCodeAtomosIsStopping, Message: "atomos is stopping"}
	ErrAtomosIsNotRunning           error = &Error{Code: ErrCodeAtomosIsNotRunning, Message: "atomos is not running"}
	ErrAtomosPushTimeoutHandling    error = &Error{Code: ErrCodeAtomosPushTimeoutHandling, Message: "push timed out while the atom was handling"}
	ErrAtomosPushTimeoutReject      error = &Error{Code: ErrCodeAtomosPushTimeoutReject, Message: "push rejected because the atom is busy"}
	ErrAtomosTaskNotCancellable     error = &Error{Code: ErrCodeAtomosTaskNotCancellable, Message: "task is not cancellable"}
	ErrIDFirstSyncCallDeadlock      error = &Error{Code: ErrCodeIDFirstSyncCallDeadlock, Message: "first sync call deadlock"}
	ErrAtomNotExists                error = &Error{Code: ErrCodeAtomNotExists, Message: "atom does not exist"}
	ErrAtomAlreadyExists            error = &Error{Code: ErrCodeAtomAlreadyExists, Message: "atom already exists"}
	ErrAtomSpawning                 error = &Error{Code: ErrCodeAtomSpawning, Message: "atom is spawning"}
	ErrAtomIsStopping               error = &Error{Code: ErrCodeAtomIsStopping, Message: "atom is stopping"}
	ErrAtomNotRunning               error = &Error{Code: ErrCodeAtomNotRunning, Message: "atom is not running"}
	ErrAtomInstanceMismatch         error = &Error{Code: ErrCodeAtomInstanceMismatch, Message: "atom instance mismatch"}
	ErrCosmosNodeDraining           error = &Error{Code: ErrCodeCosmosNodeDraining, Message: "cosmos node is draining"}
	ErrAsyncCallbackCancelledByHalt error = &Error{Code: ErrCodeAsyncCallbackCancelledByHalt, Message: "async callback cancelled by atom halt"}
	ErrAsyncRetryQueueFull          error = &Error{Code: ErrCodeAsyncRetryQueueFull, Message: "async retry queue is full"}
	ErrAsyncDuplicateMessage        error = &Error{Code: ErrCodeAsyncDuplicateMessage, Message: "duplicate asynchronous message"}
	ErrAsyncRetryUnavailable        error = &Error{Code: ErrCodeAsyncRetryUnavailable, Message: "async retry is unavailable"}
	ErrOwnershipLost                error = &Error{Code: ErrCodeOwnershipLost, Message: "atom ownership lost"}
	ErrOwnershipNotReady            error = &Error{Code: ErrCodeOwnershipNotReady, Message: "atom ownership is not ready"}
	ErrStaleFence                   error = &Error{Code: ErrCodeStaleFence, Message: "stale atom fence"}
	ErrOperationRunning             error = &Error{Code: ErrCodeOperationRunning, Message: "atom operation is running"}
	ErrOperationNotFound            error = &Error{Code: ErrCodeOperationNotFound, Message: "atom operation not found"}
	ErrOperationConflict            error = &Error{Code: ErrCodeOperationConflict, Message: "atom operation conflict"}
	ErrOperationContended           error = &Error{Code: ErrCodeOperationContended, Message: "atom operation contended"}
	ErrTakeoverRequired             error = &Error{Code: ErrCodeTakeoverRequired, Message: "atom takeover required"}
	ErrOperationOutcomeUnknown      error = &Error{Code: ErrCodeOperationOutcomeUnknown, Message: "atom operation outcome unknown"}
	ErrFencingRequired              error = &Error{Code: ErrCodeFencingRequired, Message: "atom fencing required"}
)

// Is reports whether x matches target by error code: target must be an
// *Error with the same Code. This makes *Error work with errors.Is against
// the package sentinels regardless of message or call stacks.
func (x *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return x != nil && t != nil && x.Code == t.Code
}

// CodeOf extracts the error code from err. It returns OK for nil, the code
// of the first *Error in err's unwrap chain, and ErrCodeUnknown for any
// other error.
func CodeOf(err error) int64 {
	if err == nil {
		return OK
	}
	var e *Error
	if errors.As(err, &e) && e != nil {
		return e.Code
	}
	return ErrCodeUnknown
}
