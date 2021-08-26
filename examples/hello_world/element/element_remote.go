package element

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"google.golang.org/protobuf/proto"
)

// Firstly, create a struct to implement ElementDeveloper interface.

type RemoteBoothElement struct {
	mainId atomos.MainId
}

func (r *RemoteBoothElement) Load(mainId atomos.MainId) error {
	r.mainId = mainId
	r.mainId.Log().Info("RemoteBoothElement is loading")
	return nil
}

func (r *RemoteBoothElement) Unload() {
	r.mainId.Log().Info("RemoteBoothElement is loading")
}

func (r *RemoteBoothElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return "RemoteBooth", 1, atomos.LogLevel_Debug, 1
}

func (r *RemoteBoothElement) AtomConstructor() atomos.Atom {
	panic("implement me")
}

func (r *RemoteBoothElement) Persistence() atomos.ElementPersistence {
	panic("implement me")
}

func (r *RemoteBoothElement) AtomCanKill(id atomos.Id) bool {
	panic("implement me")
}

// Secondly, create a struct to implement RemoteBoothAtom.

type RemoteBoothAtom struct {
	self atomos.AtomSelf
}

func (r *RemoteBoothAtom) Spawn(self atomos.AtomSelf, arg *api.RemoteBoothSpawnArg, data *api.RemoteBoothData) error {
}

func (r *RemoteBoothAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
}

func (r *RemoteBoothAtom) SayHello(from atomos.Id, in *api.RemoteSayHelloReq) (*api.RemoteSayHelloResp, error) {
}
