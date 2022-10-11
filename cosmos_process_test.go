package go_atomos

import (
	"syscall"
	"testing"
	"time"
)

func TestCosmosProcess(t *testing.T) {
	runnable := newTestFakeRunnable(t)

	time.AfterFunc(1*time.Second, func() {
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	})

	CosmosProcessMainFn(*runnable)
}
