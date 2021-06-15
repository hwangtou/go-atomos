package go_atomos

import "github.com/golang/protobuf/proto"

type Element interface {
	GetName() string
	GetAtomId(name string) (Id, error)
	SpawnAtom(atomName string, arg proto.Message) (*AtomCore, error)
	CallAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error)
	KillAtom(fromId, toId Id) error
}
