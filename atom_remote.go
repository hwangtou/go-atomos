package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"time"
)

type AtomRemote struct {
	element *ElementRemote
	context atomosIDContextRemote
	version string

	callerCounter int
}

func newAtomRemote(e *ElementRemote, info *IDInfo, version string) *AtomRemote {
	a := &AtomRemote{
		element:       e,
		context:       atomosIDContextRemote{},
		version:       version,
		callerCounter: 0,
	}
	initAtomosIDContextRemote(&a.context, info)
	return a
}

//
// Implementation of ID
//

func (a *AtomRemote) GetIDContext() IDContext {
	return &a.context
}

func (a *AtomRemote) GetIDInfo() *IDInfo {
	return a.context.info
}

func (a *AtomRemote) String() string {
	return a.GetIDInfo().Info()
}

func (a *AtomRemote) Cosmos() CosmosNode {
	return a.element.cosmos
}

func (a *AtomRemote) State() AtomosState {
	client, ctx, cancel, err := a.element.cosmos.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		a.element.cosmos.process.local.Log().Error("AtomRemote: State failed. err=(%v)", err)
		return AtomosState(0)
	}
	defer cancel()

	rsp, er := client.GetIDState(ctx, &CosmosRemoteGetIDStateReq{Id: a.context.info})
	if er != nil {
		a.element.cosmos.process.local.Log().Error("AtomRemote: State failed. err=(%v)", er)
		return AtomosState(0)
	}
	return AtomosState(rsp.State)
}

func (a *AtomRemote) IdleTime() time.Duration {
	client, ctx, cancel, err := a.element.cosmos.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		a.element.cosmos.process.local.Log().Error("AtomRemote: IdleTime failed. err=(%v)", err)
		return 0
	}
	defer cancel()

	rsp, er := client.GetIDIdleTime(ctx, &CosmosRemoteGetIDIdleTimeReq{Id: a.context.info})
	if er != nil {
		a.element.cosmos.process.local.Log().Error("AtomRemote: IdleTime failed. err=(%v)", er)
		return 0
	}
	return time.Duration(rsp.IdleTime)
}

func (a *AtomRemote) SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error) {
	if callerID == nil {
		return nil, NewError(ErrFrameworkIncorrectUsage, "AtomRemote: SyncMessagingByName without fromID.").AddStack(nil)
	}

	var er error
	var arg *anypb.Any
	if in != nil {
		arg, er = anypb.New(in)
		if er != nil {
			return nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "AtomRemote: SyncMessagingByName arg error. err=(%v)", er).AddStack(nil)
		}
	}

	client, ctx, cancel, err := a.element.cosmos.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		return nil, err.AddStack(nil)
	}
	defer cancel()

	callerIdInfo := callerID.GetIDInfo()
	toIDInfo := a.context.info

	rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId: callerIdInfo,
		CallerContext: &IDContextInfo{
			IdChain: append(callerID.GetIDContext().FromCallChain(), callerID.GetIDInfo().Info()),
		},
		To:        toIDInfo,
		Timeout:   int64(timeout),
		NeedReply: true,
		Message:   name,
		Args:      arg,
	})
	if er != nil {
		return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "AtomRemote: SyncMessagingByName response error. err=(%v)", er).AddStack(nil)
	}
	if rsp.Reply != nil {
		out, er = rsp.Reply.UnmarshalNew()
		if er != nil {
			return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "AtomRemote: SyncMessagingByName reply unmarshal error. err=(%v)", er).AddStack(nil)
		}
	}
	if rsp.Error != nil {
		err = rsp.Error.AddStack(nil)
	}
	return out, err
}

func (a *AtomRemote) AsyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message, callback func(out proto.Message, err *Error)) {
	if callerID == nil {
		if callback != nil {
			callback(nil, NewError(ErrFrameworkIncorrectUsage, "AtomRemote: AsyncMessagingByName without fromID.").AddStack(nil))
		}
		a.element.cosmos.process.local.Log().Error("AtomRemote: AsyncMessagingByName without fromID.")
		return
	}

	var er error
	var arg *anypb.Any
	if in != nil {
		arg, er = anypb.New(in)
		if er != nil {
			if callback != nil {
				callback(nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "AtomRemote: AsyncMessagingByName arg error. err=(%v)", er).AddStack(nil))
			}
			a.element.cosmos.process.local.Log().Error("AtomRemote: AsyncMessagingByName arg error. err=(%v)", er)
			return
		}
	}

	client, ctx, cancel, err := a.element.cosmos.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		if callback != nil {
			callback(nil, err.AddStack(nil))
		}
		a.element.cosmos.process.local.Log().Error("AtomRemote: AsyncMessagingByName client error. err=(%v)", err)
		return
	}
	defer cancel()

	callerIdInfo := callerID.GetIDInfo()
	toIDInfo := a.context.info
	needReply := callback != nil

	a.element.cosmos.process.local.Parallel(func() {
		out, err := func() (out proto.Message, err *Error) {

			rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
				CallerId: callerIdInfo,
				CallerContext: &IDContextInfo{
					IdChain: []string{},
				},
				To:        toIDInfo,
				Timeout:   int64(timeout),
				NeedReply: needReply,
				Message:   name,
				Args:      arg,
			})
			if er != nil {
				return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: SyncMessagingByName reply error. rsp=(%v),err=(%v)", rsp, er).AddStack(nil)
			}
			if needReply {
				if rsp.Reply != nil {
					out, er = rsp.Reply.UnmarshalNew()
					if er != nil {
						return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: SyncMessagingByName reply unmarshal error. err=(%v)", er).AddStack(nil)
					}
				}
				if rsp.Error != nil {
					err = rsp.Error.AddStack(nil)
				}
			}

			return out, err
		}()

		if needReply {
			callerID.getAtomos().PushAsyncMessageCallbackMailAndWaitReply(callerID, name, out, err, callback)
		}
	})
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

func (a *AtomRemote) Kill(callerID SelfID, timeout time.Duration) *Error {
	if callerID == nil {
		return NewError(ErrFrameworkIncorrectUsage, "AtomRemote: SyncMessagingByName without fromID.").AddStack(nil)
	}

	client, ctx, cancel, err := a.element.cosmos.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		return err.AddStack(nil)
	}
	defer cancel()

	rsp, er := client.KillAtom(ctx, &CosmosRemoteKillAtomReq{
		CallerId: callerID.GetIDInfo(),
		CallerContext: &IDContextInfo{
			IdChain: append(callerID.GetIDContext().FromCallChain(), callerID.GetIDInfo().Info()),
		},
		Id:      a.context.info,
		Timeout: int64(timeout),
	})
	if er != nil {
		return NewError(ErrCosmosRemoteResponseInvalid, "AtomRemote: KillAtom response error.").AddStack(nil)
	}

	if rsp.Error != nil {
		return rsp.Error.AddStack(nil)
	}
	return nil
}

func (a *AtomRemote) SendWormhole(callerID SelfID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	return NewErrorf(ErrAtomosNotSupportWormhole, "AtomRemote: Cannot send remote atom wormhole.").AddStack(nil)
}

func (a *AtomRemote) getGoID() uint64 {
	//return a.info.GoId
	return 0
}

// remoteAtomFakeSelfID 用于在远程Atom中实现SelfID接口
// 由于远程Atom的SelfID是不可用的，所以这里实现一个Fake的SelfID。
// 这个Fake的SelfID只能用于获取Atom的ID，不能用于其他操作。

type remoteAtomFakeSelfID struct {
	*AtomRemote
	callerIDInfo    *IDInfo
	callerIDContext *IDContextInfo
}

func (e *ElementRemote) newRemoteAtomFromCaller(callerIDInfo *IDInfo, callerIDContext *IDContextInfo) *remoteAtomFakeSelfID {
	e.lock.Lock()
	a, has := e.atoms[callerIDInfo.Atom]
	if !has {
		a = newAtomRemote(e, callerIDInfo, e.version)
		e.atoms[callerIDInfo.Atom] = a
	}
	e.lock.Unlock()
	return &remoteAtomFakeSelfID{
		AtomRemote:      a,
		callerIDInfo:    callerIDInfo,
		callerIDContext: callerIDContext,
	}
}

func (a *remoteAtomFakeSelfID) callerCounterRelease() {
	a.element.lock.Lock()
	if a.callerCounter > 0 {
		a.callerCounter -= 1
	}
	a.element.lock.Unlock()
}

func (a *remoteAtomFakeSelfID) Log() Logging {
	panic("not supported, should not be called")
}

func (a *remoteAtomFakeSelfID) Task() Task {
	panic("not supported, should not be called")
}

func (a *remoteAtomFakeSelfID) CosmosMain() *CosmosLocal {
	panic("not supported, should not be called")
}

func (a *remoteAtomFakeSelfID) KillSelf() {
	panic("not supported, should not be called")
}

func (a *remoteAtomFakeSelfID) Parallel(f func()) {
	panic("not supported, should not be called")
}

func (a *remoteAtomFakeSelfID) Config() map[string][]byte {
	panic("not supported, should not be called")
}

func (a *remoteAtomFakeSelfID) getAtomos() *BaseAtomos {
	panic("not supported, should not be called")
}
