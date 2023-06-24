package go_atomos

import (
	"context"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"time"
)

type AtomRemote struct {
	element *ElementRemote
	info    *IDInfo
	version string

	callerCounter int
}

func newAtomRemote(e *ElementRemote, info *IDInfo, version string) *AtomRemote {
	a := &AtomRemote{
		element: e,
		info:    info,
		version: version,
	}
	return a
}

//
// Implementation of ID
//

func (a *AtomRemote) GetIDInfo() *IDInfo {
	return a.info
}

func (a *AtomRemote) String() string {
	return a.GetIDInfo().Info()
}

func (a *AtomRemote) Cosmos() CosmosNode {
	return a.element.cosmos
}

func (a *AtomRemote) State() AtomosState {
	cli := a.element.cosmos.getCurrentClient()
	if cli == nil {
		return AtomosState(0)
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rsp, er := client.GetIDState(ctx, &CosmosRemoteGetIDStateReq{
		Id: a.info,
	})
	if er != nil {
		a.element.cosmos.process.local.Log().Error("AtomRemote: GetIDState failed. err=(%v)", er)
		return AtomosState(0)
	}

	return AtomosState(rsp.State)
}

func (a *AtomRemote) IdleTime() time.Duration {
	cli := a.element.cosmos.getCurrentClient()
	if cli == nil {
		return 0
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rsp, er := client.GetIDIdleTime(ctx, &CosmosRemoteGetIDIdleTimeReq{
		Id: a.info,
	})
	if er != nil {
		a.element.cosmos.process.local.Log().Error("AtomRemote: GetIDIdleTime failed. err=(%v)", er)
		return 0
	}

	return time.Duration(rsp.IdleTime)
}

func (a *AtomRemote) SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error) {
	cli := a.element.cosmos.getCurrentClient()
	if cli == nil {
		return nil, NewError(ErrCosmosRemoteConnectFailed, "AtomRemote: SyncMessagingByName client error.").AddStack(nil)
	}

	firstSyncCall := ""
	if callerFirst := callerID.getCurFirstSyncCall(); callerFirst == "" {
		// 要从调用者开始算起，所以要从调用者的ID中获取。
		firstSyncCall = callerID.nextFirstSyncCall()
		if err := callerID.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
			return nil, err.AddStack(nil)
		}
		defer callerID.unsetSyncMessageAndFirstCall()
	} else {
		firstSyncCall = callerFirst
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Second)
	defer cancel()
	arg, er := anypb.New(in)
	if er != nil {
		return nil, NewError(ErrCosmosRemoteRequestInvalid, "AtomRemote: SyncMessagingByName arg error.").AddStack(nil)
	}
	rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId:               callerID.GetIDInfo(),
		CallerCurFirstSyncCall: firstSyncCall,
		To:                     a.info,
		Timeout:                int64(timeout),
		Message:                name,
		Args:                   arg,
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
	cli := a.element.cosmos.getCurrentClient()
	if cli == nil {
		callback(nil, NewError(ErrCosmosRemoteConnectFailed, "AtomRemote: AsyncMessagingByName client error.").AddStack(nil))
		return
	}
	if callerID == nil {
		callback(nil, NewError(ErrFrameworkIncorrectUsage, "AtomRemote: AsyncMessagingByName without fromID.").AddStack(nil))
		return
	}

	// 这种情况需要创建新的FirstSyncCall，因为这是一个新的调用链，调用的开端是push向的ID。
	callerIDInfo := callerID.GetIDInfo()
	firstSyncCall := callerID.nextFirstSyncCall()

	a.element.cosmos.process.local.Parallel(func() {
		out, err := func() (out proto.Message, err *Error) {
			client := NewAtomosRemoteServiceClient(cli)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			arg, er := anypb.New(in)
			if er != nil {
				return nil, NewError(ErrCosmosRemoteRequestInvalid, "AtomRemote: SyncMessagingByName arg error.").AddStack(nil)
			}
			rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
				CallerId:               callerIDInfo,
				CallerCurFirstSyncCall: firstSyncCall,
				To:                     a.info,
				Timeout:                int64(timeout),
				Message:                name,
				Args:                   arg,
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
		}()
		callerID.pushAsyncMessageCallbackMailAndWaitReply(name, firstSyncCall, out, err, callback)
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
	cli := a.element.cosmos.getCurrentClient()
	if cli == nil {
		return NewError(ErrCosmosRemoteConnectFailed, "AtomRemote: SyncMessagingByName client error.").AddStack(nil)
	}

	firstSyncCall := ""
	if callerFirst := callerID.getCurFirstSyncCall(); callerFirst == "" {
		// 要从调用者开始算起，所以要从调用者的ID中获取。
		firstSyncCall = callerID.nextFirstSyncCall()
		if err := callerID.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
			return err.AddStack(nil)
		}
		defer callerID.unsetSyncMessageAndFirstCall()
	} else {
		firstSyncCall = callerFirst
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Second)
	defer cancel()

	rsp, er := client.KillAtom(ctx, &CosmosRemoteKillAtomReq{
		CallerId:               callerID.GetIDInfo(),
		CallerCurFirstSyncCall: firstSyncCall,
		Id:                     a.info,
		Timeout:                int64(timeout),
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
	return NewErrorf(ErrAtomosNotSupportWormhole, "AtomRemote: Cannot send remote atom wormhole.")
}

func (a *AtomRemote) getGoID() uint64 {
	return a.info.GoId
}

// remoteAtomFakeSelfID 用于在远程Atom中实现SelfID接口
// 由于远程Atom的SelfID是不可用的，所以这里实现一个Fake的SelfID。
// 这个Fake的SelfID只能用于获取Atom的ID，不能用于其他操作。

type remoteAtomFakeSelfID struct {
	*AtomRemote
	firstSyncCall string
}

func (e *ElementRemote) newRemoteAtomFromCaller(id *IDInfo, call string) *remoteAtomFakeSelfID {
	e.lock.Lock()
	a, has := e.atoms[id.Atom]
	if !has {
		a = newAtomRemote(e, id, e.version)
		e.atoms[id.Atom] = a
	}
	a.callerCounter += 1
	e.lock.Unlock()
	return &remoteAtomFakeSelfID{
		AtomRemote:    a,
		firstSyncCall: call,
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

func (a *remoteAtomFakeSelfID) getCurFirstSyncCall() string {
	return a.firstSyncCall
}

func (a *remoteAtomFakeSelfID) setSyncMessageAndFirstCall(s string) *Error {
	panic("not supported, should not be called")
}

func (a *remoteAtomFakeSelfID) unsetSyncMessageAndFirstCall() {
	panic("not supported, should not be called")
}

func (a *remoteAtomFakeSelfID) nextFirstSyncCall() string {
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

func (a *remoteAtomFakeSelfID) pushAsyncMessageCallbackMailAndWaitReply(name, firstSyncCall string, in proto.Message, err *Error, callback func(out proto.Message, err *Error)) {
	panic("not supported, should not be called")
}
