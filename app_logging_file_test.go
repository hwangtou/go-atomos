package atomos

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// File-logging backend tests. The old TestAppLogging was disabled long ago
// (it depended on 1s sleeps); these replace it with bounded, deterministic
// coverage of the behaviors that audits fixed: concurrent write
// serialization (B6), rotation, cleanup-safe Close.

// withRestoredStd guards AppLoggingToFile's redirectStd, which replaces the
// process-global os.Stdout/os.Stderr with log files.
func withRestoredStd(t *testing.T) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	t.Cleanup(func() {
		os.Stdout, os.Stderr = oldOut, oldErr
	})
}

func readTotalSize(t *testing.T, dir string) int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range entries {
		info, er := e.Info()
		if er != nil {
			t.Fatal(er)
		}
		total += info.Size()
	}
	return total
}

func Test_AppLoggingToFile_WriteAndRotate(t *testing.T) {
	withRestoredStd(t)
	dir := t.TempDir()

	// 64-byte max size, auto cleanup off: rotation is exercised, path-size
	// cleanup is not (it would delete the files under assertion).
	l, err := NewAppLoggingToFile(dir, 64, AppLoggingAutoCleanupOff, nil)
	if err != nil {
		t.Fatalf("NewAppLoggingToFile: %v", err)
	}
	defer l.Close()

	line := strings.Repeat("a", 15) + "\n" // 16 bytes: 4 lines = 1 rotation
	for i := 0; i < 8; i++ {
		l.WriteAccessLog(line)
	}
	l.WriteErrorLog("boom\n")

	l.writeMu.Lock()
	l.curAccessLog.Sync()
	l.curErrorLog.Sync()
	l.writeMu.Unlock()

	// All bytes written must be present on disk exactly once. Rotation may
	// reuse a same-second filename (O_APPEND), so assert totals, not per-file.
	// WriteErrorLog lands twice: mirrored into the access log plus the error
	// log's own file.
	const wantTotal = int64(8*16) + 2*int64(len("boom\n"))
	if got := readTotalSize(t, dir); got != wantTotal {
		t.Errorf("total bytes=%d, want %d", got, wantTotal)
	}

	// The error log received its own copy.
	entries, _ := os.ReadDir(dir)
	var errorBytes int64
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), AppLoggingErrorPrefix) {
			info, _ := e.Info()
			errorBytes += info.Size()
		}
	}
	if errorBytes != int64(len("boom\n")) {
		t.Errorf("error log bytes=%d, want %d", errorBytes, len("boom\n"))
	}
}

func Test_AppLoggingToFile_ConcurrentWrites(t *testing.T) {
	withRestoredStd(t)
	dir := t.TempDir()
	l, err := NewAppLoggingToFile(dir, 128, AppLoggingAutoCleanupOff, nil)
	if err != nil {
		t.Fatalf("NewAppLoggingToFile: %v", err)
	}

	// B6 regression: the mailbox path and the direct-write fallback path can
	// both call Write*Log from different goroutines. Under the writeMu fix
	// no write may be lost or interleaved mid-line.
	const writers, writes = 8, 64
	line := strings.Repeat("w", 31) + "\n" // 32 bytes
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < writes; j++ {
				l.WriteAccessLog(line)
			}
		}()
	}
	wg.Wait()
	l.Close() // must flush everything written so far

	const want = int64(writers * writes * 32)
	if got := readTotalSize(t, dir); got != want {
		t.Errorf("total bytes=%d, want %d (lost or duplicated writes)", got, want)
	}
}

func Test_AppLoggingToFile_CloseIsIdempotent(t *testing.T) {
	withRestoredStd(t)
	dir := t.TempDir()
	l, err := NewAppLoggingToFile(dir, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewAppLoggingToFile: %v", err)
	}

	// Defaults: 0 file size -> AppLoggingDefaultMaxSize, ratio 0 -> 30x.
	if l.logFileMaxSize != AppLoggingDefaultMaxSize {
		t.Errorf("file max size=%d, want default %d", l.logFileMaxSize, AppLoggingDefaultMaxSize)
	}
	if l.logPathMaxSize != AppLoggingDefaultMaxSize*AppLoggingAutoCleanupRatio {
		t.Errorf("path max size=%d, want %d", l.logPathMaxSize, AppLoggingDefaultMaxSize*AppLoggingAutoCleanupRatio)
	}

	l.Close()
	l.Close() // second Close must be a safe no-op
	l.WriteAccessLog("after close\n")
	l.WriteErrorLog("after close\n")
}

func Test_AppLoggingToFile_InvalidPath(t *testing.T) {
	withRestoredStd(t)
	// A regular file where a directory is required.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAppLoggingToFile(filepath.Join(blocker, "sub"), 0, 0, nil); err == nil {
		t.Fatal("expected error for unwritable log path, got nil")
	}
}

func Test_AppLoggingToConsole(t *testing.T) {
	l := NewAppLoggingToConsole()
	l.WriteAccessLog("access\n")
	l.WriteErrorLog("error\n")
	l.Close() // no-op, must not close process stdout
}

func Test_shouldLogToStdout(t *testing.T) {
	if !shouldLogToStdout(&Config{Customize: map[string][]byte{configKeyLogStdout: []byte("1")}}) {
		t.Error("customize flag=1 should select stdout")
	}
	if shouldLogToStdout(&Config{Customize: map[string][]byte{}}) && !isRunningInDocker() {
		t.Error("no flag and not in docker should not select stdout")
	}
}
