package element

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"google.golang.org/protobuf/proto"
)

// Firstly, create a struct to implement ElementDeveloper interface.

type LocalBoothElement struct {
	mainId atomos.MainId
}

func (r *LocalBoothElement) Load(mainId atomos.MainId) error {
	r.mainId = mainId
	r.mainId.Log().Info("LocalBoothElement is loading")
	return nil
}

func (r *LocalBoothElement) Unload() {
	r.mainId.Log().Info("LocalBoothElement is loading")
}

func (r *LocalBoothElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return "LocalBooth", 1, atomos.LogLevel_Debug, 1
}

func (r *LocalBoothElement) AtomConstructor() atomos.Atom {
	return &LocalBoothAtom{}
}

func (r *LocalBoothElement) Persistence() atomos.ElementPersistence {
	return nil
}

func (r *LocalBoothElement) AtomCanKill(id atomos.Id) bool {
	return true
}

// Secondly, create a struct to implement LocalBoothAtom.

type LocalBoothAtom struct {
	self atomos.AtomSelf
}

func (r *LocalBoothAtom) Spawn(self atomos.AtomSelf, arg *api.LocalBoothSpawnArg, data *api.LocalBoothSpawnData) error {
	r.self = self
	remoteCosmos, err := self.CosmosSelf().Connect(api.RemoteName, api.NodeRemoteAddr)
	if err != nil {
		return err
	}
	remoteBoothId, err := api.GetRemoteBoothId(remoteCosmos, api.RemoteBoothMainAtomName)
	if err != nil {
		return err
	}
	_, err = remoteBoothId.Watch(r.self, &api.RemoteWatchReq{})
	return err
}

func (r *LocalBoothAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	return nil
}

func (r *LocalBoothAtom) RemoteNotice(from atomos.Id, in *api.LocalRemoteNoticeReq) (*api.LocalRemoteNoticeResp, error) {
	r.self.Log().Info("RemoteNotice, in=%s", in.String())
	return nil, nil
}
