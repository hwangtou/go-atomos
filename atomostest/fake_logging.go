package atomostest

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	atomos "github.com/hwangtou/go-atomos"
)

// LogEntry is a single log record captured by FakeLogging.
type LogEntry struct {
	// Level is the log level the entry was emitted at.
	Level atomos.LogLevel
	// Text is the fully formatted message (format verb already applied).
	Text string
}

// FakeLogging implements atomos.Logging by recording every entry in memory
// instead of writing to files, so tests can assert on what an actor logged.
//
// Level filtering mirrors the framework: an entry is recorded only when its
// level is at or above the level set via SetLevel. The default level is
// atomos.LogLevel_Debug, so everything is recorded until SetLevel is called.
type FakeLogging struct {
	mu      sync.Mutex
	level   atomos.LogLevel
	entries []LogEntry
}

// NewFakeLogging returns a FakeLogging with level atomos.LogLevel_Debug.
func NewFakeLogging() *FakeLogging {
	return &FakeLogging{level: atomos.LogLevel_Debug}
}

var _ atomos.Logging = (*FakeLogging)(nil)

// Write implements io.Writer. The payload is recorded as a
// atomos.LogLevel_CoreInfo entry with trailing newlines trimmed, matching the
// framework logger's Write behavior.
func (f *FakeLogging) Write(p []byte) (int, error) {
	msg := string(bytes.TrimRight(p, "\n"))
	f.record(atomos.LogLevel_CoreInfo, msg)
	return len(p), nil
}

// SetLevel sets the minimum level recorded by this logger.
func (f *FakeLogging) SetLevel(lv atomos.LogLevel) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.level = lv
}

func (f *FakeLogging) Debug(format string, args ...any) {
	f.recordf(atomos.LogLevel_Debug, format, args...)
}

func (f *FakeLogging) Info(format string, args ...any) {
	f.recordf(atomos.LogLevel_Info, format, args...)
}

func (f *FakeLogging) Warn(format string, args ...any) {
	f.recordf(atomos.LogLevel_Warn, format, args...)
}

func (f *FakeLogging) Error(format string, args ...any) {
	f.recordf(atomos.LogLevel_Err, format, args...)
}

func (f *FakeLogging) Fatal(format string, args ...any) {
	// Unlike the framework logger, Fatal never aborts the process; it only
	// records, so tests stay alive.
	f.recordf(atomos.LogLevel_Fatal, format, args...)
}

func (f *FakeLogging) recordf(level atomos.LogLevel, format string, args ...any) {
	f.record(level, fmt.Sprintf(format, args...))
}

func (f *FakeLogging) record(level atomos.LogLevel, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.level > level {
		return
	}
	f.entries = append(f.entries, LogEntry{Level: level, Text: text})
}

// Entries returns a copy of all recorded entries in emission order.
func (f *FakeLogging) Entries() []LogEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]LogEntry, len(f.entries))
	copy(out, f.entries)
	return out
}

// Contains reports whether any recorded entry's text contains substr.
func (f *FakeLogging) Contains(substr string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.entries {
		if strings.Contains(e.Text, substr) {
			return true
		}
	}
	return false
}

// Reset discards all recorded entries.
func (f *FakeLogging) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = nil
}
