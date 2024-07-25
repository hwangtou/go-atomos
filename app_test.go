package go_atomos

import (
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestSpawnAtom(t *testing.T) {
	testInitWebSocketConnsManager(t)

	s := self
	n := 100000
	am := make(map[string]*TestAtomID, n)
	for i := 0; i < n; i++ {
		name := strconv.FormatUint(uint64(i), 10)
		atom, err := SpawnTestAtom(self, self.Cosmos(), name, nil)
		if err != nil {
			t.Fatal(err)
		}
		am[name] = atom
		o, err := atom.AtomMessage(self, &String{S: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if o == nil || len(o.Ss) == 0 || o.Ss[0] != "test" {
			t.Fatal("test fail")
		}
	}

	t.Logf("Spawn %d atoms %d", n, runtime.NumGoroutine())

	for k, id := range am {
		id.Release()
		if err := id.Kill(self, 0); err != nil {
			t.Fatal(err)
		}
		delete(am, k)
	}

	t.Logf("Release %d atoms %d", n, runtime.NumGoroutine())

	runtime.GC()

	<-time.After(10 * time.Second)
	s.Log().Info("GC")

	for i := 0; i < n; i++ {
		name := strconv.FormatUint(uint64(i), 10)
		atom, err := SpawnTestAtom(self, self.Cosmos(), name, nil)
		if err != nil {
			t.Fatal(err)
		}
		am[name] = atom
		o, err := atom.AtomMessage(self, &String{S: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if o == nil || len(o.Ss) == 0 || o.Ss[0] != "test" {
			t.Fatal("test fail")
		}
	}

	t.Logf("Spawn %d atoms %d", n, runtime.NumGoroutine())

	for k, id := range am {
		id.Release()
		if err := id.Kill(self, 0); err != nil {
			t.Fatal(err)
		}
		delete(am, k)
	}

	t.Logf("Release %d atoms %d", n, runtime.NumGoroutine())

	<-time.After(10 * time.Second)
	runtime.GC()

	<-time.After(100 * time.Second)
	s.Log().Info("GC")
}
