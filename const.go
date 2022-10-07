package go_atomos

// LoggingAtomos
const (
	logTimeFmt = "2006-01-02 15:04:05.000000"
)

// Main
const (
	// MainElementName
	// Name of main element
	MainElementName = "Main"
)

const (
	RemoteAtomosURIPrefix = "/atomos"

	RemoteAtomosConnect          = RemoteAtomosURIPrefix + "/connect"
	RemoteAtomosElementMessaging = RemoteAtomosURIPrefix + "/element/messaging"
	RemoteAtomosElementScaling   = RemoteAtomosURIPrefix + "/element/scaling"
	RemoteAtomosGetAtom          = RemoteAtomosURIPrefix + "/atom/get"
	RemoteAtomosAtomMessaging    = RemoteAtomosURIPrefix + "/atom/message"
)
