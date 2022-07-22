package go_atomos

import (
	"google.golang.org/protobuf/proto"
)

// Cosmos生命周期
// Cosmos Life Cycle

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

//////////////////////////////////////////////////
////////////
// Runnable

type CosmosRunnable struct {
	config         *Config
	interfaces     map[string]*ElementInterface
	interfaceOrder []*ElementInterface
	implements     map[string]*ElementImplementation
	implementOrder []*ElementImplementation
	mainScript     Script
	mainLogLevel   LogLevel
	reloadScript   ReloadScript
}

// Script
// Runnable相关入口脚本
// Entrance script of runnable.
type Script func(main *CosmosMain, killSignal chan bool)
type ReloadScript func(main *CosmosMain)

func (r *CosmosRunnable) AddElementInterface(i *ElementInterface) *CosmosRunnable {
	if r.interfaces == nil {
		r.interfaces = map[string]*ElementInterface{}
	}
	if _, has := r.interfaces[i.Config.Name]; !has {
		r.interfaces[i.Config.Name] = i
		r.interfaceOrder = append(r.interfaceOrder, i)
	}
	return r
}

// CosmosRunnable构造器方法，用于添加Element。
// Construct method of CosmosRunnable, uses to add Element.
func (r *CosmosRunnable) AddElementImplementation(i *ElementImplementation) *CosmosRunnable {
	r.AddElementInterface(i.Interface)
	if r.implements == nil {
		r.implements = map[string]*ElementImplementation{}
	}
	if _, has := r.implements[i.Interface.Config.Name]; !has {
		r.implements[i.Interface.Config.Name] = i
		r.implementOrder = append(r.implementOrder, i)
	}
	return r
}

// CosmosRunnable构造器方法，用于设置Script。
// Construct method of CosmosRunnable, uses to set Script.
func (r *CosmosRunnable) SetScript(script Script) *CosmosRunnable {
	r.mainScript = script
	return r
}

func (r *CosmosRunnable) SetReloadScript(script ReloadScript) *CosmosRunnable {
	r.reloadScript = script
	return r
}
