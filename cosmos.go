package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"time"
)

// Cosmos生命周期
// Cosmos Life Cycle

// Cosmos节点需要支持的接口内容
// 仅供生成器内部使用

type CosmosNode interface {
	GetNodeName() string

	CosmosIsLocal() bool

	CosmosGetElementID(elem string) (ID, *Error)

	// GetElementAtomID
	// 通过Element和Atom的名称获得某个Atom类型的Atom的引用。
	// Get the AtomID of an Atom by Element nodeName and Atom nodeName.

	CosmosGetElementAtomID(elem, name string) (ID, *IDTracker, *Error)

	// SpawnElementAtom
	// 启动某个Atom类型并命名和传入参数。
	// Spawn an Atom with a naming and argument.
	// TODO: 如果已经存在，是否应该返回，应该如何返回？

	CosmosSpawnElementAtom(elem, name string, arg proto.Message) (ID, *IDTracker, *Error)

	// MessageAtom
	// 向一个Atom发送消息。
	// Send Message to an Atom/Element.

	CosmosMessageElement(fromID, toID ID, message string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error)
	CosmosMessageAtom(fromID, toID ID, message string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error)

	CosmosScaleElementGetAtomID(fromID ID, elem, message string, timeout time.Duration, args proto.Message) (ID ID, err *Error)
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
	mainScript     CosmosMainScript

	hookAtomSpawning hookAtomFn
	hookAtomSpawn    hookAtomFn
	hookAtomStopping hookAtomFn
	hookAtomHalt     hookAtomFn
}

type hookAtomFn func(elem string, name string)

func (r *CosmosRunnable) Check() *Error {
	if r.config == nil {
		return NewError(ErrMainRunnableConfigNotFound, "Runnable: Config not found.").AddStack(nil)
	}
	if r.mainScript == nil {
		return NewError(ErrMainRunnableScriptNotFound, "Runnable: Script not found").AddStack(nil)
	}
	if r.interfaces == nil {
		r.interfaces = map[string]*ElementInterface{}
	}
	if r.implements == nil {
		r.implements = map[string]*ElementImplementation{}
	}
	return nil
}

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

// AddElementImplementation
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

func (r *CosmosRunnable) SetConfig(config *Config) *CosmosRunnable {
	r.config = config
	return r
}

func (r *CosmosRunnable) SetMainScript(script CosmosMainScript) *CosmosRunnable {
	r.mainScript = script
	return r
}
