package atomos

import "time"

// These defaults are used when initializing a CosmosProcess. The setters below
// operate on the singleton sharedCosmosProcess for convenience. New code should
// set these directly on the CosmosProcess instance for test isolation.

// SetMessageTimeoutTracer enables or disables the message timeout tracer on the
// default CosmosProcess. When enabled, messages that exceed timeout are reported
// via the onIDMessageTimeout hook.
func SetMessageTimeoutTracer(v bool, timeout time.Duration) {
	if p := SharedCosmosProcess(); p != nil {
		p.messageTimeoutTracer = v
		p.messageTimeoutDefault = timeout
	}
}

// MuteKeepaliveLog silences or un-silences the periodic etcd keepalive log
// messages on the default CosmosProcess.
func MuteKeepaliveLog(b bool) {
	if p := SharedCosmosProcess(); p != nil {
		p.muteKeepaliveLog = b
	}
}

// SetIDTrackerDebug toggles IDTracker debug diagnostics on the default
// CosmosProcess. See CosmosProcess.idTrackerDebug.
func SetIDTrackerDebug(v bool) {
	if p := SharedCosmosProcess(); p != nil {
		p.idTrackerDebug = v
	}
}
