package go_atomos

// CHECKED!

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// ElementImplementation
// 从*.proto文件生成到*_atomos.pb.go文件中的，ElementImplementation对象。
// ElementImplementation in *_atomos.pb.go, which is generated from developer defined *.proto file.
type ElementImplementation struct {
	Developer ElementDeveloper
	Interface *ElementInterface

	ElementHandlers map[string]MessageHandler
	AtomHandlers    map[string]MessageHandler
}

// ElementInterface
// 从*.proto文件生成到*_atomos.pb.go文件中的，ElementInterface对象。
// ElementInterface in *_atomos.pb.go, which is generated from developer defined *.proto file.
type ElementInterface struct {
	//// Element的名称。
	//// Name of Element
	//Name string

	// Element的配置。
	// Configuration of the Element.
	Config *ElementConfig

	// AtomSpawner

	ElementSpawner ElementSpawner
	AtomSpawner    AtomSpawner

	//// AtomId的构造器。
	//// Constructor of AtomId.
	////AtomIdConstructor AtomIdConstructor
	//
	////ElementIDConstructor IDConstructor
	////AtomIDConstructor    IDConstructor
	//
	//// 一个存储Atom的Call方法的容器。
	//// A holder to store all the Message method of Atom.
	//ElementMessages map[string]*ElementAtomMessage
	//AtomMessages    map[string]*ElementAtomMessage
}

type ElementSpawner func(s ElementSelfID, a Atomos, data proto.Message) *ErrorInfo
type AtomSpawner func(s AtomSelfID, a Atomos, arg, data proto.Message) *ErrorInfo

// AtomIdConstructor
// AtomId构造器的函数类型，CosmosNode可以是Local和Remote。
// Constructor Function Type of AtomId, CosmosNode can be Local or Remote.
//type AtomIdConstructor func(ID) ID
type IDConstructor func(ID) ID

// MessageHandler
// Message处理器
type MessageHandler func(from ID, to Atomos, in proto.Message) (out proto.Message, err *ErrorInfo)

// MessageDecoder
// Message解码器
type MessageDecoder func(buf []byte) (proto.Message, error)

// ElementAtomMessage
// Element的Atom的调用信息。
// Element Atom Message Info.
type ElementAtomMessage struct {
	InDec  MessageDecoder
	OutDec MessageDecoder
}

// NewInterfaceFromDeveloper
// For creating ElementInterface instance in *_atomos.pb.go.
func NewInterfaceFromDeveloper(name string, implement ElementDeveloper) *ElementInterface {
	var version uint64
	var logLevel LogLevel
	var atomInitNum int
	if customizeVersion, ok := implement.(ElementCustomizeVersion); ok {
		version = customizeVersion.GetElementVersion()
	}
	if customizeLogLevel, ok := implement.(ElementCustomizeLogLevel); ok {
		logLevel = customizeLogLevel.GetElementLogLevel()
	}
	if customizeAtomInitNum, ok := implement.(ElementCustomizeAtomInitNum); ok {
		atomInitNum = customizeAtomInitNum.GetElementAtomsInitNum()
	}
	return &ElementInterface{
		Config: &ElementConfig{
			Name:        name,
			Version:     version,
			LogLevel:    logLevel,
			AtomInitNum: int32(atomInitNum),
			Messages:    map[string]*AtomMessageConfig{},
		},
		ElementSpawner: nil,
		AtomSpawner:    nil,
		//AtomMessages:   nil,
	}
}

func NewImplementationFromDeveloper(developer ElementDeveloper) *ElementImplementation {
	return &ElementImplementation{
		Developer: developer,
	}
}

// For creating AtomMessageConfig instance in ElementInterface of *_atomos.pb.go.
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
