package go_atomos

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// messageTrackerManager

type messageTrackerManager struct {
	atomos  *BaseAtomos
	counter int64

	spawningAt, spawnAt   time.Time
	stoppingAt, stoppedAt time.Time

	messages map[string]MessageTrackInfo
}

// Warn: Remember to lock with atomos mutex.

func (t *messageTrackerManager) init(atomos *BaseAtomos, num int) {
	t.atomos = atomos
	t.spawningAt = time.Now()
	t.messages = make(map[string]MessageTrackInfo, num)
}

// Internal

func (t *messageTrackerManager) GetMessagingInfo() string {
	t.atomos.mailbox.mutex.Lock()
	defer t.atomos.mailbox.mutex.Unlock()
	return t.dump()
}

func (t *messageTrackerManager) IdleTime() time.Duration {
	t.atomos.mailbox.mutex.Lock()
	defer t.atomos.mailbox.mutex.Unlock()
	return t.idleTime()
}

func (t *messageTrackerManager) idleTime() time.Duration {
	if len(t.messages) == 0 {
		return time.Now().Sub(t.spawnAt)
	}
	var lastEnd time.Time
	for _, info := range t.messages {
		if info.LastEnd.After(lastEnd) {
			lastEnd = info.LastEnd
		}
	}
	if lastEnd.IsZero() {
		return time.Now().Sub(t.spawnAt)
	}
	return time.Now().Sub(lastEnd)
}

func (t *messageTrackerManager) Spawn() {
	t.spawnAt = time.Now()
}

func (t *messageTrackerManager) Set(message string) {
	m, has := t.messages[message]
	if !has {
		m = MessageTrackInfo{}
	}
	m.Handling = true
	m.LastBegin = time.Now()
	m.Count += 1
	t.messages[message] = m

	counter := atomic.AddInt64(&t.counter, 1)
	if MessageTimeoutTracer {
		time.AfterFunc(DefaultTimeout, func() {
			if atomic.LoadInt64(&t.counter) == counter {
				SharedLogging().PushLogging(t.atomos.id, LogLevel_Warn,
					fmt.Sprintf("MessageTracker: Message Timeout. id=(%v),message=(%s)", t.atomos.id, message))
			}
		})
	}
}

func (t *messageTrackerManager) Unset(message string) {
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

func (t *messageTrackerManager) Stopping() {
	t.stoppingAt = time.Now()
}

func (t *messageTrackerManager) Halted() {
	t.stoppedAt = time.Now()
}

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

type MessageTrackInfo struct {
	Handling bool

	LastBegin, LastEnd time.Time

	Min, Max, Total time.Duration
	Count           int
}

func (m MessageTrackInfo) String() string {
	return fmt.Sprintf("count=(%d),min=(%v),max=(%v),total=(%v)", m.Count, m.Min, m.Max, m.Total)
}
