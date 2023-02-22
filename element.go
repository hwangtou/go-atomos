package go_atomos

// CHECKED!

import (
	"google.golang.org/protobuf/proto"
	"time"
)

// 给Element生成器使用。
// For Element Generator use.

// Element 类型
// Element type
//
// 我们可以把Element理解成Atom的类型和管理器容器，类似面向对象的class和对应实例的容器管理器的概念。
// 两种Element类型，本地Element和远程Element。
//
// A specific Element type is a type of the Atom and holder of Atoms, it's similar to the concept "class" of OOP,
// and it contains its instance. There are two kinds of Element, Local Element and Remote Element.
type Element interface {
	ID

	// GetElementName
	// Element和其相关的Atom的名称。
	// Name of the Element and its Atoms.
	GetElementName() string

	// GetAtomID
	// 通过Atom名称获取指定的Atom的ID。
	// Get AtomID by name of Atom.
	GetAtomID(name string, tracker *IDTrackerInfo) (ID, *IDTracker, *Error)

	GetAtomsNum() int
	GetActiveAtomsNum() int
	GetAllInactiveAtomsIDTrackerInfo() map[string]string

	// SpawnAtom
	// 启动一个Atom。
	// Spawn an Atom.
	SpawnAtom(name string, arg proto.Message, tracker *IDTrackerInfo) (*AtomLocal, *IDTracker, *Error)

	// MessageAtom
	// 向一个Atom发送消息。
	// Send Message to an Atom.

	MessageElement(fromID, toID ID, message string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error)
	MessageAtom(fromID, toID ID, message string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error)

	ScaleGetAtomID(fromID ID, message string, timeout time.Duration, args proto.Message, tracker *IDTrackerInfo) (ID, *IDTracker, *Error)

	// KillAtom
	// 向一个Atom发送Kill。
	// Send Kill to an Atom.
	KillAtom(fromID, toID ID, timeout time.Duration) *Error
}

type ElementCustomizeVersion interface {
	GetElementVersion() uint64
}

type ElementCustomizeAtomInitNum interface {
	GetElementAtomsInitNum() int
}

type ElementCustomizeLogLevel interface {
	GetElementLogLevel() LogLevel
}

type ElementCustomizeExit interface {
	StopTimeout() time.Duration
	StopGap() time.Duration
}

type ElementCustomizeAutoDataPersistence interface {
	// AtomAutoDataPersistence
	// 数据持久化助手
	// Data Persistence Helper
	// If returns nil, that means the element is not under control of helper.
	AtomAutoDataPersistence() AtomAutoDataPersistence
	ElementAutoDataPersistence() ElementAutoDataPersistence
}

type ElementCustomizeAutoLoadPersistence interface {
	Load(self ElementSelfID, config map[string][]byte) *Error
	Unload() *Error
}

type ElementCustomizeStartRunning interface {
	StartRunning()
}

type ElementCustomizeAuthorization interface {
	// AtomCanKill
	// Atom是否可以被该ID的Atom终止。
	// Atom保存函数的函数类型，只有有状态的Atom会被保存。
	// Whether the Atom can be killed by the ID or not.
	// Saver Function Type of Atom, only stateful Atom will be saved.
	AtomCanKill(ID) *Error
}

// ElementDeveloper
// 从*.proto文件生成到*_atomos.pb.go文件中的，具体的Element对象。
// Concrete Element instance in *_atomos.pb.go, which is generated from developer defined *.proto file.
type ElementDeveloper interface {
	// AtomConstructor
	// Atom构造器
	// Atom构造器的函数类型，由用户定义，只会构建本地Atom。
	// Atom Constructor.
	// Constructor Function Type of Atom, which is defined by developer, will construct local Atom only.
	AtomConstructor(name string) Atomos
	ElementConstructor() Atomos
}

type AtomAutoDataPersistence interface {
	// GetAtomData
	// 读取Atom
	// Get Atom.
	// 没有数据时error应该返回nil。
	GetAtomData(name string) (proto.Message, *Error)

	// SetAtomData
	// 保存Atom
	// Save Atom.
	SetAtomData(name string, data proto.Message) *Error
}

type ElementAutoDataPersistence interface {
	// GetElementData
	// 读取Element
	// Get Element.
	// 没有数据时error应该返回nil。
	GetElementData() (proto.Message, *Error)

	// SetElementData
	// 保存Element
	// Save Element.
	SetElementData(data proto.Message) *Error
}

// TODO:
// 未来版本需要考虑支持Atom的负载均衡支持。
// Support in the future version, this function is used for load balance of Atom.
// type ElementLordScheduler func(CosmosClusterHelper)
