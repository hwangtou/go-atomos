package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// 给Atom的ElementDefine生成器使用。
// For Atom Element Define Generator use.

// Element类型
// Element type
//
// 我们可以把Element理解成Atom的类型和管理器容器，类似面向对象的class和对应实例的容器管理器的概念。
// 两种Element类型，本地Element和远程Element。
//
// A specific Element is the type and container of an Atom, it's similar to concept "class" of OOP and it contains its
// instance. There are two kinds of Element, Local Element and Remote Element.
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

	// 向一个Atom发送Kill消息。
	// Send Kill to an Atom.
	KillAtom(fromId, toId Id) error
}

// 从*.proto文件生成到*_atomos.pb.go文件中的，ElementDefine对象。
// ElementDefine in *_atomos.pb.go, which is generated from developer defined *.proto file.
type ElementDefine struct {
	// Element的配置。
	// Configuration of the Element.
	Config *ElementConfig

	// AtomId的构造器。
	// Constructor of AtomId.
	AtomIdConstructor AtomIdConstructor

	// Atom的构造器。
	// Constructor of Atom.
	AtomConstructor AtomConstructor

	// 一个用于保存Atom数据的函数。
	// A method to save Atom data.
	AtomSaver AtomSaver

	// 一个鉴别Kill信号是否合法的函数。
	// A method to authorize the Kill signal is allowed or not.
	AtomCanKill AtomCanKill

	// 一个存储Atom的Call方法的容器。
	// A container to store all the Call method of Atom.
	AtomCalls map[string]*ElementAtomMessage
}

// AtomId构造器的函数类型，CosmosNode可以是Local和Remote。
// Constructor Function Type of AtomId, CosmosNode can be Local or Remote.
type AtomIdConstructor func(c CosmosNode, atomName string) (Id, error)

// Atom构造器的函数类型，由用户定义，只会构建本地Atom。
// Constructor Function Type of Atom, which is defined by developer, will construct local Atom only.
type AtomConstructor func() Atom

// Atom保存函数的函数类型，只有有状态的Atom会被保存。
// Saver Function Type of Atom, only stateful Atom will be saved.
type AtomSaver func(Id, AtomStateful) error

// Atom鉴别Kill信号合法的函数。
// Kill Signal Authorization Function Type.
type AtomCanKill func(Id) bool

// TODO:
// 未来版本需要考虑支持Atom的负载均衡支持。
// Support in the future version, this function is used for load balance of Atom.
// type ElementLordScheduler func(CosmosClusterHelper)

// 从*.proto文件生成到*_atomos.pb.go文件中的，具体的Element对象。
// Concrete Element instance in *_atomos.pb.go, which is generated from developer defined *.proto file.
type ElementImplement interface {
	// 检查ElementImplement是否合法。
	// Check whether ElementImplement is legal or not.
	Check() error

	// 当前ElementImplement的信息，例如Element名称、版本号、日志记录级别、初始化的Atom数量。
	// Information of ElementImplement, such as name of Element, version, Log level and initial atom quantity.
	Info() (name string, version uint64, logLevel LogLevel, initNum int)

	// Atom构造器
	// Atom Constructor.
	AtomConstructor() Atom

	// Atom保存器
	// Atom Saver.
	AtomSaver(Id, AtomStateful) error

	// Atom是否可以被该Id的Atom终止。
	// Whether the Atom can be killed by the Id or not.
	AtomCanKill(Id) bool

	// TODO: Load balance
	// LordScheduler(CosmosClusterHelper)
	//AtomIdConstructor(*CosmosSelf, string, *AtomCore) Id
}

// Message处理器
type MessageHandler func(from Id, to Atom, in proto.Message) (out proto.Message, err error)

// Message解码器
type MessageDecoder func(buf []byte) (proto.Message, error)

// Element的Atom的调用信息。
// Element Atom Message Info.
type ElementAtomMessage struct {
	Handler MessageHandler
	InDec   MessageDecoder
	OutDec  MessageDecoder
}

// For creating ElementDefine instance in *_atomos.pb.go.
func NewDefineFromImplement(implement ElementImplement) *ElementDefine {
	name, version, logLevel, initNum := implement.Info()
	return &ElementDefine{
		Config: &ElementConfig{
			Name:        name,
			Version:     version,
			LogLevel:    logLevel,
			AtomInitNum: int32(initNum),
		},
		AtomConstructor: implement.AtomConstructor,
		AtomSaver:       implement.AtomSaver,
		AtomCanKill:     implement.AtomCanKill,
	}
}

// For creating AtomMessageConfig instance in ElementDefine of *_atomos.pb.go.
func NewAtomCallConfig(in, out proto.Message) *AtomMessageConfig {
	return &AtomMessageConfig{
		In:  MessageToAny(in),
		Out: MessageToAny(out),
	}
}

func MessageToAny(p proto.Message) *anypb.Any {
	any, _ := anypb.New(p)
	return any
}

func MessageUnmarshal(b []byte, p proto.Message) (proto.Message, error) {
	return p, proto.Unmarshal(b, p)
}
