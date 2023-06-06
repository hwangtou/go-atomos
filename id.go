package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"time"
)

// ID 是对Atomos的标识符，类似指针和句柄的概念。
// ID, an instance that similar to file descriptor of the Atom.
type ID interface {
	// GetIDInfo
	// 获取ID信息。
	GetIDInfo() *IDInfo
	// String
	// 获取ID的字符串表示。
	String() string

	// Cosmos 所在Cosmos节点的ID。
	// ID of the Cosmos node where the Cosmos is located.
	Cosmos() CosmosNode

	// State 当前运行状态
	// Current running state
	State() AtomosState
	// IdleTime 空闲的时长
	// Idle time
	IdleTime() time.Duration

	// SyncMessagingByName 同步调用
	// Sync call
	SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error)
	// AsyncMessagingByName 异步调用
	// Async call
	AsyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message, callback func(out proto.Message, err *Error))

	// DecoderByName 获得某个消息的解码器
	// Get decoder of a message
	DecoderByName(name string) (in, out MessageDecoder)

	// Kill 从其它Atom或者main发送Kill消息。
	// Send Kill message from other Atom or main.
	Kill(callerID SelfID, timeout time.Duration) *Error

	// SendWormhole
	// 发送Wormhole消息。
	// Send Wormhole message.
	SendWormhole(callerID SelfID, timeout time.Duration, wormhole AtomosWormhole) *Error

	// Internal

	// 当前的ID跟踪管理器
	// Current ID tracker manager.
	getIDTrackerManager() *idTrackerManager
	// 当前ID的邮箱GoID
	// GoID of the mailbox of current ID.
	getGoID() uint64
}

// ReleasableID 可以释放的ID，每当我们成功获得一个ID之后，都应该defer Release。
// ReleasableID, an ID that can be released, we should defer Release after we get an ID successfully.
type ReleasableID interface {
	ID
	Release()
}

// SelfID 是让Atomos对象内部访问的ID的概念。
// 通过SelfID，Atomos内部可以访问到自己的Cosmos（CosmosProcess）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
// SelfID is the concept of ID that can be accessed by Atomos object internally.
// Through SelfID, Atomos can access its Cosmos (CosmosProcess) internally, kill itself (KillSelf), and provide Log and Task related functions.
type SelfID interface {
	ID
	AtomosUtilities
	idFirstSyncCall

	// CosmosMain 获取Cosmos的本地实现。
	// Get local implementation of Cosmos.
	CosmosMain() *CosmosLocal

	// KillSelf Atomos从内部杀死自己。
	// Atomos kills itself internally.
	KillSelf()

	// Parallel 提供由recover保护的并发支持。
	// Provide concurrent support protected by recover.
	Parallel(func())

	// Config 获取自定义的配置
	// Get customized config
	Config() map[string][]byte

	// Internal

	pushAsyncMessageCallbackMailAndWaitReply(name string, in proto.Message, err *Error, callback func(out proto.Message, err *Error))
}

// AtomSelfID 是Atom的SelfID。
// AtomSelfID is SelfID of Atom.
type AtomSelfID interface {
	SelfID

	// Persistence 如果有实现AtomAutoData接口，那么可以通过该方法获取AtomAutoData。
	// If AtomAutoData interface is implemented, AtomAutoData can be obtained through this method.
	Persistence() AtomAutoData
}

// ElementSelfID 是Element的SelfID。
// ElementSelfID is SelfID of Element.
type ElementSelfID interface {
	SelfID

	// Persistence 如果有实现AutoDataPersistence接口，那么可以通过该方法获取AutoDataPersistence。
	// If AutoDataPersistence interface is implemented, AutoDataPersistence can be obtained through this method.
	Persistence() AutoDataPersistence

	// GetAtoms 获取当前Element中的所有的Atom。
	// Get all Atoms in current Element.
	GetAtoms() []*AtomLocal
	// GetAtomsInPattern 获取当前Element中所有符合pattern的Atom。
	// Get all Atoms that match pattern in current Element.
	GetAtomsInPattern(pattern string) []*AtomLocal
}
