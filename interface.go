package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"time"
)

// 开发者按需实现的接口
// Interfaces that developer implements as needed.

// 必须实现的接口
// Interfaces that must be implemented.

// CosmosMainScript
// Cosmos的Main脚本，由用户实现，用于在进程启动和关闭时执行一些操作。
// Main script of Cosmos, which is implemented by developer, will execute some operations when process starts and closes.
type CosmosMainScript interface {
	OnBoot(local *CosmosProcess) *Error
	OnStartUp(local *CosmosProcess) *Error
	OnShutdown() *Error
}

// ElementDeveloper
// 开发者实现的Element的构造器，将具体的Element对象和Atom的具体Atom对象构造出来，并在启动时传入到Atomos中。
// Element Constructor of developer implements. It will construct Element object and Atom object, and pass them to Atomos when starts.
type ElementDeveloper interface {
	// ElementConstructor Element构造器
	// Element构造器的函数类型，由用户定义，只会构建本地Element。
	// Element Constructor.
	// Constructor Function Type of Element, which is defined by developer, will construct local Element only.
	ElementConstructor() Atomos

	// AtomConstructor Atom构造器
	// Atom构造器的函数类型，由用户定义，只会构建本地Atom。
	// Atom Constructor.
	// Constructor Function Type of Atom, which is defined by developer, will construct local Atom only.
	AtomConstructor(name string) Atomos
}

// 按需实现的接口：自动数据持久化
// Interfaces that developer implements as needed: Auto Data Persistence.

// AutoData
// 自动数据持久化的构造器，将具体的Atom自动数据持久化对象和Element自动数据持久化对象构造出来。
// Auto Data Persistence Constructor, it will construct Atom Auto Data Persistence object and Element Auto Data Persistence object.
type AutoData interface {
	// AtomAutoData
	// 数据持久化助手
	// Data Persistence Helper
	// If returns nil, that means the element is not under control of helper.
	AtomAutoData() AtomAutoData
	ElementAutoData() ElementAutoData
}

// AutoDataLoader
// 自动数据持久化的加载器的加载方法和卸载方法，用于加载和卸载数据库的资源。
// Auto Data Persistence Loader's Load and Unload method, used to load and unload database resource.
type AutoDataLoader interface {
	Load(self ElementSelfID, config map[string][]byte) *Error
	Unload() *Error
}

// AtomAutoData
// Atom的自动持久化对象，要提供Atom数据的Getter和Setter。在Element Spawn时，会调用Getter获取数据；在Element Halt时，会调用Setter保存数据。
// Atom Auto Data Persistence object, it should provide Getter and Setter of Atom data.
// When Element Spawn, it will call Getter to get data; when Element Halt, it will call Setter to save data.
type AtomAutoData interface {
	// GetAtomData
	// Atom数据的Getter，没有数据时error应该返回nil。
	// Getter of Atom data, if no data, error should return nil.
	GetAtomData(name string) (proto.Message, *Error)

	// SetAtomData
	// Atom数据的Setter，保存Atom。
	// Setter of Atom data, save Atom.
	SetAtomData(name string, data proto.Message) *Error
}

// ElementAutoData
// Element的自动持久化对象，要提供Element数据的Getter和Setter。
// Element Auto Data Persistence object, it should provide Getter and Setter of Element data.
type ElementAutoData interface {
	// GetElementData
	// Element数据的Getter，没有数据时error应该返回nil。
	// Getter of Element data, if no data, error should return nil.
	GetElementData() (proto.Message, *Error)

	// SetElementData
	// Element数据的Setter。
	// Setter of Element data.
	SetElementData(data proto.Message) *Error
}

// 按需实现的接口：生命周期相关
// Interfaces that developer implements as needed: Lifecycle.

// ElementStartRunning
// Element的自定义启动函数，当Element启动时，启动一个新的goroutine调用该函数。
// Element Start Running Function, when Element starts, it will start a new goroutine to call this function.
type ElementStartRunning interface {
	StartRunning()
}

// ElementAtomExit
// Element中Atom的自定义退出超时时间和退出间隔时间，用于控制退出。
// Element Customize Exit, used for controlling Atom kill.
type ElementAtomExit interface {
	// StopTimeout
	// Element中Atom的自定义退出超时时间，用于控制退出。
	StopTimeout() time.Duration
	// StopGap
	// Element中Atom的自定义退出间隔时间，用于控制退出。
	StopGap() time.Duration
}

// ElementAuthorization
// Element的鉴权，用于检查操作是否满足权限。
// Element Authorization, used for checking whether the operation meets the permission.
type ElementAuthorization interface {
	// AtomCanKill
	// Atom是否可以被该ID的Atom终止。
	// Atom保存函数的函数类型，只有有状态的Atom会被保存。
	// Whether the Atom can be killed by the ID or not.
	// Saver Function Type of Atom, only stateful Atom will be saved.
	AtomCanKill(ID) *Error
}

// 按需实现的接口：描述信息相关
// Interfaces that developer implements as needed: Description.

// ElementVersion
// Element的自定义版本号，用于版本控制。
// Element Customize Version, used for version control.
type ElementVersion interface {
	GetElementVersion() uint64
}

// ElementAtomInitNum
// Element的自定义Atom初始数量，用于初始化Atom的数量。
// Element Customize Atom Initial Number, used for initializing Atom number.
type ElementAtomInitNum interface {
	GetElementAtomsInitNum() int
}

// ElementLogLevel
// Element的自定义日志级别，用于控制日志输出。
// Element Customize Log Level, used for controlling log output.
type ElementLogLevel interface {
	GetElementLogLevel() LogLevel
}

// 按需实现的接口：Atomos - 发送和接收虫洞
// Interfaces that developer implements as needed: Atomos - Send to and Accept from wormhole

// AtomosAcceptWormhole
// 接收虫洞中的内容，如果不实现，则无法正常使用ID中的SendWormhole方法。
// Accept object from wormhole, if not implemented, SendWormhole method of ID will not work properly.
type AtomosAcceptWormhole interface {
	AcceptWormhole(fromID ID, wormhole AtomosWormhole) *Error
}

// 按需实现的接口：Atomos - 各种崩溃恢复的处理
// Interfaces that developer implements as needed: Atomos - Recover from various crashes

// AtomosRecover
// Atomos的各种崩溃恢复的处理，如果不实现，将使用默认的处理。
// Recover from various crashes of Atomos, if not implemented, default processing will be used.
type AtomosRecover interface {
	ParallelRecover(err *Error)
	SpawnRecover(arg proto.Message, err *Error)
	MessageRecover(name string, arg proto.Message, err *Error)
	ScaleRecover(name string, arg proto.Message, err *Error)
	TaskRecover(taskID uint64, name string, arg proto.Message, err *Error)
	StopRecover(err *Error)
}
