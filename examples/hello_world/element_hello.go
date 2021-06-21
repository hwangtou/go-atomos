package main

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"google.golang.org/protobuf/proto"
)

func init() {
	runnable.AddElement(api.GetGreeterDefine(&helloElement{}))
}

type helloElement struct {
}

func (h *helloElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return "Greeter", 1, atomos.LogLevel_Debug, 100
}

func (h *helloElement) LordScheduler(manager atomos.CosmosClusterHelper) {
	panic("implement me")
}

func (h *helloElement) AtomCreator() atomos.Atom {
	return &helloAtom{}
}

func (h *helloElement) AtomSaver(id atomos.Id, stateful atomos.AtomStateful) error {
	panic("implement me")
}

func (h *helloElement) AtomCanKill(id atomos.Id) bool {
	// todo
	return true
}

type helloAtom struct {
	self atomos.AtomSelf
	arg proto.Message
}

func (h *helloAtom) Spawn(self atomos.AtomSelf, arg proto.Message) error {
	self.Log().Info("Spawn")
	h.self = self
	h.arg = arg
	return nil
}

func (h *helloAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) {
	h.self.Log().Info("Halt")
}

func (h *helloAtom) SayHello(from atomos.Id, in *api.HelloRequest) (*api.HelloReply, error) {
	h.self.Log().Info("SayHello")
	return nil, nil
}
