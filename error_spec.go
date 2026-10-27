package atomos

import "google.golang.org/grpc/codes"

type errorClass uint8

const (
	errorClassUnspecified errorClass = iota
	errorClassInvalidRequest
	errorClassNotFound
	errorClassAlreadyExists
	errorClassFailedPrecondition
	errorClassConflict
	errorClassTimeout
	errorClassUnavailable
	errorClassPermissionDenied
	errorClassDataLoss
	errorClassInternal
	errorClassCanceled
	errorClassResourceExhausted
	errorClassUnauthenticated
	errorClassUnsupported
)

type errorOutcome uint8

const (
	errorOutcomeNotApplicable errorOutcome = iota
	errorOutcomeNotExecuted
	errorOutcomeUnknown
)

type recoveryKind uint8

const (
	recoveryNone recoveryKind = iota
	recoveryBackoff
	recoveryRebind
	recoveryQueryOperation
	recoveryManual
)

type errorSpec struct {
	class    errorClass
	outcome  errorOutcome
	recovery recoveryKind
	grpc     codes.Code
}

func newErrorSpec(class errorClass, outcome errorOutcome, recovery recoveryKind, grpc codes.Code) errorSpec {
	return errorSpec{class: class, outcome: outcome, recovery: recovery, grpc: grpc}
}

var errorSpecs = map[int64]errorSpec{
	ErrCodeFrameworkInternalError:    newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeFrameworkRecoverFromPanic: newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeFrameworkIncorrectUsage:   newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeFrameworkInvalidID:        newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeOperationUnsupported:      newErrorSpec(errorClassUnsupported, errorOutcomeNotExecuted, recoveryNone, codes.Unimplemented),
	ErrCodeAtomosInvalidArguments:    newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeAtomosIsStopping:          newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeAtomosIsNotRunning:        newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),

	ErrCodeCosmosProcessHasNotInitialized:       newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeCosmosProcessHasBeenStarted:          newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeCosmosProcessOnStartupPanic:          newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeCosmosProcessOnShutdownPanic:         newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeCosmosProcessCannotStopPrepareState:  newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeCosmosProcessCannotStopStartupState:  newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeCosmosProcessCannotStopShutdownState: newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeCosmosProcessCannotStopOffState:      newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeCosmosProcessInvalidState:            newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeCosmosProcessInvalidTransition:       newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeMainStartRunningPanic:                newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeMainCannotMessage:                    newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeMainRunnableNotFound:                 newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeRunnableConfigNotFound:               newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeRunnableScriptNotFound:               newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeCosmosConfigInvalid:                  newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeCosmosConfigReadFailed:               newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryBackoff, codes.Unavailable),
	ErrCodeCosmosEtcdClusterTLSInvalid:          newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeCosmosEtcdClusterVersionsCheckFailed: newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),

	ErrCodeCosmosRemoteNodeNotFound:             newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeCosmosRemoteListenFailed:             newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryNone, codes.Unavailable),
	ErrCodeCosmosRemoteAddressNotFound:          newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeCosmosRemoteConnectFailed:            newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryBackoff, codes.Unavailable),
	ErrCodeCosmosRemoteRequestInvalid:           newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeCosmosRemoteResponseFailed:           newErrorSpec(errorClassUnavailable, errorOutcomeUnknown, recoveryNone, codes.Unknown),
	ErrCodeCosmosRemoteResponseInvalid:          newErrorSpec(errorClassDataLoss, errorOutcomeUnknown, recoveryNone, codes.DataLoss),
	ErrCodeCosmosRemoteStatusNotFound:           newErrorSpec(errorClassNotFound, errorOutcomeUnknown, recoveryNone, codes.NotFound),
	ErrCodeCosmosRemoteStatusFailedPrecondition: newErrorSpec(errorClassFailedPrecondition, errorOutcomeUnknown, recoveryNone, codes.FailedPrecondition),
	ErrCodeCosmosRemoteStatusInvalidArgument:    newErrorSpec(errorClassInvalidRequest, errorOutcomeUnknown, recoveryNone, codes.InvalidArgument),
	ErrCodeCosmosRemoteStatusDeadlineExceeded:   newErrorSpec(errorClassTimeout, errorOutcomeUnknown, recoveryBackoff, codes.DeadlineExceeded),
	ErrCodeCosmosRemoteStatusUnavailable:        newErrorSpec(errorClassUnavailable, errorOutcomeUnknown, recoveryBackoff, codes.Unavailable),
	ErrCodeCosmosRemoteStatusInternal:           newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeCosmosRemoteStatusPermissionDenied:   newErrorSpec(errorClassPermissionDenied, errorOutcomeUnknown, recoveryNone, codes.PermissionDenied),
	ErrCodeCosmosRemoteStatusCanceled:           newErrorSpec(errorClassCanceled, errorOutcomeUnknown, recoveryNone, codes.Canceled),
	ErrCodeCosmosRemoteStatusResourceExhausted:  newErrorSpec(errorClassResourceExhausted, errorOutcomeUnknown, recoveryBackoff, codes.ResourceExhausted),
	ErrCodeCosmosRemoteStatusUnauthenticated:    newErrorSpec(errorClassUnauthenticated, errorOutcomeUnknown, recoveryNone, codes.Unauthenticated),

	ErrCodeElementNotFound:                                newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeElementLoaded:                                  newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeMessageHandlerNotFound:                         newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeMessageDecoderNotFound:                         newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeMessageReplyTypeInvalid:                        newErrorSpec(errorClassInvalidRequest, errorOutcomeUnknown, recoveryNone, codes.InvalidArgument),
	ErrCodeMessageArgTypeInvalid:                          newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeAtomMessageAtomType:                            newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeElementNotImplemented:                          newErrorSpec(errorClassUnsupported, errorOutcomeNotExecuted, recoveryNone, codes.Unimplemented),
	ErrCodeAtomNotImplemented:                             newErrorSpec(errorClassUnsupported, errorOutcomeNotExecuted, recoveryNone, codes.Unimplemented),
	ErrCodeMessageFromIDInvalid:                           newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeMessageToIDInvalid:                             newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeAtomKillElementNoImplement:                     newErrorSpec(errorClassUnsupported, errorOutcomeNotExecuted, recoveryNone, codes.Unimplemented),
	ErrCodeAtomKillElementNotImplementAutoDataPersistence: newErrorSpec(errorClassUnsupported, errorOutcomeNotExecuted, recoveryNone, codes.Unimplemented),
	ErrCodeAtomDataNotFound:                               newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeAtomNotExists:                                  newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryRebind, codes.NotFound),
	ErrCodeAtomAlreadyExists:                              newErrorSpec(errorClassAlreadyExists, errorOutcomeNotExecuted, recoveryNone, codes.AlreadyExists),
	ErrCodeAtomIsStopping:                                 newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeAtomInstanceMismatch:                           newErrorSpec(errorClassConflict, errorOutcomeNotExecuted, recoveryRebind, codes.Aborted),
	ErrCodeIDFirstSyncCallDeadlock:                        newErrorSpec(errorClassConflict, errorOutcomeNotExecuted, recoveryNone, codes.Aborted),
	ErrCodeAtomSpawning:                                   newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryBackoff, codes.Unavailable),
	ErrCodeAtomNotRunning:                                 newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),

	ErrCodeAtomosTaskInvalidFn:           newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeAtomosTaskNotExists:           newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeAtomosTaskAddCrontabFailed:    newErrorSpec(errorClassInternal, errorOutcomeNotExecuted, recoveryNone, codes.Internal),
	ErrCodeAtomosTaskRemoveCrontabFailed: newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeAtomosTaskNotCancellable:      newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryNone, codes.FailedPrecondition),
	ErrCodeAtomosPushTimeoutHandling:     newErrorSpec(errorClassTimeout, errorOutcomeUnknown, recoveryBackoff, codes.DeadlineExceeded),
	ErrCodeAtomosPushTimeoutReject:       newErrorSpec(errorClassTimeout, errorOutcomeNotExecuted, recoveryBackoff, codes.DeadlineExceeded),
	ErrCodeAsyncCallbackCancelledByHalt:  newErrorSpec(errorClassCanceled, errorOutcomeNotExecuted, recoveryNone, codes.Canceled),
	ErrCodeAsyncRetryQueueFull:           newErrorSpec(errorClassResourceExhausted, errorOutcomeNotExecuted, recoveryBackoff, codes.ResourceExhausted),
	ErrCodeAsyncDuplicateMessage:         newErrorSpec(errorClassAlreadyExists, errorOutcomeNotExecuted, recoveryNone, codes.AlreadyExists),
	ErrCodeAsyncRetryUnavailable:         newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryNone, codes.Unavailable),

	ErrCodeCosmosNodeDraining:      newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryRebind, codes.Unavailable),
	ErrCodeOwnershipLost:           newErrorSpec(errorClassConflict, errorOutcomeUnknown, recoveryManual, codes.Aborted),
	ErrCodeStaleFence:              newErrorSpec(errorClassConflict, errorOutcomeNotExecuted, recoveryManual, codes.Aborted),
	ErrCodeOperationRunning:        newErrorSpec(errorClassFailedPrecondition, errorOutcomeUnknown, recoveryQueryOperation, codes.FailedPrecondition),
	ErrCodeOperationNotFound:       newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeOperationConflict:       newErrorSpec(errorClassAlreadyExists, errorOutcomeNotExecuted, recoveryNone, codes.AlreadyExists),
	ErrCodeOperationContended:      newErrorSpec(errorClassConflict, errorOutcomeUnknown, recoveryBackoff, codes.Aborted),
	ErrCodeTakeoverRequired:        newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryManual, codes.FailedPrecondition),
	ErrCodeOperationOutcomeUnknown: newErrorSpec(errorClassUnavailable, errorOutcomeUnknown, recoveryQueryOperation, codes.Unknown),
	ErrCodeFencingRequired:         newErrorSpec(errorClassFailedPrecondition, errorOutcomeNotExecuted, recoveryManual, codes.FailedPrecondition),
	ErrCodeOwnershipNotReady:       newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryBackoff, codes.Unavailable),

	ErrCodeStorageUnavailable:         newErrorSpec(errorClassUnavailable, errorOutcomeUnknown, recoveryBackoff, codes.Unavailable),
	ErrCodeStorageKeyInvalid:          newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeStorageDataCorrupted:       newErrorSpec(errorClassDataLoss, errorOutcomeUnknown, recoveryManual, codes.DataLoss),
	ErrCodeStorageSerializationFailed: newErrorSpec(errorClassInternal, errorOutcomeNotExecuted, recoveryNone, codes.Internal),
	ErrCodeStorageConflict:            newErrorSpec(errorClassConflict, errorOutcomeUnknown, recoveryBackoff, codes.Aborted),

	ErrCodeAppEnvGetExecutableFailed:            newErrorSpec(errorClassInternal, errorOutcomeNotExecuted, recoveryNone, codes.Internal),
	ErrCodeAppEnvLaunchedFailed:                 newErrorSpec(errorClassUnavailable, errorOutcomeUnknown, recoveryNone, codes.Unavailable),
	ErrCodeAppEnvRunPathInvalid:                 newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeAppEnvRunPathPIDFileInvalid:          newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeAppEnvRunPathPIDIsRunning:            newErrorSpec(errorClassAlreadyExists, errorOutcomeNotExecuted, recoveryNone, codes.AlreadyExists),
	ErrCodeAppEnvRunPathWritePIDFileFailed:      newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeAppEnvRunPathRemovePIDFailed:         newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeAppEnvLoggingFileOpenFailed:          newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryNone, codes.Unavailable),
	ErrCodeAppEnvLoggingPathInvalid:             newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeAppEnvLoggingFileWriteFailed:         newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeAppEnvLoggingFileCloseFailed:         newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeUtilOSStatError:                      newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryNone, codes.Unavailable),
	ErrCodeUtilReadDirectoryFailed:              newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryNone, codes.Unavailable),
	ErrCodeUtilNotSupportedOS:                   newErrorSpec(errorClassUnsupported, errorOutcomeNotExecuted, recoveryNone, codes.Unimplemented),
	ErrCodeUtilPathShouldBeDirectory:            newErrorSpec(errorClassInvalidRequest, errorOutcomeNotExecuted, recoveryNone, codes.InvalidArgument),
	ErrCodeUtilDirectoryNotExist:                newErrorSpec(errorClassNotFound, errorOutcomeNotExecuted, recoveryNone, codes.NotFound),
	ErrCodeUtilGetUserGroupIDsFailed:            newErrorSpec(errorClassInternal, errorOutcomeNotExecuted, recoveryNone, codes.Internal),
	ErrCodeUtilUsersGroupsHaveNotOwnedDirectory: newErrorSpec(errorClassPermissionDenied, errorOutcomeNotExecuted, recoveryNone, codes.PermissionDenied),
	ErrCodeUtilFileModePermNotMatch:             newErrorSpec(errorClassPermissionDenied, errorOutcomeNotExecuted, recoveryNone, codes.PermissionDenied),
	ErrCodeUtilFileMakeDirectoryFailed:          newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeUtilFileChangeOwnerAndModeFailed:     newErrorSpec(errorClassPermissionDenied, errorOutcomeUnknown, recoveryNone, codes.PermissionDenied),
	ErrCodeUtilFileConfirmOwnerAndModeFailed:    newErrorSpec(errorClassPermissionDenied, errorOutcomeNotExecuted, recoveryNone, codes.PermissionDenied),
	ErrCodeUtilCreateFileFailed:                 newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeUtilFileEnsureDirectoryFailed:        newErrorSpec(errorClassInternal, errorOutcomeUnknown, recoveryNone, codes.Internal),
	ErrCodeUtilFileExistCheckFailed:             newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryNone, codes.Unavailable),
	ErrCodeUtilFileGetDirectorySizeFailed:       newErrorSpec(errorClassUnavailable, errorOutcomeNotExecuted, recoveryNone, codes.Unavailable),
}

func errorSpecOf(code int64) (errorSpec, bool) {
	spec, ok := errorSpecs[code]
	return spec, ok
}

func errorRecoveryOf(code int64) recoveryKind {
	spec, ok := errorSpecOf(code)
	if !ok {
		return recoveryNone
	}
	return spec.recovery
}
