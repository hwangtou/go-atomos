package views

import (
	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/protobuf/proto"
	"hello_atomos/api"
	"sync"
	"time"
)

type atom struct {
	self atomos.AtomSelfID
	arg  *api.HASpawnArg
	data *api.HAData
}

var (
	TestMap      = map[api.HADoTestI_TestMode]int{}
	TestMapMutex sync.Mutex

	TestMultiTaskMap      = map[int]int{}
	TestMultiTaskMapMutex sync.Mutex
)

func NewAtom(string) api.HelloAtomosAtom {
	return &atom{}
}

func (a *atom) String() string {
	//TODO implement me
	panic("implement me")
}

func (a *atom) Spawn(self atomos.AtomSelfID, arg *api.HASpawnArg, data *api.HAData) *atomos.Error {
	a.self = self
	a.arg = arg
	a.data = data
	switch arg.Message {
	case "SpawnSelfCallDeadlock":
		_, err := self.SyncMessagingByName(self, "SpawnSelfCallDeadlock", 0, nil)
		if err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
			return atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}

	case "SpawnRingCallDeadlock":
		gotSelfID, err := api.GetHelloAtomosAtomID(self.Cosmos(), "hello2")
		if err != nil {
			return err.AddStack(self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.DoTest(self, &api.HADoTestI{Mode: api.HADoTestI_SpawnRingCallDeadlockStep2})
		if err != nil {
			return err.AddStack(self)
		}

	case "RemoteSpawnRingCallDeadlock":
		gotSelfID, err := api.GetHelloAtomosAtomID(self.CosmosMain().GetCosmosNode("a"), "hello2")
		if err != nil {
			return err.AddStack(self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.DoTest(self, &api.HADoTestI{Mode: api.HADoTestI_RemoteSpawnRingCallDeadlockStep2})
		if err != nil {
			return err.AddStack(self)
		}

	}
	return nil
}

func (a *atom) Halt(from atomos.ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

func (a *atom) Greeting(from atomos.ID, in *api.HAGreetingI) (out *api.HAGreetingO, err *atomos.Error) {
	a.self.Log().Info("Greeting: in=(%v)", in)
	switch in.Message {
	case "wait100ms":
		<-time.After(time.Millisecond * 100)
	case "RemoteAsyncRingCallNoReplyNoDeadlockStep2":
		TestMapMutex.Lock()
		TestMap[api.HADoTestI_RemoteAsyncRingCallNoReplyNoDeadlock] = 2
		TestMapMutex.Unlock()
	}
	return &api.HAGreetingO{}, nil
}

func (a *atom) DoTest(fromID atomos.ID, in *api.HADoTestI) (out *api.HADoTestO, err *atomos.Error) {
	switch in.Mode {
	case api.HADoTestI_SpawnSelfCallDeadlock:
		id3, err := api.SpawnHelloAtomosAtom(a.self, a.self.Cosmos(), "hello3", &api.HASpawnArg{Message: "SpawnSelfCallDeadlock"})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer id3.Release()
		err = id3.Kill(a.self, 0)
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("SpawnSelfCallDeadlock PASS")
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_SpawnRingCallDeadlock:
		id3, err := api.SpawnHelloAtomosAtom(a.self, a.self.Cosmos(), "hello3", &api.HASpawnArg{Message: "SpawnRingCallDeadlock"})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer id3.Release()
		err = id3.Kill(a.self, 0)
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("SpawnRingCallDeadlock PASS")
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_SyncSelfCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), a.self.GetIDInfo().Atom)
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		a.self.Log().Info("SyncSelfCallDeadlock PASS")
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_SyncRingCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_SyncRingCallDeadlockStep2})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("SyncRingCallDeadlock PASS")
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_AsyncRingCallNoDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		gotSelfID.AsyncDoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_AsyncRingCallNoDeadlockStep2}, func(out *api.HADoTestO, err *atomos.Error) {
			if err != nil {
				a.self.Log().Fatal("AsyncRingCallNoDeadlock FAILED, should be detected. err=(%v)", err.AddStack(a.self))
			} else {
				a.self.Log().Info("AsyncRingCallNoDeadlock PASS")
				TestMapMutex.Lock()
				TestMap[in.Mode] = 2
				TestMapMutex.Unlock()
			}
		})
		return &api.HADoTestO{}, nil

	case api.HADoTestI_AsyncRingCallNoReplyNoDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		gotSelfID.AsyncDoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_AsyncRingCallNoReplyNoDeadlockStep2}, nil)
		return &api.HADoTestO{}, nil

	case api.HADoTestI_ScaleSelfCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosElementID(a.self.Cosmos())
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.ScaleDoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_ScaleSelfCallDeadlock})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		if err.Code != atomos.ErrAtomNotExists {
			return nil, err.AddStack(a.self)
		}
		return &api.HADoTestO{}, nil

	case api.HADoTestI_ScaleRingCallDeadlockCase1:
		gotSelfID, err := api.GetHelloAtomosElementID(a.self.Cosmos())
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		id, err := gotSelfID.ScaleDoTestGetID(a.self, &api.HADoTestI{Mode: api.HADoTestI_ScaleRingCallDeadlockCase1})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		// 相当于自我调用
		_, err = id.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_ScaleRingCallDeadlockCase1})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		return &api.HADoTestO{}, nil

	case api.HADoTestI_ScaleRingCallDeadlockCase2:
		gotSelfID, err := api.GetHelloAtomosElementID(a.self.Cosmos())
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.ScaleDoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_ScaleRingCallDeadlockCase2})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		return &api.HADoTestO{}, nil

	case api.HADoTestI_WormholeSelfCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), a.self.GetIDInfo().Atom)
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		err = gotSelfID.SendWormhole(a.self, 0, "WormholeSelfCallDeadlock")
		if err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		a.self.Log().Info("WormholeSelfCallDeadlock PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_WormholeRingCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		err = gotSelfID.SendWormhole(a.self, 0, "WormholeRingCallDeadlock")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		a.self.Log().Info("WormholeRingCallDeadlock PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_TaskSelfCallDeadlock:
		if _, err := a.self.Task().Add(func(_ uint64) {
			gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), a.self.GetIDInfo().Atom)
			if err != nil {
				a.self.Log().Fatal("TaskSelfCallDeadlock FAILED, should be ok. err=(%v)", err.AddStack(a.self))
				return
			}
			defer gotSelfID.Release()
			if _, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{}); err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
				a.self.Log().Fatal("TaskSelfCallDeadlock FAILED, should not be ok. err=(%v)", err.AddStack(a.self))
				return
			}
			TestMapMutex.Lock()
			TestMap[in.Mode] = 2
			TestMapMutex.Unlock()
			a.self.Log().Info("TaskSelfCallDeadlock PASS")
		}); err != nil {
			return nil, err.AddStack(a.self)
		}
		return &api.HADoTestO{}, nil

	case api.HADoTestI_TaskRingCallDeadlock:
		if _, err := a.self.Task().Add(func(_ uint64) {
			gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
			if err != nil {
				a.self.Log().Fatal("TaskRingCallDeadlock FAILED, should be ok. err=(%v)", err.AddStack(a.self))
				return
			}
			defer gotSelfID.Release()
			if _, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_TaskRingCallDeadlockStep2}); err != nil {
				a.self.Log().Fatal("TaskRingCallDeadlock FAILED, should be ok. err=(%v)", err.AddStack(a.self))
				return
			}
			TestMapMutex.Lock()
			TestMap[in.Mode] = 2
			TestMapMutex.Unlock()
			a.self.Log().Info("TaskRingCallDeadlock PASS")
		}); err != nil {
			return nil, err.AddStack(a.self)
		}
		return &api.HADoTestO{}, nil

	case api.HADoTestI_TaskRingCallNoDeadlock:
		if _, err := a.self.Task().Add(func(_ uint64) {
			gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
			if err != nil {
				a.self.Log().Fatal("TaskRingCallNoDeadlock FAILED, should be ok. err=(%v)", err.AddStack(a.self))
				return
			}
			defer gotSelfID.Release()
			if _, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{}); err != nil {
				a.self.Log().Fatal("TaskRingCallNoDeadlock FAILED, should be ok. err=(%v)", err.AddStack(a.self))
				return
			}
			TestMapMutex.Lock()
			TestMap[in.Mode] = 2
			TestMapMutex.Unlock()
			a.self.Log().Info("TaskRingCallNoDeadlock PASS")
		}); err != nil {
			return nil, err.AddStack(a.self)
		}
		return &api.HADoTestO{}, nil

	case api.HADoTestI_ParallelSelfCallNoDeadlock:
		a.self.Parallel(func() {
			gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), a.self.GetIDInfo().Atom)
			if err != nil {
				a.self.Log().Fatal("ParallelSelfCallNoDeadlock FAILED, should not be ok. err=(%v)", err.AddStack(a.self))
				return
			}
			defer gotSelfID.Release()
			_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
			if err == nil {
				a.self.Log().Fatal("ParallelSelfCallNoDeadlock FAILED, should not be ok. err=(%v)", err.AddStack(a.self))
				return
			}
			if err.Code != atomos.ErrAtomosIDCallLoop {
				a.self.Log().Fatal("ParallelSelfCallNoDeadlock FAILED, should not be ok. err=(%v)", err.AddStack(a.self))
				return
			}
			TestMapMutex.Lock()
			TestMap[in.Mode] = 2
			TestMapMutex.Unlock()
			a.self.Log().Info("ParallelSelfCallNoDeadlock PASS")
		})
		return &api.HADoTestO{}, nil

	case api.HADoTestI_ParallelRingCallDeadlock:
		a.self.Parallel(func() {
			gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
			if err != nil {
				a.self.Log().Fatal("ParallelRingCallDeadlock FAILED, should be ok. err=(%v)", err.AddStack(a.self))
				return
			}
			defer gotSelfID.Release()
			if _, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_ParallelRingCallDeadlockStep2}); err != nil {
				a.self.Log().Fatal("ParallelRingCallDeadlock FAILED, should be ok. err=(%v)", err.AddStack(a.self))
				return
			}
			TestMapMutex.Lock()
			TestMap[in.Mode] = 2
			TestMapMutex.Unlock()
			a.self.Log().Info("ParallelRingCallDeadlock PASS")
		})
		return &api.HADoTestO{}, nil

	case api.HADoTestI_KillSelfCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), a.self.GetIDInfo().Atom)
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		err = gotSelfID.Kill(a.self, 0)
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		a.self.Log().Info("KillSelfCallDeadlock PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_KillRingCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_KillRingCallDeadlockStep2})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		a.self.Log().Info("KillRingCallDeadlock PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteSpawnAtomAndHalt:
		nodeB := a.self.CosmosMain().GetCosmosNode("b")
		atomBHello1, err := api.SpawnHelloAtomosAtom(a.self, nodeB, "b_hello1", &api.HASpawnArg{})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer atomBHello1.Release()
		if err = atomBHello1.Kill(a.self, 0); err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("RemoteSpawnAtomAndHalt PASS")
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteSpawnSelfCallDeadlock:
		nodeB := a.self.CosmosMain().GetCosmosNode("b")
		id3, err := api.SpawnHelloAtomosAtom(a.self, nodeB, "b_hello1", &api.HASpawnArg{Message: "SpawnSelfCallDeadlock"})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer id3.Release()
		if err = id3.Kill(a.self, 0); err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("RemoteSpawnSelfCallDeadlock PASS")
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteSpawnRingCallDeadlock:
		nodeB := a.self.CosmosMain().GetCosmosNode("b")
		id3, err := api.SpawnHelloAtomosAtom(a.self, nodeB, "b_hello1", &api.HASpawnArg{Message: "RemoteSpawnRingCallDeadlock"})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer id3.Release()
		err = id3.Kill(a.self, 0)
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("RemoteSpawnRingCallDeadlock PASS")
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteSyncSelfCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.CosmosMain().GetCosmosNode("b"), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_RemoteSyncSelfCallDeadlockStep2})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("RemoteSyncSelfCallDeadlock PASS")
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteSyncRingCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.CosmosMain().GetCosmosNode("b"), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_RemoteSyncRingCallDeadlockStep2})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("RemoteSyncRingCallDeadlock PASS")
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteAsyncRingCallNoDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.CosmosMain().GetCosmosNode("b"), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		gotSelfID.AsyncDoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_RemoteAsyncRingCallNoDeadlockStep2}, func(out *api.HADoTestO, err *atomos.Error) {
			if err != nil {
				a.self.Log().Fatal("RemoteAsyncRingCallNoDeadlock FAILED, should be ok. err=(%v)", err.AddStack(a.self))
			} else {
				a.self.Log().Info("RemoteAsyncRingCallNoDeadlock PASS")
				TestMapMutex.Lock()
				TestMap[in.Mode] = 2
				TestMapMutex.Unlock()
			}
		})
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteAsyncRingCallNoReplyNoDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.CosmosMain().GetCosmosNode("b"), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		gotSelfID.AsyncDoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_RemoteAsyncRingCallNoReplyNoDeadlockStep2}, nil)
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteScaleSelfCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosElementID(a.self.CosmosMain().GetCosmosNode("b"))
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.ScaleDoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_RemoteScaleSelfCallDeadlock})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		if err.Code != atomos.ErrAtomNotExists {
			return nil, err.AddStack(a.self)
		}
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteScaleRingCallDeadlockCase1:
		gotSelfID, err := api.GetHelloAtomosElementID(a.self.CosmosMain().GetCosmosNode("b"))
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		id, err := gotSelfID.ScaleDoTestGetID(a.self, &api.HADoTestI{Mode: api.HADoTestI_RemoteScaleRingCallDeadlockCase1})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		// 相当于自我调用
		_, err = id.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_RemoteScaleRingCallDeadlockCase1})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteScaleRingCallDeadlockCase2:
		gotSelfID, err := api.GetHelloAtomosElementID(a.self.CosmosMain().GetCosmosNode("b"))
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.ScaleDoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_RemoteScaleRingCallDeadlockCase2})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	// Test Use

	case api.HADoTestI_SpawnRingCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello3")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock").AddStack(a.self)
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("SpawnRingCallDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_SyncRingCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock").AddStack(a.self)
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("SyncRingCallDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_AsyncRingCallNoDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("AsyncRingCallNoDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_AsyncRingCallNoReplyNoDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		TestMapMutex.Lock()
		TestMap[api.HADoTestI_AsyncRingCallNoReplyNoDeadlock] = 2
		TestMapMutex.Unlock()
		a.self.Log().Info("AsyncRingCallNoReplyNoDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_TaskRingCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), fromID.GetIDInfo().Atom)
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock").AddStack(a.self)
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("TaskRingCallDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_ParallelRingCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("ParallelRingCallDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_KillRingCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), fromID.GetIDInfo().Atom)
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		err = gotSelfID.Kill(a.self, 0)
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteSpawnRingCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.CosmosMain().GetCosmosNode("b"), "b_hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock").AddStack(a.self)
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("RemoteSpawnRingCallDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteSyncSelfCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.CosmosMain().GetCosmosNode("b"), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock").AddStack(a.self)
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("RemoteSyncSelfCallDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteSyncRingCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.CosmosMain().GetCosmosNode("a"), "hello2")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_RemoteSyncRingCallDeadlockStep3})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("RemoteSyncSelfCallDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteSyncRingCallDeadlockStep3:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.CosmosMain().GetCosmosNode("b"), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock").AddStack(a.self)
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("RemoteSyncSelfCallDeadlockStep3 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteAsyncRingCallNoDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.CosmosMain().GetCosmosNode("a"), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("RemoteAsyncRingCallNoDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_RemoteAsyncRingCallNoReplyNoDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.CosmosMain().GetCosmosNode("a"), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{Message: "RemoteAsyncRingCallNoReplyNoDeadlockStep2"})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		TestMapMutex.Lock()
		TestMap[api.HADoTestI_RemoteAsyncRingCallNoReplyNoDeadlock] = 2
		TestMapMutex.Unlock()
		a.self.Log().Info("RemoteAsyncRingCallNoReplyNoDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil
	}

	return nil, atomos.NewError(atomos.ErrFrameworkIncorrectUsage, "unknown mode").AddStack(a.self)
}

func (a *atom) ScaleBonjour(from atomos.ID, in *api.HABonjourI) (out *api.HABonjourO, err *atomos.Error) {
	a.self.Log().Info("Bonjour: in=(%v)", in)
	return &api.HABonjourO{}, nil
}

func (a *atom) ScaleDoTest(from atomos.ID, in *api.HADoTestI) (out *api.HADoTestO, err *atomos.Error) {
	switch in.Mode {
	default:
		return nil, atomos.NewError(atomos.ErrFrameworkIncorrectUsage, "should not execute")
	}
}

func (a *atom) AcceptWormhole(fromID atomos.ID, wormhole atomos.AtomosWormhole) *atomos.Error {
	switch v := wormhole.(type) {
	case string:
		switch v {
		case "WormholeSelfCallDeadlock":
			return atomos.NewError(atomos.ErrFrameworkIncorrectUsage, "expect first sync call deadlock").AddStack(a.self)
		case "WormholeRingCallDeadlock":
			gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), fromID.GetIDInfo().Atom)
			if err != nil {
				return err.AddStack(a.self)
			}
			defer gotSelfID.Release()
			err = gotSelfID.SendWormhole(a.self, 0, "WormholeRingCallDeadlock")
			if err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
				return atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
			}
			return nil
		}
		return nil
	default:
		return atomos.NewError(atomos.ErrFrameworkIncorrectUsage, "unknown wormhole type").AddStack(a.self)
	}
}
