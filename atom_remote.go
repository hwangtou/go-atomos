package atomos

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// optimise: elementRemote 中的 atomRemote map 存在内存泄漏（无释放操作）
// 经观察，version 和 callerCounter 无实际用途，考虑到这点，AtomRemote 可作为无状态结构（内存占用也不大），不再需要缓存，随用随创建

// AtomRemoteInSourceProcess 创建用于发送的目标AtomRemote
type AtomRemoteInSourceProcess struct {
	*AtomRemote
}

func newAtomRemoteInSourceProcess(e *ElementRemote, info *IDInfo) ID {
	return &AtomRemoteInSourceProcess{
		AtomRemote: &AtomRemote{
			remote:  newBaseRemote(e.cosmos, info),
			element: e,
		},
	}
}

// newAtomRemoteInSourceProcessWithPinnedConn creates an AtomRemote that pins
// the given connection, so calls to it keep targeting the same version even
// after CosmosRemote.current switches. The conn should be the one the caller
// used to resolve this atom (e.remote.getCliConn() at creation time).
func newAtomRemoteInSourceProcessWithPinnedConn(e *ElementRemote, info *IDInfo, conn *grpc.ClientConn) ID {
	return &AtomRemoteInSourceProcess{
		AtomRemote: &AtomRemote{
			remote:  newBaseRemoteWithPinnedConn(e.cosmos, info, conn),
			element: e,
		},
	}
}

func (a *AtomRemoteInSourceProcess) asyncSet(callback func(out proto.Message, err *Error)) (startupID, callbackID uint64) {
	// BUG: asyncSet should never be called on a source-process remote ID.
	// It exists only to satisfy the ID interface. Returning zeros so callers
	// get a safe no-op rather than a process crash.
	return 0, 0
}

func (a *AtomRemoteInSourceProcess) asyncCallback(callbackID ID, name string, startupID, asyncID uint64, reply proto.Message, err *Error) {
	a.remote.PushAsyncMessageCallback(a, callbackID, name, startupID, asyncID, reply, err)
}

// AtomRemoteInTargetProcess 创建用于接收自的目标AtomRemote，用于atomos_remote_service.go。
type AtomRemoteInTargetProcess struct {
	*AtomRemote
	startupID uint64
	asyncID   uint64
}

func newAtomRemoteInTargetProcess(e *ElementRemote, info *IDInfo, startupID, asyncID uint64) ID {
	return &AtomRemoteInTargetProcess{
		AtomRemote: &AtomRemote{
			remote:  newBaseRemote(e.cosmos, info),
			element: e,
		},
		startupID: startupID,
		asyncID:   asyncID,
	}
}

func (a *AtomRemoteInTargetProcess) asyncSet(callback func(out proto.Message, err *Error)) (startupID, callbackID uint64) {
	return a.startupID, a.asyncID
}

func (a *AtomRemoteInTargetProcess) asyncCallback(callbackID ID, name string, startupID, asyncID uint64, reply proto.Message, err *Error) {
	a.remote.PushAsyncMessageCallback(a, callbackID, name, startupID, asyncID, reply, err)
}

//
// Implementation of ID
//

type AtomRemote struct {
	remote  BaseRemote
	element *ElementRemote
}

func (a *AtomRemote) GetIDInfo() *IDInfo {
	return a.remote.info
}

func (a *AtomRemote) String() string {
	return a.GetIDInfo().Info()
}

func (a *AtomRemote) Cosmos() CosmosNode {
	return a.element.cosmos
}

func (a *AtomRemote) State() BaseAtomosState {
	return a.remote.State()
}

func (a *AtomRemote) IdleTime() time.Duration {
	return a.remote.IdleTime()
}

func (a *AtomRemote) SyncMessagingByName(callerID ID, name string, in proto.Message, ext []ArgsForBaseAtomos) (out proto.Message, err *Error) {
	return a.remote.PushSyncMessage(callerID, name, in, ext)
}

func (a *AtomRemote) AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(out proto.Message, err *Error), ext []ArgsForBaseAtomos) (errBeforeExec *Error) {
	return a.remote.PushAsyncMessage(callerID, name, in, callback, ext)
}

func (a *AtomRemote) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	if a.element.current == nil || a.element.current.AtomDecoders == nil {
		return nil, nil
	}
	decoderFn, has := a.element.current.AtomDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

func (a *AtomRemote) Kill(callerID ID, ext []ArgsForBaseAtomos) *Error {
	return a.remote.Kill(callerID, ext)
}

func (a *AtomRemote) SendWormhole(callerID ID, wormhole BaseAtomosWormhole, ext []ArgsForBaseAtomos) *Error {
	return NewErrorf(ErrAtomosNotSupportWormhole, "AtomRemote: Cannot send remote atom wormhole.").AddStack(nil)
}

func (a *AtomRemote) getGoID() uint64 {
	//return a.info.GoId
	return 0
}
