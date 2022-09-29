package go_atomos

// CHECKED!

import (
	"google.golang.org/protobuf/proto"
	"time"
)

//
// Atom
//

const RunnableName = "AtomosRunnable"

// 暴露给Atom开发者使用的Atom接口。
// Some methods of Atom interface that expose Atom developers to use.

//
// ID
//

// ID 是Atom的类似句柄的对象。
// ID, an instance that similar to file descriptor of the Atom.
type ID interface {
	GetIDInfo() *IDInfo
	String() string

	// Release
	// 释放ID的引用计数
	// Release reference count of ID.
	// TODO:思考是否真的需要Release
	Release()

	// Cosmos
	// Atom所在Cosmos节点。
	// Cosmos Node of the Atom.
	Cosmos() CosmosNode

	// Element
	// Atom所属的Element类型。
	// Element type of the Atom.
	Element() Element

	// GetName
	// ID名称。
	// Name of the ID.
	GetName() string

	State() AtomosState
	IdleDuration() time.Duration

	//// GetVersion
	//// ElementInterface的版本。
	//// Version of ElementInterface.
	//GetVersion() uint64

	MessageByName(from ID, name string, in proto.Message) (proto.Message, *ErrorInfo)

	DecoderByName(name string) (MessageDecoder, MessageDecoder)

	// Kill
	// 从其它Atom或者main发送Kill消息。
	// write Kill signal from other Atom or main.
	Kill(from ID) *ErrorInfo

	// SendWormhole
	// Send wormhole to atomos.
	SendWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo

	// Info

	// Transaction

	// Internal

	getCallChain() []ID
	getElementLocal() *ElementLocal
	getAtomLocal() *AtomLocal
}

//type CallName interface {
//	// CallNameWithProtoBuffer
//	// 直接接收调用
//	CallName(from ID, name string, buf []byte, protoOrJSON bool) ([]byte, *ErrorInfo)
//}

//type CallJson interface {
//	// CallNameWithJson
//	// 直接接收调用
//	CallNameWithJson(name string, buf []byte) ([]byte, error)
//}

//
// SelfID
//

// SelfID
// 是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosProcess）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// SelfID, a concept that provide Atom resource access to inner Atom.
// With SelfID, Atom can access its self-main with "CosmosProcess", can kill itself use "KillSelf" from inner.
// It also provides Log and Tasks method to inner Atom.
type SelfID interface {
	ID
	AtomosUtilities

	// CosmosMain
	// 获取Atom的CosmosProcess。
	// Access to the CosmosProcess of the Atom.
	CosmosMain() *CosmosMain

	//ElementLocal() *ElementLocal

	// KillSelf
	// Atom从内部杀死自己。
	// Atom kills itself from inner.
	KillSelf()

	Parallel(func())

	Config() map[string]string

	MessageSelfByName(from ID, name string, buf []byte, protoOrJSON bool) ([]byte, *ErrorInfo)
}

type AtomSelfID interface {
	SelfID

	Persistence() AtomAutoDataPersistence
}

type ElementSelfID interface {
	SelfID

	Persistence() ElementCustomizeAutoDataPersistence
}

//type ParallelSelf interface {
//	ID
//	CosmosMain() *CosmosMain
//	ElementSelf() *ElementLocal
//	KillSelf()
//	Log() Logging
//}

type ParallelFn func(self SelfID, message proto.Message, id ...ID)

//type ParallelFn func(self ParallelSelf, message proto.Message, id ...ID)

//
// Wormhole
//

type AtomosWormhole interface {
}

//// WormholeAtom
//// 支持WormholeAtom的Atom，可以得到Wormhole的支持。
//// Implement WormholeAtom interface to gain wormhole support.
//type WormholeAtom interface {
//	Atomos
//	AcceptWorm(control WormholeControl) error
//	CloseWorm(control WormholeControl)
//}
//
//// WormholeID
//// 是ID接口的延伸，提供向WormholeAtom发送Wormhole的可能。
//// Extend of ID, it lets send wormhole to WormholeAtom become possible.
//type WormholeID interface {
//	ID
//	Accept(daemon WormholeDaemon) error
//}
//
//// WormholeDaemon
//// 通常包装着wormhole（真实网络连接）。负责接受信息并处理，并提供操作接口。
//// WormholeDaemon generally used to wrap the real connection. It handles message processing,
//// and provides operating methods.
//type WormholeDaemon interface {
//	// StartRunning
//	// 加载&卸载
//	// Loaded & Unloaded
//	StartRunning(SelfID) error
//	WormholeControl
//}
//
//// WormholeControl
//// 向WormholeAtom提供发送和关闭接口。
//// WormholeControl provides Send and Close to WormholeAtom.
//type WormholeControl interface {
//	Send([]byte) error
//	Close(isKickByNew bool) error
//}
