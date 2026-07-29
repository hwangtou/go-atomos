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

	// Util File
	//
	// These continue the iota sequence from above (no separate numeric range).
	// Previously they were hardcoded 201..216, which risked colliding with the
	// iota block if new codes were inserted above. Keeping a single iota block
	// makes the codes dense and collision-free regardless of insertion point.

	ErrUtilOSStatError
	ErrUtilReadDirectoryFailed
	ErrUtilNotSupportedOS
	ErrUtilPathShouldBeDirectory
	ErrUtilDirectoryNotExist
	ErrUtilGetUserGroupIDsFailed
	ErrUtilUsersGroupsHaveNotOwnedDirectory
	ErrUtilFileModePermNotMatch
	ErrUtilFileMakeDirectoryFailed
	ErrUtilFileChangeOwnerAndModeFailed
	ErrUtilFileConfirmOwnerAndModeFailed
	ErrUtilCreateFileFailed
	ErrUtilFileEnsureDirectoryFailed
	ErrUtilFileFileExistFailed
	ErrUtilFileGetDirectorySizeFailed
	ErrUtilStringHashSHA256Failed
)
