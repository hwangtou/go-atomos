package core

const (
	OK = iota

	ErrFrameworkPanic

	ErrCosmosConfigInvalid
	ErrCosmosConfigNodeNameInvalid
	ErrCosmosConfigLogPathInvalid
	ErrCosmosCertConfigInvalid
	ErrCosmosHasAlreadyRun
	ErrCosmosIsBusy
	ErrCosmosIsClosed

	ErrAtomosPanic
	ErrAtomosIsNotRunning
	ErrAtomosTaskInvalidFn
	ErrAtomosTaskNotExists

	ErrAtomMessageHandlerNotExists
	ErrAtomMessageHandlerPanic
	ErrAtomKillHandlerPanic
	ErrAtomUpgradeInvalid
	ErrAtomFromIDInvalid
	ErrAtomToIDInvalid
	ErrAtomNotExists
	ErrAtomExists
	ErrAtomPersistenceRuntime
	ErrAtomSpawnArgInvalid

	ErrElementCannotKill
)
