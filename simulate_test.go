package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

// Fake Data

func newTestFakeCosmosMain(_ *testing.T) *CosmosMain {
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
	return cosmosMain
}

func newTestFakeElement(t *testing.T) *ElementImplementation {
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
				ta := a.(*testElement)
				ta.t = t
				ta.self = s
				ta.t.Log("AtomSpawner")
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
			"testDeadLoop": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *ErrorInfo) {
				ta := to.(*testAtomos)
				ta.self.Log().Info("AtomHandlers: testDeadLoop, from=(%v),to=(%v),in=(%v)", from, to, in)
				return testAtom.pushMessageMail(testAtom, "testMessage", nil)
			},
		},
		ElementHandlers: map[string]MessageHandler{
			"testMessage": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *ErrorInfo) {
				to.(*testElement).self.Log().Info("AtomHandlers: testMessage, from=(%v),to=(%v),in=(%v)", from, to, in)
				return nil, nil
			},
			"testTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *ErrorInfo) {
				ta := to.(*testElement)
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
			"testDeadLoop": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *ErrorInfo) {
				ta := to.(*testElement)
				ta.self.Log().Info("AtomHandlers: testDeadLoop, from=(%v),to=(%v),in=(%v)", from, to, in)
				return testElem.pushMessageMail(testAtom, "testMessage", nil)
			},
		},
	}
	return impl
}

type testElementDev struct {
}

func (t testElementDev) AtomConstructor() Atomos {
	return &testAtomos{}
}

func (t testElementDev) ElementConstructor() Atomos {
	return &testElement{}
}

// Test Atoms

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

// Task Element

type testElement struct {
	t    *testing.T
	self ElementSelfID
}

func (t testElement) Description() string {
	return "testElement"
}

func (t testElement) Halt(from ID, cancelled map[uint64]CancelledTask) (save bool, data proto.Message) {
	return false, nil
}

func (t testElement) Reload(oldInstance Atomos) {
}

func (t *testElement) Tasking(id uint64, _ proto.Message) {
	t.self.Log().Info("Tasking: taskId=(%d)", id)
}
