package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"os"
	"sync"
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

const (
	testLogMaxSize = 1000
)

func clearTest() {
	sharedCosmosProcess = nil
	onceInitSharedCosmosProcess = sync.Once{}
}

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
	os.Mkdir("/tmp/test_atomos_app_logging", 0777)
	os.Mkdir("/tmp/test_atomos_app_run", 0777)
	os.Mkdir("/tmp/test_atomos_app_etc", 0777)
	return &Config{
		Cosmos:            "testCosmos",
		Node:              "testNode",
		NodeList:          nil,
		KeepaliveNodeList: nil,
		ReporterUrl:       "",
		ConfigerUrl:       "",
		LogLevel:          0,
		LogPath:           "/tmp/test_atomos_app_logging",
		LogMaxSize:        0,
		BuildPath:         "",
		BinPath:           "",
		RunPath:           "/tmp/test_atomos_app_run",
		EtcPath:           "/tmp/test_atomos_app_etc",
		EnableCluster:     nil,
		Customize:         nil,
	}
}

func newTestAppLogging(t *testing.T) *appLogging {
	logging, err := NewAppLogging("/tmp/test_atomos_app_logging", testLogMaxSize)
	if err != nil {
		t.Errorf("AppLogging: err=(%v)", err)
		panic(err)
	}
	return logging
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
				return sharedTestAtom2.SyncMessagingByName(sharedTestAtom1, "testMessageDeadlockTarget", 0, nil)
			},
			"testMessageElementDeadlock": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				t.Logf("AtomHandlers: testMessageElementDeadlock. from=(%v),to=(%v),in=(%v)", from, to, in)
				return sharedTestElement1.SyncMessagingByName(sharedTestAtom1, "testMessageDeadlockTarget", 0, nil)
			},
			"testMessageDeadlockTarget": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				t.Logf("AtomHandlers: testMessageDeadlockTarget. from=(%v),to=(%v),in=(%v)", from, to, in)
				return sharedTestAtom1.SyncMessagingByName(sharedTestAtom2, "testMessageDeadlock", 0, nil)
			},
			"testTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				ta := to.(*testAtom)
				t.Logf("AtomHandlers: testTask. from=(%v),to=(%v),in=(%v)", from, to, in)
				// Append
				taskID, err := ta.self.Task().Add(func(taskID uint64) {
					ta.Tasking(taskID, nil)
				})
				if err != nil {
					ta.self.Log().Error("AtomHandlers: testTask, Append failed. err=(%v)", err)
					return nil, err
				}
				// Append then cancel
				taskID, err = ta.self.Task().Add(func(taskID uint64) {
					ta.Tasking(taskID, nil)
				})
				if err != nil {
					ta.self.Log().Error("AtomHandlers: testTask, Append failed. err=(%v)", err)
					return nil, err
				}
				err = ta.self.Task().Cancel(taskID)
				t.Logf("AtomHandlers: testTask, Cancel task, taskID=(%v),err=(%v)", taskID, err)
				if err != nil {
					ta.self.Log().Error("AtomHandlers: testTask, Append then cancel failed. err=(%v)", err)
					return nil, err
				}
				// Timer
				taskID, err = ta.self.Task().AddAfter(1*time.Millisecond, func(taskID uint64) {
					ta.Tasking(taskID, nil)
				})
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
				_, err = ta.self.Task().Add(func(taskID uint64) {
					ta.TaskingPanic(taskID, nil)
				})
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
				return sharedTestAtom1.SyncMessagingByName(sharedTestElement1, "testMessageElementDeadlock", 0, nil)
			},
			"testTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				ta := to.(*testElement)
				t.Logf("AtomHandlers: testTask. from=(%v),to=(%v),in=(%v)", from, to, in)
				// Append
				taskID, err := ta.self.Task().Add(func(taskID uint64) {
					ta.Tasking(taskID, nil)
				})
				if err != nil {
					ta.self.Log().Error("AtomHandlers: testTask, Append failed. err=(%v)", err)
					return nil, err
				}
				// Append then cancel
				taskID, err = ta.self.Task().Add(func(taskID uint64) {
					ta.Tasking(taskID, nil)
				})
				if err != nil {
					ta.self.Log().Error("AtomHandlers: testTask, Append failed. err=(%v)", err)
					return nil, err
				}
				err = ta.self.Task().Cancel(taskID)
				t.Logf("AtomHandlers: testTask, Cancel task, canceled=(%v),err=(%v)", taskID, err)
				if err != nil {
					ta.self.Log().Error("AtomHandlers: testTask, Append then cancel failed. err=(%v)", err)
					return nil, err
				}
				// Timer
				taskID, err = ta.self.Task().AddAfter(1*time.Millisecond, func(taskID uint64) {
					ta.Tasking(taskID, nil)
				})
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
				_, err = ta.self.Task().Add(func(taskID uint64) {
					ta.TaskingPanic(taskID, nil)
				})
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
		ScaleHandlers: map[string]ScaleHandler{
			"ScaleTestMessage": func(from ID, to Atomos, message string, in proto.Message) (id ID, err *Error) {
				return sharedTestAtom1, nil
			},
			"ScaleTestMessageError": func(from ID, to Atomos, message string, in proto.Message) (id ID, err *Error) {
				return sharedTestAtom1, NewError(ErrFrameworkRecoverFromPanic, "Scale ID failed")
			},
			"ScaleTestMessagePanic": func(from ID, to Atomos, message string, in proto.Message) (id ID, err *Error) {
				panic("Get scale ID panic")
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

func (t *testElementDev) Load(self ElementSelfID, config map[string][]byte) *Error {
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

func (t *testElementAutoDataDev) AtomAutoDataPersistence() AtomAutoData {
	return t
}

func (t *testElementAutoDataDev) GetAtomData(name string) (proto.Message, *Error) {
	switch name {
	case "get_data_error":
		return nil, NewError(ErrFrameworkRecoverFromPanic, "").AddStack(nil)
	case "get_data_panic":
		panic("Data panic")
	}
	return &String{S: name}, nil
}

func (t *testElementAutoDataDev) SetAtomData(name string, data proto.Message) *Error {
	switch name {
	case "set_data_error":
		return NewError(ErrFrameworkRecoverFromPanic, "").AddStack(nil)
	case "set_data_panic":
		panic("Data panic")
	}
	if data.(*String).S == "data_ok" {
		setAtomCounter += 1
	}
	return nil
}

func (t *testElementAutoDataDev) ElementAutoDataPersistence() ElementAutoData {
	return t
}

func (t *testElementAutoDataDev) GetElementData() (proto.Message, *Error) {
	if testElementGetDataError {
		return nil, NewError(ErrFrameworkRecoverFromPanic, "Get Element Data Error").AddStack(nil)
	}
	if testElementGetDataPanic {
		panic("Get Element Data Panic")
	}
	return &String{S: "ElementData"}, nil
}

func (t *testElementAutoDataDev) SetElementData(data proto.Message) *Error {
	if testElementSetDataError {
		return NewError(ErrFrameworkRecoverFromPanic, "Set Element Data Error").AddStack(nil).AddStack(nil)
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

func (t *testAtom) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	t.t.Logf("Stopping: from=(%v),cancelled=(%v)", from, cancelled)
	if t.self.GetIDInfo().Atom == "stopping_panic" {
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

func (t testElement) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
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

func testAppUDSServer(t *testing.T) (*appUDSServer, *Error) {
	s := &appUDSServer{
		config:         newTestFakeCosmosMainConfig(),
		logging:        newTestAppLogging(t),
		addr:           nil,
		listener:       nil,
		mutex:          sync.Mutex{},
		connID:         0,
		connMap:        map[int32]*AppUDSConn{},
		commandHandler: udsNodeCommandHandler,
	}
	if err := s.check(); err != nil {
		t.Errorf("testAppUDSServer: Check failed. err=(%v)", err.AddStack(nil))
		return nil, err
	}
	if err := s.daemon(); err != nil {
		t.Errorf("testAppUDSServer: Daemon failed. err=(%v)", err)
		return nil, err.AddStack(nil)
	}
	file, er := s.listener.File()
	if er != nil {
		t.Errorf("testAppUDSServer: Socket error. err=(%v)", er)
		return nil, NewErrorf(ErrFrameworkRecoverFromPanic, "Socket file error. err=(%v)", er).AddStack(nil)
	}
	t.Logf("TestAppSocketListener: File=(%v)", file)
	return s, nil
}

func testAppSocketClient(t *testing.T) *AppUDSClient {
	return &AppUDSClient{
		logging: sharedLogging.PushProcessLog,
		mutex:   sync.Mutex{},
		connID:  0,
		connMap: map[int32]*AppUDSConn{},
	}
}
