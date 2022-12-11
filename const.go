package go_atomos

import "time"

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
	IsDebug            = false
	DebugAtomosTimeout = 5 * time.Second
)

const (
	ShouldQuitBlocking  = true
	QuitBlockingTimeout = 6 * time.Second
)
