package go_atomos

// CHECKED!

import (
	"fmt"
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
	IdleTime() time.Duration

	MessageByName(from ID, name string, timeout time.Duration, in proto.Message) (proto.Message, *Error)

	DecoderByName(name string) (MessageDecoder, MessageDecoder)

	// Kill
	// 从其它Atom或者main发送Kill消息。
	// write Kill signal from other Atom or main.
	Kill(from ID, timeout time.Duration) *Error

	// SendWormhole
	// Send wormhole to atomos.
	SendWormhole(from ID, timeout time.Duration, wormhole AtomosWormhole) *Error

	// Info

	// Transaction

	// Internal

	getElementLocal() *ElementLocal
	getAtomLocal() *AtomLocal
	getElementRemote() *ElementRemote
	getAtomRemote() *AtomRemote
	getIDTrackerManager() *IDTrackerManager

	getCurCallChain() string

	First() ID
}

type FirstID struct {
	callID uint64
	ID
}

func (a *FirstID) getFirstIDCallChain() string {
	return fmt.Sprintf("%s:%d", a.ID, a.callID)
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

	// CosmosMain
	// 获取Atom的CosmosProcess。
	// Access to the CosmosProcess of the Atom.
	CosmosMain() *CosmosMain

	// KillSelf
	// Atom从内部杀死自己。
	// Atom kills itself from inner.
	KillSelf()

	Parallel(func())

	Config() map[string][]byte

	MessageSelfByName(from ID, name string, buf []byte, protoOrJSON bool) ([]byte, *Error)
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
