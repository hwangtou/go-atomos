package atomos

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// optimise: elementRemote 中的 atomRemote map 存在内存泄漏（无释放操作）
// 经观察，version 和 callerCounter 无实际用途，考虑到这点，AtomRemote 可作为无状态结构（内存占用也不大），不再需要缓存，随用随创建

// atomRemoteInSourceProcess 创建用于发送的目标AtomRemote
type atomRemoteInSourceProcess struct {
	*AtomRemote
}

func newAtomRemoteInSourceProcess(e *ElementRemote, info *IDInfo) ID {
	return &atomRemoteInSourceProcess{
		AtomRemote: &AtomRemote{
			remote:  newBaseRemote(e.cosmos, info),
			element: e,
		},
	}
}

// newAtomRemoteInSourceProcessWithPinnedConn creates an AtomRemote that pins
// the given connection, so calls to it keep targeting the same version even
// after CosmosRemote.current switches. The conn must be the exact connection
// returned by getCliWithConn for the resolution RPC.
func newAtomRemoteInSourceProcessWithPinnedConn(e *ElementRemote, info *IDInfo, conn *grpc.ClientConn) ID {
	return &atomRemoteInSourceProcess{
		AtomRemote: &AtomRemote{
			remote:  newBaseRemoteWithPinnedConn(e.cosmos, info, conn),
			element: e,
		},
	}
}

func (a *atomRemoteInSourceProcess) asyncSet(callback func(out proto.Message, err error)) (startupID, callbackID uint64) {
	return 0, 0
}

func (a *atomRemoteInSourceProcess) asyncPop(asyncID uint64) func(out proto.Message, err error) {
	return nil
}

func (a *atomRemoteInSourceProcess) asyncCallback(callbackID ID, name string, startupID, asyncID uint64, reply proto.Message, err error) {
	a.remote.PushAsyncMessageCallback(a, callbackID, name, startupID, asyncID, reply, err)
}

// atomRemoteInTargetProcess 创建用于接收自的目标AtomRemote，用于atomos_remote_service.go。
type atomRemoteInTargetProcess struct {
	*AtomRemote
	startupID uint64
	asyncID   uint64
}

func newAtomRemoteInTargetProcess(e *ElementRemote, info *IDInfo, startupID, asyncID uint64) ID {
	return &atomRemoteInTargetProcess{
		AtomRemote: &AtomRemote{
			remote:  newBaseRemote(e.cosmos, info),
			element: e,
		},
		startupID: startupID,
		asyncID:   asyncID,
	}
}

func (a *atomRemoteInTargetProcess) asyncSet(callback func(out proto.Message, err error)) (startupID, callbackID uint64) {
	return a.startupID, a.asyncID
}

func (a *atomRemoteInTargetProcess) asyncPop(asyncID uint64) func(out proto.Message, err error) {
	return nil
}

func (a *atomRemoteInTargetProcess) asyncCallback(callbackID ID, name string, startupID, asyncID uint64, reply proto.Message, err error) {
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

func (a *AtomRemote) SyncMessagingByName(callerID ID, name string, in proto.Message, ext ...ArgsForBaseAtomos) (out proto.Message, err error) {
	return a.remote.PushSyncMessage(callerID, name, in, ext...)
}

func (a *AtomRemote) AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(out proto.Message, err error), ext ...ArgsForBaseAtomos) (errBeforeExec error) {
	return a.remote.PushAsyncMessage(callerID, name, in, callback, ext...)
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

func (a *AtomRemote) Kill(callerID ID, ext ...ArgsForBaseAtomos) error {
	if a.element.cosmos != nil && a.element.cosmos.process != nil && a.element.cosmos.process.lifecycle != nil {
		return errOrNil(a.element.haltControlled(callerID, a.GetIDInfo(), ext...))
	}
	return a.remote.Kill(callerID, ext...)
}

func (a *AtomRemote) SendWormhole(callerID ID, wormhole BaseAtomosWormhole, ext ...ArgsForBaseAtomos) error {
	return NewErrorf(ErrCodeAtomosNotSupportWormhole, "AtomRemote: Cannot send remote atom wormhole.").AddStack(nil)
}

func (a *AtomRemote) getGoID() uint64 {
	//return a.info.GoId
	return 0
}
