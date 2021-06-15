package go_atomos

import "github.com/golang/protobuf/proto"

type ElementRemote struct {
}

func (e *ElementRemote) GetName() string {
	panic("")
}

func (e *ElementRemote) GetAtomId(name string) (Id, error) {
	panic("")
}

func (e *ElementRemote) SpawnAtom(atomName string, arg proto.Message) (*AtomCore, error) {
	panic("")
}

func (e *ElementRemote) CallAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
	panic("")
}

func (e *ElementRemote) KillAtom(fromId, toId Id) error {
	panic("")
}
