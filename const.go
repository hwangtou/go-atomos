package go_atomos

import "time"

// LoggingAtomos
const (
	logTimeFmt = "2006-01-02 15:04:05.000000"
	logTestOut = true
	logTestErr = true
)

var (
	LogStdout = false
	LogStderr = false
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

const (
	etcdKeepaliveInterval = 10 * time.Second
)

const (
	ElementBroadcastName = "Broadcast"
)
