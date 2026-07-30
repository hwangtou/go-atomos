package atomos

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type BaseRemote struct {
	cosmos *CosmosRemote
	info   *IDInfo
	// pinnedConn is the gRPC connection captured at ID-creation time.
	//
	// Without this, getCli would always ask cosmos.getCurrentClient(), whose
	// answer changes when the node's "current" version switches (e.g. during a
	// drain/hot-upgrade). That would silently reroute existing calls — which
	// target an atom living on the *old* version — to the *new* version, where
	// the atom does not exist, breaking in-flight stateful sessions.
	//
	// By pinning the connection at creation time, an ID keeps talking to the
	// same version/instance for its entire lifetime, regardless of current
	// changes. It may be nil (legacy/newBaseRemote path); in that case getCli
	// falls back to getCurrentClient for backward compatibility.
	pinnedConn *grpc.ClientConn
}

func newBaseRemote(cosmos *CosmosRemote, info *IDInfo) BaseRemote {
	return BaseRemote{
		cosmos: cosmos,
		info:   info,
	}
}

// newBaseRemoteWithPinnedConn creates a BaseRemote that pins the given
// connection so subsequent getCli calls always use it, independent of
// CosmosRemote.current changes. Pass the connection the caller used to resolve
// the ID (typically cosmos.getCurrentClient() at that moment).
func newBaseRemoteWithPinnedConn(cosmos *CosmosRemote, info *IDInfo, conn *grpc.ClientConn) BaseRemote {
	return BaseRemote{
		cosmos:     cosmos,
		info:       info,
		pinnedConn: conn,
	}
}

// getCliConn returns the raw gRPC connection this BaseRemote would use.
// Used by ElementRemote.GetAtomID/SpawnAtom to pin the connection into the
// newly created AtomRemote, so the atom ID keeps talking to the same version.
func (a *BaseRemote) getCliConn() *grpc.ClientConn {
	if c := a.validPinnedConn(); c != nil {
		return c
	}
	return a.cosmos.getCurrentClient()
}

// validPinnedConn returns the pinned connection if it is still usable, or nil
// if the pin is absent / has been closed (Shutdown). When the pin is dead it is
// cleared so subsequent calls fall back to getCurrentClient and (eventually)
// get re-pinned to the new version's connection.
func (a *BaseRemote) validPinnedConn() *grpc.ClientConn {
	c := a.pinnedConn
	if c == nil {
		return nil
	}
	// connectivity.Shutdown means the ClientConn was closed (e.g. the remote
	// node restarted, or setDisable ran). TransientFailure/Connecting are
	// temporary — we keep the pin and let the call retry on the live transport.
	if st := c.GetState(); st == connectivity.Shutdown {
		a.pinnedConn = nil
		return nil
	}
	return c
}

func (a *BaseRemote) getCli(timeout time.Duration) (AtomosRemoteServiceClient, context.Context, context.CancelFunc, *Error) {
	// Prefer the pinned connection (captured at ID creation) so that a current
	// switch does not reroute existing calls to the wrong version. If the pin
	// is dead (node restarted), fall back to the current client so the call can
	// reach the new version.
	conn := a.validPinnedConn()
	if conn == nil {
		conn = a.cosmos.getCurrentClient()
	}
	if conn == nil {
		return nil, nil, nil, NewError(ErrCosmosRemoteConnectFailed, "AtomRemote: SyncMessagingByName client error.").AddStack(nil)
	}

	client := NewAtomosRemoteServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), timeout+atomosClientTimeout)
	return client, ctx, cancel, nil
}

func (a *BaseRemote) State() BaseAtomosState {
	cli, ctx, cancel, err := a.getCli(atomosClientTimeout)
	if err != nil {
		return BaseAtomosInvalidState
	}
	defer cancel()

	rsp, er := cli.GetIDState(ctx, &CosmosRemoteGetIDStateReq{Id: a.info})
	if er != nil {
		return BaseAtomosInvalidState
	}

	return BaseAtomosState(rsp.State)
}

func (a *BaseRemote) IdleTime() time.Duration {
	cli, ctx, cancel, err := a.getCli(atomosClientTimeout)
	if err != nil {
		return 0
	}
	defer cancel()

	rsp, er := cli.GetIDIdleTime(ctx, &CosmosRemoteGetIDIdleTimeReq{Id: a.info})
	if er != nil {
		return 0
	}

	return time.Duration(rsp.IdleTime)
}

func (a *BaseRemote) PushSyncMessage(from ID, name string, in proto.Message, ext []ArgsForBaseAtomos) (reply proto.Message, err *Error) {
	helper := createBaseAtomosHelper(BaseAtomosMailSync, ext)
	if helper.hasErrors() {
		return nil, helper.getError()
	}

	client, ctx, cancel, err := a.getCli(helper.timeout)
	if err != nil {
		return nil, err.AddStack(nil)
	}
	defer cancel()

	var er error
	var arg *anypb.Any
	if in != nil {
		arg, er = anypb.New(in)
		if er != nil {
			return nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "BaseRemote: SyncMessagingByName arg error. err=(%v)", er).AddStack(nil)
		}
	}
	rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId:   from.GetIDInfo(),
		To:         a.info,
		CosmosArgs: helper.getRemoteArg(),
		Message:    name,
		Args:       arg,
	})
	if er != nil {
		return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "BaseRemote: SyncMessagingByName response error. err=(%v)", er).AddStack(nil)
	}
	if rsp.Reply != nil {
		reply, er = rsp.Reply.UnmarshalNew()
		if er != nil {
			return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "BaseRemote: SyncMessagingByName reply unmarshal error. err=(%v)", er).AddStack(nil)
		}
	}
	if rsp.Error != nil {
		err = rsp.Error.AddStack(nil)
	}
	return reply, err
}

func (a *BaseRemote) PushAsyncMessage(callerID ID, name string, in proto.Message, callback func(out proto.Message, err *Error), ext []ArgsForBaseAtomos) (errBeforeExec *Error) {
	helper := createBaseAtomosHelper(BaseAtomosMailAsync, ext)
	if helper.hasErrors() {
		if callback != nil {
			callback(nil, helper.getError())
		}
		return helper.getError()
	}

	var er error
	var arg *anypb.Any
	if in != nil {
		arg, er = anypb.New(in)
		if er != nil {
			if callback != nil {
				callback(nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "BaseRemote: AsyncMessagingByName arg error. err=(%v)", er).AddStack(nil))
			}
			return nil
		}
	}

	client, ctx, cancel, err := a.getCli(helper.timeout)
	if err != nil {
		return err.AddStack(nil)
	}
	defer cancel()

	var startupID, asyncID uint64
	if callback != nil {
		startupID, asyncID = callerID.asyncSet(callback)
		if startupID != a.cosmos.process.startupID {
			// This should never happen, just in case.
			a.cosmos.process.logging.pushFrameworkFatalLog("BaseRemote: AsyncMessagingByName asyncSet returned unexpected startupID. expected=(%d) actual=(%d)", a.cosmos.process.startupID, startupID)
		}
	}

	//a.cosmos.process.logging.PushLogging(callerID.GetIDInfo(), LogLevel_Debug, fmt.Sprintf("Async Step 1: (%s)=>(%s) startupID=(%d) asyncID=(%d) message=(%s) args=(%v)", callerID, a.info.Info(), startupID, asyncID, name, in))
	rsp, er := client.AsyncMessagingByName(ctx, &CosmosRemoteAsyncMessagingByNameReq{
		CallerId:   callerID.GetIDInfo(),
		ToId:       a.info,
		CosmosArgs: helper.getRemoteArg(),
		StartupId:  startupID,
		AsyncId:    asyncID,
		Message:    name,
		Args:       arg,
	})
	if er != nil {
		return NewErrorf(ErrCosmosRemoteResponseInvalid, "BaseRemote: AsyncMessagingByName response error. err=(%v)", er).AddStack(nil)
	}
	if rsp.Error != nil {
		return rsp.Error.AddStack(nil)
	}
	return nil
}

func (a *BaseRemote) PushAsyncMessageCallback(callbackID, toID ID, name string, startupID, asyncID uint64, reply proto.Message, err *Error) {
	if asyncID == 0 {
		//a.cosmos.process.logging.pushFrameworkInfoLog("PushAsyncMessageCallback called with empty asyncID. from=(%v),to=(%v),name=(%s),reply=(%v),err=(%v)", callbackID, toID, name, reply, err)
		return
	}
	//a.cosmos.process.logging.PushLogging(callbackID.GetIDInfo(), LogLevel_Debug, fmt.Sprintf("Async Step 3: (%s)=>(%s) startupID=(%d) asyncID=(%d) message=(%s) args=(%v)", callbackID, toID, startupID, asyncID, name, reply))
	cli, ctx, cancel, err := a.getCli(atomosClientTimeout)
	if err != nil {
		// TODO: need retry?
		a.cosmos.process.logging.pushFrameworkErrorLog("PushAsyncMessageCallback getCli error. name=(%s),reply=(%v),err=(%v),cliErr=(%v)", name, reply, err, err)
		return
	}
	defer cancel()

	var anyReply *anypb.Any
	var er error
	if reply != nil {
		anyReply, er = anypb.New(reply)
		if er != nil {
			a.cosmos.process.logging.pushFrameworkErrorLog("PushAsyncMessageCallback marshal reply error. name=(%s),reply=(%v),err=(%v),marshalErr=(%v)", name, reply, err, er)
			return
		}
	}
	rsp, er := cli.AsyncOnMessageCallback(ctx, &CosmosRemoteAsyncOnMessageCallbackReq{
		ToId:       toID.GetIDInfo(),
		CallbackId: callbackID.GetIDInfo(),
		StartupId:  startupID,
		AsyncId:    asyncID,
		Message:    name,
		Args:       anyReply,
		Error:      err,
	})
	if er != nil {
		a.cosmos.process.logging.pushFrameworkErrorLog("PushAsyncMessageCallback response error. name=(%s),reply=(%v),err=(%v),respErr=(%v)", name, reply, err, er)
		return
	}
	if rsp != nil && rsp.Error != nil {
		a.cosmos.process.logging.pushFrameworkErrorLog("PushAsyncMessageCallback response returned error. name=(%s),reply=(%v),err=(%v),respErr=(%v)", name, reply, err, rsp.Error)
	}
}

func (a *BaseRemote) Kill(callerID ID, ext []ArgsForBaseAtomos) *Error {
	helper := createBaseAtomosHelper(BaseAtomosMailKill, ext)
	if helper.hasErrors() {
		return helper.getError()
	}

	cli, ctx, cancel, err := a.getCli(helper.timeout)
	if err != nil {
		return err.AddStack(nil)
	}
	defer cancel()

	rsp, er := cli.KillAtom(ctx, &CosmosRemoteKillAtomReq{
		CallerId:   callerID.GetIDInfo(),
		Id:         a.info,
		CosmosArgs: helper.getRemoteArg(),
	})
	if er != nil {
		return NewError(ErrCosmosRemoteResponseInvalid, "BaseRemote: KillAtom response error.").AddStack(nil)
	}

	if rsp.Error != nil {
		return rsp.Error.AddStack(nil)
	}
	return nil
}
