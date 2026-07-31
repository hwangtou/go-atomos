package atomos

import (
	"testing"
	"time"
)

func TestLoggingAtomos_SmokeTest(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	logging := &loggingAtomos{}
	if err := logging.init(newTestLogging(t)); err != nil {
		t.Fatalf("failed to init logging: %v", err)
	}

	// Wait loggingAtomos logs popped
	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	logging.PushLogging(nil, LogLevel_Info, "This is a test log message.")
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("expected 1 allocated mail, got %d", getAllocMailDebugNum())
	}
	<-time.After(time.Millisecond)
	if getAllocMailDebugNum() != 0 {
		t.Fatalf("expected 0 allocated mails, got %d", getAllocMailDebugNum())
	}

	logging.stop()

	// Wait loggingAtomos logs popped
	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}
	t.Log("LoggingAtomos smoke test completed.")

	<-time.After(time.Millisecond)
}
