package go_atomos

// CHECKED!

import (
	"encoding/json"
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
	ScaleHandlers   map[string]ScaleHandler
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

	// 一个存储Atom的Call方法的容器。
	// A holder to store all the Message method of Atom.
	ElementDecoders map[string]*IOMessageDecoder
	AtomDecoders    map[string]*IOMessageDecoder

	//// AtomID的构造器。
	//// Constructor of AtomID.
	////AtomIDConstructor AtomIDConstructor
	//
	////ElementIDConstructor IDConstructor
	////AtomIDConstructor    IDConstructor
}

type ElementSpawner func(s ElementSelfID, a Atomos, data proto.Message) *ErrorInfo
type AtomSpawner func(s AtomSelfID, a Atomos, arg, data proto.Message) *ErrorInfo

type ScaleHandler func(from ID, e Atomos, message string, in proto.Message) (id ID, err *ErrorInfo)

// IDConstructor
// ID构造器的函数类型，CosmosNode可以是Local和Remote。
// Constructor Function Type of AtomID, CosmosNode can be Local or Remote.
type IDConstructor func(ID) ID

// MessageHandler
// Message处理器
type MessageHandler func(from ID, to Atomos, in proto.Message) (out proto.Message, err *ErrorInfo)

// MessageDecoder
// Message解码器
type MessageDecoder func(buf []byte, protoOrJSON bool) (proto.Message, *ErrorInfo)

//type JSONDecoder func(buf []byte, protoOrJSON bool) (proto.Message, *ErrorInfo)

// ElementAtomMessage
// Element的Atom的调用信息。
// Element Atom Message Info.

type IOMessageDecoder struct {
	InDec  MessageDecoder
	OutDec MessageDecoder
}

//type IOJSONDecoder struct {
//	InDec  JSONDecoder
//	OutDec JSONDecoder
//}

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

// NewAtomCallConfig
// For creating AtomMessageConfig instance in ElementInterface of *_atomos.pb.go.
func NewAtomCallConfig(in, out proto.Message) *AtomMessageConfig {
	return &AtomMessageConfig{
		In:  MessageToAny(in),
		Out: MessageToAny(out),
	}
}

func MessageToAny(p proto.Message) *anypb.Any {
	a, _ := anypb.New(p)
	return a
}

func MessageUnmarshal(b []byte, p proto.Message, protoOrJSON bool) (proto.Message, *ErrorInfo) {
	if protoOrJSON {
		if err := proto.Unmarshal(b, p); err != nil {
			return nil, NewErrorf(ErrAtomMessageArgType, "Argument unmarshal failed, err=(%v)", err)
		}
	} else {
		if err := json.Unmarshal(b, p); err != nil {
			return nil, NewErrorf(ErrAtomMessageArgType, "Argument unmarshal failed, err=(%v)", err)
		}
	}
	return p, nil
}

//func JSONUnmarshal(b []byte, p proto.Message) (proto.Message, *ErrorInfo) {
//	if err := json.Unmarshal(b, p); err != nil {
//		return nil, NewErrorf(ErrAtomMessageArgType, "Argument unmarshal failed, err=(%v)", err)
//	}
//	return p, nil
//}
