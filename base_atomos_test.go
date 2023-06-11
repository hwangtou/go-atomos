package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"testing"
)

type testMainScript struct {
	t *testing.T
}

func (s *testMainScript) OnStartUp(local *CosmosProcess) *Error {
	//s.t.Log("testMainScript: LocalReady Begin")
	//panic("startup panic")
	//s.t.Log("testMainScript: LocalReady End")
	return nil
}

func (s *testMainScript) OnShutdown() *Error {
	//s.t.Log("testMainScript: Shutdown Begin")
	//panic("shutdown panic")
	//s.t.Log("testMainScript: Shutdown End")
	return nil
}

type TestAtomosHolder struct {
	T *testing.T
}

func (t *TestAtomosHolder) OnSyncMessagingCallback(in proto.Message, err *Error, callback func(reply proto.Message, err *Error)) {
	//TODO implement me
	panic("implement me")
}

func (t *TestAtomosHolder) OnMessaging(fromID ID, firstSyncCall, name string, in proto.Message) (out proto.Message, err *Error) {
	t.T.Logf("OnMessage: from=(%v),state=(%v),name=(%s),args=(%v)", fromID, a.state, name, in)
	func() {
		defer func() {
			if r := recover(); r != nil {
				if err == nil {
					err = NewErrorf(ErrFrameworkRecoverFromPanic, "Atom: Messaging recovers from panic.").AddPanicStack(nil, 1, r)
				}
			}
		}()
		switch name {
		case "panic":
			_ = (*TestAtomosHolder)(nil).T
		}
	}()
	return
}

func (t *TestAtomosHolder) OnScaling(from ID, firstSyncCall, name string, args proto.Message, tracker *IDTracker) (id ID, err *Error) {
	panic("not supported")
}

func (t *TestAtomosHolder) OnWormhole(from ID, wormhole AtomosWormhole) *Error {
	t.T.Logf("OnWormhole: wormhole=(%v)", wormhole)
	return nil
}

func (t *TestAtomosHolder) OnStopping(from ID, cancelled []uint64) *Error {
	t.T.Logf("OnStopping: from=(%v),state=(%v),cancelled=(%v)", from, a.state, cancelled)
	return nil
}

func (t *TestAtomosHolder) Spawn() {
	t.T.Logf("Spawn")
}

func (t *TestAtomosHolder) Set(message string) {
	t.T.Logf("Set %s", message)
}

func (t *TestAtomosHolder) Unset(message string) {
	t.T.Logf("Unset %s", message)
}

func (t *TestAtomosHolder) Stopping() {
	t.T.Logf("Stopping")
}

func (t *TestAtomosHolder) Halted() {
	t.T.Logf("Halted")
}

type TestAtomosInstance struct {
	T      *testing.T
	reload int
}

func (t *TestAtomosInstance) String() string {
	return "Description"
}

func (t *TestAtomosInstance) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	t.T.Logf("Stopping: from=(%v),cancelled=(%v)", from, cancelled)
	return true, nil
}

func (t *TestAtomosInstance) Reload(oldInstance Atomos) {
	t.T.Logf("Reload: state=(%v),reload=(%v)", a.state, oldInstance)
}

func (t *TestAtomosInstance) TestTask(taskID uint64, data proto.Message) {
	t.T.Logf("TestTask: state=(%v),taskID=(%d),data=(%v),reload=(%v)", a.state, taskID, data, t.reload)
}

var a *BaseAtomos

//func TestBaseAtomos(t *testing.T) {
//	id := &IDInfo{
//		Type:    IDType_Atom,
//		Cosmos:  "cosmos",
//		Element: "element",
//		Atom:    "atomos",
//	}
//	initTestFakeCosmosProcess(t)
//	time.Sleep(10 * time.Millisecond)
//	instance := &TestAtomosInstance{T: t, reload: 1}
//	holder := &TestAtomosHolder{T: t}
//	atom := NewBaseAtomos(id, LogLevel_Debug, holder, instance)
//	_ = atom.start(nil)
//	a = atom
//	// Push Message
//	reply, err := a.PushMessageMailAndWaitReply(nil, "", "message", 0, nil)
//	if err != nil {
//		t.Errorf("PushMessageMailAndWaitReply: reply=(%v),state=(%d),err=(%v)", reply, a.GetState(), err)
//		return
//	}
//	// Push Task
//	taskID, err := a.Task().AddAfter(0, func(taskID uint64) {
//		instance.TestTask(taskID, nil)
//	})
//	if err != nil {
//		t.Errorf("TaskAddAfter: taskID=(%v),state=(%d),err=(%v)", taskID, a.GetState(), err)
//		return
//	}
//	// Push Task
//	taskID, err = a.Task().AddAfter(1*time.Second, func(taskID uint64) {
//		instance.TestTask(taskID, nil)
//	})
//	if err != nil {
//		t.Errorf("TaskAddAfter: taskID=(%v),state=(%d),err=(%v)", taskID, a.GetState(), err)
//		return
//	}
//	// Push Wormhole
//	err = a.PushWormholeMailAndWaitReply(nil, "", 0, "wormhole_message")
//	if err != nil {
//		t.Errorf("PushWormholeMailAndWaitReply: err=(%v)", err)
//		return
//	}
//	// Push Message
//	reply, err = a.PushMessageMailAndWaitReply(nil, "", "message", 0, nil)
//	if err != nil {
//		t.Errorf("PushMessageMailAndWaitReply: reply=(%v),state=(%d),err=(%v)", reply, a.GetState(), err)
//		return
//	}
//	// Push Message Panic
//	reply, err = a.PushMessageMailAndWaitReply(nil, "", "panic", 0, nil)
//	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" {
//		t.Errorf("PushMessageMailAndWaitReply: reply=(%v),state=(%d),err=(%v)", reply, a.GetState(), err)
//		return
//	}
//	// Push Kill
//	err = a.PushKillMailAndWaitReply(nil, "", true, true, 0)
//	if err != nil {
//		t.Errorf("PushKillMailAndWaitReply: state=(%d),err=(%v)", a.GetState(), err)
//		return
//	}
//	// Push Message
//	reply, err = a.PushMessageMailAndWaitReply(nil, "", "send_after_halt", 0, nil)
//	if err == nil || err.Code != ErrAtomosIsNotRunning {
//		t.Errorf("PushMessageMailAndWaitReply: reply=(%v),state=(%d),err=(%v)", reply, a.GetState(), err)
//		return
//	}
//	time.Sleep(10 * time.Millisecond)
//}

// TODO
func TestBaseAtomosReferenceCount(t *testing.T) {
}

// TODO
func TestBaseAtomosTask(t *testing.T) {
}
