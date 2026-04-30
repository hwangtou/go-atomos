package atomos

import (
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

var baMap sync.Map

//// List all the possibilities cases that may cause deadlock in BaseAtomos mailboxes.
//// Then create test cases to cover all the possibilities.
//// This test case is a smoke test to cover one of the deadlock cases.
//
//// Case 1: Two BaseAtomos sending sync messages to each other at the same time.
//// ba1 -> ba2 (sync)
//// ba2 -> ba1 (sync)
//func TestBaseAtomos_DeadlockSmoke_SendingSyncMessagesToEachOtherThenDeadlock(t *testing.T) {
//	detectDeadlock = false
//
//	c, e := "test_cosmos", "test_element"
//	id1 := &IDInfo{Type: IDType_Atom, Cosmos: c, Element: e, Atom: "test_atomos_1"}
//	id2 := &IDInfo{Type: IDType_Atom, Cosmos: c, Element: e, Atom: "test_atomos_2"}
//	p := newTestCosmosProcessWithoutCluster(t, c, "test_node")
//	ba1 := newTestBaseAtomosDeadlockMailboxBaseAtomos(t, p, id1)
//	ba2 := newTestBaseAtomosDeadlockMailboxBaseAtomos(t, p, id2)
//	baMap.Store("ba1", ba1)
//	baMap.Store("ba2", ba2)
//	var wg sync.WaitGroup
//
//	// Test deadlock: ba1->ba2, ba2->ba1 at the same time, ba1 and ba2 are running a mail which are trying to send a sync message to each other.
//	wg.Add(2)
//	ba1.atomos.task.AddToAtomosQueue(func(taskID uint64) {
//		<-time.After(time.Millisecond)
//		_, err := ba2.atomos.PushSyncMessage(ba1, "test_message_from_ba1_to_ba2", nil, []ArgsForBaseAtomos{ArgBaseAtomosTimeout(10 * time.Millisecond)})
//		if err == nil || err.Code != ErrAtomosPushTimeoutReject {
//			t.Errorf("ba1 PushSyncMessage to ba2 failed: %v", err)
//		}
//		wg.Done()
//	})
//	ba2.atomos.task.AddToAtomosQueue(func(taskID uint64) {
//		<-time.After(time.Millisecond)
//		_, err := ba1.atomos.PushSyncMessage(ba2, "test_message_from_ba2_to_ba1", nil, []ArgsForBaseAtomos{ArgBaseAtomosTimeout(10 * time.Millisecond)})
//		if err == nil || err.Code != ErrAtomosPushTimeoutReject {
//			t.Errorf("ba2 PushSyncMessage to ba1 expected timeout error, got: %v", err)
//		}
//		wg.Done()
//	})
//	wg.Wait()
//	t.Logf("Deadlock test completed successfully")
//}
//
//// Case 2: One BaseAtomos sending sync message to another BaseAtomos, which in turn sends sync message back to the first BaseAtomos.
//// ba1 -> ba2 (sync) -> ba1 (sync)
//func TestBaseAtomos_DeadlockSmoke_SendingSyncMessagesFinallyBackToSelfDeadlock(t *testing.T) {
//	detectDeadlock = false
//
//	c, e := "test_cosmos", "test_element"
//	id1 := &IDInfo{Type: IDType_Atom, Cosmos: c, Element: e, Atom: "test_atomos_1"}
//	id2 := &IDInfo{Type: IDType_Atom, Cosmos: c, Element: e, Atom: "test_atomos_2"}
//	p := newTestCosmosProcessWithoutCluster(t, c, "test_node")
//	ba1 := newTestBaseAtomosDeadlockMailboxBaseAtomos(t, p, id1)
//	ba2 := newTestBaseAtomosDeadlockMailboxBaseAtomos(t, p, id2)
//	baMap.Store("ba1", ba1)
//	baMap.Store("ba2", ba2)
//	var wg sync.WaitGroup
//
//	// Test deadlock: ba1->ba2->ba1 chain.
//	wg.Add(1)
//	ba1.atomos.task.AddToAtomosQueue(func(taskID uint64) {
//		res, err := ba2.atomos.PushSyncMessage(ba1, "test_deadlock_ba1_ba2_ba1_chain_1", nil, []ArgsForBaseAtomos{ArgBaseAtomosTimeout(10 * time.Millisecond)})
//		if err != nil {
//			t.Fatalf("ba1 PushSyncMessage failed: %v", err)
//		}
//		if res.(*String).S != "ok" {
//			t.Fatalf("ba1 PushSyncMessage to ba2 failed")
//		}
//		wg.Done()
//	})
//	wg.Wait()
//
//	t.Logf("Deadlock test completed successfully")
//}
//
//// Case 3: One BaseAtomos sending sync message to itself.
//// ba1 -> ba1 (sync)
//func TestBaseAtomos_DeadlockSmoke_SendingSyncMessageToSelfDeadlock(t *testing.T) {
//	detectDeadlock = false
//
//	c, e := "test_cosmos", "test_element"
//	id1 := &IDInfo{Type: IDType_Atom, Cosmos: c, Element: e, Atom: "test_atomos_1"}
//	p := newTestCosmosProcessWithoutCluster(t, c, "test_node")
//	ba1 := newTestBaseAtomosDeadlockMailboxBaseAtomos(t, p, id1)
//	baMap.Store("ba1", ba1)
//	var wg sync.WaitGroup
//
//	// Test deadlock: ba1->ba1
//	wg.Add(1)
//	ba1.atomos.task.AddToAtomosQueue(func(taskID uint64) {
//		_, err := ba1.atomos.PushSyncMessage(ba1, "test_deadlock_ba1_ba1_chain", nil, []ArgsForBaseAtomos{ArgBaseAtomosTimeout(10 * time.Millisecond)})
//		if err == nil || err.Code != ErrAtomosPushTimeoutReject {
//			t.Errorf("ba1 PushSyncMessage to ba1 expected timeout error, got: %v", err)
//		}
//		wg.Done()
//	})
//	wg.Wait()
//	t.Logf("Deadlock test completed successfully")
//}

type testBaseAtomosDeadlockMailboxBaseAtomos struct {
	t      *testing.T
	atomos *BaseAtomos
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) SyncMessagingByName(callerID ID, name string, in proto.Message, ext []ArgsForBaseAtomos) (out proto.Message, err *Error) {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(out proto.Message, err *Error), ext []ArgsForBaseAtomos) (errBeforeExec *Error) {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) asyncSet(callback func(out proto.Message, err *Error)) (startupID, callbackID uint64) {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) asyncCallback(callbackID ID, name string, startupID, asyncID uint64, reply proto.Message, err *Error) {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) GetIDInfo() *IDInfo {
	return t.atomos.GetIDInfo()
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) Cosmos() CosmosNode {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) State() BaseAtomosState {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) IdleTime() time.Duration {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) DecoderByName(name string) (in, out MessageDecoder) {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) Kill(callerID ID, ext []ArgsForBaseAtomos) *Error {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) SendWormhole(callerID ID, wormhole BaseAtomosWormhole, ext []ArgsForBaseAtomos) *Error {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) getGoID() uint64 {
	//TODO implement me
	panic("implement me")
}

func newTestBaseAtomosDeadlockMailboxBaseAtomos(t *testing.T, p *CosmosProcess, id *IDInfo) *testBaseAtomosDeadlockMailboxBaseAtomos {
	tba := &testBaseAtomosDeadlockMailboxBaseAtomos{}
	ba := NewBaseAtomos(tba, id, LogLevel_Debug, tba, tba, p)
	if err := ba.start(func() *Error {
		return nil
	}); err != nil {
		t.Fatalf("Failed to start BaseAtomos: %v", err)
	}
	tba.t = t
	tba.atomos = ba
	return tba
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) OnSyncMessaging(fromID ID, name string, in proto.Message) (out proto.Message, err *Error) {
	switch name {
	case "test_deadlock_ba1_ba2_ba1_chain_1":
		if fromID.GetIDInfo().Atom != "test_atomos_1" {
			panic("unexpected fromID in test_deadlock_ba1_ba2_ba1_chain_1")
		}
		if t.atomos.id.Atom != "test_atomos_2" {
			panic("unexpected atomos in test_deadlock_ba1_ba2_ba1_chain_1")
		}
		ba2Interface, ok := baMap.Load("ba2")
		if !ok {
			panic("ba2 not found in baMap")
		}
		ba2 := ba2Interface.(*testBaseAtomosDeadlockMailboxBaseAtomos)
		_, err := ba2.atomos.PushSyncMessage(t, "test_deadlock_ba1_ba2_ba1_chain_2", nil, []ArgsForBaseAtomos{ArgBaseAtomosTimeout(10 * time.Millisecond)})
		if err == nil || err.Code != ErrAtomosPushTimeoutReject {
			t.t.Errorf("ba2 PushSyncMessage to ba1 in chain expected timeout error, got: %v", err)
		}
		t.t.Log("test_deadlock_ba1_ba2_ba1_chain_1 ok")
		return &String{S: "ok"}, nil

	case "test_deadlock_ba1_ba2_ba1_chain_2":
		panic("should not reach here")

	case "test_deadlock_ba1_ba1_chain":
		panic("should not reach here")

	}
	panic("not supported")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) OnAsyncMessaging(fromID ID, name string, startupID, asyncID uint64, in proto.Message) {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) OnAsyncMessagingCallback(asyncID uint64, in proto.Message, err *Error) {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) OnFnCallback(callback *atomosCallback) {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) OnWormhole(from ID, wormhole BaseAtomosWormhole) *Error {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) OnStopping(from ID, cancelled []uint64) *Error {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) OnIDsReleased() {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) String() string {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomosDeadlockMailboxBaseAtomos) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	//TODO implement me
	panic("implement me")
}
