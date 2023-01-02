package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

// Fake Data

var (
	setAtomCounter = 0
)

func initTestFakeCosmosProcess(t *testing.T) {
	accessLog := func(s string) { t.Logf(s) }
	errorLog := func(s string) { t.Logf(s) }
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
				t.Logf("ElementSpawner. data=(%v)", data)
				return nil
			},
			AtomSpawner: func(s AtomSelfID, a Atomos, arg, data proto.Message) *Error {
				ta := a.(*testAtomos)
				ta.t = t
				ta.self = s
				ta.data = data
				t.Logf("AtomSpawner. arg=(%v),data=(%v)", arg, data)
				if arg != nil {
					switch arg.(*String).S {
					case "panic":
						panic("Spawn panic")
					}
				}
				return nil
			},
		},
		AtomHandlers: map[string]MessageHandler{
			"testMessage": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				t.Logf("AtomHandlers: testMessage. from=(%v),to=(%v),in=(%v)", from, to, in)
				return &String{S: "OK"}, nil
			},
			"testMessageTimeout": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				t.Logf("AtomHandlers: testMessageTimeout. from=(%v),to=(%v),in=(%v)", from, to, in)
				time.Sleep(2 * time.Millisecond)
				return &String{S: "OK"}, nil
			},
			"testMessageDeadlock": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				t.Logf("AtomHandlers: testMessageDeadlock. from=(%v),to=(%v),in=(%v)", from, to, in)
				return sharedTestAtom2.pushMessageMail(sharedTestAtom1, "testMessageDeadlockTarget", 0, nil)
			},
			"testMessageDeadlockTarget": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				t.Logf("AtomHandlers: testMessageDeadlockTarget. from=(%v),to=(%v),in=(%v)", from, to, in)
				return sharedTestAtom1.pushMessageMail(sharedTestAtom2, "testMessageDeadlock", 0, nil)
			},
			"testTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				ta := to.(*testAtomos)
				t.Logf("AtomHandlers: testTask. from=(%v),to=(%v),in=(%v)", from, to, in)
				// Append
				taskID, err := ta.self.Task().Append(ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("AtomHandlers: testTask, Append failed. err=(%v)", err)
					return nil, err
				}
				// Append then cancel
				taskID, err = ta.self.Task().Append(ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("AtomHandlers: testTask, Append failed. err=(%v)", err)
					return nil, err
				}
				canceled, err := ta.self.Task().Cancel(taskID)
				t.Logf("AtomHandlers: testTask, Cancel task, canceled=(%v)", canceled)
				if err != nil {
					ta.self.Log().Error("AtomHandlers: testTask, Append then cancel failed. err=(%v)", err)
					return nil, err
				}
				// Timer
				taskID, err = ta.self.Task().AddAfter(1*time.Millisecond, ta.Tasking, nil)
				if err != nil {
					ta.self.Log().Error("AtomHandlers: testTask, AddAfter failed. err=(%v)", err)
					return nil, err
				}
				t.Logf("AtomHandlers: testTask, AddAfter taskID=(%d)", taskID)
				return &String{S: "OK"}, nil
			},
			"testTaskPanic": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				ta := to.(*testAtomos)
				t.Logf("AtomHandlers: testTaskPanic. from=(%v),to=(%v),in=(%v)", from, to, in)
				// Append
				_, err = ta.self.Task().Append(ta.TaskingPanic, nil)
				if err != nil {
					ta.self.Log().Error("AtomHandlers: testTaskPanic, Append failed. err=(%v)", err)
					return nil, err
				}
				return &String{S: "OK"}, nil
			},
			"testParallel": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				sharedTestAtom1.Parallel(func() {
					panic("parallel panic")
				})
				return &String{S: "OK"}, nil
			},
			"testPanic": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				panic("test panic")
			},
			"testKillSelf": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				sharedTestAtom1.KillSelf()
				return &String{S: "OK"}, nil
			},
		},
		ElementHandlers: map[string]MessageHandler{
			"testMessage": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				t.Logf("ElementHandlers: testMessage, from=(%v),to=(%v),in=(%v)", from, to, in)
				return nil, nil
			},
			"testTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				ta := to.(*testElement)
				t.Logf("ElementHandlers: testTask, from=(%v),to=(%v),in=(%v)", from, to, in)
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
				t.Logf("Cancel task, canceled=(%v)", canceled)
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
				t.Logf("AddAfter taskID=(%d)", taskID)
				return nil, nil
			},
			"testDeadLoop": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				panic("")
			},
			"testPanic": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				panic("test panic")
			},
		},
	}
	return impl
}

type testElementDev struct {
	autoData bool
}

func (t testElementDev) AtomConstructor(name string) Atomos {
	return &testAtomos{}
}

func (t testElementDev) ElementConstructor() Atomos {
	return &testElement{}
}

func (t *testElementDev) AtomAutoDataPersistence() AtomAutoDataPersistence {
	if !t.autoData {
		return nil
	}
	return t
}

func (t *testElementDev) GetAtomData(name string) (proto.Message, *Error) {
	switch name {
	case "get_data_error":
		return nil, NewError(ErrFrameworkPanic, "").AddStack(nil)
	case "get_data_panic":
		panic("Data panic")
	}
	return &String{S: name}, nil
}

func (t *testElementDev) SetAtomData(name string, data proto.Message) *Error {
	switch name {
	case "set_data_error":
		return NewError(ErrFrameworkPanic, "").AddStack(nil)
	case "set_data_panic":
		panic("Data panic")
	}
	if data.(*String).S == "data_ok" {
		setAtomCounter += 1
	}
	return nil
}

func (t *testElementDev) ElementAutoDataPersistence() ElementAutoDataPersistence {
	if !t.autoData {
		return nil
	}
	return t
}

func (t *testElementDev) GetElementData() (proto.Message, *Error) {
	//TODO implement me
	panic("implement me")
}

func (t *testElementDev) SetElementData(data proto.Message) *Error {
	//TODO implement me
	panic("implement me")
}

// Test Atoms

type testAtomos struct {
	t    *testing.T
	self SelfID
	data proto.Message
}

func (t *testAtomos) ParallelRecover(err *Error) {
	//TODO implement me
	panic("ParallelRecover panic")
}

func (t *testAtomos) SpawnRecover(arg proto.Message, err *Error) {
	//TODO implement me
	panic("SpawnRecover panic")
}

func (t *testAtomos) MessageRecover(name string, arg proto.Message, err *Error) {
	//TODO implement me
	panic("MessageRecover panic")
}

func (t *testAtomos) ScaleRecover(name string, arg proto.Message, err *Error) {
	//TODO implement me
	panic("ScaleRecover panic")
}

func (t *testAtomos) TaskRecover(taskID uint64, name string, arg proto.Message, err *Error) {
	//TODO implement me
	panic("TaskRecover panic")
}

func (t *testAtomos) StopRecover(err *Error) {
	//TODO implement me
	panic("StopRecover panic")
}

func (t testAtomos) String() string {
	return "testAtom"
}

func (t *testAtomos) Halt(from ID, cancelled map[uint64]CancelledTask) (save bool, data proto.Message) {
	t.t.Logf("Stopping: from=(%v),cancelled=(%v)", from, cancelled)
	if t.self.GetName() == "stopping_panic" {
		panic("Halt panic")
	}
	return t.data != nil, t.data
}

func (t *testAtomos) Tasking(id uint64, _ proto.Message) {
	t.t.Logf("Tasking: taskID=(%d)", id)
	time.Sleep(1 * time.Millisecond)
}

func (t *testAtomos) TaskingPanic(_ uint64, _ proto.Message) {
	panic("task panic")
}

// Task Element

type testElement struct {
	t    *testing.T
	self ElementSelfID
}

func (t testElement) String() string {
	return "testElement"
}

func (t testElement) Halt(from ID, cancelled map[uint64]CancelledTask) (save bool, data proto.Message) {
	t.t.Logf("Stopping: from=(%v),cancelled=(%v)", from, cancelled)
	return false, nil
}

func (t *testElement) Tasking(id uint64, _ proto.Message) {
	t.t.Logf("Tasking: taskId=(%d)", id)
}
