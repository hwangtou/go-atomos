package views

import (
	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/protobuf/proto"
	"hello_atomos/api"
)

type element struct {
	self atomos.ElementSelfID
	data *api.HAEData
}

func NewElement() api.HelloAtomosElement {
	return &element{}
}

func (e *element) String() string {
	return e.self.String()
}

func (e *element) Spawn(self atomos.ElementSelfID, data *api.HAEData) *atomos.Error {
	e.self = self
	e.data = data
	return nil
}

func (e *element) Halt(from atomos.ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

func (e *element) SayHello(from atomos.ID, in *api.HAEHelloI) (out *api.HAEHelloO, err *atomos.Error) {
	//TODO implement me
	panic("implement me")
}

func (e *element) Broadcast(from atomos.ID, in *atomos.ElementBroadcastI) (out *atomos.ElementBroadcastO, err *atomos.Error) {
	//TODO implement me
	panic("implement me")
}

func (e *element) ScaleBonjour(from atomos.ID, in *api.HABonjourI) (*api.HelloAtomosAtomID, *atomos.Error) {
	//TODO implement me
	panic("implement me")
}

func (e *element) ScaleDoTest(from atomos.ID, in *api.HADoTestI) (*api.HelloAtomosAtomID, *atomos.Error) {
	switch in.Mode {
	case api.HADoTestI_ScaleSelfCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosElementID(e.self.Cosmos())
		if err != nil {
			return nil, err.AddStack(e.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.SayHello(e.self, &api.HAEHelloI{})
		if err == nil || err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return nil, nil

	case api.HADoTestI_ScaleRingCallDeadlockCase1:
		gotSelfID, err := api.GetHelloAtomosAtomID(e.self.Cosmos(), from.GetIDInfo().Atom)
		if err != nil {
			return nil, err.AddStack(e.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(e.self, &api.HAGreetingI{Message: "SyncSelfCallDeadlock"})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(e.self)
		}
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return gotSelfID, nil

	case api.HADoTestI_ScaleRingCallDeadlockCase2:
		gotSelfID, err := api.GetHelloAtomosAtomID(e.self.Cosmos(), from.GetIDInfo().Atom)
		if err != nil {
			return nil, err.AddStack(e.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(e.self, &api.HAGreetingI{Message: "SyncSelfCallDeadlock"})
		if err == nil {
			return nil, atomos.NewError(atomos.ErrAtomosIDCallLoop, "expect first sync call deadlock")
		}
		if err.Code != atomos.ErrAtomosIDCallLoop {
			return nil, err.AddStack(e.self)
		}
		TestMapMutex.Lock()
		TestMap[in.Mode] = 2
		TestMapMutex.Unlock()
		return gotSelfID, nil
	}

	return nil, atomos.NewError(atomos.ErrFrameworkIncorrectUsage, "unknown mode").AddStack(e.self)
}
