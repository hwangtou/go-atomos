package atomos

import "testing"

var errorCodeSnapshot = map[string]int64{
	"OK":                                                    0,
	"ErrCodeFrameworkInternalError":                         1001,
	"ErrCodeFrameworkRecoverFromPanic":                      1002,
	"ErrCodeFrameworkIncorrectUsage":                        1003,
	"ErrCodeFrameworkInvalidID":                             1004,
	"ErrCodeOperationUnsupported":                           1005,
	"ErrCodeAtomosInvalidArguments":                         1006,
	"ErrCodeAtomosIsStopping":                               1007,
	"ErrCodeAtomosIsNotRunning":                             1008,
	"ErrCodeCosmosProcessHasNotInitialized":                 2001,
	"ErrCodeCosmosProcessHasBeenStarted":                    2002,
	"ErrCodeCosmosProcessOnStartupPanic":                    2003,
	"ErrCodeCosmosProcessOnShutdownPanic":                   2004,
	"ErrCodeCosmosProcessCannotStopPrepareState":            2005,
	"ErrCodeCosmosProcessCannotStopStartupState":            2006,
	"ErrCodeCosmosProcessCannotStopShutdownState":           2007,
	"ErrCodeCosmosProcessCannotStopOffState":                2008,
	"ErrCodeCosmosProcessInvalidState":                      2009,
	"ErrCodeCosmosProcessInvalidTransition":                 2010,
	"ErrCodeMainStartRunningPanic":                          2011,
	"ErrCodeMainCannotMessage":                              2012,
	"ErrCodeMainRunnableNotFound":                           2013,
	"ErrCodeRunnableConfigNotFound":                         2014,
	"ErrCodeRunnableScriptNotFound":                         2015,
	"ErrCodeCosmosConfigInvalid":                            2020,
	"ErrCodeCosmosConfigReadFailed":                         2021,
	"ErrCodeCosmosEtcdClusterTLSInvalid":                    2022,
	"ErrCodeCosmosEtcdClusterVersionsCheckFailed":           2023,
	"ErrCodeCosmosRemoteNodeNotFound":                       3001,
	"ErrCodeCosmosRemoteListenFailed":                       3002,
	"ErrCodeCosmosRemoteAddressNotFound":                    3003,
	"ErrCodeCosmosRemoteConnectFailed":                      3004,
	"ErrCodeCosmosRemoteRequestInvalid":                     3005,
	"ErrCodeCosmosRemoteResponseFailed":                     3006,
	"ErrCodeCosmosRemoteResponseInvalid":                    3007,
	"ErrCodeCosmosRemoteStatusNotFound":                     3008,
	"ErrCodeCosmosRemoteStatusFailedPrecondition":           3009,
	"ErrCodeCosmosRemoteStatusInvalidArgument":              3010,
	"ErrCodeCosmosRemoteStatusDeadlineExceeded":             3011,
	"ErrCodeCosmosRemoteStatusUnavailable":                  3012,
	"ErrCodeCosmosRemoteStatusInternal":                     3013,
	"ErrCodeCosmosRemoteStatusPermissionDenied":             3014,
	"ErrCodeCosmosRemoteStatusCanceled":                     3015,
	"ErrCodeCosmosRemoteStatusResourceExhausted":            3016,
	"ErrCodeCosmosRemoteStatusUnauthenticated":              3017,
	"ErrCodeElementNotFound":                                4001,
	"ErrCodeElementLoaded":                                  4002,
	"ErrCodeMessageHandlerNotFound":                         4003,
	"ErrCodeMessageDecoderNotFound":                         4004,
	"ErrCodeMessageReplyTypeInvalid":                        4005,
	"ErrCodeMessageArgTypeInvalid":                          4006,
	"ErrCodeAtomMessageAtomType":                            4007,
	"ErrCodeElementNotImplemented":                          4008,
	"ErrCodeAtomNotImplemented":                             4009,
	"ErrCodeMessageFromIDInvalid":                           4010,
	"ErrCodeMessageToIDInvalid":                             4011,
	"ErrCodeAtomKillElementNoImplement":                     4012,
	"ErrCodeAtomKillElementNotImplementAutoDataPersistence": 4013,
	"ErrCodeAtomDataNotFound":                               4014,
	"ErrCodeAtomNotExists":                                  4015,
	"ErrCodeAtomAlreadyExists":                              4016,
	"ErrCodeAtomIsStopping":                                 4017,
	"ErrCodeAtomInstanceMismatch":                           4018,
	"ErrCodeIDFirstSyncCallDeadlock":                        4019,
	"ErrCodeAtomSpawning":                                   4020,
	"ErrCodeAtomNotRunning":                                 4021,
	"ErrCodeAtomosTaskInvalidFn":                            5001,
	"ErrCodeAtomosTaskNotExists":                            5002,
	"ErrCodeAtomosTaskAddCrontabFailed":                     5003,
	"ErrCodeAtomosTaskRemoveCrontabFailed":                  5004,
	"ErrCodeAtomosTaskNotCancellable":                       5005,
	"ErrCodeAtomosPushTimeoutHandling":                      5006,
	"ErrCodeAtomosPushTimeoutReject":                        5007,
	"ErrCodeAsyncCallbackCancelledByHalt":                   5008,
	"ErrCodeAsyncRetryQueueFull":                            5009,
	"ErrCodeAsyncDuplicateMessage":                          5010,
	"ErrCodeAsyncRetryUnavailable":                          5011,
	"ErrCodeCosmosNodeDraining":                             6001,
	"ErrCodeOwnershipLost":                                  6002,
	"ErrCodeStaleFence":                                     6003,
	"ErrCodeOperationRunning":                               6004,
	"ErrCodeOperationNotFound":                              6005,
	"ErrCodeOperationConflict":                              6006,
	"ErrCodeOperationContended":                             6007,
	"ErrCodeTakeoverRequired":                               6008,
	"ErrCodeOperationOutcomeUnknown":                        6009,
	"ErrCodeFencingRequired":                                6010,
	"ErrCodeOwnershipNotReady":                              6011,
	"ErrCodeStorageUnavailable":                             7001,
	"ErrCodeStorageKeyInvalid":                              7002,
	"ErrCodeStorageDataCorrupted":                           7003,
	"ErrCodeStorageSerializationFailed":                     7004,
	"ErrCodeStorageConflict":                                7005,
	"ErrCodeAppEnvGetExecutableFailed":                      8001,
	"ErrCodeAppEnvLaunchedFailed":                           8002,
	"ErrCodeAppEnvRunPathInvalid":                           8003,
	"ErrCodeAppEnvRunPathPIDFileInvalid":                    8004,
	"ErrCodeAppEnvRunPathPIDIsRunning":                      8005,
	"ErrCodeAppEnvRunPathWritePIDFileFailed":                8006,
	"ErrCodeAppEnvRunPathRemovePIDFailed":                   8007,
	"ErrCodeAppEnvLoggingFileOpenFailed":                    8008,
	"ErrCodeAppEnvLoggingPathInvalid":                       8009,
	"ErrCodeAppEnvLoggingFileWriteFailed":                   8010,
	"ErrCodeAppEnvLoggingFileCloseFailed":                   8011,
	"ErrCodeUtilOSStatError":                                8012,
	"ErrCodeUtilReadDirectoryFailed":                        8013,
	"ErrCodeUtilNotSupportedOS":                             8014,
	"ErrCodeUtilPathShouldBeDirectory":                      8015,
	"ErrCodeUtilDirectoryNotExist":                          8016,
	"ErrCodeUtilGetUserGroupIDsFailed":                      8017,
	"ErrCodeUtilUsersGroupsHaveNotOwnedDirectory":           8018,
	"ErrCodeUtilFileModePermNotMatch":                       8019,
	"ErrCodeUtilFileMakeDirectoryFailed":                    8020,
	"ErrCodeUtilFileChangeOwnerAndModeFailed":               8021,
	"ErrCodeUtilFileConfirmOwnerAndModeFailed":              8022,
	"ErrCodeUtilCreateFileFailed":                           8023,
	"ErrCodeUtilFileEnsureDirectoryFailed":                  8024,
	"ErrCodeUtilFileExistCheckFailed":                       8025,
	"ErrCodeUtilFileGetDirectorySizeFailed":                 8026,
}

var errorCodeValues = map[string]int64{
	"OK":                                                    OK,
	"ErrCodeFrameworkInternalError":                         ErrCodeFrameworkInternalError,
	"ErrCodeFrameworkRecoverFromPanic":                      ErrCodeFrameworkRecoverFromPanic,
	"ErrCodeFrameworkIncorrectUsage":                        ErrCodeFrameworkIncorrectUsage,
	"ErrCodeFrameworkInvalidID":                             ErrCodeFrameworkInvalidID,
	"ErrCodeOperationUnsupported":                           ErrCodeOperationUnsupported,
	"ErrCodeAtomosInvalidArguments":                         ErrCodeAtomosInvalidArguments,
	"ErrCodeAtomosIsStopping":                               ErrCodeAtomosIsStopping,
	"ErrCodeAtomosIsNotRunning":                             ErrCodeAtomosIsNotRunning,
	"ErrCodeCosmosProcessHasNotInitialized":                 ErrCodeCosmosProcessHasNotInitialized,
	"ErrCodeCosmosProcessHasBeenStarted":                    ErrCodeCosmosProcessHasBeenStarted,
	"ErrCodeCosmosProcessOnStartupPanic":                    ErrCodeCosmosProcessOnStartupPanic,
	"ErrCodeCosmosProcessOnShutdownPanic":                   ErrCodeCosmosProcessOnShutdownPanic,
	"ErrCodeCosmosProcessCannotStopPrepareState":            ErrCodeCosmosProcessCannotStopPrepareState,
	"ErrCodeCosmosProcessCannotStopStartupState":            ErrCodeCosmosProcessCannotStopStartupState,
	"ErrCodeCosmosProcessCannotStopShutdownState":           ErrCodeCosmosProcessCannotStopShutdownState,
	"ErrCodeCosmosProcessCannotStopOffState":                ErrCodeCosmosProcessCannotStopOffState,
	"ErrCodeCosmosProcessInvalidState":                      ErrCodeCosmosProcessInvalidState,
	"ErrCodeCosmosProcessInvalidTransition":                 ErrCodeCosmosProcessInvalidTransition,
	"ErrCodeMainStartRunningPanic":                          ErrCodeMainStartRunningPanic,
	"ErrCodeMainCannotMessage":                              ErrCodeMainCannotMessage,
	"ErrCodeMainRunnableNotFound":                           ErrCodeMainRunnableNotFound,
	"ErrCodeRunnableConfigNotFound":                         ErrCodeRunnableConfigNotFound,
	"ErrCodeRunnableScriptNotFound":                         ErrCodeRunnableScriptNotFound,
	"ErrCodeCosmosConfigInvalid":                            ErrCodeCosmosConfigInvalid,
	"ErrCodeCosmosConfigReadFailed":                         ErrCodeCosmosConfigReadFailed,
	"ErrCodeCosmosEtcdClusterTLSInvalid":                    ErrCodeCosmosEtcdClusterTLSInvalid,
	"ErrCodeCosmosEtcdClusterVersionsCheckFailed":           ErrCodeCosmosEtcdClusterVersionsCheckFailed,
	"ErrCodeCosmosRemoteNodeNotFound":                       ErrCodeCosmosRemoteNodeNotFound,
	"ErrCodeCosmosRemoteListenFailed":                       ErrCodeCosmosRemoteListenFailed,
	"ErrCodeCosmosRemoteAddressNotFound":                    ErrCodeCosmosRemoteAddressNotFound,
	"ErrCodeCosmosRemoteConnectFailed":                      ErrCodeCosmosRemoteConnectFailed,
	"ErrCodeCosmosRemoteRequestInvalid":                     ErrCodeCosmosRemoteRequestInvalid,
	"ErrCodeCosmosRemoteResponseFailed":                     ErrCodeCosmosRemoteResponseFailed,
	"ErrCodeCosmosRemoteResponseInvalid":                    ErrCodeCosmosRemoteResponseInvalid,
	"ErrCodeCosmosRemoteStatusNotFound":                     ErrCodeCosmosRemoteStatusNotFound,
	"ErrCodeCosmosRemoteStatusFailedPrecondition":           ErrCodeCosmosRemoteStatusFailedPrecondition,
	"ErrCodeCosmosRemoteStatusInvalidArgument":              ErrCodeCosmosRemoteStatusInvalidArgument,
	"ErrCodeCosmosRemoteStatusDeadlineExceeded":             ErrCodeCosmosRemoteStatusDeadlineExceeded,
	"ErrCodeCosmosRemoteStatusUnavailable":                  ErrCodeCosmosRemoteStatusUnavailable,
	"ErrCodeCosmosRemoteStatusInternal":                     ErrCodeCosmosRemoteStatusInternal,
	"ErrCodeCosmosRemoteStatusPermissionDenied":             ErrCodeCosmosRemoteStatusPermissionDenied,
	"ErrCodeCosmosRemoteStatusCanceled":                     ErrCodeCosmosRemoteStatusCanceled,
	"ErrCodeCosmosRemoteStatusResourceExhausted":            ErrCodeCosmosRemoteStatusResourceExhausted,
	"ErrCodeCosmosRemoteStatusUnauthenticated":              ErrCodeCosmosRemoteStatusUnauthenticated,
	"ErrCodeElementNotFound":                                ErrCodeElementNotFound,
	"ErrCodeElementLoaded":                                  ErrCodeElementLoaded,
	"ErrCodeMessageHandlerNotFound":                         ErrCodeMessageHandlerNotFound,
	"ErrCodeMessageDecoderNotFound":                         ErrCodeMessageDecoderNotFound,
	"ErrCodeMessageReplyTypeInvalid":                        ErrCodeMessageReplyTypeInvalid,
	"ErrCodeMessageArgTypeInvalid":                          ErrCodeMessageArgTypeInvalid,
	"ErrCodeAtomMessageAtomType":                            ErrCodeAtomMessageAtomType,
	"ErrCodeElementNotImplemented":                          ErrCodeElementNotImplemented,
	"ErrCodeAtomNotImplemented":                             ErrCodeAtomNotImplemented,
	"ErrCodeMessageFromIDInvalid":                           ErrCodeMessageFromIDInvalid,
	"ErrCodeMessageToIDInvalid":                             ErrCodeMessageToIDInvalid,
	"ErrCodeAtomKillElementNoImplement":                     ErrCodeAtomKillElementNoImplement,
	"ErrCodeAtomKillElementNotImplementAutoDataPersistence": ErrCodeAtomKillElementNotImplementAutoDataPersistence,
	"ErrCodeAtomDataNotFound":                               ErrCodeAtomDataNotFound,
	"ErrCodeAtomNotExists":                                  ErrCodeAtomNotExists,
	"ErrCodeAtomAlreadyExists":                              ErrCodeAtomAlreadyExists,
	"ErrCodeAtomIsStopping":                                 ErrCodeAtomIsStopping,
	"ErrCodeAtomInstanceMismatch":                           ErrCodeAtomInstanceMismatch,
	"ErrCodeIDFirstSyncCallDeadlock":                        ErrCodeIDFirstSyncCallDeadlock,
	"ErrCodeAtomSpawning":                                   ErrCodeAtomSpawning,
	"ErrCodeAtomNotRunning":                                 ErrCodeAtomNotRunning,
	"ErrCodeAtomosTaskInvalidFn":                            ErrCodeAtomosTaskInvalidFn,
	"ErrCodeAtomosTaskNotExists":                            ErrCodeAtomosTaskNotExists,
	"ErrCodeAtomosTaskAddCrontabFailed":                     ErrCodeAtomosTaskAddCrontabFailed,
	"ErrCodeAtomosTaskRemoveCrontabFailed":                  ErrCodeAtomosTaskRemoveCrontabFailed,
	"ErrCodeAtomosTaskNotCancellable":                       ErrCodeAtomosTaskNotCancellable,
	"ErrCodeAtomosPushTimeoutHandling":                      ErrCodeAtomosPushTimeoutHandling,
	"ErrCodeAtomosPushTimeoutReject":                        ErrCodeAtomosPushTimeoutReject,
	"ErrCodeAsyncCallbackCancelledByHalt":                   ErrCodeAsyncCallbackCancelledByHalt,
	"ErrCodeAsyncRetryQueueFull":                            ErrCodeAsyncRetryQueueFull,
	"ErrCodeAsyncDuplicateMessage":                          ErrCodeAsyncDuplicateMessage,
	"ErrCodeAsyncRetryUnavailable":                          ErrCodeAsyncRetryUnavailable,
	"ErrCodeCosmosNodeDraining":                             ErrCodeCosmosNodeDraining,
	"ErrCodeOwnershipLost":                                  ErrCodeOwnershipLost,
	"ErrCodeStaleFence":                                     ErrCodeStaleFence,
	"ErrCodeOperationRunning":                               ErrCodeOperationRunning,
	"ErrCodeOperationNotFound":                              ErrCodeOperationNotFound,
	"ErrCodeOperationConflict":                              ErrCodeOperationConflict,
	"ErrCodeOperationContended":                             ErrCodeOperationContended,
	"ErrCodeTakeoverRequired":                               ErrCodeTakeoverRequired,
	"ErrCodeOperationOutcomeUnknown":                        ErrCodeOperationOutcomeUnknown,
	"ErrCodeFencingRequired":                                ErrCodeFencingRequired,
	"ErrCodeOwnershipNotReady":                              ErrCodeOwnershipNotReady,
	"ErrCodeStorageUnavailable":                             ErrCodeStorageUnavailable,
	"ErrCodeStorageKeyInvalid":                              ErrCodeStorageKeyInvalid,
	"ErrCodeStorageDataCorrupted":                           ErrCodeStorageDataCorrupted,
	"ErrCodeStorageSerializationFailed":                     ErrCodeStorageSerializationFailed,
	"ErrCodeStorageConflict":                                ErrCodeStorageConflict,
	"ErrCodeAppEnvGetExecutableFailed":                      ErrCodeAppEnvGetExecutableFailed,
	"ErrCodeAppEnvLaunchedFailed":                           ErrCodeAppEnvLaunchedFailed,
	"ErrCodeAppEnvRunPathInvalid":                           ErrCodeAppEnvRunPathInvalid,
	"ErrCodeAppEnvRunPathPIDFileInvalid":                    ErrCodeAppEnvRunPathPIDFileInvalid,
	"ErrCodeAppEnvRunPathPIDIsRunning":                      ErrCodeAppEnvRunPathPIDIsRunning,
	"ErrCodeAppEnvRunPathWritePIDFileFailed":                ErrCodeAppEnvRunPathWritePIDFileFailed,
	"ErrCodeAppEnvRunPathRemovePIDFailed":                   ErrCodeAppEnvRunPathRemovePIDFailed,
	"ErrCodeAppEnvLoggingFileOpenFailed":                    ErrCodeAppEnvLoggingFileOpenFailed,
	"ErrCodeAppEnvLoggingPathInvalid":                       ErrCodeAppEnvLoggingPathInvalid,
	"ErrCodeAppEnvLoggingFileWriteFailed":                   ErrCodeAppEnvLoggingFileWriteFailed,
	"ErrCodeAppEnvLoggingFileCloseFailed":                   ErrCodeAppEnvLoggingFileCloseFailed,
	"ErrCodeUtilOSStatError":                                ErrCodeUtilOSStatError,
	"ErrCodeUtilReadDirectoryFailed":                        ErrCodeUtilReadDirectoryFailed,
	"ErrCodeUtilNotSupportedOS":                             ErrCodeUtilNotSupportedOS,
	"ErrCodeUtilPathShouldBeDirectory":                      ErrCodeUtilPathShouldBeDirectory,
	"ErrCodeUtilDirectoryNotExist":                          ErrCodeUtilDirectoryNotExist,
	"ErrCodeUtilGetUserGroupIDsFailed":                      ErrCodeUtilGetUserGroupIDsFailed,
	"ErrCodeUtilUsersGroupsHaveNotOwnedDirectory":           ErrCodeUtilUsersGroupsHaveNotOwnedDirectory,
	"ErrCodeUtilFileModePermNotMatch":                       ErrCodeUtilFileModePermNotMatch,
	"ErrCodeUtilFileMakeDirectoryFailed":                    ErrCodeUtilFileMakeDirectoryFailed,
	"ErrCodeUtilFileChangeOwnerAndModeFailed":               ErrCodeUtilFileChangeOwnerAndModeFailed,
	"ErrCodeUtilFileConfirmOwnerAndModeFailed":              ErrCodeUtilFileConfirmOwnerAndModeFailed,
	"ErrCodeUtilCreateFileFailed":                           ErrCodeUtilCreateFileFailed,
	"ErrCodeUtilFileEnsureDirectoryFailed":                  ErrCodeUtilFileEnsureDirectoryFailed,
	"ErrCodeUtilFileExistCheckFailed":                       ErrCodeUtilFileExistCheckFailed,
	"ErrCodeUtilFileGetDirectorySizeFailed":                 ErrCodeUtilFileGetDirectorySizeFailed,
}

func TestErrorCodeValuesAndSpecs(t *testing.T) {
	if len(errorCodeValues) != len(errorCodeSnapshot) {
		t.Fatalf("snapshot has %d entries but %d codes exist", len(errorCodeSnapshot), len(errorCodeValues))
	}
	byValue := make(map[int64]string, len(errorCodeValues))
	for name, got := range errorCodeValues {
		want, ok := errorCodeSnapshot[name]
		if !ok {
			t.Errorf("%s is missing from the snapshot", name)
			continue
		}
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
		if name != "OK" && (got < 1000 || got >= 9000) {
			t.Errorf("%s = %d is outside framework domain ranges", name, got)
		}
		if prev, dup := byValue[got]; dup {
			t.Errorf("error code %d is shared by %s and %s", got, prev, name)
		}
		byValue[got] = name
		if name == "OK" {
			continue
		}
		spec, ok := errorSpecOf(got)
		if !ok {
			t.Errorf("%s has no error specification", name)
			continue
		}
		if spec.class == errorClassUnspecified {
			t.Errorf("%s has no error class", name)
		}
		if spec.outcome > errorOutcomeUnknown || spec.recovery > recoveryManual {
			t.Errorf("%s has invalid outcome/recovery metadata: %+v", name, spec)
		}
	}
	if len(errorSpecs) != len(errorCodeValues)-1 {
		t.Errorf("registry has %d specs, want %d", len(errorSpecs), len(errorCodeValues)-1)
	}
	for code := range errorSpecs {
		if _, ok := byValue[code]; !ok {
			t.Errorf("registry contains unassigned error code %d", code)
		}
	}
}

func TestErrorRecoverySemantics(t *testing.T) {
	for _, code := range []int64{ErrCodeAtomInstanceMismatch, ErrCodeAtomNotExists, ErrCodeCosmosNodeDraining} {
		if !shouldRebind(code) {
			t.Errorf("shouldRebind(%d) = false", code)
		}
	}
	for _, code := range []int64{ErrCodeOwnershipLost, ErrCodeCosmosRemoteNodeNotFound, ErrCodeAtomAlreadyExists} {
		if shouldRebind(code) {
			t.Errorf("shouldRebind(%d) = true", code)
		}
	}

	retryable := []int64{
		ErrCodeCosmosRemoteConnectFailed,
		ErrCodeCosmosRemoteStatusDeadlineExceeded,
		ErrCodeCosmosRemoteStatusUnavailable,
		ErrCodeAtomosPushTimeoutHandling,
		ErrCodeAtomosPushTimeoutReject,
		ErrCodeAtomSpawning,
		ErrCodeOwnershipNotReady,
	}
	for _, code := range retryable {
		if !isAsyncRetryableError(NewError(code, "test")) {
			t.Errorf("isAsyncRetryableError(%d) = false", code)
		}
	}
	notRetryable := []int64{ErrCodeOperationRunning, ErrCodeOperationContended, ErrCodeStorageUnavailable, ErrCodeAsyncRetryQueueFull, 1000001}
	for _, code := range notRetryable {
		if isAsyncRetryableError(NewError(code, "test")) {
			t.Errorf("isAsyncRetryableError(%d) = true", code)
		}
	}

	if spec, _ := errorSpecOf(ErrCodeAtomosPushTimeoutHandling); spec.outcome != errorOutcomeUnknown {
		t.Error("handling timeout must preserve an unknown execution outcome")
	}
	if spec, _ := errorSpecOf(ErrCodeAtomosPushTimeoutReject); spec.outcome != errorOutcomeNotExecuted {
		t.Error("rejected push must preserve a not-executed outcome")
	}
	if spec, _ := errorSpecOf(ErrCodeOwnershipLost); spec.recovery != recoveryManual {
		t.Error("ownership loss must remain fail-closed with manual recovery")
	}
}
