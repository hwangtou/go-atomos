package views

import (
	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/protobuf/proto"
	"hello_atomos/api"
	"sync"
)

type atom struct {
	self atomos.AtomSelfID
	arg  *api.HASpawnArg
	data *api.HAData
}

var TestMap map[api.HADoTestI_TestMode]int = map[api.HADoTestI_TestMode]int{}
var TestMapMutex sync.Mutex

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
		_, err = gotSelfID.DoTest(self, &api.HADoTestI{Mode: api.HADoTestI_SpawnSelfCallDeadlockStep2})
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
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{Message: "SyncSelfCallDeadlock"})
		if err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		a.self.Log().Info("SyncSelfCallDeadlock PASS")
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return &api.HADoTestO{}, nil

	case api.HADoTestI_AsyncSelfCallNoDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), a.self.GetIDInfo().Atom)
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		gotSelfID.AsyncGreeting(a.self, &api.HAGreetingI{Message: "AsyncSelfCallDeadlock"}, func(o *api.HAGreetingO, err *atomos.Error) {
			if err != nil {
				a.self.Log().Fatal("AsyncSelfCallDeadlock FAILED, should be ok. err=(%v)", err)
			} else {
				a.self.Log().Info("AsyncSelfCallDeadlock PASS")
				TestMapMutex.Lock()
				TestMap[in.Mode] = 2
				TestMapMutex.Unlock()
			}
		})
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
				a.self.Log().Fatal("AsyncRingCallNoDeadlock FAILED, should be ok. err=(%v)", err)
			} else {
				a.self.Log().Info("AsyncRingCallNoDeadlock PASS")
				TestMapMutex.Lock()
				TestMap[in.Mode] = 2
				TestMapMutex.Unlock()
			}
		})
		return &api.HADoTestO{}, nil

	case api.HADoTestI_AsyncRingCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		gotSelfID.AsyncDoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_AsyncRingCallDeadlockStep2}, func(out *api.HADoTestO, err *atomos.Error) {
			if err != nil {
				a.self.Log().Fatal("AsyncRingCallDeadlock FAILED, should be detected. err=(%v)", err)
			} else {
				a.self.Log().Info("AsyncRingCallDeadlock PASS")
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

	case api.HADoTestI_AsyncRingCallNoReplyDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		gotSelfID.AsyncDoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_AsyncRingCallNoReplyDeadlockStep2}, nil)
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
				a.self.Log().Fatal("TaskSelfCallDeadlock FAILED, should be ok. err=(%v)", err)
				return
			}
			defer gotSelfID.Release()
			if _, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{}); err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
				a.self.Log().Fatal("TaskSelfCallDeadlock FAILED, should not be ok. err=(%v)", err)
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
				a.self.Log().Fatal("TaskRingCallDeadlock FAILED, should be ok. err=(%v)", err)
				return
			}
			defer gotSelfID.Release()
			if _, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_TaskRingCallDeadlockStep2}); err != nil {
				a.self.Log().Fatal("TaskRingCallDeadlock FAILED, should be ok. err=(%v)", err)
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
				a.self.Log().Fatal("TaskRingCallNoDeadlock FAILED, should be ok. err=(%v)", err)
				return
			}
			defer gotSelfID.Release()
			if _, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{}); err != nil {
				a.self.Log().Fatal("TaskRingCallNoDeadlock FAILED, should be ok. err=(%v)", err)
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
		wait := make(chan struct{}, 1)
		a.self.Parallel(func() {
			gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), a.self.GetIDInfo().Atom)
			if err != nil {
				a.self.Log().Fatal("ParallelSelfCallDeadlock FAILED, should be ok. err=(%v)", err)
				return
			}
			defer gotSelfID.Release()
			wait <- struct{}{}
			if _, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{}); err != nil {
				a.self.Log().Fatal("ParallelSelfCallDeadlock FAILED, should be ok. err=(%v)", err)
				return
			}
			TestMapMutex.Lock()
			TestMap[in.Mode] = 2
			TestMapMutex.Unlock()
			a.self.Log().Info("ParallelSelfCallDeadlock PASS")
		})
		<-wait
		return &api.HADoTestO{}, nil

	case api.HADoTestI_ParallelRingCallNoDeadlock:
		wait := make(chan struct{}, 1)
		a.self.Parallel(func() {
			gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
			if err != nil {
				a.self.Log().Fatal("ParallelRingCallDeadlock FAILED, should be ok. err=(%v)", err)
				return
			}
			defer gotSelfID.Release()
			wait <- struct{}{}
			if _, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_ParallelRingCallNoDeadlockStep2}); err != nil {
				a.self.Log().Fatal("ParallelRingCallDeadlock FAILED, should be ok. err=(%v)", err)
				return
			}
			TestMapMutex.Lock()
			TestMap[in.Mode] = 2
			TestMapMutex.Unlock()
			a.self.Log().Info("ParallelRingCallDeadlock PASS")
		})
		<-wait
		return &api.HADoTestO{}, nil

	case api.HADoTestI_ParallelRingCallDeadlock:
		wait := make(chan struct{}, 1)
		a.self.Parallel(func() {
			gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
			if err != nil {
				a.self.Log().Fatal("ParallelRingCallDeadlock FAILED, should be ok. err=(%v)", err)
				return
			}
			defer gotSelfID.Release()
			wait <- struct{}{}
			if _, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_ParallelRingCallDeadlockStep2}); err != nil {
				a.self.Log().Fatal("ParallelRingCallDeadlock FAILED, should be ok. err=(%v)", err)
				return
			}
			TestMapMutex.Lock()
			TestMap[in.Mode] = 2
			TestMapMutex.Unlock()
			a.self.Log().Info("ParallelRingCallDeadlock PASS")
		})
		<-wait
		return &api.HADoTestO{}, nil

	case api.HADoTestI_KillSelfCallDeadlock:
	case api.HADoTestI_KillRingCallDeadlock:

	// Test Use

	case api.HADoTestI_SpawnSelfCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello3")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("SpawnSelfCallDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_SyncRingCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_SyncRingCallDeadlock})
		if err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
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
		a.self.Log().Info("AsyncRingCallDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_AsyncRingCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_SyncRingCallDeadlock})
		if err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("AsyncRingCallDeadlockStep2 PASS")
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

	case api.HADoTestI_AsyncRingCallNoReplyDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_SyncRingCallDeadlock})
		if err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		TestMapMutex.Lock()
		TestMap[api.HADoTestI_AsyncRingCallNoReplyDeadlock] = 2
		TestMapMutex.Unlock()
		a.self.Log().Info("AsyncRingCallNoReplyDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_TaskRingCallDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), fromID.GetIDInfo().Atom)
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{})
		if err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("TaskRingCallDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_ParallelRingCallNoDeadlockStep2:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello1")
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.DoTest(a.self, &api.HADoTestI{Mode: api.HADoTestI_ParallelRingCallDeadlockStep3})
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		a.self.Log().Info("ParallelRingCallNoDeadlockStep2 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_ParallelRingCallDeadlockStep3:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), "hello2")
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
		a.self.Log().Info("ParallelRingCallDeadlockStep3 PASS")
		return &api.HADoTestO{}, nil

	case api.HADoTestI_KillSelfCallDeadlockStep2:

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
