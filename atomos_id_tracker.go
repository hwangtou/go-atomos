package go_atomos

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
)

// IDTracker

type atomosIDTracker struct {
	mutex   sync.Mutex
	atomos  *BaseAtomos
	counter uint64
	idMap   map[uint64]*IDTracker
}

func initAtomosIDTracker(it *atomosIDTracker, atomos *BaseAtomos) {
	it.atomos = atomos
	it.idMap = map[uint64]*IDTracker{}
}

func (i *atomosIDTracker) fromOld(old *atomosIDTracker) *atomosIDTracker {
	old.atomos = i.atomos
	return old
}

// addIDTracker is used to add IDTracker for local.
func (i *atomosIDTracker) addIDTracker(rt *IDTrackerInfo, localOrRemote bool) *IDTracker {
	i.mutex.Lock()
	i.counter += 1
	tracker := &IDTracker{id: i.counter}
	i.idMap[tracker.id] = tracker
	i.mutex.Unlock()

	tracker.manager = i
	tracker.file = rt.File
	tracker.line = int(rt.Line)
	tracker.name = rt.Name
	return tracker
}

// addScaleIDTracker is used to add IDTracker for scale.
func (i *atomosIDTracker) addScaleIDTracker(tracker *IDTracker) *IDTracker {
	i.mutex.Lock()
	i.counter += 1
	tracker.id = i.counter
	i.idMap[tracker.id] = tracker
	i.mutex.Unlock()

	tracker.manager = i
	return tracker
}

// refCount is used to get the number of IDTracker.
func (i *atomosIDTracker) refCount() int {
	i.mutex.Lock()
	num := len(i.idMap)
	i.mutex.Unlock()
	return num
}

// String is used to get the string of IDTrackerManager.
func (i *atomosIDTracker) String() string {
	i.mutex.Lock()
	defer i.mutex.Unlock()
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

// IDTracker is used to track the lifecycle of an ID.

type IDTracker struct {
	id uint64

	manager *atomosIDTracker

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
	if i.manager != nil {
		i.manager.mutex.Lock()
		delete(i.manager.idMap, i.id)
		num := len(i.manager.idMap)
		i.manager.mutex.Unlock()

		if num == 0 {
			i.manager.atomos.onIDReleased()
		}
	}
}

func (i *IDTracker) GetTracker() *IDTracker {
	return i
}

// IDTrackerInfo is used to create IDTracker in local.

func NewIDTrackerInfoFromLocalGoroutine(skip int) *IDTrackerInfo {
	tracker := &IDTrackerInfo{}
	caller, file, line, ok := runtime.Caller(skip)
	if ok {
		tracker.File = file
		tracker.Line = int32(line)
		if pc := runtime.FuncForPC(caller); pc != nil {
			tracker.Name = pc.Name()
		}
	}
	return tracker
}

func (x *IDTrackerInfo) newScaleIDTracker() *IDTracker {
	return &IDTracker{
		id:      0,
		manager: nil,
		file:    x.File,
		line:    int(x.Line),
		name:    x.Name,
	}
}
