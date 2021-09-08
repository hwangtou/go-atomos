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

// Element类型
// Element type
//
// 我们可以把Element理解成Atom的类型和管理器容器，类似面向对象的class和对应实例的容器管理器的概念。
// 两种Element类型，本地Element和远程Element。
//
// A specific Element type is a type of the Atom and container of Atoms, it's similar to the concept "class" of OOP,
// and it contains its instance. There are two kinds of Element, Local Element and Remote Element.
type Element interface {
	// Element和其相关的Atom的名称。
	// Name of the Element and its Atoms.
	GetName() string

	// 通过Atom名称获取指定的Atom的Id。
	// Get AtomId by name of Atom.
	GetAtomId(atomName string) (Id, error)

	// 启动一个Atom。
	// Spawn an Atom.
	SpawnAtom(atomName string, arg proto.Message) (*AtomCore, error)

	// 向一个Atom发送消息。
	// Send Message to an Atom.
	MessagingAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error)

	// 向一个Atom发送Kill。
	// Send Kill to an Atom.
	KillAtom(fromId, toId Id) error
}

// 从*.proto文件生成到*_atomos.pb.go文件中的，具体的Element对象。
// Concrete Element instance in *_atomos.pb.go, which is generated from developer defined *.proto file.
type ElementDeveloper interface {
	// 加载&卸载
	// Loaded & Unloaded
	Load(mainId MainId) error
	Unload()

	// 当前ElementImplement的信息，例如Element名称、版本号、日志记录级别、初始化的Atom数量。
	// Information of ElementDeveloper, such as nodeName of Element, version, Log level and initial atom quantity.
	Info() (name string, version uint64, logLevel LogLevel, initNum int)

	// Atom构造器
	// Atom构造器的函数类型，由用户定义，只会构建本地Atom。
	// Atom Constructor.
	// Constructor Function Type of Atom, which is defined by developer, will construct local Atom only.
	AtomConstructor() Atom

	// 数据持久化助手
	// Data Persistence Helper
	// If returns nil, that means the element is not under control of helper.
	Persistence() ElementPersistence

	// Atom是否可以被该Id的Atom终止。
	// Atom保存函数的函数类型，只有有状态的Atom会被保存。
	// Whether the Atom can be killed by the Id or not.
	// Saver Function Type of Atom, only stateful Atom will be saved.
	AtomCanKill(Id) bool
}

type ElementPersistence interface {
	// Atom读取
	// Get Atom.
	GetAtomData(name string) (proto.Message, error)

	// Atom保存
	// Save Atom.
	SetAtomData(name string, data proto.Message) error
}

// TODO:
// 未来版本需要考虑支持Atom的负载均衡支持。
// Support in the future version, this function is used for load balance of Atom.
// type ElementLordScheduler func(CosmosClusterHelper)
