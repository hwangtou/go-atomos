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
	return nil
}

func (a *atom) Halt(from atomos.ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

func (a *atom) Greeting(from atomos.ID, in *api.HAGreetingI) (out *api.HAGreetingO, err *atomos.Error) {
	a.self.Log().Info("Greeting: in=(%v)", in)
	return &api.HAGreetingO{}, nil
}

func (a *atom) DoTest(from atomos.ID, in *api.HADoTestI) (out *api.HADoTestO, err *atomos.Error) {
	switch in.Mode {
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

	// Test Use

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

	}

	return nil, atomos.NewError(atomos.ErrFrameworkIncorrectUsage, "unknown mode").AddStack(a.self)
}

func (a *atom) ScaleBonjour(from atomos.ID, in *api.HABonjourI) (out *api.HABonjourO, err *atomos.Error) {
	//TODO implement me
	panic("implement me")
}
