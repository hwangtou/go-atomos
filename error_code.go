package atomos

const (
	OK = iota

	ErrFrameworkInternalError
	ErrFrameworkRecoverFromPanic
	ErrFrameworkIncorrectUsage

	// Cosmos Process

	ErrCosmosProcessHasNotInitialized
	ErrCosmosProcessHasBeenStarted
	ErrCosmosProcessOnStartupPanic
	ErrCosmosProcessOnShutdownPanic
	ErrCosmosProcessCannotStopPrepareState
	ErrCosmosProcessCannotStopStartupState
	ErrCosmosProcessCannotStopShutdownState
	ErrCosmosProcessCannotStopOffState
	ErrCosmosProcessInvalidState

	// Cosmos Main

	ErrMainLoadCertFailed
	ErrMainElementNotFound
	ErrMainStartRunningPanic
	ErrMainCannotKill
	ErrMainCannotSendWormhole
	ErrMainCannotMessage
	ErrMainRunnableNotFound
	ErrRunnableConfigNotFound
	ErrRunnableInterfaceInvalid
	ErrRunnableImplementInvalid
	ErrRunnableScriptNotFound

	// Cosmos Global

	// Cosmos Remote

	ErrCosmosRemoteElementNotFound
	ErrCosmosRemoteListenFailed
	ErrCosmosRemoteConnectFailed
	ErrCosmosRemoteRequestInvalid
	ErrCosmosRemoteResponseFailed
	ErrCosmosRemoteResponseInvalid
	ErrCosmosRemoteServerInvalidArgs
	ErrCosmosRemoteServerInvalidFirstSyncCall
	ErrCosmosRemoteInfoInvalid
	ErrCosmosRemoteCannotMessage
	ErrCosmosRemoteCannotSendWormhole
	ErrCosmosRemoteCannotKill
	ErrElementRemoteCannotKill
	ErrElementRemoteCannotSendWormhole

	// Config

	ErrCosmosConfigInvalid
	ErrCosmosEtcdConnectFailed
	ErrCosmosEtcdClusterTLSInvalid
	ErrCosmosEtcdClusterVersionsCheckFailed
	ErrCosmosEtcdClusterVersionLockFailed
	ErrCosmosEtcdGRPCServerFailed
	ErrCosmosEtcdKeepaliveFailed
	ErrCosmosEtcdInvalidKey
	ErrCosmosEtcdUpdateFailed
	ErrCosmosEtcdGetFailed
	ErrCosmosEtcdPutFailed
	ErrCosmosEtcdDeleteFailed
	ErrCosmosConfigCertInvalid
	ErrCosmosIsClosed

	// App Env

	ErrAppEnvGetExecutableFailed
	ErrAppEnvLaunchedFailed
	ErrAppEnvRunPathInvalid
	ErrAppEnvRunPathPIDFileInvalid
	ErrAppEnvRunPathPIDIsRunning
	ErrAppEnvRunPathWritePIDFileFailed
	ErrAppEnvRunPathRemovePIDFailed
	ErrAppEnvLoggingFileOpenFailed
	ErrAppEnvLoggingPathInvalid
	ErrAppEnvLoggingFileWriteFailed
	ErrAppEnvLoggingFileCloseFailed

	// Global

	ErrCosmosGlobalNoEtcdFailed
	ErrCosmosGlobalEtcdConnectFailed

	// Unix Domain Socket

	ErrAppUnixDomainSocketFileInvalid
	ErrAppUnixDomainSocketListenFailed
	ErrAppUnixDomainSocketDialFailed
	ErrAppUnixDomainSocketConnWriteFailed

	// Logging

	ErrAppLoggingPathInvalid
	ErrAppLoggingFileOpenFailed

	// Atomos

	ErrAtomosInvalidArguments
	ErrAtomosIsStopping
	ErrAtomosIsNotRunning
	ErrAtomosTaskInvalidFn
	ErrAtomosTaskNotExists
	ErrAtomosTaskAddCrontabFailed
	ErrAtomosTaskRemoveCrontabFailed
	ErrAtomosNotSupportWormhole
	ErrAtomosPushTimeoutHandling
	ErrAtomosPushTimeoutReject

	ErrAtomosTaskCannotCancelCancelledTask
	ErrAtomosTaskCannotCancelRunningTask
	ErrAtomosTaskCannotCancelDoneTask

	// idFirstSyncCall

	ErrIDFirstSyncCallDeadlock

	// Element

	ErrElementLoaded
	ErrElementMessageHandlerNotExists
	ErrElementMessageDecoderNotExists
	ErrElementMessageReplyType
	ErrElementCannotKill
	ErrElementNoFromID
	ErrElementNotImplemented
	ErrElementFromIDInvalid
	ErrElementToIDInvalid
	ErrElementMessageArgType

	// Atom

	ErrAtomMessageHandlerNotExists
	ErrAtomMessageDecoderNotExists
	ErrAtomKillElementNoImplement
	ErrAtomKillElementNotImplementAutoDataPersistence
	ErrAtomFromIDInvalid
	ErrAtomToIDInvalid
	ErrAtomDataNotFound
	ErrAtomNotExists
	ErrAtomIsRunning
	ErrAtomIsStopping
	ErrAtomNoFromID
	ErrAtomNotImplemented
	ErrAtomMessageAtomType
	ErrAtomMessageArgType
	ErrAtomMessageReplyType
	ErrAtomSpawningAnExistedAtom

	// 异步回调因 atom 停机被清账终结(回调随实例消亡前以错误跑完,业务可据此降级/补偿)
	// 注意:追加在 iota 块末尾,不可插入中间——错误码数值随 RPC 跨服务传输,错位会导致兼容问题
	ErrAsyncCallbackCancelledByHalt

	// Util File

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
	ErrUtilFileEnsureDirectoryFailed        = 213
	ErrUtilFileFileExistFailed              = 214
	ErrUtilFileGetDirectorySizeFailed       = 215
	ErrUtilStringHashSHA256Failed           = 216
)
