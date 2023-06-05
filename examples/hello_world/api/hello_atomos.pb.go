// Code generated by protoc-gen-go-atomos. DO NOT EDIT.

package api

import (
	go_atomos "github.com/hwangtou/go-atomos"
	proto "google.golang.org/protobuf/proto"
)

const HelloName = "Hello"

////////////////////////////////////
/////////// 需要实现的接口 ///////////
////// Interface to implement //////
////////////////////////////////////

//
// The greeting service definition.
// New line
//

// HelloElement is the atomos implements of Hello element.

type HelloElement interface {
	go_atomos.Atomos
	// Spawn Element
	Spawn(self go_atomos.ElementSelfID, data *HelloData) *go_atomos.Error

	// Sends a greeting
	SayHello(from go_atomos.ID, in *HelloReq) (out *HelloResp, err *go_atomos.Error)

	// Scale Methods

	// Scale Bonjour
	ScaleBonjour(from go_atomos.ID, in *BonjourReq) (*HelloAtomID, *go_atomos.Error)
}

// HelloAtom is the atomos implements of Hello atom.

type HelloAtom interface {
	go_atomos.Atomos

	// Spawn
	Spawn(self go_atomos.AtomSelfID, arg *HelloSpawnArg, data *HelloData) *go_atomos.Error

	// Sends a greeting
	SayHello(from go_atomos.ID, in *HelloReq) (out *HelloResp, err *go_atomos.Error)
	// Build net
	BuildNet(from go_atomos.ID, in *BuildNetReq) (out *BuildNetResp, err *go_atomos.Error)
	// Make panic
	MakePanic(from go_atomos.ID, in *MakePanicIn) (out *MakePanicOut, err *go_atomos.Error)

	// Scale Methods

	// Scale Bonjour
	ScaleBonjour(from go_atomos.ID, in *BonjourReq) (out *BonjourResp, err *go_atomos.Error)
}

////////////////////////////////////
/////////////// 识别符 //////////////
//////////////// ID ////////////////
////////////////////////////////////

// Element: Hello

type HelloElementID struct {
	go_atomos.ID
	*go_atomos.IDTracker
}

// 获取某节点中的ElementID
// Get element id of node
func GetHelloElementID(c go_atomos.CosmosNode) (*HelloElementID, *go_atomos.Error) {
	ca, err := c.CosmosGetElementID(HelloName)
	if err != nil {
		return nil, err
	}
	return &HelloElementID{ca, nil}, nil
}

// Sends a greeting
// Sync
func (c *HelloElementID) SayHello(callerID go_atomos.SelfID, in *HelloReq, ext ...interface{}) (out *HelloResp, err *go_atomos.Error) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloElementValue.SayHello }
	return helloElementMessengerValue.SayHello().SyncElement(c, callerID, in, ext...)
}

// Async
func (c *HelloElementID) AsyncSayHello(callerID go_atomos.SelfID, in *HelloReq, callback func(out *HelloResp, err *go_atomos.Error), ext ...interface{}) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloElementValue.SayHello }
	helloElementMessengerValue.SayHello().AsyncElement(c, callerID, in, callback, ext...)
}

// GetID
func (c *HelloElementID) ScaleBonjourGetID(callerID go_atomos.SelfID, in *BonjourReq, ext ...interface{}) (id *HelloAtomID, err *go_atomos.Error) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloElementValue.ScaleBonjour }
	i, tracker, err := helloElementMessengerValue.ScaleBonjour().GetScaleID(c, callerID, HelloName, in, ext...)
	if err != nil {
		return nil, err.AddStack(nil)
	}
	return &HelloAtomID{i, tracker}, nil
}

// Sync
func (c *HelloElementID) ScaleBonjour(callerID go_atomos.SelfID, in *BonjourReq, ext ...interface{}) (out *BonjourResp, err *go_atomos.Error) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloElementValue.ScaleBonjour }
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloAtomValue.ScaleBonjour }
	id, err := c.ScaleBonjourGetID(callerID, in, ext...)
	if err != nil {
		return nil, err.AddStack(nil)
	}
	defer id.Release()
	return helloAtomMessengerValue.ScaleBonjour().SyncAtom(id, callerID, in)
}

// Async
func (c *HelloElementID) ScaleAsyncBonjour(callerID go_atomos.SelfID, in *BonjourReq, callback func(*BonjourResp, *go_atomos.Error), ext ...interface{}) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloElementValue.ScaleBonjour }
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloAtomValue.ScaleBonjour }
	id, err := c.ScaleBonjourGetID(callerID, in, ext...)
	if err != nil {
		callback(nil, err.AddStack(nil))
		return
	}
	defer id.Release()
	helloAtomMessengerValue.ScaleBonjour().AsyncAtom(id, callerID, in, callback, ext...)
}

// Atom: Hello

type HelloAtomID struct {
	go_atomos.ID
	*go_atomos.IDTracker
}

// 创建（自旋）某节点中的一个Atom，并返回AtomID
// Create (spin) an atom in a node and return the AtomID
func SpawnHelloAtom(c go_atomos.CosmosNode, name string, arg *HelloSpawnArg) (*HelloAtomID, *go_atomos.Error) {
	id, tracker, err := c.CosmosSpawnAtom(HelloName, name, arg)
	if id == nil {
		return nil, err.AddStack(nil)
	}
	return &HelloAtomID{id, tracker}, err
}

// 获取某节点中的AtomID
// Get atom id of node
func GetHelloAtomID(c go_atomos.CosmosNode, name string) (*HelloAtomID, *go_atomos.Error) {
	ca, tracker, err := c.CosmosGetAtomID(HelloName, name)
	if err != nil {
		return nil, err
	}
	return &HelloAtomID{ca, tracker}, nil
}

// Sync
func (c *HelloAtomID) ScaleBonjour(callerID go_atomos.SelfID, in *BonjourReq, ext ...interface{}) (out *BonjourResp, err *go_atomos.Error) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloAtomValue.ScaleBonjour }
	return helloAtomMessengerValue.ScaleBonjour().SyncAtom(c, callerID, in, ext...)
}

// Async
func (c *HelloAtomID) AsyncScaleBonjour(callerID go_atomos.SelfID, in *BonjourReq, callback func(out *BonjourResp, err *go_atomos.Error), ext ...interface{}) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloAtomValue.ScaleBonjour }
	helloAtomMessengerValue.ScaleBonjour().AsyncAtom(c, callerID, in, callback, ext...)
}

// Sync
func (c *HelloAtomID) SayHello(callerID go_atomos.SelfID, in *HelloReq, ext ...interface{}) (out *HelloResp, err *go_atomos.Error) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloAtomValue.SayHello }
	return helloAtomMessengerValue.SayHello().SyncAtom(c, callerID, in, ext...)
}

// Async
func (c *HelloAtomID) AsyncSayHello(callerID go_atomos.SelfID, in *HelloReq, callback func(out *HelloResp, err *go_atomos.Error), ext ...interface{}) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloAtomValue.SayHello }
	helloAtomMessengerValue.SayHello().AsyncAtom(c, callerID, in, callback, ext...)
}

// Sync
func (c *HelloAtomID) BuildNet(callerID go_atomos.SelfID, in *BuildNetReq, ext ...interface{}) (out *BuildNetResp, err *go_atomos.Error) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloAtomValue.BuildNet }
	return helloAtomMessengerValue.BuildNet().SyncAtom(c, callerID, in, ext...)
}

// Async
func (c *HelloAtomID) AsyncBuildNet(callerID go_atomos.SelfID, in *BuildNetReq, callback func(out *BuildNetResp, err *go_atomos.Error), ext ...interface{}) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloAtomValue.BuildNet }
	helloAtomMessengerValue.BuildNet().AsyncAtom(c, callerID, in, callback, ext...)
}

// Sync
func (c *HelloAtomID) MakePanic(callerID go_atomos.SelfID, in *MakePanicIn, ext ...interface{}) (out *MakePanicOut, err *go_atomos.Error) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloAtomValue.MakePanic }
	return helloAtomMessengerValue.MakePanic().SyncAtom(c, callerID, in, ext...)
}

// Async
func (c *HelloAtomID) AsyncMakePanic(callerID go_atomos.SelfID, in *MakePanicIn, callback func(out *MakePanicOut, err *go_atomos.Error), ext ...interface{}) {
	/* CODE JUMPER 代码跳转 */ _ = func() { _ = helloAtomValue.MakePanic }
	helloAtomMessengerValue.MakePanic().AsyncAtom(c, callerID, in, callback, ext...)
}

// Atomos Interface

func GetHelloImplement(dev go_atomos.ElementDeveloper) *go_atomos.ElementImplementation {
	elem := go_atomos.NewImplementationFromDeveloper(dev)
	elem.Interface = GetHelloInterface(dev)
	elem.ElementHandlers = map[string]go_atomos.MessageHandler{
		"SayHello": func(from go_atomos.ID, to go_atomos.Atomos, in proto.Message) (proto.Message, *go_atomos.Error) {
			a, i, err := helloElementMessengerValue.SayHello().ExecuteAtom(to, in)
			if err != nil {
				return nil, err.AddStack(nil)
			}
			return a.SayHello(from, i)
		},
	}
	elem.AtomHandlers = map[string]go_atomos.MessageHandler{
		"ScaleBonjour": func(from go_atomos.ID, to go_atomos.Atomos, in proto.Message) (proto.Message, *go_atomos.Error) {
			a, i, err := helloAtomMessengerValue.ScaleBonjour().ExecuteAtom(to, in)
			if err != nil {
				return nil, err.AddStack(nil)
			}
			return a.ScaleBonjour(from, i)
		},
		"SayHello": func(from go_atomos.ID, to go_atomos.Atomos, in proto.Message) (proto.Message, *go_atomos.Error) {
			a, i, err := helloAtomMessengerValue.SayHello().ExecuteAtom(to, in)
			if err != nil {
				return nil, err.AddStack(nil)
			}
			return a.SayHello(from, i)
		},
		"BuildNet": func(from go_atomos.ID, to go_atomos.Atomos, in proto.Message) (proto.Message, *go_atomos.Error) {
			a, i, err := helloAtomMessengerValue.BuildNet().ExecuteAtom(to, in)
			if err != nil {
				return nil, err.AddStack(nil)
			}
			return a.BuildNet(from, i)
		},
		"MakePanic": func(from go_atomos.ID, to go_atomos.Atomos, in proto.Message) (proto.Message, *go_atomos.Error) {
			a, i, err := helloAtomMessengerValue.MakePanic().ExecuteAtom(to, in)
			if err != nil {
				return nil, err.AddStack(nil)
			}
			return a.MakePanic(from, i)
		},
	}
	elem.ScaleHandlers = map[string]go_atomos.ScaleHandler{
		"ScaleBonjour": func(from go_atomos.ID, e go_atomos.Atomos, message string, in proto.Message) (id go_atomos.ID, err *go_atomos.Error) {
			a, i, err := helloElementMessengerValue.ScaleBonjour().ExecuteScale(e, in)
			if err != nil {
				return nil, err.AddStack(nil)
			}
			return a.ScaleBonjour(from, i)
		},
	}
	return elem
}
func GetHelloInterface(dev go_atomos.ElementDeveloper) *go_atomos.ElementInterface {
	elem := go_atomos.NewInterfaceFromDeveloper(HelloName, dev)
	elem.ElementSpawner = func(s go_atomos.ElementSelfID, a go_atomos.Atomos, data proto.Message) *go_atomos.Error {
		dataT, _ := data.(*HelloData)
		elem, ok := a.(HelloElement)
		if !ok {
			return go_atomos.NewErrorf(go_atomos.ErrElementNotImplemented, "Element not implemented, type=(HelloElement)")
		}
		return elem.Spawn(s, dataT)
	}
	elem.AtomSpawner = func(s go_atomos.AtomSelfID, a go_atomos.Atomos, arg, data proto.Message) *go_atomos.Error {
		argT, _ := arg.(*HelloSpawnArg)
		dataT, _ := data.(*HelloData)
		atom, ok := a.(HelloAtom)
		if !ok {
			return go_atomos.NewErrorf(go_atomos.ErrAtomNotImplemented, "Atom not implemented, type=(HelloAtom)")
		}
		return atom.Spawn(s, argT, dataT)
	}
	elem.ElementDecoders = map[string]*go_atomos.IOMessageDecoder{
		"SayHello":     helloElementMessengerValue.SayHello().Decoder(&HelloReq{}, &HelloResp{}),
		"ScaleBonjour": helloElementMessengerValue.ScaleBonjour().Decoder(&BonjourReq{}, &BonjourResp{}),
	}
	elem.AtomDecoders = map[string]*go_atomos.IOMessageDecoder{
		"ScaleBonjour": helloAtomMessengerValue.ScaleBonjour().Decoder(&BonjourReq{}, &BonjourResp{}),
		"SayHello":     helloAtomMessengerValue.SayHello().Decoder(&HelloReq{}, &HelloResp{}),
		"BuildNet":     helloAtomMessengerValue.BuildNet().Decoder(&BuildNetReq{}, &BuildNetResp{}),
		"MakePanic":    helloAtomMessengerValue.MakePanic().Decoder(&MakePanicIn{}, &MakePanicOut{}),
	}
	return elem
}

// Atomos Internal

// Element Define

type helloElementMessenger struct{}

func (m helloElementMessenger) SayHello() go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloElement, *HelloReq, *HelloResp] {
	return go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloElement, *HelloReq, *HelloResp]{nil, nil, "SayHello"}
}
func (m helloElementMessenger) ScaleBonjour() go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloElement, *BonjourReq, *BonjourResp] {
	return go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloElement, *BonjourReq, *BonjourResp]{nil, nil, "ScaleBonjour"}
}

var helloElementMessengerValue helloElementMessenger
var helloElementValue HelloElement

// Atom Define

type helloAtomMessenger struct{}

func (m helloAtomMessenger) ScaleBonjour() go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloAtom, *BonjourReq, *BonjourResp] {
	return go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloAtom, *BonjourReq, *BonjourResp]{nil, nil, "ScaleBonjour"}
}
func (m helloAtomMessenger) SayHello() go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloAtom, *HelloReq, *HelloResp] {
	return go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloAtom, *HelloReq, *HelloResp]{nil, nil, "SayHello"}
}
func (m helloAtomMessenger) BuildNet() go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloAtom, *BuildNetReq, *BuildNetResp] {
	return go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloAtom, *BuildNetReq, *BuildNetResp]{nil, nil, "BuildNet"}
}
func (m helloAtomMessenger) MakePanic() go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloAtom, *MakePanicIn, *MakePanicOut] {
	return go_atomos.Messenger[*HelloElementID, *HelloAtomID, HelloAtom, *MakePanicIn, *MakePanicOut]{nil, nil, "MakePanic"}
}

var helloAtomMessengerValue helloAtomMessenger
var helloAtomValue HelloAtom
