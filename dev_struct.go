package go_atomos

import (
	"google.golang.org/protobuf/proto"
)

// NewImplementationFromDeveloper
// 用于把开发者的实现转换成*_atomos.pb.go中的ElementImplementation实例。
// For converting developer's implementation to ElementImplementation instance in *_atomos.pb.go.
func NewImplementationFromDeveloper(developer ElementDeveloper) *ElementImplementation {
	return &ElementImplementation{
		Developer: developer,
	}
}

// NewInterfaceFromDeveloper
// 用于把开发者的实现转换成*_atomos.pb.go中的ElementInterface实例。
// For creating ElementInterface instance in *_atomos.pb.go.
func NewInterfaceFromDeveloper(name string, implement ElementDeveloper) *ElementInterface {
	var version uint64
	var logLevel LogLevel
	var atomInitNum int
	if customizeVersion, ok := implement.(ElementVersion); ok {
		version = customizeVersion.GetElementVersion()
	}
	if customizeLogLevel, ok := implement.(ElementLogLevel); ok {
		logLevel = customizeLogLevel.GetElementLogLevel()
	}
	if customizeAtomInitNum, ok := implement.(ElementAtomInitNum); ok {
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
		ElementSpawner:  nil,
		AtomSpawner:     nil,
		ElementDecoders: nil,
		AtomDecoders:    nil,
	}
}

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

type MessageHandler func(from ID, to Atomos, in proto.Message) (out proto.Message, err *Error)
type ScaleHandler func(from ID, e Atomos, message string, in proto.Message) (id ID, err *Error)

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
}

type IOMessageDecoder struct {
	InDec  MessageDecoder
	OutDec MessageDecoder
}
type MessageDecoder func(buf []byte, protoOrJSON bool) (proto.Message, *Error)
type ElementSpawner func(s ElementSelfID, a Atomos, data proto.Message) *Error
type AtomSpawner func(s AtomSelfID, a Atomos, arg, data proto.Message) *Error
