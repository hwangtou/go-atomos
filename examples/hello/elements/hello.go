package elements

import (
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello/api"
	"google.golang.org/protobuf/proto"
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

// Atom

type HelloAtom struct {
	self atomos.AtomSelfID
	data *api.HelloData
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

func (h *HelloAtom) BuildNet(from atomos.ID, in *api.BuildNetReq) (*api.BuildNetResp, *atomos.ErrorInfo) {
	nextId := in.Id + 1
	if nextId == 3 {
		panic("test")
		//return &api.BuildNetResp{}, nil
	}
	h.self.Log().Info("BuildNet: %d", nextId)
	name := fmt.Sprintf("hello:%d", nextId)
	helloId, err := api.SpawnHelloAtom(h.self.Cosmos(), name, &api.HelloSpawnArg{Id: nextId})
	if err != nil {
		return nil, err
	}
	_, err = helloId.BuildNet(h.self, &api.BuildNetReq{Id: nextId})
	if err != nil {
		//if in.Id == 0 {
		//	return nil, err
		//}
		return nil, err.AutoStack(h.self, in)
	}
	return &api.BuildNetResp{}, nil
}

func (h *HelloAtom) MakePanic(from atomos.ID, in *api.MakePanicIn) (*api.MakePanicOut, *atomos.ErrorInfo) {
	//TODO implement me
	panic("implement me")
}
