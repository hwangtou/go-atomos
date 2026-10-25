package atomos

import (
	"time"

	"google.golang.org/protobuf/proto"
)

// ID is the handle of an Atomos instance — conceptually similar to a file
// descriptor: it identifies the target of messaging and lifecycle calls.
// 对Atomos的标识符，类似指针和句柄的概念。
type ID interface {
	// GetIDInfo returns the structured identity of this ID.
	// 获取ID信息。
	GetIDInfo() *IDInfo
	// String returns the string representation of this ID.
	// 获取ID的字符串表示。
	String() string

	// Cosmos returns the Cosmos node where this ID lives.
	// 所在Cosmos节点的ID。
	Cosmos() CosmosNode

	// State returns the current running state.
	// 当前运行状态。
	State() BaseAtomosState
	// IdleTime returns how long the actor has been idle.
	// 空闲的时长。
	IdleTime() time.Duration

	// SyncMessagingByName makes a synchronous message call and blocks until
	// the reply arrives or the call fails.
	// 同步调用。
	SyncMessagingByName(callerID ID, name string, in proto.Message, ext ...ArgsForBaseAtomos) (out proto.Message, err error)

	// AsyncMessagingByName makes an asynchronous message call; the reply is
	// delivered to callback. A non-nil return means the call failed before
	// execution and callback will never be invoked.
	// 异步调用。
	AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(out proto.Message, err error), ext ...ArgsForBaseAtomos) (errBeforeExec error)

	// DecoderByName returns the input/output decoders of a message.
	// 获得某个消息的解码器。
	DecoderByName(name string) (in, out MessageDecoder)

	// Kill asks this actor to stop, sent from another Atom or from main.
	// 从其它Atom或者main发送Kill消息。
	Kill(callerID ID, ext ...ArgsForBaseAtomos) error

	// SendWormhole sends a wormhole message carrying arbitrary typed data.
	// 发送Wormhole消息。
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
	// asyncPop removes a pending callback when delivery is no longer possible.
	asyncPop(asyncID uint64) func(out proto.Message, err error)
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

// ReleasableID is an ID that can be released. Whenever an ID is acquired
// successfully, Release should be deferred.
// 可以释放的ID，每当我们成功获得一个ID之后，都应该defer Release。
type ReleasableID interface {
	ID
	Release()
}

// SelfID is the ID an Atomos instance uses to access itself from the inside.
// Through SelfID an actor can reach its Cosmos node (CosmosMain), kill itself
// (KillSelf), and use the Log and Task utilities.
// 让Atomos对象内部访问的ID的概念。
// 通过SelfID，Atomos内部可以访问到自己的Cosmos（CosmosProcess）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
type SelfID interface {
	ID
	AtomosUtilities

	// CosmosMain returns the Cosmos node (the local implementation, as a
	// CosmosNode). Callers needing *CosmosLocal specifically can type-assert.
	// 获取Cosmos节点（本地实现以 CosmosNode 接口返回）。
	// 需要 *CosmosLocal 具体类型处自行类型断言。
	CosmosMain() CosmosNode

	// KillSelf makes the actor kill itself from the inside.
	// Atomos从内部杀死自己。
	KillSelf()

	// AsyncCallbackPending returns the number of in-flight async callbacks.
	// Idle-cleanup logic can check it before KillSelf: when non-zero, defer
	// the self-kill to leave a drain window for in-flight replies (at atom
	// halt the framework terminates them with ErrAsyncCallbackCancelledByHalt).
	// 返回在途异步回调数量。
	AsyncCallbackPending() int

	// Parallel provides concurrency protected by recover.
	// 提供由recover保护的并发支持。
	Parallel(func())

	// Config returns the customized config.
	// 获取自定义的配置。
	Config() map[string][]byte
}

// AtomSelfID is the SelfID of an Atom.
// Atom的SelfID。
type AtomSelfID interface {
	SelfID

	// Persistence returns the AtomAutoData when the ElementDeveloper
	// implements the AutoData interface, nil otherwise.
	// 如果有实现AtomAutoData接口，那么可以通过该方法获取AtomAutoData。
	Persistence() AtomAutoData
}

// ElementSelfID is the SelfID of an Element.
// Element的SelfID。
type ElementSelfID interface {
	Element
	SelfID

	// Persistence returns the AutoData when the ElementDeveloper implements
	// the AutoData interface, nil otherwise.
	// 如果有实现AutoData接口，那么可以通过该方法获取AutoData。
	Persistence() AutoData

	// GetAtoms returns all Atoms of this Element.
	// 获取当前Element中的所有的Atom。
	GetAtoms() []*AtomLocal
	// GetAtomsInPattern returns all Atoms of this Element matching pattern.
	// 获取当前Element中所有符合pattern的Atom。
	GetAtomsInPattern(pattern string) []*AtomLocal
}

type remoteFakeSelfID interface {
	SelfID
	callerCounterRelease()
}
