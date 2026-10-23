package atomos

import (
	"time"

	"google.golang.org/protobuf/proto"
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
	State() BaseAtomosState
	// IdleTime 空闲的时长
	// Idle time
	IdleTime() time.Duration

	// SyncMessagingByName 同步调用
	// Sync call
	SyncMessagingByName(callerID ID, name string, in proto.Message, ext ...ArgsForBaseAtomos) (out proto.Message, err error)

	// AsyncMessagingByName 异步调用
	// Async call
	AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(out proto.Message, err error), ext ...ArgsForBaseAtomos) (errBeforeExec error)

	// DecoderByName 获得某个消息的解码器
	// Get decoder of a message
	DecoderByName(name string) (in, out MessageDecoder)

	// Kill 从其它Atom或者main发送Kill消息。
	// Send Kill message from other Atom or main.
	Kill(callerID ID, ext ...ArgsForBaseAtomos) error

	// SendWormhole
	// 发送Wormhole消息。
	// Send Wormhole message.
	SendWormhole(callerID ID, wormhole BaseAtomosWormhole, ext ...ArgsForBaseAtomos) error
}

// idInternal is the framework-internal extension of ID. Every ID produced by
// the framework implements it; user-implemented IDs (mocks) never need to.
// Kernel paths that need these methods assert via asIDInternal and fail
// explicitly with ErrFrameworkInvalidID instead of silently misbehaving.
type idInternal interface {
	ID

	// asyncSet registers an async reply callback on the caller side.
	asyncSet(callback func(out proto.Message, err error)) (startupID, callbackID uint64)
	// asyncCallback delivers an async reply to the caller's registered callback.
	asyncCallback(callbackID ID, name string, startupID, asyncID uint64, reply proto.Message, err error)
	// getGoID returns the GoID of the mailbox goroutine of this ID.
	getGoID() uint64
}

// asIDInternal asserts that id carries the framework-internal extension. A
// user-implemented ID reaching a kernel path is a programming error, so the
// failure is reported explicitly with ErrFrameworkInvalidID.
func asIDInternal(id ID) (idInternal, *Error) {
	if ii, ok := id.(idInternal); ok {
		return ii, nil
	}
	return nil, NewErrorf(ErrCodeFrameworkInvalidID,
		"ID: implementation %T does not provide the framework-internal ID extension; passing a user-implemented ID to framework kernel paths is a programming error.", id).AddStack(nil)
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

	// CosmosMain 获取Cosmos的本地实现。
	// Get local implementation of Cosmos.
	CosmosMain() *CosmosLocal

	// KillSelf Atomos从内部杀死自己。
	// Atomos kills itself internally.
	KillSelf()

	// AsyncCallbackPending 返回在途异步回调数量。
	// 业务空闲清理可在 KillSelf 前检查:非空时推迟自杀,给在途回复留排空窗口
	// (atom 停机时框架会把在途回调以 ErrAsyncCallbackCancelledByHalt 终结)。
	AsyncCallbackPending() int

	// Parallel 提供由recover保护的并发支持。
	// Provide concurrent support protected by recover.
	Parallel(func())

	// Config 获取自定义的配置
	// Get customized config
	Config() map[string][]byte
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
	Element
	SelfID

	// Persistence 如果有实现AutoDataPersistence接口，那么可以通过该方法获取AutoDataPersistence。
	// If AutoData interface is implemented, AutoData can be obtained through this method.
	Persistence() AutoData

	// GetAtoms 获取当前Element中的所有的Atom。
	// Get all Atoms in current Element.
	GetAtoms() []*AtomLocal
	// GetAtomsInPattern 获取当前Element中所有符合pattern的Atom。
	// Get all Atoms that match pattern in current Element.
	GetAtomsInPattern(pattern string) []*AtomLocal
}

type remoteFakeSelfID interface {
	SelfID
	callerCounterRelease()
}
