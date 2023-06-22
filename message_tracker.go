package go_atomos

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"
)

// messageTrackerManager
// 消息跟踪管理器

type messageTrackerManager struct {
	counter int64

	spawningAt, spawnAt   time.Time
	stoppingAt, stoppedAt time.Time

	messages map[string]MessageTrackInfo
}

func initAtomosMessageTracker(mt *messageTrackerManager) {
	mt.messages = make(map[string]MessageTrackInfo)
}

// Internal

// idleTime returns the idle time of the Atomos.
// 返回Atomos的空闲时间。
func (t *messageTrackerManager) idleTime() time.Duration {
	// If no messages after spawning, return the time since spawning.
	if len(t.messages) == 0 {
		return time.Now().Sub(t.spawnAt)
	}
	// Find the last end time.
	var lastEnd time.Time
	for _, info := range t.messages {
		if info.LastEnd.After(lastEnd) {
			lastEnd = info.LastEnd
		}
	}
	// If no end time, return the time since spawn.
	if lastEnd.IsZero() {
		return time.Now().Sub(t.spawnAt)
	}
	// Return the time since last end.
	return time.Now().Sub(lastEnd)
}

func (t *messageTrackerManager) spawning() {
	t.spawningAt = time.Now()
}

// spawn sets the spawn time, after spawning.
func (t *messageTrackerManager) spawn() {
	t.spawnAt = time.Now()
}

// set sets the message handling info and start time.
func (t *messageTrackerManager) set(message string, info *IDInfo, process *CosmosProcess) {
	m, has := t.messages[message]
	if !has {
		m = MessageTrackInfo{}
	}
	m.Handling = true
	m.LastBegin = time.Now()
	m.Count += 1
	t.messages[message] = m

	counter := atomic.AddInt64(&t.counter, 1)
	if messageTimeoutTracer {
		time.AfterFunc(messageTimeoutDefault, func() {
			if atomic.LoadInt64(&t.counter) == counter {
				defer func() {
					if r := recover(); r != nil {
						process.logging.PushLogging(info, LogLevel_Fatal, fmt.Sprintf("MessageTracker: Recover from panic. reason=(%v),stack=(%s)",
							r, string(debug.Stack())))
					}
				}()
				process.onIDMessageTimeout(info, message)
			}
		})
	}
}

// unset sets the message handling info and end time.
func (t *messageTrackerManager) unset(message string) {
	m := t.messages[message]
	m.LastEnd = time.Now()
	d := m.LastEnd.Sub(m.LastBegin)
	if m.Min == 0 {
		m.Min = d
	} else if m.Min > d {
		m.Min = d
	}
	if m.Max < d {
		m.Max = d
	}
	m.Total += d
	m.Handling = false
	t.messages[message] = m

	atomic.AddInt64(&t.counter, 1)
}

// stopping sets the stopping time.
func (t *messageTrackerManager) stopping() {
	t.stoppingAt = time.Now()
}

// halted sets the halted time.
func (t *messageTrackerManager) halted() {
	t.stoppedAt = time.Now()
}

// dump returns the message tracking info.
func (t *messageTrackerManager) dump() string {
	if len(t.messages) == 0 {
		return "No MessageTracker"
	}
	b := strings.Builder{}
	b.WriteString("MessageTracker Info:")
	for message, info := range t.messages {
		b.WriteString("\n\t")
		b.WriteString(message)
		b.WriteString(": ")
		b.WriteString(info.String())
	}
	return b.String()
}

// MessageTrackInfo is the message tracking info.
// MessageTrackInfo是消息跟踪信息。

type MessageTrackInfo struct {
	Handling bool

	LastBegin, LastEnd time.Time

	Min, Max, Total time.Duration
	Count           int
}

// String returns the string of the MessageTrackInfo.
func (m MessageTrackInfo) String() string {
	return fmt.Sprintf("count=(%d),min=(%v),max=(%v),total=(%v)", m.Count, m.Min, m.Max, m.Total)
}
