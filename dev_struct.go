package atomos

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
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
	if customizeVersion, ok := implement.(ElementVersion); ok {
		version = customizeVersion.GetElementVersion()
	}
	return &ElementInterface{
		Config: &ElementConfig{
			Name:     name,
			Version:  version,
			Messages: map[string]*AtomMessageConfig{},
		},
		ElementSpawner:  nil,
		AtomSpawner:     nil,
		ElementDecoders: nil,
		AtomDecoders:    nil,
	}
}

// fillElementMessages populates Config.Messages — the element's message
// schema metadata — from the registered decoders: for every element/atom
// message name, the in and out message types as Any-wrapped empty
// instances. Consumed by introspection surfaces (debug server, future
// management/MCP APIs). Idempotent: a non-empty map (hand- or
// generator-filled) is left untouched, so element reload is safe.
func fillElementMessages(iface *ElementInterface) {
	if iface == nil || iface.Config == nil || len(iface.Config.Messages) > 0 {
		return
	}
	put := func(name string, dec *IOMessageDecoder) {
		if dec == nil || dec.InDec == nil || dec.OutDec == nil {
			return
		}
		in, errIn := dec.InDec(nil, true)
		out, errOut := dec.OutDec(nil, true)
		if errIn != nil || errOut != nil || in == nil || out == nil {
			return
		}
		inAny, errInAny := anypb.New(in)
		outAny, errOutAny := anypb.New(out)
		if errInAny != nil || errOutAny != nil {
			return
		}
		iface.Config.Messages[name] = &AtomMessageConfig{In: inAny, Out: outAny}
	}
	for name, dec := range iface.ElementDecoders {
		put(name, dec)
	}
	for name, dec := range iface.AtomDecoders {
		put(name, dec)
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
}

type MessageHandler func(from ID, to Atomos, in proto.Message) (out proto.Message, err error)

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
type MessageDecoder func(buf []byte, protoOrJSON bool) (proto.Message, error)
type ElementSpawner func(s ElementSelfID, a Atomos, data proto.Message, args ...ArgsForBaseAtomos) error
type AtomSpawner func(s AtomSelfID, a Atomos, arg, data proto.Message, args ...ArgsForBaseAtomos) error
