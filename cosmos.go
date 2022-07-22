package go_atomos

import (
	"google.golang.org/protobuf/proto"
)

// Cosmos生命周期
// Cosmos Life Cycle

//type CosmosCycle interface {
//	Daemon(*Config) (chan struct{}, *ErrorInfo)
//	Send(DaemonCommand) *ErrorInfo
//	WaitKillSignal()
//	Close()
//}
//
//func NewCosmosCycle() (CosmosCycle, *ErrorInfo) {
//	return newCosmosProcess()
//}

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

// 命令
// Command

type DaemonCommand interface {
	Type() DaemonCommandType
	GetConfig() *Config
	GetRunnable() *CosmosRunnable
	Check() *ErrorInfo
}

// 命令类型
// Command Type
type DaemonCommandType int

const (
	DaemonCommandExit = 0
	// 运行Runnable
	DaemonCommandExecuteRunnable = 1
	DaemonCommandStopRunnable    = 2
	DaemonCommandReloadRunnable  = 3
)

//var (
//	ErrDaemonIsRunning = errors.New("mainFn daemon is running")
//	ErrDaemonIsBusy    = errors.New("mainFn daemon is busy")
//	ErrRunnableInvalid = errors.New("mainFn runnable invalid")
//)

func NewRunnableCommand(runnable *CosmosRunnable) DaemonCommand {
	return &command{
		cmdType:  DaemonCommandExecuteRunnable,
		runnable: runnable,
	}
}

func NewRunnableUpdateCommand(runnable *CosmosRunnable) DaemonCommand {
	return &command{
		cmdType:  DaemonCommandReloadRunnable,
		runnable: runnable,
	}
}

func NewStopCommand() DaemonCommand {
	return &command{
		cmdType:  DaemonCommandStopRunnable,
		runnable: nil,
	}
}

func NewExitCommand() DaemonCommand {
	return &command{
		cmdType:  DaemonCommandExit,
		runnable: nil,
	}
}

// Command具体实现
type command struct {
	conf     *Config
	cmdType  DaemonCommandType
	runnable *CosmosRunnable
}

func (c command) Type() DaemonCommandType {
	return c.cmdType
}

func (c command) GetRunnable() *CosmosRunnable {
	return c.runnable
}

func (c command) GetConfig() *Config {
	return c.conf
}

func (c command) Check() *ErrorInfo {
	switch c.cmdType {
	case DaemonCommandExit, DaemonCommandStopRunnable:
		return nil
	case DaemonCommandExecuteRunnable:
		if c.runnable == nil {
			return NewError(ErrProcessRunnableInvalid, "Command: Runnable is nil")
		}
		if c.runnable.mainScript == nil {
			return NewError(ErrProcessRunnableInvalid, "Command: Main script is nil")
		}
		return nil
	case DaemonCommandReloadRunnable:
		if c.runnable == nil {
			return NewError(ErrProcessRunnableInvalid, "Command: Runnable is nil")
		}
		if c.runnable.reloadScript == nil {
			return NewError(ErrProcessRunnableInvalid, "Command: Reload script is nil")
		}
		return nil
	default:
		return NewError(ErrProcessRunnableInvalid, "Command: Unknown command type")
	}
}
