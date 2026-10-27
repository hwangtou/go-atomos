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

func TestAtomosTaskManager_AddRejectedByLifecycleState(t *testing.T) {
	tests := []struct {
		name     string
		stopping bool
		wantCode int64
		add      func(*atomosTaskManager) (uint64, error)
	}{
		{
			name:     "add stopping",
			stopping: true,
			wantCode: ErrCodeAtomosIsStopping,
			add: func(at *atomosTaskManager) (uint64, error) {
				return at.Add(func(uint64) {})
			},
		},
		{
			name:     "add after stopping",
			stopping: true,
			wantCode: ErrCodeAtomosIsStopping,
			add: func(at *atomosTaskManager) (uint64, error) {
				return at.AddAfter(time.Second, func(uint64) {})
			},
		},
		{
			name:     "add crontab stopping",
			stopping: true,
			wantCode: ErrCodeAtomosIsStopping,
			add: func(at *atomosTaskManager) (uint64, error) {
				return at.AddCrontab("@every 1h", func(uint64) {})
			},
		},
		{
			name:     "add not running",
			wantCode: ErrCodeAtomosIsNotRunning,
			add: func(at *atomosTaskManager) (uint64, error) {
				return at.Add(func(uint64) {})
			},
		},
		{
			name:     "add after not running",
			wantCode: ErrCodeAtomosIsNotRunning,
			add: func(at *atomosTaskManager) (uint64, error) {
				return at.AddAfter(time.Second, func(uint64) {})
			},
		},
		{
			name:     "add crontab not running",
			wantCode: ErrCodeAtomosIsNotRunning,
			add: func(at *atomosTaskManager) (uint64, error) {
				return at.AddCrontab("@every 1h", func(uint64) {})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := &atomosTaskManager{stopping: tt.stopping}
			id, err := tt.add(at)
			if id != 0 || CodeOf(err) != tt.wantCode {
				t.Fatalf("id = %d, error = %v, want id 0 and code %d", id, err, tt.wantCode)
			}
		})
	}
}
