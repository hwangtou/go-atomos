package go_atomos

import (
	"testing"
	"time"
)

func TestElementLocal(t *testing.T) {
	initTestFakeCosmosProcess(t)
	if err := SharedCosmosProcess().Start(newTestFakeRunnable(t)); err != nil {
		t.Errorf("CosmosMain: Start failed. err=(%v)", err)
		return
	}
	elemName := "testElement"
	process := SharedCosmosProcess()
	elem := process.main.elements[elemName]
	impl := process.main.runnable.implements[elemName]

	// Element
	{
		// Push Message.
		reply, err := elem.pushMessageMail(process.main, "testMessage", 0, nil)
		if err != nil {
			t.Errorf("PushMessage: TestMessage Failed, state=(%d),err=(%v)", elem.atomos.GetState(), err)
			return
		}
		t.Logf("PushMessage: TestMessage Succeed. state=(%d),reply=(%v)", elem.atomos.GetState(), reply)

		// Get Atom
		atomA, tracker, err := elem.elementAtomGet("atomA", 1)
		if err == nil {
			t.Errorf("GetAtom: Get not exists atom.")
			return
		}
		t.Logf("GetAtom: Get not exists correct. atom=(%v),tracker=(%v),err=(%v)", atomA, tracker, err)

		// Spawn Atom.
		atomA, tracker, err = elem.elementAtomSpawn("atomA", nil, impl, nil, 1)
		if err != nil {
			t.Errorf("SpawnAtom: Spawn failed. err=(%v)", err)
			return
		}
		t.Logf("SpawnAtom: Spawn succeed. atomA=(%v),trackerDump=(%v),tracker=(%v)", atomA, atomA.idTracker.RefCount(), tracker)

		// Message Atom.
		reply, err = atomA.pushMessageMail(process.main, "testMessage", 0, nil)
		if err != nil {
			t.Errorf("MessageAtom: Message failed. err=(%v)", err)
			return
		}
		t.Logf("MessageAtom: Message succeed. reply=(%v),trackerDump=(%v),tracker=(%v)", atomA, atomA.idTracker.RefCount(), tracker)

		// Message Atom Panic.
		reply, err = atomA.pushMessageMail(process.main, "testPanic", 0, nil)
		if err == nil {
			t.Errorf("MessageAtom: Message failed, should panic.")
			return
		}
		t.Logf("MessageAtom: Message succeed. reply=(%v),err=(%v),trackerDump=(%v),tracker=(%v)", reply, err, atomA.idTracker.RefCount(), tracker)

		// Get Atom.
		atomA2, trackerA2, err := elem.elementAtomGet("atomA", 1)
		if err != nil {
			t.Errorf("GetAtom: Get failed. err=(%v)", err)
			return
		}
		t.Logf("GetAtom: Get succeed. atomA=(%v),trackerDump=(%v),tracker=(%v)", atomA2, atomA.idTracker.RefCount(), trackerA2)
		trackerA2.Release()
		t.Logf("GetAtom: Get succeed then release. atomA=(%v),trackerDump=(%v),tracker=(%v)", atomA2, atomA.idTracker.RefCount(), trackerA2)

		// Stopping Atom.
		err = atomA.pushKillMail(nil, true, 0)
		if err != nil {
			t.Errorf("KillAtom: Kill failed. err=(%v)", err)
			return
		}
		t.Logf("KillAtom: Kill succeed. atomAState=(%d),trackerDump=(%v),tracker=(%v)", atomA.getAtomLocal().atomos.GetState(), atomA.idTracker.RefCount(), trackerA2)

		// Release Atom.
		tracker.Release()
		trackerA2.Release()
		t.Logf("GetAtom: Get succeed then release. atomAState=(%d),trackDump=(%v),tracker=(%v)", atomA.getAtomLocal().atomos.GetState(), atomA.idTracker.RefCount(), trackerA2)
		// Release too much.
		trackerA2.Release()
	}

	// Push Tasking.
	reply, err := elem.pushMessageMail(process.main, "testTask", 0, nil)
	if err != nil {
		t.Errorf("PushMessage: TestTask Failed. state=(%d),err=(%v)", elem.atomos.GetState(), err)
		return
	}
	t.Logf("PushMessage: TestTask Succeed. state=(%d),reply=(%v)", elem.atomos.GetState(), reply)

	// Message Atom Panic.
	reply, err = elem.pushMessageMail(process.main, "testPanic", 0, nil)
	if err == nil {
		t.Errorf("PushMessage: Message failed, should panic.")
		return
	}
	t.Logf("PushMessage: Message succeed. reply=(%v),err=(%v)", reply, err)

	// Push Deadlock Message.
	reply, err = elem.pushMessageMail(elem, "testDeadLoop", 0, nil)
	if err == nil {
		t.Errorf("PushMessage: TestDeadLoop Failed. state=(%d),callChain=(%v)", elem.atomos.GetState(), elem.callChain)
		return
	}
	t.Logf("PushMessage: TestDeadLoop Succeed. state=(%d),callChain=(%v),err=(%v)", elem.atomos.GetState(), elem.callChain, err)

	// Push Stopping.
	err = elem.pushKillMail(nil, true, 0)
	if err != nil {
		t.Errorf("PushKill: Failed, state=(%d).err=(%v)", elem.atomos.GetState(), err)
		return
	}
	t.Logf("PushKill: Succeed. state=(%d)", elem.atomos.GetState())

	// Push Message.
	reply, err = elem.pushMessageMail(process.main, "testMessage", 0, nil)
	if err == nil {
		t.Errorf("PushMessage: Succeed after halted. state=(%d),err=(%v)", elem.atomos.GetState(), err)
		return
	}
	t.Logf("PushMessage: Message failed after halted. state=(%d),reply=(%v)", elem.atomos.GetState(), reply)

	time.Sleep(100 * time.Millisecond)
}
