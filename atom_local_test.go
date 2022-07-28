package go_atomos

import (
	"testing"
	"time"
)

var testAtom *AtomLocal

func TestAtomLocal(t *testing.T) {
	main := newTestFakeCosmosMain(t)
	impl := newTestFakeElement(t)
	elem := newElementLocal(main, main.runnable, impl)

	// Spawn.
	atom := newAtomLocal("testAtom", elem, elem.atomos.reloads, impl, elem.atomos.logging, impl.Interface.Config.LogLevel)
	atom.Log().Info("TestAtomLocal")
	atom.atomos.setSpawning()
	err := impl.Interface.AtomSpawner(atom, atom.atomos.instance, nil, nil)
	if err != nil {
		t.Errorf("AtomSpawner: Failed, state=(%v),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	atom.atomos.setWaiting()

	testAtom = atom

	// Push Message.
	reply, err := atom.pushMessageMail(nil, "testMessage", nil)
	if err != nil {
		t.Errorf("PushMessage: TestMessage Failed, state=(%v),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	t.Logf("PushMessage: TestMessage Succeed, state=(%v),reply=(%v)", atom.atomos.GetState(), reply)

	// Push Tasking.
	reply, err = atom.pushMessageMail(nil, "testTask", nil)
	if err != nil {
		t.Errorf("PushMessage: TestTask Failed, state=(%v),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	t.Logf("PushMessage: TestTask Succeed, state=(%v),reply=(%v)", atom.atomos.GetState(), reply)

	// Push Deadlock Message.
	atom.callChain = append(atom.callChain, atom)
	reply, err = atom.pushMessageMail(atom, "testDeadLoop", nil)
	if err == nil {
		t.Errorf("PushMessage: TestDeadLoop Failed, state=(%v),callChain=(%v)", atom.atomos.GetState(), atom.callChain)
		return
	}
	t.Logf("PushMessage: TestDeadLoop Succeed, state=(%v),callChain=(%v),err=(%v)", atom.atomos.GetState(), atom.callChain, err)

	// Push Reload.
	t.Logf("PushReload: Begin, state=(%v),reload=(%d)", atom.atomos.GetState(), atom.atomos.reloads)
	err = atom.pushReloadMail(nil, impl, 2)
	if err != nil {
		t.Errorf("PushReload: Failed, state=(%v),reload=(%d),err=(%v)", atom.atomos.GetState(), atom.atomos.reloads, err)
		return
	}
	t.Logf("PushReload: Succeed, state=(%v),reload=(%d)", atom.atomos.GetState(), atom.atomos.reloads)

	// Push Halt.
	err = atom.pushKillMail(nil, true)
	if err != nil {
		t.Errorf("PushKill: Failed, state=(%v),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	t.Logf("PushKill: Succeed, state=(%v)", atom.atomos.GetState())

	// Push Message.
	reply, err = atom.pushMessageMail(nil, "testMessage", nil)
	if err == nil {
		t.Errorf("PushMessage: Succeed after halted, state=(%v),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	t.Logf("PushMessage: Failed after halted, state=(%v),reply=(%v)", atom.atomos.GetState(), reply)

	time.Sleep(100 * time.Millisecond)
}
