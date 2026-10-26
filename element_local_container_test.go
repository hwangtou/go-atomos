package atomos

import (
	"container/list"
	"sync"
	"testing"
	"time"
)

func newElementContainerTestElement() *ElementLocal {
	return &ElementLocal{
		atoms:     map[string]*AtomLocal{},
		names:     list.New(),
		refStates: map[string]*atomRefState{},
	}
}

func newElementContainerTestAtom(e *ElementLocal, name string, instID uint64) *AtomLocal {
	atom := &AtomLocal{element: e}
	atom.atomos = &BaseAtomos{
		id:         &IDInfo{Type: IDType_Atom, Element: "container_test", Atom: name},
		instanceID: instID,
		state:      BaseAtomosHalt,
		mailbox:    newMailBox(name, nil, nil),
	}
	return atom
}

func publishElementContainerTestRespawn(e *ElementLocal, name string) (*AtomLocal, *AtomLocal, *atomRefState) {
	oldAtom := newElementContainerTestAtom(e, name, 1)
	newAtom := newElementContainerTestAtom(e, name, 2)
	newAtom.atomos.state = BaseAtomosWaiting
	nameElement := e.names.PushBack(name)
	oldAtom.nameElement = nameElement
	newAtom.nameElement = nameElement
	st := newAtomRefState(e, name)
	e.atoms[name] = newAtom
	e.refStates[name] = st
	return oldAtom, newAtom, st
}

func checkElementContainerTestRespawn(t *testing.T, e *ElementLocal, name string, newAtom *AtomLocal, st *atomRefState) {
	t.Helper()
	e.lock.RLock()
	defer e.lock.RUnlock()
	if e.atoms[name] != newAtom {
		t.Fatal("stale cleanup removed or replaced the current atom")
	}
	if e.names.Len() != 1 || e.names.Back() != newAtom.nameElement || e.names.Back().Value != name {
		t.Fatal("stale cleanup removed the current atom's name node")
	}
	if e.refStates[name] != st {
		t.Fatal("stale cleanup removed the current atom's refState")
	}
}

func TestElementLocalStoppingOldInstanceAfterRespawn(t *testing.T) {
	const name = "atom"
	e := newElementContainerTestElement()
	oldAtom, newAtom, st := publishElementContainerTestRespawn(e, name)

	e.elementAtomStopping(oldAtom)

	checkElementContainerTestRespawn(t, e, name, newAtom, st)
}

func TestElementLocalLateCleanupAfterRespawn(t *testing.T) {
	tests := []struct {
		name    string
		cleanup func(*ElementLocal, *AtomLocal, *atomRefState)
	}{
		{
			name: "release",
			cleanup: func(e *ElementLocal, oldAtom *AtomLocal, _ *atomRefState) {
				e.elementAtomRelease(oldAtom)
			},
		},
		{
			name: "refs_drained",
			cleanup: func(e *ElementLocal, _ *AtomLocal, st *atomRefState) {
				e.onInstanceRefsDrained(st, 1)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const name = "atom"
			e := newElementContainerTestElement()
			oldAtom, newAtom, st := publishElementContainerTestRespawn(e, name)

			tc.cleanup(e, oldAtom, st)

			checkElementContainerTestRespawn(t, e, name, newAtom, st)
		})
	}
}

func TestElementLocalCleanupCurrentInstance(t *testing.T) {
	tests := []struct {
		name    string
		cleanup func(*ElementLocal, *AtomLocal, *atomRefState)
	}{
		{
			name: "stopping",
			cleanup: func(e *ElementLocal, atom *AtomLocal, _ *atomRefState) {
				e.elementAtomStopping(atom)
			},
		},
		{
			name: "release",
			cleanup: func(e *ElementLocal, atom *AtomLocal, _ *atomRefState) {
				e.elementAtomRelease(atom)
			},
		},
		{
			name: "refs_drained",
			cleanup: func(e *ElementLocal, atom *AtomLocal, st *atomRefState) {
				e.onInstanceRefsDrained(st, atom.atomos.instanceID)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const name = "atom"
			e := newElementContainerTestElement()
			atom := newElementContainerTestAtom(e, name, 1)
			atom.nameElement = e.names.PushBack(name)
			st := newAtomRefState(e, name)
			e.atoms[name] = atom
			e.refStates[name] = st

			tc.cleanup(e, atom, st)

			e.lock.RLock()
			defer e.lock.RUnlock()
			if _, has := e.atoms[name]; has {
				t.Fatal("current halted atom was not removed")
			}
			if e.names.Len() != 0 || atom.nameElement != nil {
				t.Fatal("current atom's name node was not removed")
			}
			if _, has := e.refStates[name]; has {
				t.Fatal("empty refState was not removed")
			}
		})
	}
}

func TestElementLocalConcurrentStaleCleanup(t *testing.T) {
	const (
		name     = "atom"
		cleaners = 12
		rounds   = 20
	)
	for round := 0; round < rounds; round++ {
		e := newElementContainerTestElement()
		oldAtom, newAtom, st := publishElementContainerTestRespawn(e, name)
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(cleaners)
		for i := 0; i < cleaners; i++ {
			go func(i int) {
				defer wg.Done()
				<-start
				switch i % 3 {
				case 0:
					e.elementAtomStopping(oldAtom)
				case 1:
					e.elementAtomRelease(oldAtom)
				case 2:
					e.onInstanceRefsDrained(st, oldAtom.atomos.instanceID)
				}
			}(i)
		}
		close(start)
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("round %d: concurrent cleanup did not finish", round)
		}
		checkElementContainerTestRespawn(t, e, name, newAtom, st)
	}
}
