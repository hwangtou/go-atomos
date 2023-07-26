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

var localSuccessCounter int
var remoteSuccessCounter int

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
	InitCosmosProcess("", "", accessLog, errorLog)
}

func initTestFakeCosmosProcessBenchmark(b *testing.B) {
	accessLog := func(s string) { b.Logf(s) }
	errorLog := func(s string) { b.Logf(s) }
	InitCosmosProcess("", "", accessLog, errorLog)
}

func newTestFakeRunnable(t *testing.T, process *CosmosProcess, autoData bool) *CosmosRunnable {
	runnable := &CosmosRunnable{}
	runnable.
		SetConfig(newTestFakeCosmosMainConfig()).
		SetMainScript(&testMainScript{t: t}).
		AddElementImplementation(newTestFakeElement(t, process, autoData)).
		SetElementSpawn("testElement")
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

func newTestFakeElement(t *testing.T, process *CosmosProcess, autoData bool) *ElementImplementation {
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
				//t.Logf("ElementSpawner. data=(%v)", data)
				return nil
			},
			AtomSpawner: func(self AtomSelfID, a Atomos, arg, data proto.Message) *Error {
				ta := a.(*testAtom)
				ta.t = t
				ta.self = self
				ta.data = data
				t.Logf("AtomSpawner. arg=(%v),data=(%v)", arg, data)
				if arg != nil {
					switch ar := arg.(type) {
					case *String:
						switch ar.S {
						case "panic":
							panic("Spawn panic")
						}
					case *Strings:
						switch ar.Ss[0] {
						case "deadlock":
							//selfCosmos := self.(*AtomLocal).element.cosmosLocal.process
							//selfCosmos.cluster.remoteMutex.RLock()
							//c2cr1 := selfCosmos.cluster.remoteCosmos["c1"]
							//selfCosmos.cluster.remoteMutex.RUnlock()

							// SelfID
							aID, aT, err := self.Cosmos().CosmosGetAtomID(ar.Ss[1], ar.Ss[2])
							if err != nil {
								return err.AddStack(nil)
							}
							aT.Release()
							out, err := aID.SyncMessagingByName(self, "testMessage", 0, nil)
							self.Log().Info("Spawn deadlock=(%v),out=(%v),err=(%v)", aID, out, err)
							if err != nil {
								if err.Code == ErrIDFirstSyncCallDeadlock {
									return nil
								}
								return err.AddStack(nil)
							}
							return nil
						}
					}
				}
				return nil
			},
			ElementDecoders: nil,
			AtomDecoders:    nil,
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

			// 测试本地同步调用到自己的死锁，传入的from和to都是自己，in是自己的名字
			"testingLocalSyncSelfFirstSyncCallDeadlock": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				selfName := in.(*String).S
				selfAtom, selfAtomTracker, err := from.Cosmos().(*CosmosLocal).process.local.CosmosGetAtomID("testElement", selfName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				selfAtomLocal := selfAtom.(*AtomLocal)
				defer selfAtomTracker.Release()
				out, err = selfAtom.SyncMessagingByName(selfAtomLocal, "testMessage", 0, nil)
				if err == nil {
					return nil, NewError(ErrIDFirstSyncCallDeadlock, "expect first sync call deadlock").AddStack(nil)
				} else if err.Code != ErrIDFirstSyncCallDeadlock {
					return nil, err.AddStack(nil)
				}
				return &String{S: "OK"}, nil
			},

			// 测试本地异步调用到自己的情况，传入的from和to都是自己，in是自己的名字
			"testingLocalAsyncSelfFirstSyncCall": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				selfName := in.(*String).S
				selfAtom, selfAtomTracker, err := from.Cosmos().CosmosGetAtomID("testElement", selfName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				selfAtomLocal := selfAtom.(*AtomLocal)
				defer selfAtomTracker.Release()
				selfAtom.AsyncMessagingByName(selfAtomLocal, "testMessage", 0, &String{S: "in"}, func(out proto.Message, err *Error) {
					if selfAtomLocal.atomos.fsc.curFirstSyncCall == "" {
						selfAtomLocal.Log().Error("testingLocalAsyncSelfFirstSyncCall: curFirstSyncCall is empty")
						return
					}
					if getGoID() != selfAtomLocal.atomos.GetGoID() {
						selfAtomLocal.Log().Error("testingLocalAsyncSelfFirstSyncCall: goID not match")
						return
					}
					if err != nil {
						selfAtomLocal.Log().Error("testingLocalAsyncSelfFirstSyncCall: err=(%v)", err)
						return
					}
					if out.(*String).S != "OK" {
						selfAtomLocal.Log().Error("testingLocalAsyncSelfFirstSyncCall: out=(%v)", out)
						return
					}
					localSuccessCounter += 1
				})
				return &String{S: "OK"}, nil
			},

			// 测试本地同步调用外部的情况，传入的from是自己，to是外部的名字，in是自己的名字。不会出现死锁。
			"testingLocalSyncAndAsyncOtherFirstSyncCall": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				selfName, otherName := in.(*Strings).Ss[0], in.(*Strings).Ss[1]

				selfAtom, selfAtomTracker, err := from.Cosmos().CosmosGetAtomID("testElement", selfName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				selfAtomLocal := selfAtom.(*AtomLocal)
				defer selfAtomTracker.Release()

				otherAtom, otherAtomTracker, err := from.Cosmos().CosmosGetAtomID("testElement", otherName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				otherAtomLocal := otherAtom.(*AtomLocal)
				defer otherAtomTracker.Release()

				out, err = otherAtomLocal.SyncMessagingByName(selfAtomLocal, "testMessage", 0, nil)
				if err != nil {
					return nil, err.AddStack(nil)
				}

				selfAtom.AsyncMessagingByName(selfAtomLocal, "testMessage", 0, &String{S: "in"}, func(out proto.Message, err *Error) {
					if selfAtomLocal.atomos.fsc.curFirstSyncCall == "" {
						selfAtomLocal.Log().Error("testingLocalSyncAndAsyncOtherFirstSyncCall: curFirstSyncCall is empty")
						return
					}
					if getGoID() != selfAtomLocal.atomos.GetGoID() {
						selfAtomLocal.Log().Error("testingLocalSyncAndAsyncOtherFirstSyncCall: goID not match")
						return
					}
					if err != nil {
						selfAtomLocal.Log().Error("testingLocalSyncAndAsyncOtherFirstSyncCall: err=(%v)", err)
						return
					}
					if out.(*String).S != "OK" {
						selfAtomLocal.Log().Error("testingLocalSyncAndAsyncOtherFirstSyncCall: out=(%v)", out)
						return
					}
					localSuccessCounter += 1
				})

				out, err = otherAtomLocal.SyncMessagingByName(selfAtomLocal, "testMessage", 0, nil)
				if err != nil {
					return nil, err.AddStack(nil)
				}

				selfAtom.AsyncMessagingByName(selfAtomLocal, "testMessage", 0, &String{S: "in"}, func(out proto.Message, err *Error) {
					if selfAtomLocal.atomos.fsc.curFirstSyncCall == "" {
						selfAtomLocal.Log().Error("testingLocalSyncAndAsyncOtherFirstSyncCall: curFirstSyncCall is empty")
						return
					}
					if getGoID() != selfAtomLocal.atomos.GetGoID() {
						selfAtomLocal.Log().Error("testingLocalSyncAndAsyncOtherFirstSyncCall: goID not match")
						return
					}
					if err != nil {
						selfAtomLocal.Log().Error("testingLocalSyncAndAsyncOtherFirstSyncCall: err=(%v)", err)
						return
					}
					if out.(*String).S != "OK" {
						selfAtomLocal.Log().Error("testingLocalSyncAndAsyncOtherFirstSyncCall: out=(%v)", out)
						return
					}
					localSuccessCounter += 1
				})

				return &String{S: "OK"}, nil
			},

			// 测试本地同步调用外部，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
			"testingLocalSyncChainSelfFirstSyncCallDeadlock": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				selfName, otherName := in.(*Strings).Ss[0], in.(*Strings).Ss[1]

				selfAtom, selfAtomTracker, err := from.Cosmos().CosmosGetAtomID("testElement", selfName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				selfAtomLocal := selfAtom.(*AtomLocal)
				defer selfAtomTracker.Release()

				otherAtom, otherAtomTracker, err := from.Cosmos().CosmosGetAtomID("testElement", otherName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				otherAtomLocal := otherAtom.(*AtomLocal)
				defer otherAtomTracker.Release()

				out, err = otherAtomLocal.SyncMessagingByName(selfAtomLocal, "testingUtilLocalSyncChain", 0, in)
				if err == nil {
					return nil, NewError(ErrIDFirstSyncCallDeadlock, "expect first sync call deadlock")
				} else if err.Code != ErrIDFirstSyncCallDeadlock {
					return nil, err.AddStack(nil)
				}
				return &String{S: "OK"}, nil
			},

			// 测试本地异步调用外部，并回调到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。不会出现死锁。
			"testingLocalAsyncChainSelfFirstSyncCall": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				selfName, otherName := in.(*Strings).Ss[0], in.(*Strings).Ss[1]

				selfAtom, selfAtomTracker, err := from.Cosmos().CosmosGetAtomID("testElement", selfName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				selfAtomLocal := selfAtom.(*AtomLocal)
				defer selfAtomTracker.Release()

				otherAtom, otherAtomTracker, err := from.Cosmos().CosmosGetAtomID("testElement", otherName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				otherAtomLocal := otherAtom.(*AtomLocal)
				defer otherAtomTracker.Release()

				otherAtomLocal.AsyncMessagingByName(selfAtomLocal, "testingUtilLocalSyncChain", 0, in, func(out proto.Message, err *Error) {
					if selfAtomLocal.atomos.fsc.curFirstSyncCall == "" {
						selfAtomLocal.Log().Error("testingLocalAsyncChainSelfFirstSyncCall: curFirstSyncCall is empty")
						return
					}
					if getGoID() != selfAtomLocal.atomos.GetGoID() {
						selfAtomLocal.Log().Error("testingLocalAsyncChainSelfFirstSyncCall: goID not match")
						return
					}
					if err != nil {
						selfAtomLocal.Log().Error("testingLocalAsyncChainSelfFirstSyncCall: err=(%v)", err)
						return
					}
					if out.(*String).S != "OK" {
						selfAtomLocal.Log().Error("testingLocalAsyncChainSelfFirstSyncCall: out=(%v)", out)
						return
					}
					localSuccessCounter += 1
				})
				return &String{S: "OK"}, nil
			},

			// 测试本地同步执行任务，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
			"testingLocalTaskChainSelfFirstSyncCall": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				selfName, otherName := in.(*Strings).Ss[0], in.(*Strings).Ss[1]

				selfAtom, selfAtomTracker, err := from.Cosmos().CosmosGetAtomID("testElement", selfName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				selfAtomLocal := selfAtom.(*AtomLocal)
				defer selfAtomTracker.Release()

				otherAtom, otherAtomTracker, err := from.Cosmos().CosmosGetAtomID("testElement", otherName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				otherAtomLocal := otherAtom.(*AtomLocal)
				defer otherAtomTracker.Release()

				var id uint64
				id, err = selfAtomLocal.Task().Add(func(taskID uint64) {
					if id != taskID {
						selfAtomLocal.Log().Error("testingLocalSyncChainSelfFirstSyncCallDeadlock: taskID not match")
						return
					}
					if selfAtomLocal.atomos.fsc.curFirstSyncCall != "" {
						selfAtomLocal.Log().Error("testingLocalSyncChainSelfFirstSyncCallDeadlock: curFirstSyncCall is empty")
						return
					}
					if getGoID() != selfAtomLocal.atomos.GetGoID() {
						selfAtomLocal.Log().Error("testingLocalSyncChainSelfFirstSyncCallDeadlock: goID not match")
						return
					}
					out, err = otherAtomLocal.SyncMessagingByName(selfAtomLocal, "testingUtilLocalSyncChain", 0, in)
					if err == nil || err.Code != ErrIDFirstSyncCallDeadlock {
						selfAtomLocal.Log().Error("testingLocalSyncChainSelfFirstSyncCallDeadlock: err=(%v)", err)
						return
					}
					localSuccessCounter += 1
				})
				if err != nil {
					return nil, err.AddStack(nil)
				}
				return &String{S: "OK"}, nil
			},

			// 测试本地发送Wormhole，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
			"testingLocalWormholeChainSelfFirstSyncCall": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				// TODO TEST
				return &String{S: "OK"}, nil
			},

			// 测试本地同步Scale调用，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
			"testingLocalScaleChainSelfFirstSyncCall": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				// TODO TEST
				return &String{S: "OK"}, nil
			},

			// 测试本地同步Kill调用，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
			"testingLocalKillChainSelfFirstSyncCall": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				// TODO TEST
				return &String{S: "OK"}, nil
			},

			// 测试本地同步KillSelf调用，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
			"testingLocalKillSelfChainSelfFirstSyncCall": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				// TODO TEST
				return &String{S: "OK"}, nil
			},

			// 测试本地同步Spawn调用，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
			"testingLocalSpawnSelfChainSelfFirstSyncCall": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				// TODO TEST
				return &String{S: "OK"}, nil
			},

			// 测试工具方法
			"testingUtilLocalSyncChain": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				selfName, otherName := in.(*Strings).Ss[1], in.(*Strings).Ss[0]

				selfAtom, selfAtomTracker, err := from.Cosmos().CosmosGetAtomID("testElement", selfName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				selfAtomLocal := selfAtom.(*AtomLocal)
				defer selfAtomTracker.Release()

				otherAtom, otherAtomTracker, err := from.Cosmos().CosmosGetAtomID("testElement", otherName)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				otherAtomLocal := otherAtom.(*AtomLocal)
				defer otherAtomTracker.Release()

				return otherAtomLocal.SyncMessagingByName(selfAtomLocal, "testMessage", 0, nil)
			},

			// Remote First Sync Call

			// node1的测试任务
			"testingRemoteTask": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				selfCosmos := from.(*CosmosLocal).process
				selfCosmos.cluster.remoteMutex.RLock()
				c1cr2 := selfCosmos.cluster.remoteCosmos["c2"]
				selfCosmos.cluster.remoteMutex.RUnlock()

				testElem, testSelf, testTarget := in.(*Strings).Ss[0], in.(*Strings).Ss[1], in.(*Strings).Ss[2]

				// SelfID
				selfAtomID, selfTrackerID, err := selfCosmos.local.CosmosGetAtomID(testElem, testSelf)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				selfTrackerID.Release()
				selfID := selfAtomID.(*AtomLocal)

				// Remote Atom ID
				c1cr2e, err := c1cr2.getElement(in.(*Strings).Ss[0])
				if err != nil {
					return nil, err.AddStack(nil)
				}
				id2, t2, err := c1cr2e.GetAtomID(testTarget, nil, false)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				if id2 == nil || t2 != nil {
					return nil, NewError(ErrFrameworkInternalError, "expect id2 != nil && t2 == nil").AddStack(nil)
				}
				t2.Release()

				// Test Normal
				out, err = id2.SyncMessagingByName(selfID, "testMessage", 0, nil)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				if out.(*String).S != "OK" {
					return nil, NewError(ErrFrameworkInternalError, "expect out == OK").AddStack(nil)
				}

				// Test Timeout
				out, err = id2.SyncMessagingByName(selfID, "testMessageTimeout", 1*time.Millisecond, nil)
				if err == nil || err.Code != ErrAtomosPushTimeoutHandling {
					return nil, NewError(ErrFrameworkInternalError, "expect err != nil").AddStack(nil)
				}

				for i := 0; i < 3; i++ {
					// Test Sync deadlock
					out, err = id2.SyncMessagingByName(selfID, "testingRemoteSyncFirstSyncCallDeadlock", 5*time.Second, &Strings{Ss: []string{testTarget}})
					if err == nil || err.Code != ErrIDFirstSyncCallDeadlock {
						return nil, NewErrorf(ErrFrameworkInternalError, "expect err != nil. err=(%v)", err).AddStack(nil)
					}
				}

				for i := 0; i < 3; i++ {
					// Test Async and no deadlock
					id2.AsyncMessagingByName(selfID, "testingRemoteSyncFirstSyncCallDeadlock", 5*time.Second, &Strings{Ss: []string{testTarget, "mark1"}}, func(out proto.Message, err *Error) {
						if err != nil {
							selfID.Log().Error("testingRemoteSyncFirstSyncCallDeadlock. err=(%v)", err)
							return
						}
						selfID.Log().Info("testingRemoteSyncFirstSyncCallDeadlock. out=(%v)", out)
						remoteSuccessCounter += 1
					})
				}

				for i := 0; i < 3; i++ {
					// Test Sync deadlock
					out, err = id2.SyncMessagingByName(selfID, "testingRemoteSyncFirstSyncCallDeadlock", 5*time.Second, &Strings{Ss: []string{testTarget, "mark2"}})
					if err == nil || err.Code != ErrIDFirstSyncCallDeadlock {
						return nil, NewErrorf(ErrFrameworkInternalError, "expect err != nil. err=(%v)", err).AddStack(nil)
					}
				}

				for i := 0; i < 3; i++ {
					// Test Async and no deadlock
					id2.AsyncMessagingByName(selfID, "testingRemoteSyncFirstSyncCallDeadlock", 5*time.Second, &Strings{Ss: []string{testTarget, "mark1"}}, func(out proto.Message, err *Error) {
						if err != nil {
							selfID.Log().Error("testingRemoteSyncFirstSyncCallDeadlock. err=(%v)", err)
							return
						}
						selfID.Log().Info("testingRemoteSyncFirstSyncCallDeadlock. out=(%v)", out)
						remoteSuccessCounter += 1
					})
				}

				// Test Spawn deadlock
				out, err = id2.SyncMessagingByName(selfID, "testingRemoteFirstSyncCallSpawnDeadlock", 5*time.Second, in)
				if err == nil || err.Code != ErrIDFirstSyncCallDeadlock {
					return nil, NewErrorf(ErrFrameworkInternalError, "expect err != nil. err=(%v)", err).AddStack(nil)
				}

				// Test Spawn deadlock
				id2.AsyncMessagingByName(selfID, "testingRemoteFirstSyncCallSpawnDeadlock", 5*time.Second, &Strings{Ss: []string{testElem, testSelf, testTarget}}, func(out proto.Message, err *Error) {
					if err != nil {
						selfID.Log().Error("testingRemoteFirstSyncCallSpawnDeadlock. err=(%v)", err)
						return
					}
					selfID.Log().Info("testingRemoteFirstSyncCallSpawnDeadlock. out=(%v)", out)
					remoteSuccessCounter += 1
				})

				return &String{S: "OK"}, nil
			},

			// node2接收到node1的Sync和Async的调用。
			"testingRemoteSyncFirstSyncCallDeadlock": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				// SelfID
				selfCosmos := from.(*remoteAtomFakeSelfID).AtomRemote.element.cosmos.process
				selfAtomID, selfTrackerID, err := selfCosmos.local.CosmosGetAtomID(from.GetIDInfo().Element, in.(*Strings).Ss[0])
				if err != nil {
					return nil, err.AddStack(nil)
				}
				selfTrackerID.Release()
				selfID := selfAtomID.(*AtomLocal)

				// Concrete
				out, err = from.SyncMessagingByName(selfID, "testMessage", 0, nil)
				return out, err
			},

			// node2接收到node1的Spawn调用。
			"testingRemoteFirstSyncCallSpawnDeadlock": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				testElem, testSelf, testTarget := in.(*Strings).Ss[0], in.(*Strings).Ss[1], in.(*Strings).Ss[2]
				// SelfID
				selfCosmos := from.(*remoteAtomFakeSelfID).AtomRemote.element.cosmos.process
				selfAtomID, selfTrackerID, err := selfCosmos.local.CosmosGetAtomID(testElem, testTarget)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				selfTrackerID.Release()
				selfID := selfAtomID.(*AtomLocal)

				// Concrete
				remoteElem, err := from.Cosmos().CosmosGetElementID(testElem)
				rID, rT, err := remoteElem.(*ElementRemote).SpawnAtom(selfID, "testDeadlock", &Strings{Ss: []string{"deadlock", testElem, testSelf}}, nil, false)
				if err != nil {
					return nil, err.AddStack(nil)
				}
				err = nil
				if rID == nil || rT != nil {
					return nil, NewError(ErrFrameworkInternalError, "expect rID != nil && rT == nil").AddStack(nil)
				}
				return out, err
			},

			// Old test cases
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
			"testSpawnAtoms": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
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
			"Broadcast": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				return nil, nil
			},
			"testKillSelf": func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error) {
				sharedTestAtom1.KillSelf()
				return &String{S: "OK"}, nil
			},
		},
		ScaleHandlers: map[string]ScaleHandler{
			"ScaleTestMessage": func(from ID, to Atomos, message string, in proto.Message) (id ID, err *Error) {
				elem := process.local.elements["testElement"]
				return elem.atoms["testAtom"], nil
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

func (t *testElementAutoDataDev) AtomAutoData() AtomAutoData {
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

func (t *testElementAutoDataDev) ElementAutoData() ElementAutoData {
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
	//t.t.Logf("Stopping: from=(%v),cancelled=(%v)", from, cancelled)
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

//func testAppSocketClient(t *testing.T) *AppUDSClient {
//	return &AppUDSClient{
//		logging: sharedLogging.PushProcessLog,
//		mutex:   sync.Mutex{},
//		connID:  0,
//		connMap: map[int32]*AppUDSConn{},
//	}
//}
