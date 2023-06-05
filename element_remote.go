package go_atomos

import (
	"context"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"sync"
	"time"
)

type ElementRemote struct {
	cosmos  *CosmosRemote
	info    *IDInfo
	current *ElementInterface

	atoms map[string]*AtomRemote
	lock  sync.RWMutex

	*idFirstSyncCallLocal
	enable bool
}

func newElementRemote(c *CosmosRemote, info *IDInfo, i *ElementInterface) *ElementRemote {
	e := &ElementRemote{
		cosmos:               c,
		info:                 info,
		current:              i,
		atoms:                map[string]*AtomRemote{},
		lock:                 sync.RWMutex{},
		idFirstSyncCallLocal: &idFirstSyncCallLocal{},
		enable:               false,
	}
	e.idFirstSyncCallLocal.init(info)
	return e
}

// Implementation of ID

func (e *ElementRemote) GetIDInfo() *IDInfo {
	return e.info
}

func (e *ElementRemote) String() string {
	return e.GetIDInfo().Info()
}

func (e *ElementRemote) Cosmos() CosmosNode {
	return e.cosmos
}

func (e *ElementRemote) State() AtomosState {
	cli := e.cosmos.getCurrentClient()
	if cli == nil {
		return AtomosState(0)
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rsp, er := client.GetIDState(ctx, &CosmosRemoteGetIDStateReq{
		Id: e.info,
	})
	if er != nil {
		e.cosmos.process.local.Log().Error("ElementRemote: State failed. err=(%v)", er)
		return AtomosState(0)
	}

	return AtomosState(rsp.State)
}

func (e *ElementRemote) IdleTime() time.Duration {
	cli := e.cosmos.getCurrentClient()
	if cli == nil {
		return 0
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rsp, er := client.GetIDIdleTime(ctx, &CosmosRemoteGetIDIdleTimeReq{
		Id: e.info,
	})
	if er != nil {
		e.cosmos.process.local.Log().Error("ElementRemote: IDIdleTime failed. err=(%v)", er)
		return 0
	}

	return time.Duration(rsp.IdleTime)
}

func (e *ElementRemote) SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error) {
	cli := e.cosmos.getCurrentClient()
	if cli == nil {
		return nil, NewError(ErrCosmosRemoteConnectFailed, "ElementRemote: SyncMessagingByName client error.").AddStack(nil)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	arg, er := anypb.New(in)
	if er != nil {
		return nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "ElementRemote: SyncMessagingByName arg error. err=(%v)", er).AddStack(nil)
	}
	rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId:               callerID.GetIDInfo(),
		CallerCurFirstSyncCall: firstSyncCall,
		To:                     e.info,
		Timeout:                int64(timeout),
		Message:                name,
		Args:                   arg,
	})
	if er != nil {
		return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: SyncMessagingByName response error. err=(%v)", er).AddStack(nil)
	}
	out, er = rsp.Reply.UnmarshalNew()
	if er != nil {
		return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: SyncMessagingByName reply error. err=(%v)", er).AddStack(nil)
	}

	if rsp.Error != nil {
		err = rsp.Error.AddStack(nil)
	}
	return out, err
}

func (e *ElementRemote) AsyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message, callback func(out proto.Message, err *Error)) {
	cli := e.cosmos.getCurrentClient()
	if cli == nil {
		callback(nil, NewError(ErrCosmosRemoteConnectFailed, "ElementRemote: AsyncMessagingByName client error.").AddStack(nil))
		return
	}
	if callerID == nil {
		callback(nil, NewError(ErrFrameworkIncorrectUsage, "ElementRemote: AsyncMessagingByName without fromID.").AddStack(nil))
		return
	}

	// 这种情况需要创建新的FirstSyncCall，因为这是一个新的调用链，调用的开端是push向的ID。
	callerIDInfo := callerID.GetIDInfo()
	firstSyncCall := e.nextFirstSyncCall()

	e.cosmos.process.local.Parallel(func() {
		out, err := func() (out proto.Message, err *Error) {
			client := NewAtomosRemoteServiceClient(cli)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			arg, er := anypb.New(in)
			if er != nil {
				return nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "ElementRemote: AsyncMessagingByName arg error. err=(%v)", er).AddStack(nil)
			}
			rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
				CallerId:               callerIDInfo,
				CallerCurFirstSyncCall: firstSyncCall,
				To:                     e.info,
				Timeout:                int64(timeout),
				Message:                name,
				Args:                   arg,
			})
			if er != nil {
				return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: AsyncMessagingByName response error. err=(%v)", er).AddStack(nil)
			}
			out, er = rsp.Reply.UnmarshalNew()
			if er != nil {
				return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: AsyncMessagingByName reply error. err=(%v)", er).AddStack(nil)
			}
			if rsp.Error != nil {
				err = rsp.Error.AddStack(nil)
			}
			return out, err
		}()
		callerID.pushAsyncMessageCallbackMailAndWaitReply(name, out, err, callback)
	})
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

func (e *ElementRemote) Kill(callerID SelfID, timeout time.Duration) *Error {
	return NewError(ErrElementRemoteCannotKill, "ElementRemote: Cannot kill remote element.").AddStack(nil)
}

func (e *ElementRemote) SendWormhole(callerID SelfID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	return NewErrorf(ErrElementRemoteCannotSendWormhole, "ElementRemote: Cannot send remote wormhole.").AddStack(nil)
}

func (e *ElementRemote) getIDTrackerManager() *idTrackerManager {
	panic("ElementRemote: getIDTrackerManager should not be called.")
}

func (e *ElementRemote) getGoID() uint64 {
	return e.info.GoId
}

// Implementation of Element

func (e *ElementRemote) GetAtomID(name string, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	cli := e.cosmos.getCurrentClient()
	if cli == nil {
		return nil, nil, NewError(ErrCosmosRemoteConnectFailed, "ElementRemote: GetAtomID client error.").AddStack(nil)
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rsp, er := client.GetAtomID(ctx, &CosmosRemoteGetAtomIDReq{
		FromTracker: tracker,
		Element:     e.info.Element,
		Atom:        name,
	})
	if er != nil {
		return nil, nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: GetAtomID response error. err=(%v)", er).AddStack(nil)
	}
	if rsp.Error != nil {
		return nil, nil, rsp.Error.AddStack(nil)
	}

	e.lock.Lock()
	a, has := e.atoms[name]
	if !has {
		a = newAtomRemote(e, rsp.Id)
		e.atoms[name] = a
	}
	e.lock.Unlock()

	return a, tracker.newIDTrackerFromRemoteTrackerID(e, rsp.TrackerId), nil
}

func (e *ElementRemote) GetAtomsNum() int {
	cli := e.cosmos.getCurrentClient()
	if cli == nil {
		return 0
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rsp, er := client.GetElementInfo(ctx, &CosmosRemoteGetElementInfoReq{
		Element: e.info.Element,
	})
	if er != nil {
		return 0
	}

	return int(rsp.AtomsNum)
}

func (e *ElementRemote) GetActiveAtomsNum() int {
	cli := e.cosmos.getCurrentClient()
	if cli == nil {
		return 0
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rsp, er := client.GetElementInfo(ctx, &CosmosRemoteGetElementInfoReq{
		Element: e.info.Element,
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

func (e *ElementRemote) SpawnAtom(name string, arg proto.Message, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	cli := e.cosmos.getCurrentClient()
	if cli == nil {
		return nil, nil, NewError(ErrCosmosRemoteConnectFailed, "ElementRemote: SpawnAtom client error.").AddStack(nil)
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	anyArg, er := anypb.New(arg)
	if er != nil {
		return nil, nil, NewError(ErrCosmosRemoteRequestInvalid, "ElementRemote: SpawnAtom arg error.").AddStack(nil)
	}
	rsp, er := client.SpawnAtom(ctx, &CosmosRemoteSpawnAtomReq{
		Element: e.info.Element,
		Atom:    name,
		Args:    anyArg,
	})
	if er != nil {
		return nil, nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: SpawnAtom response error. err=(%v)", er).AddStack(nil)
	}
	if rsp.Error != nil {
		return nil, nil, rsp.Error.AddStack(nil)
	}

	e.lock.Lock()
	a, has := e.atoms[name]
	if !has {
		a = newAtomRemote(e, rsp.Id)
		e.atoms[name] = a
	}
	e.lock.Unlock()

	return a, tracker.newIDTrackerFromRemoteTrackerID(e, rsp.TrackerId), nil
}

func (e *ElementRemote) ScaleGetAtomID(callerID SelfID, name string, timeout time.Duration, in proto.Message, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	cli := e.cosmos.getCurrentClient()
	if cli == nil {
		return nil, nil, NewError(ErrCosmosRemoteConnectFailed, "ElementRemote: ScaleGetAtomID client error.").AddStack(nil)
	}

	firstSyncCall := ""
	if callerFirst := callerID.getCurFirstSyncCall(); callerFirst == "" {
		// 要从调用者开始算起，所以要从调用者的ID中获取。
		firstSyncCall = callerID.nextFirstSyncCall()
		if err := callerID.setSyncMessageAndFirstCall(firstSyncCall); err != nil {
			return nil, nil, err.AddStack(nil)
		}
		defer callerID.unsetSyncMessageAndFirstCall()
	} else {
		firstSyncCall = callerFirst
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	arg, er := anypb.New(in)
	if er != nil {
		return nil, nil, NewError(ErrCosmosRemoteRequestInvalid, "ElementRemote: ScaleGetAtomID arg error.").AddStack(nil)
	}
	rsp, er := client.ScaleGetAtomID(ctx, &CosmosRemoteScaleGetAtomIDReq{
		CallerId:               callerID.GetIDInfo(),
		CallerCurFirstSyncCall: firstSyncCall,
		To:                     e.cosmos.info.Id,
		Timeout:                int64(timeout),
		Message:                name,
		Args:                   arg,
	})
	if er != nil {
		return nil, nil, NewError(ErrCosmosRemoteResponseInvalid, "ElementRemote: ScaleGetAtomID reply error.").AddStack(nil)
	}
	if rsp.Error != nil {
		return nil, nil, rsp.Error.AddStack(nil)
	}

	e.lock.Lock()
	a, has := e.atoms[name]
	if !has {
		a = newAtomRemote(e, rsp.Id)
		e.atoms[name] = a
	}
	e.lock.Unlock()

	return a, tracker.newIDTrackerFromRemoteTrackerID(e, rsp.TrackerId), nil
}

// 内部实现
// INTERNAL

func (e *ElementRemote) elementAtomRelease(tracker *IDTracker) {
	cli := e.cosmos.getCurrentClient()
	if cli == nil {
		// TODO: log
		return
	}
	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	rsp, er := client.ReleaseID(ctx, &CosmosRemoteReleaseIDReq{
		TrackerId: tracker.id,
	})
	if er != nil {
		// TODO: log
		return
	}
	if rsp.Error != nil {
		// TODO: log
		return
	}
}

func (e *ElementRemote) setDisable() {
	e.enable = false
}

// remoteElementFakeSelfID is a fake SelfID for remote element.

type remoteElementFakeSelfID struct {
	*ElementRemote
	firstSyncCall string
}

func (e *ElementRemote) newRemoteElementFromCaller(id *IDInfo, call string) *remoteElementFakeSelfID {
	return &remoteElementFakeSelfID{
		ElementRemote: e,
		firstSyncCall: call,
	}
}

func (r *remoteElementFakeSelfID) Log() Logging {
	panic("not supported, should not be called")
}

func (r *remoteElementFakeSelfID) Task() Task {
	panic("not supported, should not be called")
}

func (r *remoteElementFakeSelfID) CosmosMain() *CosmosLocal {
	panic("not supported, should not be called")
}

func (r *remoteElementFakeSelfID) KillSelf() {
	panic("not supported, should not be called")
}

func (r *remoteElementFakeSelfID) Parallel(f func()) {
	panic("not supported, should not be called")
}

func (r *remoteElementFakeSelfID) Config() map[string][]byte {
	panic("not supported, should not be called")
}

func (r *remoteElementFakeSelfID) pushAsyncMessageCallbackMailAndWaitReply(name string, in proto.Message, err *Error, callback func(out proto.Message, err *Error)) {
	panic("not supported, should not be called")
}
