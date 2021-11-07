package elements

import (
	"errors"
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/protobuf/proto"

	"github.com/hwangtou/go-atomos/examples/hello/api"
)

type HelloElement struct {
	mainId atomos.MainId
}

func (h *HelloElement) Load(mainId atomos.MainId) error {
	h.mainId = mainId
	h.mainId.Log().Info("HelloElement is loading")
	return nil
}

func (h *HelloElement) Unload() {
	h.mainId.Log().Info("HelloElement is unloading")
}

func (h *HelloElement) Info() (version uint64, logLevel atomos.LogLevel, initNum int) {
	return 1, atomos.LogLevel_Debug, 10
}

func (h *HelloElement) AtomConstructor() atomos.Atom {
	return &HelloAtom{}
}

func (h *HelloElement) Persistence() atomos.ElementPersistence {
	return nil
}

func (h *HelloElement) AtomCanKill(id atomos.Id) bool {
	return true
}

type HelloAtom struct {
	self atomos.AtomSelf
}

func (h *HelloAtom) Spawn(self atomos.AtomSelf, arg *api.HelloSpawnArg, data *api.HelloData) error {
	h.self = self
	h.self.Log().Info("Spawn")
	return nil
}

func (h *HelloAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	return nil
}

func (h *HelloAtom) SayHello(from atomos.Id, in *api.HelloReq) (*api.HelloResp, error) {
	h.self.Log().Info("Hello, %s", in.Name)
	return nil, nil
}

func (h *HelloAtom) BuildNet(from atomos.Id, in *api.BuildNetReq) (*api.BuildNetResp, error) {
	nextId := in.Id + 1
	if nextId == 10 {
		return nil, errors.New("over")
	}
	name := fmt.Sprintf("hello:%d", nextId)
	helloId, err := api.SpawnHello(h.self.Cosmos(), name, &api.HelloSpawnArg{Id: nextId})
	if err != nil {
		return nil, err
	}
	_, err = helloId.BuildNet(h.self, &api.BuildNetReq{Id: nextId})
	if err != nil {
		return nil, err
	}
	return &api.BuildNetResp{}, nil
}
