package views

import (
	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/protobuf/proto"
	"hello_atomos/api"
)

type element struct {
	self atomos.ElementSelfID
	data *api.HAEData
}

func NewElement() api.HelloAtomosElement {
	return &element{}
}

func (e *element) String() string {
	return e.self.String()
}

func (e *element) Spawn(self atomos.ElementSelfID, data *api.HAEData) *atomos.Error {
	e.self = self
	e.data = data
	return nil
}

func (e *element) Halt(from atomos.ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

func (e *element) SayHello(from atomos.ID, in *api.HAEHelloI) (out *api.HAEHelloO, err *atomos.Error) {
	//TODO implement me
	panic("implement me")
}

func (e *element) Broadcast(from atomos.ID, in *atomos.ElementBroadcastI) (out *atomos.ElementBroadcastO, err *atomos.Error) {
	//TODO implement me
	panic("implement me")
}

func (e *element) ScaleBonjour(from atomos.ID, in *api.HABonjourI) (*api.HelloAtomosAtomID, *atomos.Error) {
	//TODO implement me
	panic("implement me")
}
