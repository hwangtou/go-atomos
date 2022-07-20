package go_atomos

import (
	"testing"
	"time"
)

var testElem *ElementLocal

func TestElementLocal(t *testing.T) {
	main := newTestFakeCosmosMain(t)
	impl := newTestFakeElement(t)
	elem := newElementLocal(main, impl)
	elem.Log().Info("TestElementLocal")

	// Element Load.
	err := elem.load(impl)
	if err != nil {
		t.Errorf("PushMessage: Load failed, state=(%v),err=(%v)", elem.atomos.GetState(), err)
		return
	}

	// Spawn.
	elem.atomos.setSpawning()
	err = impl.Interface.ElementSpawner(elem, elem.atomos.instance, nil, nil)
	if err != nil {
		t.Errorf("ElementSpawner: Failed, state=(%v),err=(%v)", elem.atomos.GetState(), err)
		return
	}
	elem.atomos.setWaiting()

	testElem = elem

	// Atom
	{
		// Push Message.
		reply, err := elem.pushMessageMail(nil, "testMessage", nil)
		if err != nil {
			t.Errorf("PushMessage: TestMessage Failed, state=(%v),err=(%v)", elem.atomos.GetState(), err)
			return
		}
		t.Logf("PushMessage: TestMessage Succeed, state=(%v),reply=(%v)", elem.atomos.GetState(), reply)

		// Get Atom
		atomA, err := elem.elementAtomGet("atomA")
		if err == nil {
			t.Errorf("GetAtom: Get not exists atom")
			return
		}
		t.Logf("GetAtom: Get not exists failed, atom=(%v),err=(%v)", atomA, err)

		// Spawn Atom.
		atomA, err = elem.elementAtomSpawn("atomA", nil, impl, nil)
		if err != nil {
			t.Errorf("SpawnAtom: Spawn failed, err=(%v)", err)
			return
		}
		t.Logf("SpawnAtom: Spawn succeed, atomA=(%v),ref=(%d)", atomA, atomA.count)

		// Message Atom.
		reply, err = atomA.pushMessageMail(nil, "testMessage", nil)
		if err != nil {
			t.Errorf("MessageAtom: Message failed, err=(%v)", err)
			return
		}
		t.Logf("MessageAtom: Message succeed, reply=(%v),ref=(%d)", atomA, atomA.count)

		// Get Atom.
		atomA2, err := elem.elementAtomGet("atomA")
		if err != nil {
			t.Errorf("GetAtom: Get failed, err=(%v)", err)
			return
		}
		t.Logf("GetAtom: Get succeed, atomA=(%v),ref=(%d),mapVal=(%v)", atomA2, atomA2.count, elem.atoms["atomA"])
		atomA2.Release()
		t.Logf("GetAtom: Get succeed then release, atomA=(%v),ref=(%d),mapVal=(%v)", atomA2, atomA2.count, elem.atoms["atomA"])

		// Halt Atom.
		err = atomA.pushKillMail(nil, true)
		if err != nil {
			t.Errorf("KillAtom: Kill failed, err=(%v)", err)
			return
		}
		t.Logf("KillAtom: Kill succeed, atomA=(%v),ref=(%d),mapVal=(%v)", atomA, atomA.count, elem.atoms["atomA"])

		// Release Atom.
		atomA2.Release()
		t.Logf("GetAtom: Get succeed then release, atomA=(%v),ref=(%d),mapVal=(%v)", atomA2, atomA2.count, elem.atoms["atomA"])
		// Release too much.
		atomA2.Release()
	}

	// Push Tasking.
	reply, err := elem.pushMessageMail(nil, "testTask", nil)
	if err != nil {
		t.Errorf("PushMessage: TestTask Failed, state=(%v),err=(%v)", elem.atomos.GetState(), err)
		return
	}
	t.Logf("PushMessage: TestTask Succeed, state=(%v),reply=(%v)", elem.atomos.GetState(), reply)

	// Push Deadlock Message.
	elem.callChain = append(elem.callChain, elem)
	reply, err = elem.pushMessageMail(elem, "testDeadLoop", nil)
	if err == nil {
		t.Errorf("PushMessage: TestDeadLoop Failed, state=(%v),callChain=(%v)", elem.atomos.GetState(), elem.callChain)
		return
	}
	t.Logf("PushMessage: TestDeadLoop Succeed, state=(%v),callChain=(%v),err=(%v)", elem.atomos.GetState(), elem.callChain, err)

	// Push Reload.
	t.Logf("PushReload: Begin, state=(%v),reload=(%d)", elem.atomos.GetState(), elem.atomos.reloads)
	err = elem.pushReloadMail(nil, impl, 2)
	if err != nil {
		t.Errorf("PushReload: Failed, state=(%v),reload=(%d),err=(%v)", elem.atomos.GetState(), elem.atomos.reloads, err)
		return
	}
	t.Logf("PushReload: Succeed, state=(%v),reload=(%d)", elem.atomos.GetState(), elem.atomos.reloads)

	// Push Halt.
	err = elem.pushKillMail(nil, true)
	if err != nil {
		t.Errorf("PushKill: Failed, state=(%v),err=(%v)", elem.atomos.GetState(), err)
		return
	}
	t.Logf("PushKill: Succeed, state=(%v)", elem.atomos.GetState())

	// Push Message.
	reply, err = elem.pushMessageMail(nil, "testMessage", nil)
	if err == nil {
		t.Errorf("PushMessage: Succeed after halted, state=(%v),err=(%v)", elem.atomos.GetState(), err)
		return
	}
	t.Logf("PushMessage: Failed after halted, state=(%v),reply=(%v)", elem.atomos.GetState(), reply)

	time.Sleep(100 * time.Millisecond)
}
