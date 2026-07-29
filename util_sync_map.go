package atomos

import "sync"

type Map[K comparable, V any] interface {
	Get(key K) (value V, ok bool)
	GetOrPut(key K, value V) (v V, put bool)
	Put(key K, value V)
	Remove(key K)
	Contains(key K) bool
	Keys() []K
	Values() []V
	Iterate(fn func(key K, value V) bool)
	Len() int
	Clear()
}

// MapGo is a thread-safe map implementation using sync.RWMutex for synchronization.

type MapGo[K comparable, V any] struct {
	sync.RWMutex
	m map[K]V
}

func NewMapGo[K comparable, V any]() Map[K, V] {
	return &MapGo[K, V]{m: make(map[K]V)}
}

func (m *MapGo[K, V]) Get(key K) (value V, ok bool) {
	m.RLock()
	defer m.RUnlock()
	value, ok = m.m[key]
	return
}

func (m *MapGo[K, V]) GetOrPut(key K, value V) (v V, put bool) {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.m[key]; !ok {
		m.m[key] = value
		return value, true
	}
	return m.m[key], false
}

func (m *MapGo[K, V]) Put(key K, value V) {
	m.Lock()
	defer m.Unlock()
	m.m[key] = value
}

func (m *MapGo[K, V]) Remove(key K) {
	m.Lock()
	defer m.Unlock()
	delete(m.m, key)
}

func (m *MapGo[K, V]) Contains(key K) bool {
	m.RLock()
	defer m.RUnlock()
	_, ok := m.m[key]
	return ok
}

func (m *MapGo[K, V]) Keys() []K {
	m.RLock()
	defer m.RUnlock()
	keys := make([]K, 0, len(m.m))
	for k := range m.m {
		keys = append(keys, k)
	}
	return keys
}

func (m *MapGo[K, V]) Iterate(fn func(key K, value V) bool) {
	iterMap := make(map[K]V)
	func() {
		m.RLock()
		defer m.RUnlock()
		for k, v := range m.m {
			iterMap[k] = v
		}
	}()
	for k, v := range iterMap {
		if !fn(k, v) {
			break
		}
	}
}

func (m *MapGo[K, V]) Values() []V {
	m.RLock()
	defer m.RUnlock()
	values := make([]V, 0, len(m.m))
	for _, v := range m.m {
		values = append(values, v)
	}
	return values
}

func (m *MapGo[K, V]) Len() int {
	m.RLock()
	defer m.RUnlock()
	return len(m.m)
}

func (m *MapGo[K, V]) Clear() {
	m.Lock()
	defer m.Unlock()
	m.m = make(map[K]V)
}

// MapGoWithRefCount is a thread-safe map implementation that also maintains a reference count for each key.

type MapGoWithRefCount[K comparable, V any] struct {
	sync.RWMutex
	m map[K]V
	r map[K]int
}

func NewMapGoWithRefCount[K comparable, V any]() Map[K, V] {
	return &MapGoWithRefCount[K, V]{m: make(map[K]V), r: make(map[K]int)}
}

func (m *MapGoWithRefCount[K, V]) Get(key K) (value V, ok bool) {
	m.RLock()
	defer m.RUnlock()
	value, ok = m.m[key]
	return
}

func (m *MapGoWithRefCount[K, V]) GetOrPut(key K, value V) (v V, put bool) {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.m[key]; !ok {
		m.m[key] = value
		m.r[key] = 1
		return value, true
	}
	m.r[key]++
	return m.m[key], false
}

// Put sets value for key with overwrite semantics.
//
// Unlike GetOrPut (which increments the ref count on a hit), Put replaces the
// value without touching the ref count — a "set" operation should not invent
// extra references. A brand-new key is initialised with ref count 1 so that a
// subsequent Remove actually removes it.
func (m *MapGoWithRefCount[K, V]) Put(key K, value V) {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.r[key]; !ok {
		m.r[key] = 1
	}
	m.m[key] = value
}

func (m *MapGoWithRefCount[K, V]) Remove(key K) {
	m.Lock()
	defer m.Unlock()
	if count, ok := m.r[key]; ok {
		if count > 1 {
			m.r[key]--
		} else {
			delete(m.m, key)
			delete(m.r, key)
		}
	}
}

func (m *MapGoWithRefCount[K, V]) Contains(key K) bool {
	m.RLock()
	defer m.RUnlock()
	_, ok := m.m[key]
	return ok
}

func (m *MapGoWithRefCount[K, V]) Keys() []K {
	m.RLock()
	defer m.RUnlock()
	keys := make([]K, 0, len(m.m))
	for k := range m.m {
		keys = append(keys, k)
	}
	return keys
}

func (m *MapGoWithRefCount[K, V]) Iterate(fn func(key K, value V) bool) {
	iterMap := make(map[K]V)
	func() {
		m.RLock()
		defer m.RUnlock()
		for k, v := range m.m {
			iterMap[k] = v
		}
	}()
	for k, v := range iterMap {
		if !fn(k, v) {
			break
		}
	}
}

func (m *MapGoWithRefCount[K, V]) Values() []V {
	m.RLock()
	defer m.RUnlock()
	values := make([]V, 0, len(m.m))
	for _, v := range m.m {
		values = append(values, v)
	}
	return values
}

func (m *MapGoWithRefCount[K, V]) Len() int {
	m.RLock()
	defer m.RUnlock()
	return len(m.m)
}

func (m *MapGoWithRefCount[K, V]) Clear() {
	m.Lock()
	defer m.Unlock()
	m.m = make(map[K]V)
	m.r = make(map[K]int)
}
