package go_atomos

import (
	"google.golang.org/protobuf/proto"
)

// Cosmos生命周期
// Cosmos Life Cycle

type CosmosCycle interface {
	Daemon(*Config) (chan struct{}, *ErrorInfo)
	Send(DaemonCommand) *ErrorInfo
	WaitKillSignal()
	Close()
}

func NewCosmosCycle() (CosmosCycle, *ErrorInfo) {
	return newCosmosProcess()
}

// Cosmos节点需要支持的接口内容
// 仅供生成器内部使用

type CosmosNode interface {
	GetNodeName() string

	IsLocal() bool

	// GetElementAtomId
	// 通过Element和Atom的名称获得某个Atom类型的Atom的引用。
	// Get the AtomId of an Atom by Element nodeName and Atom nodeName.
	GetElementAtomId(elem, name string) (ID, *ErrorInfo)

	// SpawnElementAtom
	// 启动某个Atom类型并命名和传入参数。
	// Spawn an Atom with a naming and argument.
	SpawnElementAtom(elem, name string, arg proto.Message) (ID, *ErrorInfo)

	// MessageAtom
	// 向一个Atom发送消息。
	// Send Message to an Atom.
	MessageAtom(fromId, toId ID, message string, args proto.Message) (reply proto.Message, err *ErrorInfo)

	// KillAtom
	// 向一个Atom发送Kill。
	// Send Kill to an Atom.
	KillAtom(fromId, toId ID) *ErrorInfo
}

// CosmosSelf

//func (c *CosmosProcess) atomosHalt(a *baseAtomos) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (c *CosmosProcess) elementAtomRelease(a *baseAtomos) {
//	//TODO implement me
//	panic("implement me")
//}

//// Interface
//
//func (c *CosmosProcess) Local() *CosmosMainFn {
//	return c.main
//}
