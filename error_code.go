package atomos

// Error codes are frozen wire values: they travel over RPC and persist in
// databases, so a code's numeric value must never change once assigned.
// Append-only: add new codes at the tail with an explicit unused value,
// and never reuse the value of a deleted code.
const (
	OK = 0

	ErrFrameworkInternalError = 1
	ErrFrameworkRecoverFromPanic = 2
	ErrFrameworkIncorrectUsage = 3

	// Cosmos Process

	ErrCosmosProcessHasNotInitialized = 4
	ErrCosmosProcessHasBeenStarted = 5
	ErrCosmosProcessOnStartupPanic = 6
	ErrCosmosProcessOnShutdownPanic = 7
	ErrCosmosProcessCannotStopPrepareState = 8
	ErrCosmosProcessCannotStopStartupState = 9
	ErrCosmosProcessCannotStopShutdownState = 10
	ErrCosmosProcessCannotStopOffState = 11
	ErrCosmosProcessInvalidState = 12

	// Cosmos Main

	ErrMainLoadCertFailed = 13
	ErrMainElementNotFound = 14
	ErrMainStartRunningPanic = 15
	ErrMainCannotKill = 16
	ErrMainCannotSendWormhole = 17
	ErrMainCannotMessage = 18
	ErrMainRunnableNotFound = 19
	ErrRunnableConfigNotFound = 20
	ErrRunnableInterfaceInvalid = 21
	ErrRunnableImplementInvalid = 22
	ErrRunnableScriptNotFound = 23

	// Cosmos Global

	// Cosmos Remote

	ErrCosmosRemoteElementNotFound = 24
	ErrCosmosRemoteListenFailed = 25
	ErrCosmosRemoteConnectFailed = 26
	ErrCosmosRemoteRequestInvalid = 27
	ErrCosmosRemoteResponseFailed = 28
	ErrCosmosRemoteResponseInvalid = 29
	ErrCosmosRemoteServerInvalidArgs = 30
	ErrCosmosRemoteServerInvalidFirstSyncCall = 31
	ErrCosmosRemoteInfoInvalid = 32
	ErrCosmosRemoteCannotMessage = 33
	ErrCosmosRemoteCannotSendWormhole = 34
	ErrCosmosRemoteCannotKill = 35
	ErrElementRemoteCannotKill = 36
	ErrElementRemoteCannotSendWormhole = 37

	// Config

	ErrCosmosConfigInvalid = 38
	ErrCosmosEtcdConnectFailed = 39
	ErrCosmosEtcdClusterTLSInvalid = 40
	ErrCosmosEtcdClusterVersionsCheckFailed = 41
	ErrCosmosEtcdClusterVersionLockFailed = 42
	ErrCosmosEtcdGRPCServerFailed = 43
	ErrCosmosEtcdKeepaliveFailed = 44
	ErrCosmosEtcdInvalidKey = 45
	ErrCosmosEtcdUpdateFailed = 46
	ErrCosmosEtcdGetFailed = 47
	ErrCosmosEtcdPutFailed = 48
	ErrCosmosEtcdDeleteFailed = 49
	ErrCosmosConfigCertInvalid = 50
	ErrCosmosIsClosed = 51

	// App Env

	ErrAppEnvGetExecutableFailed = 52
	ErrAppEnvLaunchedFailed = 53
	ErrAppEnvRunPathInvalid = 54
	ErrAppEnvRunPathPIDFileInvalid = 55
	ErrAppEnvRunPathPIDIsRunning = 56
	ErrAppEnvRunPathWritePIDFileFailed = 57
	ErrAppEnvRunPathRemovePIDFailed = 58
	ErrAppEnvLoggingFileOpenFailed = 59
	ErrAppEnvLoggingPathInvalid = 60
	ErrAppEnvLoggingFileWriteFailed = 61
	ErrAppEnvLoggingFileCloseFailed = 62

	// Global

	ErrCosmosGlobalNoEtcdFailed = 63
	ErrCosmosGlobalEtcdConnectFailed = 64

	// Unix Domain Socket

	ErrAppUnixDomainSocketFileInvalid = 65
	ErrAppUnixDomainSocketListenFailed = 66
	ErrAppUnixDomainSocketDialFailed = 67
	ErrAppUnixDomainSocketConnWriteFailed = 68

	// Logging

	ErrAppLoggingPathInvalid = 69
	ErrAppLoggingFileOpenFailed = 70

	// Atomos

	ErrAtomosInvalidArguments = 71
	ErrAtomosIsStopping = 72
	ErrAtomosIsNotRunning = 73
	ErrAtomosTaskInvalidFn = 74
	ErrAtomosTaskNotExists = 75
	ErrAtomosTaskAddCrontabFailed = 76
	ErrAtomosTaskRemoveCrontabFailed = 77
	ErrAtomosNotSupportWormhole = 78
	ErrAtomosPushTimeoutHandling = 79
	ErrAtomosPushTimeoutReject = 80

	ErrAtomosTaskCannotCancelCancelledTask = 81
	ErrAtomosTaskCannotCancelRunningTask = 82
	ErrAtomosTaskCannotCancelDoneTask = 83

	// idFirstSyncCall

	ErrIDFirstSyncCallDeadlock = 84

	// Element

	ErrElementLoaded = 85
	ErrElementMessageHandlerNotExists = 86
	ErrElementMessageDecoderNotExists = 87
	ErrElementMessageReplyType = 88
	ErrElementCannotKill = 89
	ErrElementNoFromID = 90
	ErrElementNotImplemented = 91
	ErrElementFromIDInvalid = 92
	ErrElementToIDInvalid = 93
	ErrElementMessageArgType = 94

	// Atom

	ErrAtomMessageHandlerNotExists = 95
	ErrAtomMessageDecoderNotExists = 96
	ErrAtomKillElementNoImplement = 97
	ErrAtomKillElementNotImplementAutoDataPersistence = 98
	ErrAtomFromIDInvalid = 99
	ErrAtomToIDInvalid = 100
	ErrAtomDataNotFound = 101
	ErrAtomNotExists = 102
	ErrAtomIsRunning = 103
	ErrAtomIsStopping = 104
	ErrAtomNoFromID = 105
	ErrAtomNotImplemented = 106
	ErrAtomMessageAtomType = 107
	ErrAtomMessageArgType = 108
	ErrAtomMessageReplyType = 109
	ErrAtomSpawningAnExistedAtom = 110
	ErrAtomInstanceMismatch = 111

	// Util File

	ErrUtilOSStatError = 112
	ErrUtilReadDirectoryFailed = 113
	ErrUtilNotSupportedOS = 114
	ErrUtilPathShouldBeDirectory = 115
	ErrUtilDirectoryNotExist = 116
	ErrUtilGetUserGroupIDsFailed = 117
	ErrUtilUsersGroupsHaveNotOwnedDirectory = 118
	ErrUtilFileModePermNotMatch = 119
	ErrUtilFileMakeDirectoryFailed = 120
	ErrUtilFileChangeOwnerAndModeFailed = 121
	ErrUtilFileConfirmOwnerAndModeFailed = 122
	ErrUtilCreateFileFailed = 123
	ErrUtilFileEnsureDirectoryFailed = 124
	ErrUtilFileFileExistFailed = 125
	ErrUtilFileGetDirectorySizeFailed = 126
	ErrUtilStringHashSHA256Failed = 127

	// Cluster / Drain
	//
	// Returned when a node in Draining state rejects a new atom spawn. Routing
	// should steer callers away from draining nodes, but stale routes or pinned
	// connections can still deliver a spawn — the node must refuse it locally so
	// the caller can re-resolve and land on a Started node.
	ErrCosmosNodeDraining = 128

	// 异步回调因 atom 停机被清账终结(回调随实例消亡前以错误跑完,业务可据此降级/补偿)
	ErrAsyncCallbackCancelledByHalt = 129

	// ErrFrameworkInvalidID marks passing a user-implemented ID (one that does
	// not carry the framework-internal idInternal extension) into a framework
	// kernel path. It is a programming error, not a runtime condition.
	ErrFrameworkInvalidID = 130
)
