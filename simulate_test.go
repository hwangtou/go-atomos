package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"
	"sync"
	"testing"
	"time"
)

// Fake Data

func newTestFakeCosmosProcess(_ *testing.T) *CosmosProcess {
	return &CosmosProcess{
		sharedLog: NewLoggingAtomos(),
	}
}

func newTestFakeCosmosMain(t *testing.T) *CosmosMain {
	cosmosMain := &CosmosMain{
		process:    newTestFakeCosmosProcess(t),
		runnable:   newTestFakeRunnable(t),
		mainKillCh: nil,
		atomos: &BaseAtomos{
			id: &IDInfo{
				state:         protoimpl.MessageState{},
				sizeCache:     0,
				unknownFields: nil,
				Type:          0,
				Cosmos:        "",
				Element:       "",
				Atomos:        "",
			},
			state:    0,
			logging:  nil,
			mailbox:  nil,
			holder:   nil,
			instance: nil,
			reloads:  0,
			task:     atomosTaskManager{},
			log:      atomosLoggingManager{},
		},
		elements:   nil,
		mutex:      sync.RWMutex{},
		listenCert: nil,
		clientCert: nil,
		callChain:  nil,
	}
	return cosmosMain
}

func newTestFakeRunnable(t *testing.T) *CosmosRunnable {
	runnable := &CosmosRunnable{
		config:         newTestFakeCosmosMainConfig(),
		interfaces:     nil,
		interfaceOrder: nil,
		implements:     nil,
		implementOrder: nil,
		mainScript: func(main *CosmosMain, killSignal chan bool) {
			main.Log().Info("Executing Main Script")
			defer main.Log().Info("Defer Main Script")

			elem, err := main.getElement("testElement")
			if err != nil {
				t.Errorf("MainScript: Get element failed, err=(%s)", err)
				return
			}
			main.Log().Info("MainScript: Get element, elem=(%v),err=(%v)", elem, err)

			// Push Element Message.
			reply, err := elem.pushMessageMail(nil, "testMessage", nil)
			if err != nil {
				t.Errorf("MainScript: TestMessage Failed, state=(%v),err=(%v)", elem.atomos.GetState(), err)
				return
			}
			main.Log().Info("MainScript: TestMessage Succeed, state=(%v),reply=(%v)", elem.atomos.GetState(), reply)

			// Spawn Atom.
			atom, err := elem.SpawnAtom("testAtomA", nil)
			if err != nil {
				t.Errorf("MainScript: Spawn failed, state=(%v),err=(%v)", elem.atomos.GetState(), err)
				return
			}
			atom.Release()

			main.Log().Info("Executing Main Script is waiting Kill")
			<-killSignal
			main.Log().Info("Executing Main Script has been killed")
		},
		mainLogLevel: 0,
		reloadScript: func(main *CosmosMain) {
			main.Log().Info("Executing Reload Script")
			defer main.Log().Info("Defer Reload Script")
		},
	}
	runnable.AddElementImplementation(newTestFakeElement(t))
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
			ElementSpawner: func(s ElementSelfID, a Atomos, data proto.Message) *ErrorInfo {
				ta := a.(*testElement)
				ta.t = t
				ta.self = s
				ta.self.Log().Info("ElementSpawner")
				return nil
			},
			AtomSpawner: func(s AtomSelfID, a Atomos, arg, data proto.Message) *ErrorInfo {
				ta := a.(*testAtomos)
				ta.t = t
				ta.self = s
				ta.self.Log().Info("AtomSpawner")
				return nil
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
				to.(*testElement).self.Log().Info("ElementHandlers: testMessage, from=(%v),to=(%v),in=(%v)", from, to, in)
				return nil, nil
			},
			"testTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *ErrorInfo) {
				ta := to.(*testElement)
				ta.self.Log().Info("ElementHandlers: testTask, from=(%v),to=(%v),in=(%v)", from, to, in)
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
				ta.self.Log().Info("ElementHandlers: testDeadLoop, from=(%v),to=(%v),in=(%v)", from, to, in)
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
	t.self.Log().Info("Halt: from=(%v),cancelled=(%v)", from, cancelled)
	return false, nil
}

func (t *testAtomos) Reload(oldInstance Atomos) {
	// Transform
	t.t = oldInstance.(*testAtomos).t
	t.self = oldInstance.(*testAtomos).self
	// Log
	t.self.Log().Info("Reload: oldInstance=(%v)", oldInstance)
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
	t.self.Log().Info("Halt: from=(%v),cancelled=(%v)", from, cancelled)
	return false, nil
}

func (t *testElement) Reload(oldInstance Atomos) {
	// Transform
	t.t = oldInstance.(*testElement).t
	t.self = oldInstance.(*testElement).self
	// Log
	t.self.Log().Info("Reload: oldInstance=(%v)", oldInstance)
}

func (t *testElement) Tasking(id uint64, _ proto.Message) {
	t.self.Log().Info("Tasking: taskId=(%d)", id)
}
