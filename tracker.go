package go_atomos

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// IDTracker

type IDTrackerManager struct {
	sync.Mutex
	release AtomosRelease
	counter int
	idMap   map[int]*IDTracker
}

func NewIDTrackerManager(release AtomosRelease) *IDTrackerManager {
	return &IDTrackerManager{
		Mutex:   sync.Mutex{},
		release: release,
		counter: 0,
		idMap:   map[int]*IDTracker{},
	}
}

func (i *IDTrackerManager) NewIDTracker(skip int) *IDTracker {
	i.Lock()
	i.counter += 1
	tracker := &IDTracker{id: i.counter}
	i.idMap[tracker.id] = tracker
	i.Unlock()

	tracker.manager = i
	caller, file, line, ok := runtime.Caller(skip)
	if ok {
		tracker.file = file
		tracker.line = line
		if pc := runtime.FuncForPC(caller); pc != nil {
			tracker.name = pc.Name()
		}
	}
	return tracker
}

func (i *IDTrackerManager) NewScaleIDTracker(tracker *IDTracker) {
	if tracker == nil {
		return
	}
	i.Lock()
	i.counter += 1
	tracker.id = i.counter
	i.idMap[tracker.id] = tracker
	i.Unlock()

	tracker.manager = i
}

func (i *IDTrackerManager) Release(tracker *IDTracker) {
	if tracker == nil {
		return
	}
	i.Lock()
	delete(i.idMap, tracker.id)
	i.Unlock()
}

func (i *IDTrackerManager) RefCount() int {
	i.Lock()
	num := len(i.idMap)
	i.Unlock()
	return num
}

func (i *IDTrackerManager) String() string {
	i.Lock()
	defer i.Unlock()
	if len(i.idMap) == 0 {
		return "No IDTracker remain"
	}
	b := strings.Builder{}
	b.WriteString("IDTracker Info:")
	for _, tracker := range i.idMap {
		b.WriteString("\n\t")
		b.WriteString(tracker.ToString())
	}
	return b.String()
}

type IDTracker struct {
	id      int
	manager *IDTrackerManager

	file string
	line int
	name string
}

func NewScaleIDTracker(skip int) *IDTracker {
	tracker := &IDTracker{}
	caller, file, line, ok := runtime.Caller(skip)
	if ok {
		tracker.file = file
		tracker.line = line
		if pc := runtime.FuncForPC(caller); pc != nil {
			tracker.name = pc.Name()
		}
	}
	return tracker
}

func (i *IDTracker) ToString() string {
	if i == nil {
		return "nil"
	}
	return fmt.Sprintf("%d-%s:%d", i.id, i.file, i.line)
}

func (i *IDTracker) Release() {
	if i == nil {
		return
	}
	i.manager.Lock()
	delete(i.manager.idMap, i.id)
	i.manager.Unlock()
	i.manager.release.Release(i)
}

// MessageTrackerManager

type MessageTrackerManager struct {
	id      *IDInfo
	counter int64

	spawningAt, spawnAt   time.Time
	stoppingAt, stoppedAt time.Time

	messages map[string]MessageTrackInfo
}

// Warn: Remember to lock with atomos mutex.

func NewMessageTrackerManager(id *IDInfo, num int) *MessageTrackerManager {
	return &MessageTrackerManager{
		id:         id,
		spawningAt: time.Now(),
		messages:   make(map[string]MessageTrackInfo, num),
	}
}

func (t *MessageTrackerManager) idleTime() time.Duration {
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

func (t *MessageTrackerManager) Start() {
	t.spawnAt = time.Now()
}

func (t *MessageTrackerManager) Set(message string) {
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
				SharedLogging().PushLogging(t.id, LogLevel_Warn,
					fmt.Sprintf("MessageTracker: Message Timeout. id=(%v),message=(%s)", t.id, message))
			}
		})
	}
}

func (t *MessageTrackerManager) Unset(message string) {
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

func (t *MessageTrackerManager) Stopping() {
	t.stoppingAt = time.Now()
}

func (t *MessageTrackerManager) Halt() {
	t.stoppedAt = time.Now()
}

func (t *MessageTrackerManager) dump() string {
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
