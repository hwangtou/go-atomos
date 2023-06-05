package elements

import (
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/protobuf/proto"
	"hello_world/api"
	"runtime"
	"strconv"
	"time"
)

// Constructor

type Hello struct {
}

func (h *Hello) ElementConstructor() atomos.Atomos {
	return NewHelloElement()
}

func (h *Hello) AtomConstructor(name string) atomos.Atomos {
	return NewHelloAtom()
}

// Element

type HelloElement struct {
	self atomos.ElementSelfID
	data *api.HelloData

	scaleID int
	scale   map[string]*api.HelloAtomID
}

func NewHelloElement() api.HelloElement {
	return &HelloElement{}
}

func (h *HelloElement) String() string {
	return h.self.GetIDInfo().Atom
}

func (h *HelloElement) Spawn(self atomos.ElementSelfID, data *api.HelloData) *atomos.Error {
	h.self = self
	h.data = data
	h.scale = map[string]*api.HelloAtomID{}
	h.self.Log().Info("Spawn")

	h.CheckClear(0)
	return nil
}

func (h *HelloElement) Halt(from atomos.ID, cancelled []uint64) (save bool, data proto.Message) {
	h.self.Log().Info("Halt")
	return false, nil
}

func (h *HelloElement) SayHello(from atomos.ID, in *api.HelloReq) (*api.HelloResp, *atomos.Error) {
	h.self.Log().Info("Hello World!")
	return &api.HelloResp{}, nil
}

func (h *HelloElement) ScaleBonjour(from atomos.ID, in *api.BonjourReq) (*api.HelloAtomID, *atomos.Error) {
	// 不是一个完美的测试方法，因为scale中的waiting的atom，可能是上一个请求创建出来还未被使用的。
	for name, id := range h.scale {
		switch id.State() {
		case atomos.AtomosWaiting:
			return id, nil
		case atomos.AtomosHalt:
			delete(h.scale, name)
		}
	}
	name := strconv.FormatInt(int64(h.scaleID), 10)
	id, err := api.SpawnHelloAtom(h.self.CosmosMain(), name, &api.HelloSpawnArg{
		Id: int32(h.scaleID),
	})
	if err != nil {
		return nil, err
	}
	h.scale[name] = id
	h.scaleID += 1
	return id, nil
}

func (h *HelloElement) CheckClear(id uint64) {
	// 因为只有atomos运行时，才会调用到闭包里的内容，所以安全，且线程安全。
	h.self.Task().AddAfter(5*time.Second, func(taskID uint64) {
		h.CheckClear(taskID)
	})
	h.self.Log().Info("CheckClear scaleID=(%d)", h.scaleID)
	for name, atomID := range h.scale {
		switch atomID.State() {
		case atomos.AtomosWaiting:
			if err := atomID.Kill(h.self, 0); err != nil {
				h.self.Log().Info("Kill failed, err=(%v)", err)
			}
		case atomos.AtomosHalt:
			delete(h.scale, name)
			h.self.Log().Info("Delete, name=(%s)", name)
		}
	}
	runtime.GC()
}

// Atom

type HelloAtom struct {
	self atomos.AtomSelfID
	data *api.HelloData

	updated int64
}

func NewHelloAtom() api.HelloAtom {
	return &HelloAtom{}
}

func (h *HelloAtom) String() string {
	return h.self.GetIDInfo().Atom
}

func (h *HelloAtom) Spawn(self atomos.AtomSelfID, arg *api.HelloSpawnArg, data *api.HelloData) *atomos.Error {
	h.self = self
	h.data = data
	h.self.Log().Info("Spawn")
	return nil
}

func (h *HelloAtom) Halt(from atomos.ID, cancelled []uint64) (save bool, data proto.Message) {
	h.self.Log().Info("Halt")
	return false, nil
}

func (h *HelloAtom) SayHello(from atomos.ID, in *api.HelloReq) (*api.HelloResp, *atomos.Error) {
	h.self.Log().Info("Hello World!")
	return &api.HelloResp{}, nil
}

func (h *HelloAtom) BuildNet(from atomos.ID, in *api.BuildNetReq) (*api.BuildNetResp, *atomos.Error) {
	nextID := in.Id + 1
	if nextID == 10 {
		//panic("test")
		return &api.BuildNetResp{}, nil
	}
	h.self.Log().Info("BuildNet: %d", nextID)
	name := fmt.Sprintf("hello:%d", nextID)
	helloID, err := api.SpawnHelloAtom(h.self.Cosmos(), name, &api.HelloSpawnArg{Id: nextID})
	if err != nil {
		return nil, err
	}
	_, err = helloID.BuildNet(h.self, &api.BuildNetReq{Id: nextID})
	if err != nil {
		//if in.ID == 0 {
		//	return nil, err
		//}
		return nil, err.AddStack(h.self, in)
	}
	return &api.BuildNetResp{}, nil
}

func (h *HelloAtom) MakePanic(from atomos.ID, in *api.MakePanicIn) (*api.MakePanicOut, *atomos.Error) {
	panic("make panic")
}

func (h *HelloAtom) ScaleBonjour(from atomos.ID, in *api.BonjourReq) (*api.BonjourResp, *atomos.Error) {
	h.self.Log().Info("Bonjour, %s", h.self.GetIDInfo().Atom)
	time.Sleep(1 * time.Second)
	h.self.Log().Info("Bye, %s", h.self.GetIDInfo().Atom)
	return &api.BonjourResp{}, nil
}
