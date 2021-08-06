package go_atomos

// CHECKED!

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// 从*.proto文件生成到*_atomos.pb.go文件中的，ElementImplementation对象。
// ElementImplementation in *_atomos.pb.go, which is generated from developer defined *.proto file.
type ElementImplementation struct {
	Developer ElementDeveloper

	Interface *ElementInterface

	AtomHandlers map[string]MessageHandler
}

// 从*.proto文件生成到*_atomos.pb.go文件中的，ElementInterface对象。
// ElementInterface in *_atomos.pb.go, which is generated from developer defined *.proto file.
type ElementInterface struct {
	// Element的配置。
	// Configuration of the Element.
	Config *ElementConfig

	// AtomId的构造器。
	// Constructor of AtomId.
	AtomIdConstructor AtomIdConstructor

	// 一个存储Atom的Call方法的容器。
	// A container to store all the Message method of Atom.
	AtomMessages map[string]*ElementAtomMessage
}

// AtomId构造器的函数类型，CosmosNode可以是Local和Remote。
// Constructor Function Type of AtomId, CosmosNode can be Local or Remote.
type AtomIdConstructor func(Id) Id

// Message处理器
type MessageHandler func(from Id, to Atom, in proto.Message) (out proto.Message, err error)

// Message解码器
type MessageDecoder func(buf []byte) (proto.Message, error)

// Element的Atom的调用信息。
// Element Atom Message Info.
type ElementAtomMessage struct {
	InDec  MessageDecoder
	OutDec MessageDecoder
}

// For creating ElementInterface instance in *_atomos.pb.go.
func NewInterfaceFromDeveloper(implement ElementDeveloper) *ElementInterface {
	name, version, _, _ := implement.Info()
	return &ElementInterface{
		Config: &ElementConfig{
			Name:     name,
			Version:  version,
			Messages: map[string]*AtomMessageConfig{},
		},
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
