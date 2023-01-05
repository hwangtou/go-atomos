package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

// Fake Data

var (
	setAtomCounter          = 0
	testElementLoadPanic    = false
	testElementHaltPanic    = false
	testElementUnloadPanic  = false
	testElementGetDataError = false
	testElementSetDataError = false
	testElementGetDataPanic = false
	testElementSetDataPanic = false
	testRecoverPanic        = false
)

func initTestFakeCosmosProcess(t *testing.T) {
	accessLog := func(s string) { t.Logf(s) }
	errorLog := func(s string) { t.Logf(s) }
	InitCosmosProcess(accessLog, errorLog)
}

func initTestFakeCosmosProcessBenchmark(b *testing.B) {
	accessLog := func(s string) { b.Logf(s) }
	errorLog := func(s string) { b.Logf(s) }
	InitCosmosProcess(accessLog, errorLog)
}

func newTestFakeRunnable(t *testing.T, autoData bool) *CosmosRunnable {
	runnable := &CosmosRunnable{}
	runnable.
		SetConfig(newTestFakeCosmosMainConfig()).
		SetMainScript(&testMainScript{t: t}).
		AddElementImplementation(newTestFakeElement(t, autoData))
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

func newTestFakeElement(t *testing.T, autoData bool) *ElementImplementation {
	var dev ElementDeveloper
	if autoData {
		dev = &testElementAutoDataDev{}
	} else {
		dev = &testElementDev{}
	}
	impl := &ElementImplementation{
		Developer: dev,
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
				ta := a.(*testAtom)
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
				time.Sleep(5 * time.Millisecond)
				return &String{S: "OK"}, nil
			},
			"testMessageDeadlock": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				t.Logf("AtomHandlers: testMessageDeadlock. from=(%v),to=(%v),in=(%v)", from, to, in)
				return sharedTestAtom2.pushMessageMail(sharedTestAtom1, "testMessageDeadlockTarget", 0, nil)
			},
			"testMessageElementDeadlock": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				t.Logf("AtomHandlers: testMessageElementDeadlock. from=(%v),to=(%v),in=(%v)", from, to, in)
				return sharedTestElement1.pushMessageMail(sharedTestAtom1, "testMessageDeadlockTarget", 0, nil)
			},
			"testMessageDeadlockTarget": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				t.Logf("AtomHandlers: testMessageDeadlockTarget. from=(%v),to=(%v),in=(%v)", from, to, in)
				return sharedTestAtom1.pushMessageMail(sharedTestAtom2, "testMessageDeadlock", 0, nil)
			},
			"testTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				ta := to.(*testAtom)
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
				ta := to.(*testAtom)
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
				tmp := sharedTestAtom1
				tmp.Parallel(func() {
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
				return sharedTestAtom1.pushMessageMail(sharedTestElement1, "testMessageElementDeadlock", 0, nil)
			},
			"testTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				ta := to.(*testElement)
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
				ta := to.(*testElement)
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
				tmp := sharedTestElement1
				tmp.Parallel(func() {
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
	}
	return impl
}

// Dev

type testElementDev struct {
}

func (t *testElementDev) AtomConstructor(name string) Atomos {
	return &testAtom{}
}

func (t *testElementDev) ElementConstructor() Atomos {
	return &testElement{}
}

func (t *testElementDev) Load(self ElementSelfID, config map[string]string) *Error {
	if testElementLoadPanic {
		panic("Test Element Load Panic")
	}
	return nil
}

func (t *testElementDev) Unload() *Error {
	if testElementUnloadPanic {
		panic("Test Element Unload Panic")
	}
	return nil
}

type testElementAutoDataDev struct {
}

func (t testElementAutoDataDev) AtomConstructor(name string) Atomos {
	return &testAtom{}
}

func (t testElementAutoDataDev) ElementConstructor() Atomos {
	return &testElement{}
}

func (t *testElementAutoDataDev) AtomAutoDataPersistence() AtomAutoDataPersistence {
	return t
}

func (t *testElementAutoDataDev) GetAtomData(name string) (proto.Message, *Error) {
	switch name {
	case "get_data_error":
		return nil, NewError(ErrFrameworkPanic, "").AddStack(nil)
	case "get_data_panic":
		panic("Data panic")
	}
	return &String{S: name}, nil
}

func (t *testElementAutoDataDev) SetAtomData(name string, data proto.Message) *Error {
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

func (t *testElementAutoDataDev) ElementAutoDataPersistence() ElementAutoDataPersistence {
	return t
}

func (t *testElementAutoDataDev) GetElementData() (proto.Message, *Error) {
	if testElementGetDataError {
		return nil, NewError(ErrFrameworkPanic, "Get Element Data Error")
	}
	if testElementGetDataPanic {
		panic("Get Element Data Panic")
	}
	return &String{S: "ElementData"}, nil
}

func (t *testElementAutoDataDev) SetElementData(data proto.Message) *Error {
	if testElementSetDataError {
		return NewError(ErrFrameworkPanic, "Set Element Data Error")
	}
	if testElementSetDataPanic {
		panic("Set Element Data Panic")
	}
	return nil
}

// Test Atoms

type testAtom struct {
	t    *testing.T
	self SelfID
	data proto.Message
}

func (t *testAtom) ParallelRecover(err *Error) {
	if testRecoverPanic {
		panic("ParallelRecover panic")
	}
}

func (t *testAtom) SpawnRecover(arg proto.Message, err *Error) {
	if testRecoverPanic {
		panic("SpawnRecover panic")
	}
}

func (t *testAtom) MessageRecover(name string, arg proto.Message, err *Error) {
	if testRecoverPanic {
		panic("MessageRecover panic")
	}
}

func (t *testAtom) ScaleRecover(name string, arg proto.Message, err *Error) {
	if testRecoverPanic {
		panic("ScaleRecover panic")
	}
}

func (t *testAtom) TaskRecover(taskID uint64, name string, arg proto.Message, err *Error) {
	if testRecoverPanic {
		panic("TaskRecover panic")
	}
}

func (t *testAtom) StopRecover(err *Error) {
	if testRecoverPanic {
		panic("StopRecover panic")
	}
}

func (t testAtom) String() string {
	return "testAtom"
}

func (t *testAtom) Halt(from ID, cancelled map[uint64]CancelledTask) (save bool, data proto.Message) {
	t.t.Logf("Stopping: from=(%v),cancelled=(%v)", from, cancelled)
	if t.self.GetName() == "stopping_panic" {
		panic("Halt panic")
	}
	return t.data != nil, t.data
}

func (t *testAtom) Tasking(id uint64, _ proto.Message) {
	t.t.Logf("Tasking: taskID=(%d)", id)
	time.Sleep(1 * time.Millisecond)
}

func (t *testAtom) TaskingPanic(_ uint64, _ proto.Message) {
	panic("task panic")
}

// Task Element

type testElement struct {
	t    *testing.T
	self ElementSelfID
}

func (t *testElement) ParallelRecover(err *Error) {
	if testRecoverPanic {
		panic("ParallelRecover panic")
	}
}

func (t *testElement) SpawnRecover(arg proto.Message, err *Error) {
	if testRecoverPanic {
		panic("SpawnRecover panic")
	}
}

func (t *testElement) MessageRecover(name string, arg proto.Message, err *Error) {
	if testRecoverPanic {
		panic("MessageRecover panic")
	}
}

func (t *testElement) ScaleRecover(name string, arg proto.Message, err *Error) {
	if testRecoverPanic {
		panic("ScaleRecover panic")
	}
}

func (t *testElement) TaskRecover(taskID uint64, name string, arg proto.Message, err *Error) {
	if testRecoverPanic {
		panic("TaskRecover panic")
	}
}

func (t *testElement) StopRecover(err *Error) {
	if testRecoverPanic {
		panic("StopRecover panic")
	}
}

func (t testElement) String() string {
	return "testElement"
}

func (t testElement) Halt(from ID, cancelled map[uint64]CancelledTask) (save bool, data proto.Message) {
	t.t.Logf("Stopping: from=(%v),cancelled=(%v)", from, cancelled)
	if testElementHaltPanic {
		panic("Element Halt Panic")
	}
	return false, nil
}

func (t *testElement) Tasking(id uint64, _ proto.Message) {
	t.t.Logf("Tasking: taskId=(%d)", id)
	time.Sleep(1 * time.Millisecond)
}

func (t *testElement) TaskingPanic(_ uint64, _ proto.Message) {
	panic("task panic")
}

func testElementSpawnPanic(s ElementSelfID, a Atomos, data proto.Message) *Error {
	ta := a.(*testElement)
	ta.self = s
	panic("Element Spawn Panic")
	return nil
}
