package go_atomos

import (
	"crypto/tls"
	"os"
	"sync"

	"google.golang.org/protobuf/proto"
)

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

	// 设置虫洞的Conn
	// Set conn of wormhole.
	SetWormholeConn(fromId, toId Id, conn interface{}) error
}

// CosmosSelf

type CosmosSelf struct {
	// Loads at NewCosmosCycle.
	config *Config

	// Loads at DaemonWithRunnable or Runnable.
	local *CosmosLocal

	// 集群助手，帮助访问远程的Cosmos。
	// Cluster helper helps access to remote Cosmos.
	remotes *cosmosRemotesHelper

	// 关注Daemon命令的管道。
	// A channel focus on Daemon Command.
	daemonCmdCh chan *DaemonCommand

	// 关注系统进程信号的管道。
	// A channel focus on OS process.
	signCh chan os.Signal

	// Lock
	mutex sync.Mutex
	// Log
	log *MailBox
	// TLS if exists
	clientCert *tls.Config
	listenCert *tls.Config
}

func newCosmosSelf() *CosmosSelf {
	c := &CosmosSelf{}
	// Cosmos log is initialized once and available all the time.
	c.log = NewMailBox(MailBoxHandler{
		OnReceive: c.onLogMessage,
		OnPanic:   c.onLogPanic,
		OnStop:    c.onLogStop,
	})
	c.log.Name = "logger"
	c.log.Start()
	return c
}

// Interface

func (c *CosmosSelf) Local() *CosmosLocal {
	return c.local
}

func (c *CosmosSelf) GetName() string {
	return c.config.Node
}

func (c *CosmosSelf) Connect(nodeName, nodeAddr string) (*cosmosRemote, error) {
	return c.remotes.getOrConnectRemote(nodeName, nodeAddr)
}
