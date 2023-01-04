package go_atomos

import (
	"fmt"
	"runtime"
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

func (i *IDTrackerManager) NewTracker(skip int) *IDTracker {
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

type IDTracker struct {
	id      int
	manager *IDTrackerManager

	file string
	line int
	name string
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

func NewMessageTrackerManager(id *IDInfo, num int) *MessageTrackerManager {
	return &MessageTrackerManager{
		id:         id,
		spawningAt: time.Now(),
		messages:   make(map[string]MessageTrackInfo, num),
	}
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

func (t *MessageTrackerManager) Dump() map[string]MessageTrackInfo {
	messages := make(map[string]MessageTrackInfo, len(t.messages))
	for message, info := range t.messages {
		messages[message] = info
	}
	return messages
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
