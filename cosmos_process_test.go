package go_atomos

import (
	"testing"
	"time"
)

func TestCosmosProcess(t *testing.T) {
	// Load runnable.
	cosmos, err := NewCosmosProcess()
	if err != nil {
		t.Errorf(err.Message)
		return
	}
	defer cosmos.Close()

	exitCh, err := cosmos.Daemon()
	if err != nil {
		t.Errorf(err.Message)
		return
	}

	go func() {
		time.Sleep(1 * time.Second)
		if err := cosmos.Send(NewExitCommand()); err != nil {
			t.Errorf(err.Message)
		}
	}()

	runnable := newTestFakeRunnable(t)
	if err = cosmos.Send(NewRunnableCommand(runnable)); err != nil {
		t.Errorf(err.Message)
		return
	}

	cosmos.WaitKillSignal()
	<-exitCh
}
