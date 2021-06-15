package go_atomos

import "github.com/golang/protobuf/proto"

// Cosmos节点需要支持的接口内容
// 仅供生成器内部使用

type CosmosNode interface {
	IsLocal() bool
	// 获得某个Atom类型的Atom的引用
	GetAtomId(elem, name string) (Id, error)

	SpawnAtom(elem, name string, arg proto.Message) (Id, error)

	// 调用某个Atom类型的Atom的引用
	CallAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error)

	// 关闭某个Atom类型的Atom
	KillAtom(fromId, toId Id) error
}

// CosmosNode

type CosmosSelf struct {
	config  *Config
	local   *CosmosLocal
	manager *CosmosManager
}

func NewCosmosSelf() *CosmosSelf {
	return &CosmosSelf{
		config:  nil,
		local:   nil,
		manager: nil,
	}
}

type CosmosElementRegister struct {
	defines map[string]*ElementDefine
}

func (r *CosmosElementRegister) Add(define *ElementDefine) {
	if r.defines == nil {
		r.defines = map[string]*ElementDefine{}
	}
	r.defines[define.Name] = define
}

func (c *CosmosSelf) Load(config *Config, register *CosmosElementRegister) error {
	c.config = config
	c.local = newCosmosLocal()
	return c.local.load(config, c, register.defines)
}

func (c *CosmosSelf) Unload() {
	c.local.unload()
	c.local = nil
	c.config = nil
}

func (c *CosmosSelf) GetName() string {
	return c.config.Node
}
