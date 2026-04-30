package atomos

import (
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestAtomosTaskManager_ClosureInfo(t *testing.T) {
	at := newTestAtomosTaskManager(t)
	testAtomosTaskManagerClosureInfo := func(at *atomosTaskManager) string {
		return at.closureInfo(0)
	}
	ci := testAtomosTaskManagerClosureInfo(at)
	_, filename, line, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Failed to get caller info for closureInfo test.")
	}
	name := filename + ":" + strconv.FormatInt(int64(line-1), 10)
	if name != ci {
		t.Fatalf("Expected closure info to be '%s', got '%s'", name, ci)
	}

	t.Log("Closure info:", ci)

	<-time.After(time.Millisecond)
}
