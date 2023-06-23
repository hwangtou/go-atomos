package go_atomos

import "time"

var (
	// messageTimeoutTracer is the switch of message timeout tracer.
	messageTimeoutTracer = true
	// messageTimeoutDefault is the default timeout duration.
	messageTimeoutDefault = 2 * time.Second
)

func SetMessageTimeoutTracer(v bool, timeout time.Duration) {
	messageTimeoutTracer = v
	messageTimeoutDefault = timeout
}
