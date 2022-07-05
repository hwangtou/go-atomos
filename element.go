package go_atomos

// CHECKED!

import (
	"errors"

	"google.golang.org/protobuf/proto"
)

// Element Error

var (
	ErrElementNotLoaded = errors.New("element not loaded")
)

// 给Element生成器使用。
// For Element Generator use.

// Element 类型
// Element type
//
// 我们可以把Element理解成Atom的类型和管理器容器，类似面向对象的class和对应实例的容器管理器的概念。
// 两种Element类型，本地Element和远程Element。
//
// A specific Element type is a type of the Atom and container of Atoms, it's similar to the concept "class" of OOP,
// and it contains its instance. There are two kinds of Element, Local Element and Remote Element.
type Element interface {
	// GetElementName
	// Element和其相关的Atom的名称。
	// Name of the Element and its Atoms.
	GetElementName() string

	// GetAtomId
	// 通过Atom名称获取指定的Atom的Id。
	// Get AtomId by name of Atom.
	GetAtomId(atomName string) (ID, error)

	// SpawnAtom
	// 启动一个Atom。
	// Spawn an Atom.
	SpawnAtom(atomName string, arg proto.Message) (*AtomCore, error)

	// MessagingAtom
	// 向一个Atom发送消息。
	// Send Message to an Atom.
	MessagingAtom(fromId, toId ID, message string, args proto.Message) (reply proto.Message, err error)

	// KillAtom
	// 向一个Atom发送Kill。
	// Send Kill to an Atom.
	KillAtom(fromId, toId ID) error
}

type ElementId interface {
	// Log
	// Atom日志。
	// Atom Logs.
	Log() *atomLogsManager

	// Task
	// Atom任务
	// Atom Tasks.
	Task() TaskManager

	// Connect
	// To remote CosmosNode.
	Connect(name, addr string) (CosmosNode, error)

	// Config
	// Clone of Config
	Config() *Config

	// CustomizeConfig
	// Get Customize Config.
	CustomizeConfig(name string) (string, error)
}

type ElementLoadable interface {
	Load(mainId ElementId, isReload bool) error
	Unload()
}

type ElementCustomizeVersion interface {
	GetElementVersion() uint64
}

type ElementCustomizeAtomsInitNum interface {
	GetElementAtomsInitNum() int
}

type ElementCustomizeLogLevel interface {
	GetElementLogLevel() LogLevel
}

type ElementCustomizePersistence interface {
	// Persistence
	// 数据持久化助手
	// Data Persistence Helper
	// If returns nil, that means the element is not under control of helper.
	Persistence() ElementPersistence
}

type ElementCustomizeAuthorization interface {
	// AtomCanKill
	// Atom是否可以被该Id的Atom终止。
	// Atom保存函数的函数类型，只有有状态的Atom会被保存。
	// Whether the Atom can be killed by the ID or not.
	// Saver Function Type of Atom, only stateful Atom will be saved.
	// TODO: 以后考虑改成AtomCanAdmin，就改个名字。
	AtomCanKill(ID) bool
}

// ElementDeveloper
// 从*.proto文件生成到*_atomos.pb.go文件中的，具体的Element对象。
// Concrete Element instance in *_atomos.pb.go, which is generated from developer defined *.proto file.
type ElementDeveloper interface {
	//// Info
	//// 当前ElementImplement的信息，例如Element名称、版本号、日志记录级别、初始化的Atom数量。
	//// Information of ElementDeveloper, such as nodeName of Element, version, Log level and initial atom quantity.
	//Info() (version uint64, logLevel LogLevel, initNum int)

	// AtomConstructor
	// Atom构造器
	// Atom构造器的函数类型，由用户定义，只会构建本地Atom。
	// Atom Constructor.
	// Constructor Function Type of Atom, which is defined by developer, will construct local Atom only.
	AtomConstructor() Atom
}

type ElementWormholeDeveloper interface {
	Daemon(isUpgrade bool)
}

type ElementPersistence interface {
	// GetAtomData
	// 读取Atom
	// Get Atom.
	// 没有数据时error应该返回nil。
	GetAtomData(name string) (proto.Message, error)

	// SetAtomData
	// 保存Atom
	// Save Atom.
	SetAtomData(name string, data proto.Message) error
}

// TODO:
// 未来版本需要考虑支持Atom的负载均衡支持。
// Support in the future version, this function is used for load balance of Atom.
// type ElementLordScheduler func(CosmosClusterHelper)
