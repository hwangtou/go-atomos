package atomos

import (
	"runtime"
	"testing"
	"time"
)

func TestElementLocalDoesNotHoldContainerLockWhileReadingAtomState(t *testing.T) {
	const (
		name   = "atom"
		instID = uint64(1)
	)
	tests := []struct {
		name  string
		state BaseAtomosState
		call  func(*ElementLocal)
	}{
		{
			name:  "get_atoms",
			state: BaseAtomosWaiting,
			call: func(e *ElementLocal) {
				_ = e.GetAtoms()
			},
		},
		{
			name:  "get_atoms_in_pattern",
			state: BaseAtomosWaiting,
			call: func(e *ElementLocal) {
				_ = e.GetAtomsInPattern("atom")
			},
		},
		{
			name:  "active_count",
			state: BaseAtomosWaiting,
			call: func(e *ElementLocal) {
				_ = e.GetActiveAtomsNum()
			},
		},
		{
			name:  "inactive_trackers",
			state: BaseAtomosHalt,
			call: func(e *ElementLocal) {
				_ = e.GetAllInactiveAtomsIDTrackerInfo()
			},
		},
		{
			name:  "refs_drained",
			state: BaseAtomosHalt,
			call: func(e *ElementLocal) {
				e.onInstanceRefsDrained(e.refStates[name], instID)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newElementContainerTestElement()
			atom := newElementContainerTestAtom(e, name, instID)
			atom.atomos.state = tc.state
			atom.nameElement = e.names.PushBack(name)
			e.atoms[name] = atom
			e.refStates[name] = newAtomRefState(e, name)

			atom.atomos.mailbox.mutex.Lock()
			mailboxLocked := true
			unlockMailbox := func() {
				if mailboxLocked {
					atom.atomos.mailbox.mutex.Unlock()
					mailboxLocked = false
				}
			}
			defer unlockMailbox()

			done := make(chan struct{})
			go func() {
				tc.call(e)
				close(done)
			}()

			// Let the caller reach the mailbox state read. With the old lock order
			// it holds e.lock while blocked here, so the write lock never succeeds.
			time.Sleep(10 * time.Millisecond)
			deadline := time.Now().Add(time.Second)
			for !e.lock.TryLock() {
				if time.Now().After(deadline) {
					unlockMailbox()
					<-done
					t.Fatal("container write lock remained blocked by a mailbox state read")
				}
				runtime.Gosched()
			}
			e.lock.Unlock()
			unlockMailbox()

			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("container operation did not finish after releasing the mailbox lock")
			}
		})
	}
}
