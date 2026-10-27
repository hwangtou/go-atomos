package atomos

// Error codes are frozen wire values: they travel over RPC and persist in
// databases, so a code's numeric value must never change once assigned.
// Append-only: add new codes at the tail with an explicit unused value,
// and never reuse the value of a deleted code.
const (
	OK = 0

	ErrCodeFrameworkInternalError    = 1
	ErrCodeFrameworkRecoverFromPanic = 2
	ErrCodeFrameworkIncorrectUsage   = 3

	// Cosmos Process

	ErrCodeCosmosProcessHasNotInitialized       = 4
	ErrCodeCosmosProcessHasBeenStarted          = 5
	ErrCodeCosmosProcessOnStartupPanic          = 6
	ErrCodeCosmosProcessOnShutdownPanic         = 7
	ErrCodeCosmosProcessCannotStopPrepareState  = 8
	ErrCodeCosmosProcessCannotStopStartupState  = 9
	ErrCodeCosmosProcessCannotStopShutdownState = 10
	ErrCodeCosmosProcessCannotStopOffState      = 11
	ErrCodeCosmosProcessInvalidState            = 12

	// Cosmos Main

	ErrCodeMainLoadCertFailed       = 13
	ErrCodeMainElementNotFound      = 14
	ErrCodeMainStartRunningPanic    = 15
	ErrCodeMainCannotKill           = 16
	ErrCodeMainCannotSendWormhole   = 17
	ErrCodeMainCannotMessage        = 18
	ErrCodeMainRunnableNotFound     = 19
	ErrCodeRunnableConfigNotFound   = 20
	ErrCodeRunnableInterfaceInvalid = 21
	ErrCodeRunnableImplementInvalid = 22
	ErrCodeRunnableScriptNotFound   = 23

	// Cosmos Global

	// Cosmos Remote

	ErrCodeCosmosRemoteElementNotFound            = 24
	ErrCodeCosmosRemoteListenFailed               = 25
	ErrCodeCosmosRemoteConnectFailed              = 26
	ErrCodeCosmosRemoteRequestInvalid             = 27
	ErrCodeCosmosRemoteResponseFailed             = 28
	ErrCodeCosmosRemoteResponseInvalid            = 29
	ErrCodeCosmosRemoteServerInvalidArgs          = 30
	ErrCodeCosmosRemoteServerInvalidFirstSyncCall = 31
	ErrCodeCosmosRemoteInfoInvalid                = 32
	ErrCodeCosmosRemoteCannotMessage              = 33
	ErrCodeCosmosRemoteCannotSendWormhole         = 34
	ErrCodeCosmosRemoteCannotKill                 = 35
	ErrCodeElementRemoteCannotKill                = 36
	ErrCodeElementRemoteCannotSendWormhole        = 37

	// Config

	ErrCodeCosmosConfigInvalid                  = 38
	ErrCodeCosmosEtcdConnectFailed              = 39
	ErrCodeCosmosEtcdClusterTLSInvalid          = 40
	ErrCodeCosmosEtcdClusterVersionsCheckFailed = 41
	ErrCodeCosmosEtcdClusterVersionLockFailed   = 42
	ErrCodeCosmosEtcdGRPCServerFailed           = 43
	ErrCodeCosmosEtcdKeepaliveFailed            = 44
	ErrCodeCosmosEtcdInvalidKey                 = 45
	ErrCodeCosmosEtcdUpdateFailed               = 46
	ErrCodeCosmosEtcdGetFailed                  = 47
	ErrCodeCosmosEtcdPutFailed                  = 48
	ErrCodeCosmosEtcdDeleteFailed               = 49
	ErrCodeCosmosConfigCertInvalid              = 50
	ErrCodeCosmosIsClosed                       = 51

	// App Env

	ErrCodeAppEnvGetExecutableFailed       = 52
	ErrCodeAppEnvLaunchedFailed            = 53
	ErrCodeAppEnvRunPathInvalid            = 54
	ErrCodeAppEnvRunPathPIDFileInvalid     = 55
	ErrCodeAppEnvRunPathPIDIsRunning       = 56
	ErrCodeAppEnvRunPathWritePIDFileFailed = 57
	ErrCodeAppEnvRunPathRemovePIDFailed    = 58
	ErrCodeAppEnvLoggingFileOpenFailed     = 59
	ErrCodeAppEnvLoggingPathInvalid        = 60
	ErrCodeAppEnvLoggingFileWriteFailed    = 61
	ErrCodeAppEnvLoggingFileCloseFailed    = 62

	// Global

	ErrCodeCosmosGlobalNoEtcdFailed      = 63
	ErrCodeCosmosGlobalEtcdConnectFailed = 64

	// Unix Domain Socket

	ErrCodeAppUnixDomainSocketFileInvalid     = 65
	ErrCodeAppUnixDomainSocketListenFailed    = 66
	ErrCodeAppUnixDomainSocketDialFailed      = 67
	ErrCodeAppUnixDomainSocketConnWriteFailed = 68

	// Logging

	ErrCodeAppLoggingPathInvalid    = 69
	ErrCodeAppLoggingFileOpenFailed = 70

	// Atomos

	ErrCodeAtomosInvalidArguments        = 71
	ErrCodeAtomosIsStopping              = 72
	ErrCodeAtomosIsNotRunning            = 73
	ErrCodeAtomosTaskInvalidFn           = 74
	ErrCodeAtomosTaskNotExists           = 75
	ErrCodeAtomosTaskAddCrontabFailed    = 76
	ErrCodeAtomosTaskRemoveCrontabFailed = 77
	ErrCodeAtomosNotSupportWormhole      = 78
	ErrCodeAtomosPushTimeoutHandling     = 79
	ErrCodeAtomosPushTimeoutReject       = 80

	ErrCodeAtomosTaskCannotCancelCancelledTask = 81
	ErrCodeAtomosTaskCannotCancelRunningTask   = 82
	ErrCodeAtomosTaskCannotCancelDoneTask      = 83

	// idFirstSyncCall

	ErrCodeIDFirstSyncCallDeadlock = 84

	// Element

	ErrCodeElementLoaded                  = 85
	ErrCodeElementMessageHandlerNotExists = 86
	ErrCodeElementMessageDecoderNotExists = 87
	ErrCodeElementMessageReplyType        = 88
	ErrCodeElementCannotKill              = 89
	ErrCodeElementNoFromID                = 90
	ErrCodeElementNotImplemented          = 91
	ErrCodeElementFromIDInvalid           = 92
	ErrCodeElementToIDInvalid             = 93
	ErrCodeElementMessageArgType          = 94

	// Atom

	ErrCodeAtomMessageHandlerNotExists                    = 95
	ErrCodeAtomMessageDecoderNotExists                    = 96
	ErrCodeAtomKillElementNoImplement                     = 97
	ErrCodeAtomKillElementNotImplementAutoDataPersistence = 98
	ErrCodeAtomFromIDInvalid                              = 99
	ErrCodeAtomToIDInvalid                                = 100
	ErrCodeAtomDataNotFound                               = 101
	ErrCodeAtomNotExists                                  = 102
	ErrCodeAtomIsRunning                                  = 103
	ErrCodeAtomIsStopping                                 = 104
	ErrCodeAtomNoFromID                                   = 105
	ErrCodeAtomNotImplemented                             = 106
	ErrCodeAtomMessageAtomType                            = 107
	ErrCodeAtomMessageArgType                             = 108
	ErrCodeAtomMessageReplyType                           = 109
	ErrCodeAtomSpawningAnExistedAtom                      = 110
	ErrCodeAtomInstanceMismatch                           = 111

	// Util File

	ErrCodeUtilOSStatError                      = 112
	ErrCodeUtilReadDirectoryFailed              = 113
	ErrCodeUtilNotSupportedOS                   = 114
	ErrCodeUtilPathShouldBeDirectory            = 115
	ErrCodeUtilDirectoryNotExist                = 116
	ErrCodeUtilGetUserGroupIDsFailed            = 117
	ErrCodeUtilUsersGroupsHaveNotOwnedDirectory = 118
	ErrCodeUtilFileModePermNotMatch             = 119
	ErrCodeUtilFileMakeDirectoryFailed          = 120
	ErrCodeUtilFileChangeOwnerAndModeFailed     = 121
	ErrCodeUtilFileConfirmOwnerAndModeFailed    = 122
	ErrCodeUtilCreateFileFailed                 = 123
	ErrCodeUtilFileEnsureDirectoryFailed        = 124
	ErrCodeUtilFileFileExistFailed              = 125
	ErrCodeUtilFileGetDirectorySizeFailed       = 126
	ErrCodeUtilStringHashSHA256Failed           = 127

	// Cluster / Drain
	//
	// Returned when a node in Draining state rejects a new atom spawn. Routing
	// should steer callers away from draining nodes, but stale routes or pinned
	// connections can still deliver a spawn — the node must refuse it locally so
	// the caller can re-resolve and land on a Started node.
	ErrCodeCosmosNodeDraining = 128

	// 异步回调因 atom 停机被清账终结(回调随实例消亡前以错误跑完,业务可据此降级/补偿)
	ErrCodeAsyncCallbackCancelledByHalt = 129

	// ErrFrameworkInvalidID marks passing a user-implemented ID (one that does
	// not carry the framework-internal idInternal extension) into a framework
	// kernel path. It is a programming error, not a runtime condition.
	ErrCodeFrameworkInvalidID = 130

	// Distributed atom lifecycle control.
	ErrCodeOwnershipLost         = 131
	ErrCodeStaleFence            = 132
	ErrCodeOperationStillRunning = 133
	ErrCodeOperationNotFound     = 134
	ErrCodeOperationConflict     = 135
	ErrCodeTakeoverRequired      = 136
	ErrCodeOperationUnknown      = 137
	ErrCodeFencingRequired       = 138
)
