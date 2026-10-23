package atomos

import "errors"

// ErrCodeUnknown is returned by CodeOf when the error is not an *Error and
// carries no *Error in its unwrap chain. It is a reserved sentinel value and
// is never assigned to a framework error code.
const ErrCodeUnknown int64 = -1

// Sentinel errors for the codes callers most often compare against. Test an
// error with errors.Is(err, ErrXxx), or extract its code with CodeOf(err).
//
// Sentinels are package-shared: never mutate them through a type assertion
// (e.g. do not call AddStack on them). To build an error that carries
// stacks, construct a fresh one with NewError/NewErrorf and the matching
// ErrCode constant.
var (
	ErrFrameworkInternalError              error = &Error{Code: ErrCodeFrameworkInternalError, Message: "framework internal error"}
	ErrFrameworkRecoverFromPanic           error = &Error{Code: ErrCodeFrameworkRecoverFromPanic, Message: "framework recovered from panic"}
	ErrFrameworkInvalidID                  error = &Error{Code: ErrCodeFrameworkInvalidID, Message: "ID does not implement the framework-internal extension"}
	ErrMainElementNotFound                 error = &Error{Code: ErrCodeMainElementNotFound, Message: "element not found"}
	ErrAtomosInvalidArguments              error = &Error{Code: ErrCodeAtomosInvalidArguments, Message: "invalid arguments"}
	ErrAtomosIsStopping                    error = &Error{Code: ErrCodeAtomosIsStopping, Message: "atomos is stopping"}
	ErrAtomosIsNotRunning                  error = &Error{Code: ErrCodeAtomosIsNotRunning, Message: "atomos is not running"}
	ErrAtomosNotSupportWormhole            error = &Error{Code: ErrCodeAtomosNotSupportWormhole, Message: "atomos does not support wormhole"}
	ErrAtomosPushTimeoutHandling           error = &Error{Code: ErrCodeAtomosPushTimeoutHandling, Message: "push timed out while the atom was handling"}
	ErrAtomosPushTimeoutReject             error = &Error{Code: ErrCodeAtomosPushTimeoutReject, Message: "push rejected because the atom is busy"}
	ErrAtomosTaskCannotCancelCancelledTask error = &Error{Code: ErrCodeAtomosTaskCannotCancelCancelledTask, Message: "cannot cancel a cancelled task"}
	ErrAtomosTaskCannotCancelRunningTask   error = &Error{Code: ErrCodeAtomosTaskCannotCancelRunningTask, Message: "cannot cancel a running task"}
	ErrAtomosTaskCannotCancelDoneTask      error = &Error{Code: ErrCodeAtomosTaskCannotCancelDoneTask, Message: "cannot cancel a done task"}
	ErrIDFirstSyncCallDeadlock             error = &Error{Code: ErrCodeIDFirstSyncCallDeadlock, Message: "first sync call deadlock"}
	ErrAtomNotExists                       error = &Error{Code: ErrCodeAtomNotExists, Message: "atom does not exist"}
	ErrAtomIsRunning                       error = &Error{Code: ErrCodeAtomIsRunning, Message: "atom is running"}
	ErrAtomIsStopping                      error = &Error{Code: ErrCodeAtomIsStopping, Message: "atom is stopping"}
	ErrAtomSpawningAnExistedAtom           error = &Error{Code: ErrCodeAtomSpawningAnExistedAtom, Message: "spawning an existed atom"}
	ErrAtomInstanceMismatch                error = &Error{Code: ErrCodeAtomInstanceMismatch, Message: "atom instance mismatch"}
	ErrCosmosNodeDraining                  error = &Error{Code: ErrCodeCosmosNodeDraining, Message: "cosmos node is draining"}
	ErrAsyncCallbackCancelledByHalt        error = &Error{Code: ErrCodeAsyncCallbackCancelledByHalt, Message: "async callback cancelled by atom halt"}
)

// Is reports whether x matches target by error code: target must be an
// *Error with the same Code. This makes *Error work with errors.Is against
// the package sentinels regardless of message or call stacks.
func (x *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return x != nil && x.Code == t.Code
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
