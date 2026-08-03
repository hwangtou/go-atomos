package atomos

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// TestBaseAtomos_LifeCycle

func TestBaseAtomos_LifeCycle(t *testing.T) {
	id := &IDInfo{
		Type:    IDType_Atom,
		Cosmos:  "test_cosmos",
		Node:    "test_node",
		Element: "test_element",
		Atom:    "test_atomos",
		Version: 0,
	}
	process := newTestCosmosProcessWithoutCluster(t, id.Cosmos, id.Node)
	tba := newBaseAtomosForTest(t, id, process)

	// Base test
	if !proto.Equal(tba.GetIDInfo(), id) {
		t.Fatalf("BaseAtomos IDInfo incorrect: %v", tba.GetIDInfo())
	}
	if tba.GetIDInfo().String() != id.String() {
		t.Fatalf("BaseAtomos String incorrect: %v %v", tba.GetIDInfo().String(), id.String())
	}
	if tba.atomos.GetInstance() != tba.instance {
		t.Fatalf("BaseAtomos instance incorrect: %v", tba.atomos.GetInstance())
	}
	if tba.atomos.GetGoID() == 0 {
		t.Fatalf("BaseAtomos GoID incorrect: %v", tba.atomos.GetGoID())
	}
	t.Log("Base test passed")

	tba.atomos.log.logging.PushLogging(id, LogLevel_Debug, "BaseAtomos test")
	taskDone := make(chan bool)
	tba.atomos.task.AddToAtomosQueue(func(taskID uint64) {
		t.Log("BaseAtomos task executed:", taskID)
		taskDone <- true
	})
	<-taskDone
	t.Log("BaseAtomos task test passed")

	// Sync Messaging test
	out, err := tba.SyncMessagingByName(process.local,
		testCaseBaseAtomosSyncMessaging1Name,
		&Strings{Ss: []string{testCaseBaseAtomosSyncMessaging1In}},
		nil)
	if err != nil {
		t.Fatalf("Failed to start BaseAtomos: %v", err)
	}
	if out.(*Strings).Ss[0] != testCaseBaseAtomosSyncMessaging1Out {
		t.Fatalf("Incorrect output from BaseAtomos: %v", out)
	}
	t.Log("Sync Messaging test passed")

	// Async Messaging test
	asyncDone := make(chan bool)
	err = tba.AsyncMessagingByName(process.local,
		testCaseBaseAtomosAsyncMessaging1Name,
		&Strings{Ss: []string{testCaseBaseAtomosAsyncMessaging1In}},
		func(out proto.Message, err *Error) {
			if err != nil {
				t.Errorf("Async Messaging returned error: %v", err)
			} else if out.(*Strings).Ss[0] != testCaseBaseAtomosAsyncMessaging1Out {
				t.Errorf("Incorrect output from Async Messaging: %v", out)
			} else {
				t.Log("Async Messaging output correct")
			}
			asyncDone <- true
		},
		nil)
	if err != nil {
		t.Fatalf("Failed to start Async Messaging: %v", err)
	}
	<-asyncDone
	t.Log("Async Messaging test passed")

	// Kill
	if err := tba.Kill(process.local,
		[]ArgsForBaseAtomos{ArgBaseAtomosWaitKilled()}); err != nil {
		t.Fatalf("Failed to kill BaseAtomos: %v", err)
	}
	if tba.State() != BaseAtomosHalt {
		t.Fatalf("BaseAtomos did not halt properly, current state: %v", tba.State())
	}
	t.Log("Kill test passed")
}

// BaseAtomos -> BaseAtomos

func TestBaseAtomos_SyncToBaseAtomos(t *testing.T) {

}

// BaseAtomos -> BaseRemote

// internal

func newBaseAtomosForTest(t *testing.T, id *IDInfo, process *CosmosProcess) *testBaseAtomos {
	tba := &testBaseAtomos{
		t:       t,
		process: process,
	}
	ti := &testInstance{}
	ba := NewBaseAtomos(tba, id, LogLevel_Debug, tba, ti, process)
	tba.atomos = ba
	tba.instance = ti
	if err := tba.atomos.start(func() *Error {
		return nil
	}); err != nil {
		t.Fatalf("Failed to start BaseAtomos: %v", err)
	}
	return tba
}

type testBaseAtomos struct {
	t        *testing.T
	process  *CosmosProcess
	atomos   *BaseAtomos
	instance *testInstance
}

const (
	testCaseBaseAtomosSyncMessaging1Name = "TestCaseBaseAtomos_SyncMessaging_1"
	testCaseBaseAtomosSyncMessaging1In   = "TestCaseBaseAtomos_SyncMessaging_1: Hello to BaseAtomos!"
	testCaseBaseAtomosSyncMessaging1Out  = "TestCaseBaseAtomos_SyncMessaging_1: Hello from BaseAtomos!"

	testCaseBaseAtomosAsyncMessaging1Name = "TestCaseBaseAtomos_AsyncMessaging_1"
	testCaseBaseAtomosAsyncMessaging1In   = "TestCaseBaseAtomos_AsyncMessaging_1: Hello to BaseAtomos!"
	testCaseBaseAtomosAsyncMessaging1Out  = "TestCaseBaseAtomos_AsyncMessaging_1: Hello from BaseAtomos!"
)

func (t *testBaseAtomos) GetIDInfo() *IDInfo {
	return t.atomos.GetIDInfo()
}

func (t *testBaseAtomos) String() string {
	return t.atomos.String()
}

func (t *testBaseAtomos) Cosmos() CosmosNode {
	return t.process.local
}

func (t *testBaseAtomos) State() BaseAtomosState {
	return t.atomos.GetState()
}

func (t *testBaseAtomos) IdleTime() time.Duration {
	return t.atomos.idleTime()
}

func (t *testBaseAtomos) SyncMessagingByName(callerID ID, name string, in proto.Message, ext []ArgsForBaseAtomos) (out proto.Message, err *Error) {
	return t.atomos.PushSyncMessage(callerID, name, in, ext)
}

func (t *testBaseAtomos) AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(out proto.Message, err *Error), ext []ArgsForBaseAtomos) (errBeforeExec *Error) {
	return t.atomos.PushAsyncMessage(callerID, name, in, callback, ext)
}

func (t *testBaseAtomos) asyncSet(callback func(out proto.Message, err *Error)) (startupID, callbackID uint64) {
	t.t.Log("asyncSet called")
	return t.atomos.asyncSet(callback)
}

func (t *testBaseAtomos) asyncCallback(callerID ID, name string, startupID, asyncID uint64, reply proto.Message, err *Error) {
	t.t.Log("asyncCallback called:", callerID, name, asyncID, reply, err)
	t.atomos.PushAsyncMessageCallback(callerID, name, startupID, asyncID, reply, err)
}

func (t *testBaseAtomos) DecoderByName(name string) (in, out MessageDecoder) {
	panic("not implemented")
}

func (t *testBaseAtomos) Kill(callerID ID, ext []ArgsForBaseAtomos) *Error {
	return t.atomos.PushKillMail(callerID, ext)
}

func (t *testBaseAtomos) SendWormhole(callerID ID, wormhole BaseAtomosWormhole, ext []ArgsForBaseAtomos) *Error {
	return t.atomos.PushWormholeMailAndWaitReply(callerID, wormhole, ext)
}

func (t *testBaseAtomos) getGoID() uint64 {
	return t.atomos.GetGoID()
}

func (t *testBaseAtomos) OnSyncMessaging(fromID ID, name string, in proto.Message) (out proto.Message, err *Error) {
	t.t.Log("Sync Messaging test received:", fromID, name, in)
	handler := func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
		switch name {
		case testCaseBaseAtomosSyncMessaging1Name:
			if in.(*Strings).Ss[0] != testCaseBaseAtomosSyncMessaging1In {
				panic("incorrect value")
			}
			return &Strings{Ss: []string{testCaseBaseAtomosSyncMessaging1Out}}, nil
		default:
			panic("not implemented" + name)
		}
	}
	return t.atomos.OnSyncMessaging(fromID, name, handler, in)
}

func (t *testBaseAtomos) OnAsyncMessaging(fromID ID, name string, startupID, asyncID uint64, in proto.Message) {
	t.t.Log("Async Messaging test received:", fromID, name, asyncID, in)
	handler := func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
		switch name {
		case testCaseBaseAtomosAsyncMessaging1Name:
			if in.(*Strings).Ss[0] != testCaseBaseAtomosAsyncMessaging1In {
				panic("incorrect value")
			}
			return &Strings{Ss: []string{testCaseBaseAtomosAsyncMessaging1Out}}, nil
		default:
			panic("not implemented" + name)
		}
	}
	t.atomos.OnAsyncMessaging(fromID, t, name, handler, startupID, asyncID, in)
}

func (t *testBaseAtomos) OnAsyncMessagingCallback(asyncID uint64, in proto.Message, err *Error) {
	t.t.Log("Async Messaging test received:", asyncID, in)
}

func (t *testBaseAtomos) OnFnCallback(callback *atomosCallback) {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomos) OnWormhole(from ID, wormhole BaseAtomosWormhole) *Error {
	//TODO implement me
	panic("implement me")
}

func (t *testBaseAtomos) OnStopping(from ID, cancelled []uint64) *Error {
	return nil
}

type testInstance struct {
}

func (t *testInstance) String() string {
	//TODO implement me
	panic("implement me")
}

func (t *testInstance) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	//TODO implement me
	panic("implement me")
}
