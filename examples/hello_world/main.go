package main

import (
	"github.com/golang/protobuf/proto"
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
)

func main() {
	cosmos := atomos.NewCosmosSelf()
	config := atomos.Config{
		Node:        "cosmos",
		LogPath:     "/tmp/cosmos_log/",
		CosmosLocal: &atomos.CosmosLocalConfig{
			Elements: map[string]*atomos.ElementConfig{
				{
					Name:        "",
					Version:     0,
					LogLevel:    0,
					AtomInitNum: 0,
					Calls:       nil,
				},
			},
		},
	}
	register := atomos.CosmosElementRegister{}
	register.Add(api.GetGreeterDefine(&helloElement{}))
	if err := cosmos.Load(&config, &register); err != nil {
		panic(err)
	}
}

type helloElement struct {
}

func (h *helloElement) LordScheduler(manager atomos.CosmosManager) {
	panic("implement me")
}

func (h *helloElement) AtomCreator() atomos.Atom {
	return &helloAtom{}
}

func (h *helloElement) AtomSaver(id atomos.Id, stateful atomos.AtomStateful) error {
	panic("implement me")
}

func (h *helloElement) AtomCanKill(id atomos.Id) bool {
	panic("implement me")
}

type helloAtom struct {
	self atomos.AtomSelf
	arg proto.Message
}

func (h *helloAtom) Spawn(self atomos.AtomSelf, arg proto.Message) error {
	h.self = self
	h.arg = arg
	return nil
}

func (h *helloAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) {
}

func (h *helloAtom) SayHello(from atomos.Id, in *api.HelloRequest) (*api.HelloReply, error) {
	return nil, nil
}

