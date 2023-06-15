package go_atomos

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
)

// IDTracker

type idTrackerManager struct {
	mutex   sync.Mutex
	release AtomosRelease
	counter uint64
	idMap   map[uint64]*IDTracker
}

type AtomosRelease interface {
	Release(tracker *IDTracker)
}

func (i *idTrackerManager) init(release AtomosRelease) {
	i.release = release
	i.idMap = map[uint64]*IDTracker{}
}

// addIDTracker is used to add IDTracker for local.
func (i *idTrackerManager) addIDTracker(rt *IDTrackerInfo) *IDTracker {
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
func (i *idTrackerManager) addScaleIDTracker(tracker *IDTracker) {
	i.mutex.Lock()
	i.counter += 1
	tracker.id = i.counter
	i.idMap[tracker.id] = tracker
	i.mutex.Unlock()

	tracker.manager = i
}

// refCount is used to get the number of IDTracker.
func (i *idTrackerManager) refCount() int {
	i.mutex.Lock()
	num := len(i.idMap)
	i.mutex.Unlock()
	return num
}

// Release is used to release IDTracker.
func (i *idTrackerManager) Release(tracker *IDTracker) {
	if tracker == nil {
		return
	}
	i.mutex.Lock()
	delete(i.idMap, tracker.id)
	i.mutex.Unlock()
}

// String is used to get the string of IDTrackerManager.
func (i *idTrackerManager) String() string {
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

	er      *ElementRemote
	manager *idTrackerManager

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
	if i.er != nil {
		i.er.elementAtomRelease(i)
	}
	if i.manager != nil {
		i.manager.mutex.Lock()
		delete(i.manager.idMap, i.id)
		i.manager.mutex.Unlock()
		i.manager.release.Release(i)
	}
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
		er:      nil,
		manager: nil,
		file:    x.File,
		line:    int(x.Line),
		name:    x.Name,
	}
}

func (x *IDTrackerInfo) newIDTrackerFromRemoteTrackerID(er *ElementRemote, trackerID uint64) *IDTracker {
	return &IDTracker{
		id:      trackerID,
		er:      er,
		manager: nil,
		file:    x.File,
		line:    int(x.Line),
		name:    x.Name,
	}
}
