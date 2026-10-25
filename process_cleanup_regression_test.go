package atomos

import (
	"testing"
	"time"
)

func TestStartFailureStopsFrameworkMailboxes(t *testing.T) {
	p, cerr := newCosmosProcess("start_failure_cleanup", "node", &reproLogger{})
	if cerr != nil {
		t.Fatalf("newCosmosProcess: %v", cerr)
	}
	t.Cleanup(func() {
		if p.local.atomos.mailbox.isRunning() {
			_ = p.local.atomos.PushKillMail(p.local)
			waitMailboxGoroutineExit(p.local.atomos.mailbox, 5*time.Second)
		}
		if p.logging.logBox != nil && p.logging.logBox.isRunning() {
			p.logging.stop()
		}
	})
	if err := p.Start(&CosmosRunnable{}); err == nil {
		t.Fatal("invalid runnable must fail Start")
	}
	waitMailboxGoroutineExit(p.local.atomos.mailbox, 5*time.Second)
	if p.logging.logBox != nil {
		waitMailboxGoroutineExit(p.logging.logBox, 5*time.Second)
	}
	if p.local.atomos.mailbox.isRunning() {
		t.Fatal("local cosmos mailbox survived Start failure")
	}
	if p.logging.logBox != nil && p.logging.logBox.isRunning() {
		t.Fatal("logging mailbox survived Start failure")
	}
}
