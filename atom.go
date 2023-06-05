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
//
// ID，相当于Atom的句柄的概念。
// 通过ID，可以访问到Atom所在的Cosmos、Element、Name，以及发送Kill信息，但是否能成功Kill，还需要AtomCanKill函数的认证。
// 直接用AtomLocal继承ID，因此本地的ID直接使用AtomLocal的引用即可。
//
// ID, a concept similar to file descriptor of an atomos.
// With ID, we can access the Cosmos, Element and Name of the Atom. We can also send Kill signal to the Atom,
// then the AtomCanKill method judge kill it or not.
// AtomLocal implements ID interface directly, so local ID is able to use AtomLocal reference directly.
type ID interface {
	GetIDInfo() *IDInfo
	String() string

	// Cosmos
	// Atom所在Cosmos节点。
	// Cosmos Node of the Atom.
	Cosmos() CosmosNode

	State() AtomosState
	IdleTime() time.Duration

	SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error)
	AsyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message, callback func(out proto.Message, err *Error))

	DecoderByName(name string) (in, out MessageDecoder)

	// Kill
	// 从其它Atom或者main发送Kill消息。
	// write Kill signal from other Atom or main.
	Kill(callerID SelfID, timeout time.Duration) *Error

	// SendWormhole
	// Send wormhole to atomos.
	SendWormhole(callerID SelfID, timeout time.Duration, wormhole AtomosWormhole) *Error

	// Internal

	getIDTrackerManager() *idTrackerManager
	getGoID() uint64
}

type ReleasableID interface {
	ID
	Release()
}

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
	idFirstSyncCall

	// CosmosLocal
	// 获取Atom的CosmosProcess。
	// Access to the CosmosProcess of the Atom.
	CosmosMain() *CosmosLocal

	// KillSelf
	// Atom从内部杀死自己。
	// Atom kills itself from inner.
	KillSelf()

	Parallel(func())

	Config() map[string][]byte

	pushAsyncMessageCallbackMailAndWaitReply(name string, in proto.Message, err *Error, callback func(out proto.Message, err *Error))
	//MessageSelfByName(from ID, name string, buf []byte, protoOrJSON bool) ([]byte, *Error)
}

type AtomSelfID interface {
	SelfID

	Persistence() AtomAutoDataPersistence
}

type ElementSelfID interface {
	SelfID

	Persistence() ElementCustomizeAutoDataPersistence
	GetAtoms() []*AtomLocal
}

type ParallelFn func(self SelfID, message proto.Message, id ...ID)

//
// Wormhole
//

type AtomosWormhole interface {
}
