package elements

import (
	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/protobuf/proto"
	"remote/server/api"
	"strconv"
	"time"
)

// Constructor

type Hello struct {
}

func (h *Hello) ElementConstructor() atomos.Atomos {
	return &HelloElement{}
}

func (h *Hello) AtomConstructor(name string) atomos.Atomos {
	return &HelloAtom{}
}

// Element

type HelloElement struct {
	self atomos.ElementSelfID
	data *api.HelloData

	scaleID int
}

func (h *HelloElement) Description() string {
	return h.self.GetName()
}

func (h *HelloElement) Spawn(self atomos.ElementSelfID, data *api.HelloData) *atomos.ErrorInfo {
	h.self = self
	h.data = data
	h.self.Log().Info("Spawn")

	return nil
}

func (h *HelloElement) Halt(from atomos.ID, cancelled map[uint64]atomos.CancelledTask) (save bool, data proto.Message) {
	h.self.Log().Info("Halt")
	return false, nil
}

func (h *HelloElement) Reload(oldInstance atomos.Atomos) {
	h.self.Log().Info("Reload")
	*h = *oldInstance.(*HelloElement)
}

func (h *HelloElement) SayHello(from atomos.ID, in *api.HelloReq) (*api.HelloResp, *atomos.ErrorInfo) {
	h.self.Log().Info("Hello World!")
	return &api.HelloResp{}, nil
}

func (h *HelloElement) ScaleBonjour(from atomos.ID, in *api.BonjourReq) (api.HelloAtomID, *atomos.ErrorInfo) {
	name := strconv.FormatInt(int64(h.scaleID), 10)
	id, err := api.SpawnHelloAtom(h.self.CosmosMain(), name, &api.HelloSpawnArg{
		Id: int32(h.scaleID),
	})
	if err != nil {
		return nil, err
	}
	h.scaleID += 1
	return id, nil
}

// Atom

type HelloAtom struct {
	self atomos.AtomSelfID
	data *api.HelloData

	updated int64
}

func (h *HelloAtom) Description() string {
	return h.self.GetName()
}

func (h *HelloAtom) Spawn(self atomos.AtomSelfID, arg *api.HelloSpawnArg, data *api.HelloData) *atomos.ErrorInfo {
	h.self = self
	h.data = data
	h.self.Log().Info("Spawn")
	return nil
}

func (h *HelloAtom) Halt(from atomos.ID, cancelled map[uint64]atomos.CancelledTask) (save bool, data proto.Message) {
	h.self.Log().Info("Halt")
	return false, nil
}

func (h *HelloAtom) Reload(oldInstance atomos.Atomos) {
	h.self.Log().Info("Reload")
	*h = *oldInstance.(*HelloAtom)
}

func (h *HelloAtom) SayHello(from atomos.ID, in *api.HelloReq) (*api.HelloResp, *atomos.ErrorInfo) {
	h.self.Log().Info("Hello World!")
	return &api.HelloResp{}, nil
}

func (h *HelloAtom) Bonjour(from atomos.ID, in *api.BonjourReq) (*api.BonjourResp, *atomos.ErrorInfo) {
	h.self.Log().Info("Bonjour, %s", h.self.GetName())
	time.Sleep(1 * time.Second)
	h.self.Log().Info("Bye, %s", h.self.GetName())
	return &api.BonjourResp{}, nil
}
