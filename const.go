package go_atomos

// LoggingAtomos
const (
	logTimeFmt = "2006-01-02 15:04:05.000000"
	logStdout  = false
	logStderr  = false
	logTestOut = true
	logTestErr = true
)

const (
	udsConnReadBufSize = 1024
)

const (
	ShouldArgumentClone = false
)

const (
	RemoteAtomosURIPrefix = "/remote"

	RemoteAtomosConnect          = RemoteAtomosURIPrefix + "/connect"
	RemoteAtomosElementScaling   = RemoteAtomosURIPrefix + "/element/scaling"
	RemoteAtomosElementMessaging = RemoteAtomosURIPrefix + "/element/messaging"
	RemoteAtomosGetAtom          = RemoteAtomosURIPrefix + "/atom/get"
	RemoteAtomosAtomMessaging    = RemoteAtomosURIPrefix + "/atom/messaging"
	RemoteAtomosIDRelease        = RemoteAtomosURIPrefix + "/id/release"
)
