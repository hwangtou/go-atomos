package atomos

import "testing"

// Concurrency-map semantics tests. MapGoWithRefCount's Put/GetOrPut refcount
// contract was audit finding B4; the tests below pin the fixed semantics.

func Test_MapGo_Basics(t *testing.T) {
	m := NewMapGo[string, int]()

	m.Put("a", 1)
	if v, ok := m.Get("a"); !ok || v != 1 {
		t.Errorf("Get(a)=%d,%v", v, ok)
	}
	if v, put := m.GetOrPut("a", 9); put || v != 1 {
		t.Errorf("GetOrPut existing: %d,%v want 1,false", v, put)
	}
	if v, put := m.GetOrPut("b", 2); !put || v != 2 {
		t.Errorf("GetOrPut new: %d,%v want 2,true", v, put)
	}
	if !m.Contains("a") || m.Contains("z") {
		t.Error("Contains mismatch")
	}
	if m.Len() != 2 {
		t.Errorf("Len=%d", m.Len())
	}
	if keys := m.Keys(); len(keys) != 2 {
		t.Errorf("Keys=%v", keys)
	}
	if values := m.Values(); len(values) != 2 {
		t.Errorf("Values=%v", values)
	}

	seen := map[string]int{}
	m.Iterate(func(k string, v int) bool {
		seen[k] = v
		return len(seen) < 1 // stop after first
	})
	if len(seen) != 1 {
		t.Errorf("Iterate early-stop visited %d entries, want 1", len(seen))
	}

	m.Remove("a")
	if m.Contains("a") {
		t.Error("Remove left key behind")
	}
	m.Clear()
	if m.Len() != 0 {
		t.Errorf("Clear left %d entries", m.Len())
	}
}

func Test_MapGoWithRefCount_PutDoesNotInventReferences(t *testing.T) {
	// B4 regression: Put on an existing key must NOT bump the refcount —
	// otherwise one Remove per Put would never free the entry.
	m := NewMapGoWithRefCount[string, int]()
	m.Put("k", 1)
	m.Put("k", 2) // overwrite: refcount stays 1
	if v, _ := m.Get("k"); v != 2 {
		t.Errorf("value=%d, want overwritten 2", v)
	}
	m.Remove("k")
	if m.Contains("k") {
		t.Error("single Remove after Put+Put should free the entry")
	}
}

func Test_MapGoWithRefCount_SharedReferences(t *testing.T) {
	m := NewMapGoWithRefCount[string, int]()

	// GetOrPut on a hit increments the shared refcount.
	if _, put := m.GetOrPut("k", 1); !put {
		t.Fatal("first GetOrPut should put")
	}
	for i := 0; i < 3; i++ {
		if _, put := m.GetOrPut("k", 1); put {
			t.Fatal("subsequent GetOrPut should hit")
		}
	}

	// Three Removes for refcount 4 (1 initial + 3 hits) still hold one ref.
	for i := 0; i < 3; i++ {
		m.Remove("k")
		if !m.Contains("k") {
			t.Fatal("entry freed while references remain")
		}
	}
	m.Remove("k")
	if m.Contains("k") {
		t.Error("entry not freed after last reference released")
	}

	// Removing a never-added key is a no-op.
	m.Remove("ghost")
}

func Test_MapGoWithRefCount_ClearResets(t *testing.T) {
	m := NewMapGoWithRefCount[string, int]()
	m.GetOrPut("k", 1)
	m.GetOrPut("k", 1)
	m.Clear()
	if m.Len() != 0 {
		t.Fatalf("Len after Clear=%d", m.Len())
	}
	// After Clear the refcount side-map must be empty too: a fresh GetOrPut
	// starts from 1 again, so one Remove frees the entry.
	m.GetOrPut("k", 1)
	m.Remove("k")
	if m.Contains("k") {
		t.Error("refcount map leaked across Clear")
	}
}
