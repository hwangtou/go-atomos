package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

func TestAtomLocal(t *testing.T) {
	e, current := newTestElement(t)
	// Spawn.
	atom := newAtomLocal("testAtom", e, e.atomos.reloads, current, e.atomos.logging, current.Interface.Config.LogLevel)
	atom.atomos.setSpawning()
	err := current.Interface.AtomSpawner(atom, atom.atomos.instance, nil, nil)
	if err != nil {
		t.Errorf("AtomSpawner: Failed, state=(%v),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	atom.atomos.setWaiting()
	// Push Message.
	reply, err := atom.pushMessageMail(nil, "testMessage", nil)
	if err != nil {
		t.Errorf("PushMessage: Failed, state=(%v),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	t.Logf("PushMessage: Succeed, state=(%v),reply=(%v)", atom.atomos.GetState(), reply)
	// Push Tasking.
	reply, err = atom.pushMessageMail(nil, "testTask", nil)
	if err != nil {
		t.Errorf("PushMessage: Failed, state=(%v),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	t.Logf("PushMessage: Succeed, state=(%v),reply=(%v)", atom.atomos.GetState(), reply)
	// Push Reload.
	t.Logf("PushReload: Begin, state=(%v),reload=(%d)", atom.atomos.GetState(), atom.atomos.reloads)
	err = atom.pushReloadMail(nil, current, 2)
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

func newTestElement(t *testing.T) (*ElementLocal, *ElementImplementation) {
	cosmosMain := &CosmosMain{
		config: &Config{
			Node:         "testNode",
			LogPath:      "",
			LogLevel:     0,
			EnableCert:   nil,
			EnableServer: nil,
			EnableTelnet: nil,
			Customize:    nil,
		},
		process: &CosmosProcess{
			sharedLog: NewLoggingAtomos(),
		},
	}
	impl := &ElementImplementation{
		Developer: &testElementDev{},
		Interface: &ElementInterface{
			Name: "testElement",
			Config: &ElementConfig{
				Name:        "testElement",
				Version:     0,
				LogLevel:    0,
				AtomInitNum: 0,
				Messages:    nil,
			},
			ElementSpawner: func(s ElementSelfID, a Atomos, arg, data proto.Message) *ErrorInfo {
				return nil
			},
			AtomSpawner: func(s SelfID, a Atomos, arg, data proto.Message) *ErrorInfo {
				ta := a.(*testAtomos)
				ta.t = t
				ta.self = s
				ta.t.Log("AtomSpawner")
				return nil
			},
			ElementIDConstructor: func(id ID) ID {
				return id
			},
			AtomIDConstructor: func(id ID) ID {
				return id
			},
			AtomMessages: nil,
		},
		AtomHandlers: map[string]MessageHandler{
			"testMessage": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *ErrorInfo) {
				to.(*testAtomos).self.Log().Info("AtomHandlers: testMessage, from=(%v),to=(%v),in=(%v)", from, to, in)
				return nil, nil
			},
			"testTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *ErrorInfo) {
				ta := to.(*testAtomos)
				ta.self.Log().Info("AtomHandlers: testTask, from=(%v),to=(%v),in=(%v)", from, to, in)
				// Append
				taskId, err := ta.self.Task().Append(ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("Append failed, err=(%v)", err)
					return nil, err
				}
				// Append then cancel
				taskId, err = ta.self.Task().Append(ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("Append failed, err=(%v)", err)
					return nil, err
				}
				canceled, err := ta.self.Task().Cancel(taskId)
				ta.self.Log().Info("Cancel task, canceled=(%v)", canceled)
				if err != nil {
					ta.self.Log().Error("Append then cancel failed, err=(%v)", err)
					return nil, err
				}
				// Timer
				taskId, err = ta.self.Task().AddAfter(1*time.Second, ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("AddAfter failed, err=(%v)", err)
					return nil, err
				}
				ta.self.Log().Info("AddAfter taskId=(%d)", taskId)
				return nil, nil
			},
		},
		ElementHandlers: nil,
	}
	return newElementLocal(cosmosMain, impl), impl
}

type testElementDev struct {
}

func (t testElementDev) AtomConstructor() Atomos {
	return &testAtomos{}
}

func (t testElementDev) ElementConstructor() Atomos {
	return &testElement{}
}

type testAtomos struct {
	t    *testing.T
	self SelfID
}

func (t testAtomos) Description() string {
	return "testAtom"
}

func (t *testAtomos) Halt(from ID, cancelled map[uint64]CancelledTask) (save bool, data proto.Message) {
	t.t.Logf("Halt: from=(%v),cancelled=(%v)", from, cancelled)
	return false, nil
}

func (t *testAtomos) Reload(oldInstance Atomos) {
	t.t = oldInstance.(*testAtomos).t
	t.t.Logf("Reload: oldInstance=(%v)", oldInstance)
}

func (t *testAtomos) Tasking(id uint64, _ proto.Message) {
	t.self.Log().Info("Tasking: taskId=(%d)", id)
}

type testElement struct {
}

func (t testElement) Description() string {
	return "testElement"
}

func (t testElement) Halt(from ID, cancelled map[uint64]CancelledTask) (save bool, data proto.Message) {
	return false, nil
}

func (t testElement) Reload(oldInstance Atomos) {
}
