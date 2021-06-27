package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"net"
)

// TODO: 远程Cosmos管理助手，在未来版本实现。
// TODO: Remote Cosmos Helper.

type ElementRemote struct {
	name     string
	cachedId map[string]*atomIdRemote
	conn     net.Conn
}

func (e *ElementRemote) startDaemon() {
}

func (e *ElementRemote) stopDaemon() {
}

func (e *ElementRemote) GetName() string {
	return e.name
}

func (e *ElementRemote) GetAtomId(name string) (Id, error) {
	panic("")
}

func (e *ElementRemote) SpawnAtom(atomName string, arg proto.Message) (*AtomCore, error) {
	return nil, ErrAtomCannotSpawn
}

func (e *ElementRemote) MessagingAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
	panic("")
}

func (e *ElementRemote) KillAtom(fromId, toId Id) error {
	panic("")
}

type atomIdRemote struct {
	name string
}
