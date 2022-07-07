package go_atomos

const (
	OK = iota

	ErrFrameworkPanic

	ErrCosmosConfigInvalid
	ErrCosmosHasAlreadyRun
	ErrCosmosCertConfigInvalid

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
