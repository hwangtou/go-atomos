package views

import (
	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/protobuf/proto"
	"hello_atomos/api"
)

type atom struct {
	self atomos.AtomSelfID
	arg  *api.HASpawnArg
	data *api.HAData
}

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
	switch in.Mode {
	case api.HAGreetingI_SyncSelfCallDeadlock:
		gotSelfID, err := api.GetHelloAtomosAtomID(a.self.Cosmos(), a.self.GetIDInfo().Atom)
		if err != nil {
			return nil, err.AddStack(a.self)
		}
		defer gotSelfID.Release()
		_, err = gotSelfID.Greeting(a.self, &api.HAGreetingI{Mode: 0})
		if err == nil || err.Code != atomos.ErrIDFirstSyncCallDeadlock {
			return nil, atomos.NewError(atomos.ErrIDFirstSyncCallDeadlock, "expect first sync call deadlock")
		}
	}
	return &api.HAGreetingO{}, nil
}

func (a *atom) ScaleBonjour(from atomos.ID, in *api.HABonjourI) (out *api.HABonjourO, err *atomos.Error) {
	//TODO implement me
	panic("implement me")
}
