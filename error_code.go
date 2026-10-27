package atomos

// Error codes are grouped by ownership domain. Ranges allocate codes only;
// callers must use the error specification, never numeric comparisons, to
// determine classification, execution outcome, or recovery behavior.
//
// 9000-9999 is reserved for the framework, 10000-999999 for official
// extensions, and values >= 1000000 belong to applications.
const (
	OK             int64 = 0
	ErrCodeUnknown int64 = -1

	// Common.

	ErrCodeFrameworkInternalError    int64 = 1001
	ErrCodeFrameworkRecoverFromPanic int64 = 1002
	ErrCodeFrameworkIncorrectUsage   int64 = 1003
	ErrCodeFrameworkInvalidID        int64 = 1004
	ErrCodeOperationUnsupported      int64 = 1005
	ErrCodeAtomosInvalidArguments    int64 = 1006
	ErrCodeAtomosIsStopping          int64 = 1007
	ErrCodeAtomosIsNotRunning        int64 = 1008

	// Cosmos process and configuration.

	ErrCodeCosmosProcessHasNotInitialized       int64 = 2001
	ErrCodeCosmosProcessHasBeenStarted          int64 = 2002
	ErrCodeCosmosProcessOnStartupPanic          int64 = 2003
	ErrCodeCosmosProcessOnShutdownPanic         int64 = 2004
	ErrCodeCosmosProcessCannotStopPrepareState  int64 = 2005
	ErrCodeCosmosProcessCannotStopStartupState  int64 = 2006
	ErrCodeCosmosProcessCannotStopShutdownState int64 = 2007
	ErrCodeCosmosProcessCannotStopOffState      int64 = 2008
	ErrCodeCosmosProcessInvalidState            int64 = 2009
	ErrCodeCosmosProcessInvalidTransition       int64 = 2010
	ErrCodeMainStartRunningPanic                int64 = 2011
	ErrCodeMainCannotMessage                    int64 = 2012
	ErrCodeMainRunnableNotFound                 int64 = 2013
	ErrCodeRunnableConfigNotFound               int64 = 2014
	ErrCodeRunnableScriptNotFound               int64 = 2015
	ErrCodeCosmosConfigInvalid                  int64 = 2020
	ErrCodeCosmosConfigReadFailed               int64 = 2021
	ErrCodeCosmosEtcdClusterTLSInvalid          int64 = 2022
	ErrCodeCosmosEtcdClusterVersionsCheckFailed int64 = 2023

	// Remote transport and protocol.

	ErrCodeCosmosRemoteNodeNotFound             int64 = 3001
	ErrCodeCosmosRemoteListenFailed             int64 = 3002
	ErrCodeCosmosRemoteAddressNotFound          int64 = 3003
	ErrCodeCosmosRemoteConnectFailed            int64 = 3004
	ErrCodeCosmosRemoteRequestInvalid           int64 = 3005
	ErrCodeCosmosRemoteResponseFailed           int64 = 3006
	ErrCodeCosmosRemoteResponseInvalid          int64 = 3007
	ErrCodeCosmosRemoteStatusNotFound           int64 = 3008
	ErrCodeCosmosRemoteStatusFailedPrecondition int64 = 3009
	ErrCodeCosmosRemoteStatusInvalidArgument    int64 = 3010
	ErrCodeCosmosRemoteStatusDeadlineExceeded   int64 = 3011
	ErrCodeCosmosRemoteStatusUnavailable        int64 = 3012
	ErrCodeCosmosRemoteStatusInternal           int64 = 3013
	ErrCodeCosmosRemoteStatusPermissionDenied   int64 = 3014
	ErrCodeCosmosRemoteStatusCanceled           int64 = 3015
	ErrCodeCosmosRemoteStatusResourceExhausted  int64 = 3016
	ErrCodeCosmosRemoteStatusUnauthenticated    int64 = 3017

	// Element, atom, and messaging.

	ErrCodeElementNotFound                                int64 = 4001
	ErrCodeElementLoaded                                  int64 = 4002
	ErrCodeMessageHandlerNotFound                         int64 = 4003
	ErrCodeMessageDecoderNotFound                         int64 = 4004
	ErrCodeMessageReplyTypeInvalid                        int64 = 4005
	ErrCodeMessageArgTypeInvalid                          int64 = 4006
	ErrCodeAtomMessageAtomType                            int64 = 4007
	ErrCodeElementNotImplemented                          int64 = 4008
	ErrCodeAtomNotImplemented                             int64 = 4009
	ErrCodeMessageFromIDInvalid                           int64 = 4010
	ErrCodeMessageToIDInvalid                             int64 = 4011
	ErrCodeAtomKillElementNoImplement                     int64 = 4012
	ErrCodeAtomKillElementNotImplementAutoDataPersistence int64 = 4013
	ErrCodeAtomDataNotFound                               int64 = 4014
	ErrCodeAtomNotExists                                  int64 = 4015
	ErrCodeAtomAlreadyExists                              int64 = 4016
	ErrCodeAtomIsStopping                                 int64 = 4017
	ErrCodeAtomInstanceMismatch                           int64 = 4018
	ErrCodeIDFirstSyncCallDeadlock                        int64 = 4019
	ErrCodeAtomSpawning                                   int64 = 4020
	ErrCodeAtomNotRunning                                 int64 = 4021

	// Tasks, mailbox, and asynchronous delivery.

	ErrCodeAtomosTaskInvalidFn           int64 = 5001
	ErrCodeAtomosTaskNotExists           int64 = 5002
	ErrCodeAtomosTaskAddCrontabFailed    int64 = 5003
	ErrCodeAtomosTaskRemoveCrontabFailed int64 = 5004
	ErrCodeAtomosTaskNotCancellable      int64 = 5005
	ErrCodeAtomosPushTimeoutHandling     int64 = 5006
	ErrCodeAtomosPushTimeoutReject       int64 = 5007
	ErrCodeAsyncCallbackCancelledByHalt  int64 = 5008
	ErrCodeAsyncRetryQueueFull           int64 = 5009
	ErrCodeAsyncDuplicateMessage         int64 = 5010
	ErrCodeAsyncRetryUnavailable         int64 = 5011

	// Cluster and distributed lifecycle.

	ErrCodeCosmosNodeDraining      int64 = 6001
	ErrCodeOwnershipLost           int64 = 6002
	ErrCodeStaleFence              int64 = 6003
	ErrCodeOperationRunning        int64 = 6004
	ErrCodeOperationNotFound       int64 = 6005
	ErrCodeOperationConflict       int64 = 6006
	ErrCodeOperationContended      int64 = 6007
	ErrCodeTakeoverRequired        int64 = 6008
	ErrCodeOperationOutcomeUnknown int64 = 6009
	ErrCodeFencingRequired         int64 = 6010
	ErrCodeOwnershipNotReady       int64 = 6011

	// Storage and persistence.

	ErrCodeStorageUnavailable         int64 = 7001
	ErrCodeStorageKeyInvalid          int64 = 7002
	ErrCodeStorageDataCorrupted       int64 = 7003
	ErrCodeStorageSerializationFailed int64 = 7004
	ErrCodeStorageConflict            int64 = 7005

	// Host and administration.

	ErrCodeAppEnvGetExecutableFailed            int64 = 8001
	ErrCodeAppEnvLaunchedFailed                 int64 = 8002
	ErrCodeAppEnvRunPathInvalid                 int64 = 8003
	ErrCodeAppEnvRunPathPIDFileInvalid          int64 = 8004
	ErrCodeAppEnvRunPathPIDIsRunning            int64 = 8005
	ErrCodeAppEnvRunPathWritePIDFileFailed      int64 = 8006
	ErrCodeAppEnvRunPathRemovePIDFailed         int64 = 8007
	ErrCodeAppEnvLoggingFileOpenFailed          int64 = 8008
	ErrCodeAppEnvLoggingPathInvalid             int64 = 8009
	ErrCodeAppEnvLoggingFileWriteFailed         int64 = 8010
	ErrCodeAppEnvLoggingFileCloseFailed         int64 = 8011
	ErrCodeUtilOSStatError                      int64 = 8012
	ErrCodeUtilReadDirectoryFailed              int64 = 8013
	ErrCodeUtilNotSupportedOS                   int64 = 8014
	ErrCodeUtilPathShouldBeDirectory            int64 = 8015
	ErrCodeUtilDirectoryNotExist                int64 = 8016
	ErrCodeUtilGetUserGroupIDsFailed            int64 = 8017
	ErrCodeUtilUsersGroupsHaveNotOwnedDirectory int64 = 8018
	ErrCodeUtilFileModePermNotMatch             int64 = 8019
	ErrCodeUtilFileMakeDirectoryFailed          int64 = 8020
	ErrCodeUtilFileChangeOwnerAndModeFailed     int64 = 8021
	ErrCodeUtilFileConfirmOwnerAndModeFailed    int64 = 8022
	ErrCodeUtilCreateFileFailed                 int64 = 8023
	ErrCodeUtilFileEnsureDirectoryFailed        int64 = 8024
	ErrCodeUtilFileExistCheckFailed             int64 = 8025
	ErrCodeUtilFileGetDirectorySizeFailed       int64 = 8026
)
