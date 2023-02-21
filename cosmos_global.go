package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"time"
)

type CosmosGlobal struct {
}

const (
	CosmosGlobalName = "Global"
)

func (c *CosmosGlobal) GetNodeName() string {
	return CosmosGlobalName
}

func (c *CosmosGlobal) CosmosIsLocal() bool {
	return false
}

func (c *CosmosGlobal) CosmosGetElementID(elem string) (ID, *Error) {
	//TODO implement me
	panic("implement me")
}

func (c *CosmosGlobal) CosmosGetElementAtomID(elem, name string) (ID, *IDTracker, *Error) {
	//TODO implement me
	panic("implement me")
}

func (c *CosmosGlobal) CosmosSpawnElementAtom(elem, name string, arg proto.Message) (ID, *IDTracker, *Error) {
	//TODO implement me
	panic("implement me")
}

func (c *CosmosGlobal) CosmosMessageElement(fromID, toID ID, message string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	//TODO implement me
	panic("implement me")
}

func (c *CosmosGlobal) CosmosMessageAtom(fromID, toID ID, message string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	//TODO implement me
	panic("implement me")
}

func (c *CosmosGlobal) CosmosScaleElementGetAtomID(fromID ID, elem, message string, timeout time.Duration, args proto.Message) (ID ID, tracker *IDTracker, err *Error) {
	//TODO implement me
	panic("implement me")
}
