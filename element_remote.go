package atomos

import (
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// ElementRemote
// 远程的Element实现。
// Implement of remote Element.

// elementRemoteFromSource 创建用于发送的目标ElementRemote
type elementRemoteFromSource struct {
	*ElementRemote
}

func newElementRemoteFromSource(c *CosmosRemote, info *IDInfo, i *ElementInterface, version string) *elementRemoteFromSource {
	return &elementRemoteFromSource{
		ElementRemote: &ElementRemote{
			cosmos:  c,
			remote:  newBaseRemote(c, info),
			current: i,
			version: version,
			enable:  false,
		},
	}
}

func (e *elementRemoteFromSource) asyncSet(callback func(out proto.Message, err error)) (startupID, callbackID uint64) {
	return 0, 0
}

func (e *elementRemoteFromSource) asyncPop(asyncID uint64) func(out proto.Message, err error) {
	return nil
}

func (e *elementRemoteFromSource) asyncCallback(callerID ID, name string, startupID, asyncID uint64, reply proto.Message, err error) {
	e.remote.PushAsyncMessageCallback(e, callerID, name, startupID, asyncID, reply, err)
}

// elementRemoteInTargetProcess 创建用于接收的目标ElementRemote，用于atomos_remote_service.go。
type elementRemoteInTargetProcess struct {
	*ElementRemote
	startupID uint64
	asyncID   uint64
}

func newElementRemoteInTargetProcess(elem *ElementRemote, startupID, asyncID uint64) ID {
	return &elementRemoteInTargetProcess{
		ElementRemote: elem,
		startupID:     startupID,
		asyncID:       asyncID,
	}
}

func (e *elementRemoteInTargetProcess) asyncSet(callback func(out proto.Message, err error)) (startupID, callbackID uint64) {
	return e.startupID, e.asyncID
}

func (e *elementRemoteInTargetProcess) asyncPop(asyncID uint64) func(out proto.Message, err error) {
	return nil
}

func (e *elementRemoteInTargetProcess) asyncCallback(callerID ID, name string, startupID, asyncID uint64, reply proto.Message, err error) {
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

func (e *ElementRemote) SyncMessagingByName(callerID ID, name string, in proto.Message, ext ...ArgsForBaseAtomos) (out proto.Message, err error) {
	return e.remote.PushSyncMessage(callerID, name, in, ext...)
}

func (e *ElementRemote) AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(out proto.Message, err error), ext ...ArgsForBaseAtomos) (errBeforeExec error) {
	return e.remote.PushAsyncMessage(callerID, name, in, callback, ext...)
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

func (e *ElementRemote) Kill(callerID ID, ext ...ArgsForBaseAtomos) error {
	return NewError(ErrCodeElementRemoteCannotKill, "ElementRemote: Cannot kill remote element.").AddStack(nil)
}

func (e *ElementRemote) SendWormhole(callerID ID, wormhole BaseAtomosWormhole, ext ...ArgsForBaseAtomos) error {
	return NewErrorf(ErrCodeElementRemoteCannotSendWormhole, "ElementRemote: Cannot send remote wormhole.").AddStack(nil)
}

func (e *ElementRemote) getGoID() uint64 {
	//return e.info.GoId
	return 0
}

// Implementation of Element

func (e *ElementRemote) GetAtomID(name string, _ *IDTrackerInfo, args ...ArgsForBaseAtomos) (ID, *IDTracker, error) {
	if e.cosmos != nil && e.cosmos.process != nil {
		defer e.cosmos.process.observeRPC("ElementGetAtomID", time.Now())
	}
	cli, conn, ctx, cancel, err := e.remote.getCliWithConn(atomosClientTimeout)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	defer cancel()

	rsp, er := cli.GetAtomID(ctx, &CosmosRemoteGetAtomIDReq{
		Element: e.remote.info.Element,
		Atom:    name,
	})
	if er != nil {
		return nil, nil, grpcErrorToError(er).AddStack(nil)
	}
	if rsp.Error != nil {
		return nil, nil, rsp.Error.AddStack(nil)
	}

	// Pin the connection used for this call so the returned atom ID keeps
	// targeting this version even after current switches (drain/hot-upgrade).
	return newAtomRemoteInSourceProcessWithPinnedConn(e, rsp.Id, conn), nil, nil
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

func (e *ElementRemote) SpawnAtom(callerID ID, name string, arg proto.Message, _ *IDTrackerInfo, args ...ArgsForBaseAtomos) (ID, *IDTracker, error) {
	if e.cosmos != nil && e.cosmos.process != nil {
		defer e.cosmos.process.observeRPC("ElementSpawnAtom", time.Now())
	}
	remoteArgs := make([]ArgsForBaseAtomos, 0, len(args))
	for _, arg := range args {
		if arg == nil {
			continue
		}
		switch arg.getArgType() {
		case ArgTypeBaseAtomosTimeout, ArgTypeBaseAtomosAppendToHead:
			remoteArgs = append(remoteArgs, arg)
		}
	}
	helper := createBaseAtomosHelper(BaseAtomosMailSync, remoteArgs)
	if helper.hasErrors() {
		return nil, nil, helper.getError()
	}
	cli, conn, ctx, cancel, err := e.remote.getCliWithConn(atomosClientTimeout)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	defer cancel()

	var er error
	var anyArg *anypb.Any
	if arg != nil {
		anyArg, er = anypb.New(arg)
		if er != nil {
			return nil, nil, NewError(ErrCodeCosmosRemoteRequestInvalid, "ElementRemote: SpawnAtom arg error.").AddStack(nil)
		}
	}
	rsp, er := cli.SpawnAtom(ctx, &CosmosRemoteSpawnAtomReq{
		CallerId:   callerID.GetIDInfo(),
		Element:    e.remote.info.Element,
		Atom:       name,
		Args:       anyArg,
		CosmosArgs: helper.getRemoteArg(),
	})
	if er != nil {
		return nil, nil, grpcErrorToError(er).AddStack(nil)
	}
	if rsp.Error != nil {
		return nil, nil, rsp.Error.AddStack(nil)
	}

	// Pin the connection used for this call (see GetAtomID for rationale).
	return newAtomRemoteInSourceProcessWithPinnedConn(e, rsp.Id, conn), nil, nil
}

// 内部实现
// INTERNAL

func (e *ElementRemote) setDisable() {
	e.enable = false
}
