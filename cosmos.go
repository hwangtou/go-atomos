package go_atomos

import (
	"crypto/tls"
	"google.golang.org/protobuf/proto"
	"sync"
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
	return newCosmosSelf()
}

// Cosmos节点需要支持的接口内容
// 仅供生成器内部使用

type CosmosNode interface {
	GetNodeName() string

	IsLocal() bool
	// 通过Element和Atom的名称获得某个Atom类型的Atom的引用。
	// Get the AtomId of an Atom by Element nodeName and Atom nodeName.
	GetAtomId(elem, name string) (Id, error)

	// 启动某个Atom类型并命名和传入参数。
	// Spawn an Atom with a naming and argument.
	SpawnAtom(elem, name string, arg proto.Message) (Id, error)

	// 调用某个Atom类型的Atom的引用。
	// Messaging an Atom with an AtomId.
	MessageAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error)

	// 发送删除消息到Atom。
	// Kill Message to an Atom.
	KillAtom(fromId, toId Id) error
}

// CosmosSelf

type CosmosSelf struct {
	// CosmosCycle
	// Cosmos循环

	// Loads at NewCosmosCycle & Daemon.
	// Log
	log *mailBox
	// State
	mutex   sync.Mutex
	running bool
	// Config
	config *Config
	// TLS if exists
	clientCert *tls.Config
	listenCert *tls.Config
	// A channel focus on Daemon Command.
	daemonCmdCh chan DaemonCommand
	upgradeCount int

	// CosmosRunnable & CosmosRuntime.
	// 可运行Cosmos & Cosmos运行时。

	// Loads at DaemonWithRunnable or Runnable.
	runtime *CosmosRuntime

	// 集群助手，帮助访问远程的Cosmos。
	// Cluster helper helps access to remote Cosmos.
	remotes *cosmosRemotesHelper

	// Telnet
	telnet *cosmosTelnet
}

// Interface

func (c *CosmosSelf) Local() *CosmosRuntime {
	return c.runtime
}

func (c *CosmosSelf) GetName() string {
	return c.config.Node
}

func (c *CosmosSelf) Connect(nodeName, nodeAddr string) (*cosmosRemote, error) {
	return c.remotes.getOrConnectRemote(nodeName, nodeAddr)
}
