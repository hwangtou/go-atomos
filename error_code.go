package go_atomos

const (
	OK = iota

	// Framework 1-10

	ErrFrameworkInternalError    = 1
	ErrFrameworkRecoverFromPanic = 2
	ErrFrameworkIncorrectUsage   = 3

	// App Env 11-30

	ErrAppEnvGetExecutableFailed       = 11
	ErrAppEnvLaunchedFailed            = 12
	ErrAppEnvRunPathInvalid            = 13
	ErrAppEnvRunPathPIDFileInvalid     = 14
	ErrAppEnvRunPathPIDIsRunning       = 15
	ErrAppEnvRunPathWritePIDFileFailed = 16
	ErrAppEnvRunPathRemovePIDFailed    = 17
	ErrAppEnvLoggingPathInvalid        = 18
	ErrAppEnvLoggingFileOpenFailed     = 19

	// Cosmos Runnable 31-40

	ErrRunnableConfigInvalid  = 31
	ErrRunnableConfigNotFound = 32
	ErrRunnableScriptNotFound = 33

	// Etcd 41-60

	ErrCosmosEtcdConnectFailed              = 41
	ErrCosmosEtcdClusterTLSInvalid          = 42
	ErrCosmosEtcdClusterVersionsCheckFailed = 43
	ErrCosmosEtcdClusterVersionLockFailed   = 44
	ErrCosmosEtcdGRPCServerFailed           = 45
	ErrCosmosEtcdKeepaliveFailed            = 46
	ErrCosmosEtcdInvalidKey                 = 47
	ErrCosmosEtcdUpdateFailed               = 48
	ErrCosmosEtcdGetFailed                  = 49
	ErrCosmosEtcdPutFailed                  = 50
	ErrCosmosEtcdDeleteFailed               = 51

	// Cosmos Process Life Cycle 61-80

	ErrCosmosProcessHasNotInitialized       = 61
	ErrCosmosProcessHasBeenStarted          = 62
	ErrCosmosProcessOnStartupPanic          = 63
	ErrCosmosProcessOnShutdownPanic         = 64
	ErrCosmosProcessCannotStopPrepareState  = 65
	ErrCosmosProcessCannotStopStartupState  = 66
	ErrCosmosProcessCannotStopShutdownState = 67
	ErrCosmosProcessCannotStopOffState      = 68
	ErrCosmosProcessInvalidState            = 69

	// Atomos 81-100

	ErrAtomosIsStopping              = 81
	ErrAtomosIsNotRunning            = 82
	ErrAtomosTaskInvalidFn           = 83
	ErrAtomosTaskNotExists           = 84
	ErrAtomosTaskAddCrontabFailed    = 85
	ErrAtomosTaskRemoveCrontabFailed = 86
	ErrAtomosNotSupportWormhole      = 87
	ErrAtomosPushTimeoutHandling     = 88
	ErrAtomosPushTimeoutReject       = 89
	ErrAtomosIDCallLoop              = 90

	// Cosmos 101-110

	ErrCosmosElementNotFound    = 101
	ErrCosmosStartRunningPanic  = 102
	ErrCosmosCannotKill         = 103
	ErrCosmosCannotSendWormhole = 104
	ErrCosmosCannotMessage      = 105
	ErrCosmosCannotScale        = 106
	ErrCosmosRunnableNotFound   = 107

	// Cosmos Remote 111-120

	ErrCosmosRemoteElementNotFound     = 111
	ErrCosmosRemoteListenFailed        = 112
	ErrCosmosRemoteConnectFailed       = 113
	ErrCosmosRemoteRequestInvalid      = 114
	ErrCosmosRemoteResponseInvalid     = 115
	ErrCosmosRemoteServerInvalidArgs   = 116
	ErrCosmosRemoteCannotSendWormhole  = 117
	ErrCosmosRemoteCannotKill          = 118
	ErrElementRemoteCannotKill         = 119
	ErrElementRemoteCannotSendWormhole = 120

	// Element 121-130

	ErrElementLoaded                  = 121
	ErrElementScaleHandlerNotExists   = 122
	ErrElementMessageHandlerNotExists = 123
	ErrElementNotImplemented          = 124

	// Atom 131-150

	ErrAtomMessageHandlerNotExists                    = 131
	ErrAtomKillElementNoImplement                     = 132
	ErrAtomKillElementNotImplementAutoDataPersistence = 133
	ErrAtomFromIDInvalid                              = 134
	ErrAtomDataNotFound                               = 135
	ErrAtomNotExists                                  = 136
	ErrAtomIsRunning                                  = 137
	ErrAtomIsStopping                                 = 138
	ErrAtomNotImplemented                             = 139
	ErrAtomMessageAtomType                            = 140
	ErrAtomMessageArgType                             = 141
	ErrAtomMessageReplyType                           = 142

	// Util 201-250

	ErrUtilOSStatError                      = 201
	ErrUtilReadDirectoryFailed              = 202
	ErrUtilNotSupportedOS                   = 203
	ErrUtilPathShouldBeDirectory            = 204
	ErrUtilDirectoryNotExist                = 205
	ErrUtilGetUserGroupIDsFailed            = 206
	ErrUtilUsersGroupsHaveNotOwnedDirectory = 207
	ErrUtilFileModePermNotMatch             = 208
	ErrUtilFileMakeDirectoryFailed          = 209
	ErrUtilFileChangeOwnerAndModeFailed     = 210
	ErrUtilFileConfirmOwnerAndModeFailed    = 211
	ErrUtilCreateFileFailed                 = 212
)
