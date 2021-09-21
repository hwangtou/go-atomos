package element

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"google.golang.org/protobuf/proto"
)

type WormholeBoothElement struct {
	mainId atomos.MainId
}

func (w *WormholeBoothElement) Load(mainId atomos.MainId) error {
	w.mainId = mainId
	return nil
}

func (w *WormholeBoothElement) Unload() {
}

func (w *WormholeBoothElement) Info() (version uint64, logLevel atomos.LogLevel, initNum int) {
	return 1, atomos.LogLevel_Debug, 10
}

func (w *WormholeBoothElement) AtomConstructor() atomos.Atom {
	return &WormholeBooth{}
}

func (w *WormholeBoothElement) Persistence() atomos.ElementPersistence {
	return nil
}

func (w *WormholeBoothElement) AtomCanKill(id atomos.Id) bool {
	return true
}

type WormholeBooth struct {
	self atomos.AtomSelf
}

func (w *WormholeBooth) Spawn(self atomos.AtomSelf, arg *api.WormBoothSpawnArg, data *api.WormBoothData) error {
	w.self = self
	w.self.Log().Info("Spawn")
	return nil
}

func (w *WormholeBooth) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	w.self.Log().Info("Halt")
	return nil
}

func (w *WormholeBooth) NewPackage(from atomos.Id, in *api.WormPackage) (*api.WormPackage, error) {
	w.self.Log().Info("NewPackage, in=%+v", in)
	return &api.WormPackage{
		Id:      in.Id,
		Handler: in.Handler,
		Buf:     in.Buf,
	}, nil
}
