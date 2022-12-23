package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

// Fake Data

func initTestFakeCosmosProcess(t *testing.T) {
	accessLog := func(s string) { t.Logf(s) }
	errorLog := func(s string) { t.Errorf(s) }
	InitCosmosProcess(accessLog, errorLog)
}

func newTestFakeRunnable(t *testing.T) *CosmosRunnable {
	runnable := &CosmosRunnable{}
	runnable.
		SetConfig(newTestFakeCosmosMainConfig()).
		SetMainScript(&testMainScript{t: t}).
		AddElementImplementation(newTestFakeElement(t))
	return runnable
}

func newTestFakeCosmosMainConfig() *Config {
	return &Config{
		Node:         "testNode",
		LogPath:      "/tmp/atomos_test.log",
		LogLevel:     0,
		EnableCert:   nil,
		EnableServer: nil,
		EnableTelnet: nil,
		Customize:    nil,
	}
}

func newTestFakeElement(t *testing.T) *ElementImplementation {
	impl := &ElementImplementation{
		Developer: &testElementDev{},
		Interface: &ElementInterface{
			Config: &ElementConfig{
				Name:        "testElement",
				Version:     0,
				LogLevel:    0,
				AtomInitNum: 0,
				Messages:    nil,
			},
			ElementSpawner: func(s ElementSelfID, a Atomos, data proto.Message) *Error {
				ta := a.(*testElement)
				ta.t = t
				ta.self = s
				ta.self.Log().Info("ElementSpawner")
				return nil
			},
			AtomSpawner: func(s AtomSelfID, a Atomos, arg, data proto.Message) *Error {
				ta := a.(*testAtomos)
				ta.t = t
				ta.self = s
				ta.self.Log().Info("AtomSpawner")
				return nil
			},
			//AtomMessages: nil,
		},
		AtomHandlers: map[string]MessageHandler{
			"testMessage": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				to.(*testAtomos).self.Log().Info("AtomHandlers: testMessage, from=(%v),to=(%v),in=(%v)", from, to, in)
				return nil, nil
			},
			"testTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				ta := to.(*testAtomos)
				ta.self.Log().Info("AtomHandlers: testTask, from=(%v),to=(%v),in=(%v)", from, to, in)
				// Append
				taskID, err := ta.self.Task().Append(ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("Append failed, err=(%v)", err)
					return nil, err
				}
				// Append then cancel
				taskID, err = ta.self.Task().Append(ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("Append failed, err=(%v)", err)
					return nil, err
				}
				canceled, err := ta.self.Task().Cancel(taskID)
				ta.self.Log().Info("Cancel task, canceled=(%v)", canceled)
				if err != nil {
					ta.self.Log().Error("Append then cancel failed, err=(%v)", err)
					return nil, err
				}
				// Timer
				taskID, err = ta.self.Task().AddAfter(1*time.Second, ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("AddAfter failed, err=(%v)", err)
					return nil, err
				}
				ta.self.Log().Info("AddAfter taskID=(%d)", taskID)
				return nil, nil
			},
			"testDeadLoop": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				panic("")
				//ta := to.(*testAtomos)
				//ta.self.Log().Info("AtomHandlers: testDeadLoop, from=(%v),to=(%v),in=(%v)", from, to, in)
				//return testAtom.pushMessageMail(testAtom, "testMessage", nil)
			},
			"testPanic": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				panic("test panic")
			},
		},
		ElementHandlers: map[string]MessageHandler{
			"testMessage": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				to.(*testElement).self.Log().Info("ElementHandlers: testMessage, from=(%v),to=(%v),in=(%v)", from, to, in)
				return nil, nil
			},
			"testTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				ta := to.(*testElement)
				ta.self.Log().Info("ElementHandlers: testTask, from=(%v),to=(%v),in=(%v)", from, to, in)
				// Append
				taskID, err := ta.self.Task().Append(ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("Append failed, err=(%v)", err)
					return nil, err
				}
				// Append then cancel
				taskID, err = ta.self.Task().Append(ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("Append failed, err=(%v)", err)
					return nil, err
				}
				canceled, err := ta.self.Task().Cancel(taskID)
				ta.self.Log().Info("Cancel task, canceled=(%v)", canceled)
				if err != nil {
					ta.self.Log().Error("Append then cancel failed, err=(%v)", err)
					return nil, err
				}
				// Timer
				taskID, err = ta.self.Task().AddAfter(1*time.Second, ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("AddAfter failed, err=(%v)", err)
					return nil, err
				}
				ta.self.Log().Info("AddAfter taskID=(%d)", taskID)
				return nil, nil
			},
			"testDeadLoop": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				panic("")
				//ta := to.(*testElement)
				//ta.self.Log().Info("ElementHandlers: testDeadLoop, from=(%v),to=(%v),in=(%v)", from, to, in)
				//return testElem.pushMessageMail(testAtom, "testMessage", nil)
			},
			"testPanic": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				panic("test panic")
			},
		},
	}
	return impl
}

type testElementDev struct {
}

func (t testElementDev) AtomConstructor(name string) Atomos {
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
	t.self.Log().Info("Stopping: from=(%v),cancelled=(%v)", from, cancelled)
	return false, nil
}

func (t *testAtomos) Tasking(id uint64, _ proto.Message) {
	t.self.Log().Info("Tasking: taskID=(%d)", id)
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
	t.self.Log().Info("Stopping: from=(%v),cancelled=(%v)", from, cancelled)
	return false, nil
}

func (t *testElement) Tasking(id uint64, _ proto.Message) {
	t.self.Log().Info("Tasking: taskId=(%d)", id)
}
