package go_atomos

import (
	"google.golang.org/protobuf/proto"
)

// Cosmos生命周期
// Cosmos Life Cycle

type CosmosCycle interface {
	Daemon(*Config) (chan struct{}, error)
	Send(DaemonCommand) error
	WaitKillSignal()
	Close()
}

func NewCosmosCycle() CosmosCycle {
	return newCosmosProcess()
}

// Cosmos节点需要支持的接口内容
// 仅供生成器内部使用

type CosmosNode interface {
	GetNodeName() string

	IsLocal() bool

	// GetAtomId
	// 通过Element和Atom的名称获得某个Atom类型的Atom的引用。
	// Get the AtomId of an Atom by Element nodeName and Atom nodeName.
	GetAtomId(elem, name string) (ID, error)

	// SpawnAtom
	// 启动某个Atom类型并命名和传入参数。
	// Spawn an Atom with a naming and argument.
	SpawnAtom(elem, name string, arg proto.Message) (ID, error)

	// MessageAtom
	// 调用某个Atom类型的Atom的引用。
	// Messaging an Atom with an AtomId.
	MessageAtom(fromId, toId ID, message string, args proto.Message) (reply proto.Message, err error)

	// KillAtom
	// 发送删除消息到Atom。
	// Kill Message to an Atom.
	KillAtom(fromId, toId ID) error
}

// CosmosSelf

func (c *CosmosProcess) atomosHalt(a *baseAtomos) {
	//TODO implement me
	panic("implement me")
}

func (c *CosmosProcess) atomosRelease(a *baseAtomos) {
	//TODO implement me
	panic("implement me")
}

// Interface

func (c *CosmosProcess) Local() *CosmosRuntime {
	return c.runtime
}

func (c *CosmosProcess) GetName() string {
	return c.config.Node
}

func (c *CosmosProcess) Connect(nodeName, nodeAddr string) (*cosmosRemote, error) {
	return c.remotes.getOrConnectRemote(nodeName, nodeAddr)
}
