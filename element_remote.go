package atomos

import (
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// ElementRemote
// 远程的Element实现。
// Implement of remote Element.

// ElementRemoteFromSource 创建用于发送的目标ElementRemote
type ElementRemoteFromSource struct {
	*ElementRemote
}

func newElementRemoteFromSource(c *CosmosRemote, info *IDInfo, i *ElementInterface, version string) *ElementRemoteFromSource {
	return &ElementRemoteFromSource{
		ElementRemote: &ElementRemote{
			cosmos:  c,
			remote:  newBaseRemote(c, info),
			current: i,
			version: version,
			enable:  false,
		},
	}
}

func (e *ElementRemoteFromSource) asyncSet(callback func(out proto.Message, err *Error)) (startupID, callbackID uint64) {
	// BUG: asyncSet should never be called on a source-process remote ID.
	// It exists only to satisfy the ID interface. Returning zeros so callers
	// get a safe no-op rather than a process crash.
	return 0, 0
}

func (e *ElementRemoteFromSource) asyncCallback(callerID ID, name string, startupID, asyncID uint64, reply proto.Message, err *Error) {
	e.remote.PushAsyncMessageCallback(e, callerID, name, startupID, asyncID, reply, err)
}

// ElementRemoteInTargetProcess 创建用于接收的目标ElementRemote，用于atomos_remote_service.go。
type ElementRemoteInTargetProcess struct {
	*ElementRemote
	startupID uint64
	asyncID   uint64
}

func newElementRemoteInTargetProcess(elem *ElementRemote, startupID, asyncID uint64) ID {
	return &ElementRemoteInTargetProcess{
		ElementRemote: elem,
		startupID:     startupID,
		asyncID:       asyncID,
	}
}

func (e *ElementRemoteInTargetProcess) asyncSet(callback func(out proto.Message, err *Error)) (startupID, callbackID uint64) {
	return e.startupID, e.asyncID
}

func (e *ElementRemoteInTargetProcess) asyncCallback(callerID ID, name string, startupID, asyncID uint64, reply proto.Message, err *Error) {
	e.remote.PushAsyncMessageCallback(e, callerID, name, startupID, asyncID, reply, err)
}

// Implementation of ID

type ElementRemote struct {
	cosmos  *CosmosRemote
	remote  BaseRemote
	current *ElementInterface

	version string

	enable bool
}

func (e *ElementRemote) GetIDInfo() *IDInfo {
	return e.remote.info
}

func (e *ElementRemote) String() string {
	return e.GetIDInfo().Info()
}

func (e *ElementRemote) Cosmos() CosmosNode {
	return e.cosmos
}

func (e *ElementRemote) State() BaseAtomosState {
	return e.remote.State()
}

func (e *ElementRemote) IdleTime() time.Duration {
	return e.remote.IdleTime()
}

func (e *ElementRemote) SyncMessagingByName(callerID ID, name string, in proto.Message, ext []ArgsForBaseAtomos) (out proto.Message, err *Error) {
	return e.remote.PushSyncMessage(callerID, name, in, ext)
}

func (e *ElementRemote) AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(out proto.Message, err *Error), ext []ArgsForBaseAtomos) (errBeforeExec *Error) {
	return e.remote.PushAsyncMessage(callerID, name, in, callback, ext)
}

func (e *ElementRemote) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	if e.current == nil || e.current.ElementDecoders == nil {
		return nil, nil
	}
	decoderFn, has := e.current.ElementDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

func (e *ElementRemote) Kill(callerID ID, ext []ArgsForBaseAtomos) *Error {
	return NewError(ErrElementRemoteCannotKill, "ElementRemote: Cannot kill remote element.").AddStack(nil)
}

func (e *ElementRemote) SendWormhole(callerID ID, wormhole BaseAtomosWormhole, ext []ArgsForBaseAtomos) *Error {
	return NewErrorf(ErrElementRemoteCannotSendWormhole, "ElementRemote: Cannot send remote wormhole.").AddStack(nil)
}

func (e *ElementRemote) getGoID() uint64 {
	//return e.info.GoId
	return 0
}

// Implementation of Element

func (e *ElementRemote) GetAtomID(name string, _ *IDTrackerInfo, fromLocalOrRemote bool, args ...any) (ID, *IDTracker, *Error) {
	cli, ctx, cancel, err := e.remote.getCli(atomosClientTimeout)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	defer cancel()

	rsp, er := cli.GetAtomID(ctx, &CosmosRemoteGetAtomIDReq{
		Element: e.remote.info.Element,
		Atom:    name,
	})
	if er != nil {
		return nil, nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: GetAtomID response error. err=(%v)", er).AddStack(nil)
	}
	if rsp.Error != nil {
		return nil, nil, rsp.Error.AddStack(nil)
	}

	return newAtomRemoteInSourceProcess(e, rsp.Id), nil, nil
}

func (e *ElementRemote) GetAtomsNum() int {
	cli, ctx, cancel, err := e.remote.getCli(atomosClientTimeout)
	if err != nil {
		return -1
	}
	defer cancel()

	rsp, er := cli.GetElementInfo(ctx, &CosmosRemoteGetElementInfoReq{
		Element: e.remote.info.Element,
	})
	if er != nil {
		return 0
	}

	return int(rsp.AtomsNum)
}

func (e *ElementRemote) GetActiveAtomsNum() int {
	cli, ctx, cancel, err := e.remote.getCli(atomosClientTimeout)
	if err != nil {
		return -1
	}
	defer cancel()

	rsp, er := cli.GetElementInfo(ctx, &CosmosRemoteGetElementInfoReq{
		Element: e.remote.info.Element,
	})
	if er != nil {
		return 0
	}

	return int(rsp.ActiveAtomsNum)
}

func (e *ElementRemote) GetAllInactiveAtomsIDTrackerInfo() map[string]string {
	// Not Supported.
	return map[string]string{}
}

func (e *ElementRemote) SpawnAtom(callerID ID, name string, arg proto.Message, _ *IDTrackerInfo, fromLocalOrRemote bool, args ...ArgsForBaseAtomos) (ID, *IDTracker, *Error) {
	cli, ctx, cancel, err := e.remote.getCli(atomosClientTimeout)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	defer cancel()

	var er error
	var anyArg *anypb.Any
	if arg != nil {
		anyArg, er = anypb.New(arg)
		if er != nil {
			return nil, nil, NewError(ErrCosmosRemoteRequestInvalid, "ElementRemote: SpawnAtom arg error.").AddStack(nil)
		}
	}
	rsp, er := cli.SpawnAtom(ctx, &CosmosRemoteSpawnAtomReq{
		CallerId: callerID.GetIDInfo(),
		Element:  e.remote.info.Element,
		Atom:     name,
		Args:     anyArg,
	})
	if er != nil {
		return nil, nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: SpawnAtom response error. err=(%v)", er).AddStack(nil)
	}
	if rsp.Error != nil && rsp.Id == nil {
		return nil, nil, rsp.Error.AddStack(nil)
	}

	return newAtomRemoteInSourceProcess(e, rsp.Id), nil, nil
}

// 内部实现
// INTERNAL

func (e *ElementRemote) setDisable() {
	e.enable = false
}
