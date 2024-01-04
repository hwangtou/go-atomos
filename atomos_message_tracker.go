package go_atomos

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"runtime/debug"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// atomosMessageTracker
// 消息跟踪管理器

type atomosMessageTracker struct {
	counter int64

	spawningAt, spawnAt   time.Time
	stoppingAt, stoppedAt time.Time

	current  string
	messages map[string]MessageTrackInfo
}

func initAtomosMessageTracker(mt *atomosMessageTracker) {
	mt.messages = make(map[string]MessageTrackInfo)
}

func releaseAtomosMessageTracker(mt *atomosMessageTracker) {
	mt.messages = nil
}

type AtomosMessageTrackerExporter struct {
	Counter               int64
	SpawningAt, SpawnAt   time.Time
	StoppingAt, StoppedAt time.Time
	Messages              []MessageTrackInfo
}

// Internal

// idleTime returns the idle time of the Atomos.
// 返回Atomos的空闲时间。
func (t *atomosMessageTracker) idleTime() time.Duration {
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

func (t *atomosMessageTracker) spawning() {
	t.spawningAt = time.Now()
}

// spawn sets the spawn time, after spawning.
func (t *atomosMessageTracker) spawn() {
	t.spawnAt = time.Now()
}

// set sets the message handling info and start time.
func (t *atomosMessageTracker) set(message string, info *IDInfo, process *CosmosProcess, arg proto.Message) {
	m, has := t.messages[message]
	if !has {
		m = MessageTrackInfo{}
	}
	m.Handling = true
	m.LastBegin = time.Now()
	m.Count += 1
	t.current = message
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
				process.onIDMessageTimeout(info, messageTimeoutDefault, message, arg)
			}
		})
	}
}

// unset sets the message handling info and end time.
func (t *atomosMessageTracker) unset(message string) {
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
	t.current = ""

	atomic.AddInt64(&t.counter, 1)
}

// stopping sets the stopping time.
func (t *atomosMessageTracker) stopping() {
	t.stoppingAt = time.Now()
}

// halted sets the halted time.
func (t *atomosMessageTracker) halted() {
	t.stoppedAt = time.Now()
}

// dump returns the message tracking info.
func (t *atomosMessageTracker) dump() string {
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

func (t *atomosMessageTracker) Export() *AtomosMessageTrackerExporter {
	messages := make([]MessageTrackInfo, 0, len(t.messages))
	for _, info := range t.messages {
		messages = append(messages, info)
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Count > messages[j].Count
	})
	return &AtomosMessageTrackerExporter{
		Counter:    t.counter,
		SpawningAt: t.spawningAt,
		SpawnAt:    t.spawnAt,
		StoppingAt: t.stoppingAt,
		StoppedAt:  t.stoppedAt,
		Messages:   messages,
	}
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
