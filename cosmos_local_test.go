package atomos

import "testing"

type cosmosLocalRecordingArg struct{ calls int }

func (a *cosmosLocalRecordingArg) getArgType() ArgType {
	a.calls++
	return ArgTypeBaseAtomosWithAny
}
func (*cosmosLocalRecordingArg) argForBaseAtomos() {}

func TestCosmosLocalGetAndSpawnForwardUserArgsWithLocalRouting(t *testing.T) {
	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		spawnArg := &cosmosLocalRecordingArg{}
		id, tracker, err := p.local.CosmosSpawnAtom(p.local, ForTestAtomosName, "forward_args_atom", &ForTestSpawnArg{}, spawnArg)
		if err != nil {
			t.Fatalf("CosmosSpawnAtom: %v", err)
		}
		if id == nil || tracker == nil {
			t.Fatal("local spawn routing must return an ID and tracker")
		}
		defer tracker.Release()
		if spawnArg.calls == 0 {
			t.Fatal("CosmosSpawnAtom discarded the user args")
		}

		getArg := &cosmosLocalRecordingArg{}
		id, tracker, err = p.local.CosmosGetAtomID(ForTestAtomosName, "forward_args_atom", getArg)
		if err != nil {
			t.Fatalf("CosmosGetAtomID: %v", err)
		}
		if id == nil || tracker == nil {
			t.Fatal("local get routing must return an ID and tracker")
		}
		defer tracker.Release()
		if getArg.calls == 0 {
			t.Fatal("CosmosGetAtomID discarded the user args")
		}
	})
}

//import (
//	"testing"
//)
//
//func TestCosmosMain(t *testing.T) {
//	initTestFakeCosmosProcess(t)
//	if err := SharedCosmosProcess().Start(newTestFakeRunnable(t, sharedCosmosProcess, false)); err != nil {
//		t.Errorf("CosmosLocal: Start failed. err=(%v)", err)
//		return
//	}
//	if err := SharedCosmosProcess().Stop(); err != nil {
//		t.Errorf("CosmosLocal: Stop failed. err=(%v)", err)
//		return
//	}
//}
