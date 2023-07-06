package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"time"
)

// Cosmos生命周期
// Cosmos Life Cycle

// Cosmos节点需要支持的接口内容
// Interfaces that Cosmos node needs to support

// CosmosNode 是Cosmos节点的接口，每个Cosmos节点都需要实现这个接口。
// CosmosNode is the interface of Cosmos node, each Cosmos node needs to implement this interface.
type CosmosNode interface {
	ID

	// GetNodeName 获取节点名称
	// Get the name of node
	GetNodeName() string

	// CosmosIsLocal 是否本地节点
	// Is local node
	CosmosIsLocal() bool

	// CosmosGetElementID 通过Element名称，获取一个Element的ID。
	// Get the ID of an Element by Element name.
	CosmosGetElementID(elem string) (ID, *Error)

	// CosmosGetAtomID 通过Element和Atom的名称获得某个Atom类型的Atom的引用。
	// Get the reference of an Atom by Element and Atom name.
	CosmosGetAtomID(elem, name string) (ID, *IDTracker, *Error)

	// CosmosGetScaleAtomID 通过Element和Atom的名称获得一个负载均衡的Atom类型的Atom的引用。
	// Get the reference of a load balancing Atom by Element and Atom name.
	CosmosGetScaleAtomID(callerID SelfID, elem, message string, timeout time.Duration, args proto.Message) (ID ID, tracker *IDTracker, err *Error)

	// CosmosSpawnAtom 启动某个Atom类型并命名和传入参数。
	// Spawn an Atom with a naming and argument.
	// TODO: 如果已经存在，是否应该返回，应该如何返回？
	CosmosSpawnAtom(callerID SelfID, elem, name string, arg proto.Message) (ID, *IDTracker, *Error)

	// ElementBroadcast 对节点下所有的Element进行广播
	// Broadcast to all Elements under the node
	ElementBroadcast(callerID SelfID, key, contentType string, contentBuffer []byte) (err *Error)
}

// CosmosRunnable 是Cosmos的可运行实例，每个Atomos的可执行文件，都需要实现和提供这个对象。
// CosmosRunnable is the runnable instance of Cosmos, each executable file of Atomos needs to implement and provide this object.
type CosmosRunnable struct {
	config *Config
	//interfaces     map[string]*ElementInterface
	//interfaceOrder []*ElementInterface
	implements     map[string]*ElementImplementation
	implementOrder []string
	mainScript     CosmosMainScript
	mainRouter     CosmosMainGlobalRouter
}

// Check 检查CosmosRunnable是否正确构造。
// Check if CosmosRunnable is constructed correctly.
func (r *CosmosRunnable) Check() *Error {
	// Config
	if r.config == nil {
		return NewError(ErrRunnableConfigNotFound, "Runnable: Config not found.").AddStack(nil)
	}
	if err := r.config.Check(); err != nil {
		return err.AddStack(nil)
	}
	//// Interfaces
	//if r.interfaces == nil {
	//	r.interfaces = map[string]*ElementInterface{}
	//}
	//if len(r.interfaces) != len(r.interfaceOrder) {
	//	return NewError(ErrRunnableInterfaceInvalid, "Runnable: Interface order not match.").AddStack(nil)
	//}
	// Implements
	if r.implements == nil {
		r.implements = map[string]*ElementImplementation{}
	}
	if len(r.implements) != len(r.implementOrder) {
		return NewError(ErrRunnableImplementInvalid, "Runnable: Implement not match.").AddStack(nil)
	}
	// MainScript
	if r.mainScript == nil {
		return NewError(ErrRunnableScriptNotFound, "Runnable: Script not found").AddStack(nil)
	}
	return nil
}

//// AddElementInterface CosmosRunnable构造器方法，用于添加ElementInterface（接口）。
//// Construct method of CosmosRunnable, uses to add ElementInterface.
//func (r *CosmosRunnable) AddElementInterface(i *ElementInterface) *CosmosRunnable {
//	if r.interfaces == nil {
//		r.interfaces = map[string]*ElementInterface{}
//	}
//	if _, has := r.interfaces[i.Config.Name]; !has {
//		r.interfaces[i.Config.Name] = i
//		r.interfaceOrder = append(r.interfaceOrder, i)
//	}
//	return r
//}

// AddElementImplementation CosmosRunnable构造器方法，用于添加ElementImplementation（实现）。
// Construct method of CosmosRunnable, uses to add ElementImplementation.
func (r *CosmosRunnable) AddElementImplementation(i *ElementImplementation) *CosmosRunnable {
	//r.AddElementInterface(i.Interface)
	if r.implements == nil {
		r.implements = map[string]*ElementImplementation{}
	}
	if _, has := r.implements[i.Interface.Config.Name]; !has {
		r.implements[i.Interface.Config.Name] = i
		r.implementOrder = append(r.implementOrder, i.Interface.Config.Name)
	}
	return r
}

// SetConfig CosmosRunnable构造器方法，用于设置Config。
// Construct method of CosmosRunnable, uses to set Config.
func (r *CosmosRunnable) SetConfig(config *Config) *CosmosRunnable {
	r.config = config
	return r
}

// SetMainScript CosmosRunnable构造器方法，用于设置MainScript。
// Construct method of CosmosRunnable, uses to set MainScript.
func (r *CosmosRunnable) SetMainScript(script CosmosMainScript) *CosmosRunnable {
	r.mainScript = script
	return r
}

func (r *CosmosRunnable) SetRouter(router CosmosMainGlobalRouter) *CosmosRunnable {
	r.mainRouter = router
	return r
}
