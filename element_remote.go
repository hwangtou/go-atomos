package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"sync"
	"time"
)

// ElementRemote
// 远程的Element实现。
// Implement of remote Element.

type ElementRemote struct {
	cosmos  *CosmosRemote
	context atomosIDContextRemote
	current *ElementInterface

	atoms map[string]*AtomRemote
	lock  sync.RWMutex

	version string

	enable bool
}

func newElementRemote(c *CosmosRemote, info *IDInfo, i *ElementInterface, version string) *ElementRemote {
	e := &ElementRemote{
		cosmos:  c,
		context: atomosIDContextRemote{},
		current: i,
		atoms:   map[string]*AtomRemote{},
		lock:    sync.RWMutex{},
		version: version,
		enable:  false,
	}
	initAtomosIDContextRemote(&e.context, info)
	return e
}

// Implementation of ID

func (e *ElementRemote) GetIDContext() IDContext {
	return &e.context
}

func (e *ElementRemote) GetIDInfo() *IDInfo {
	return e.context.info
}

func (e *ElementRemote) String() string {
	return e.GetIDInfo().Info()
}

func (e *ElementRemote) Cosmos() CosmosNode {
	return e.cosmos
}

func (e *ElementRemote) State() AtomosState {
	client, ctx, cancel, err := e.cosmos.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		e.cosmos.process.local.Log().Error("ElementRemote: State failed. err=(%v)", err)
		return AtomosState(0)
	}
	defer cancel()

	rsp, er := client.GetIDState(ctx, &CosmosRemoteGetIDStateReq{Id: e.context.info})
	if er != nil {
		e.cosmos.process.local.Log().Error("ElementRemote: State failed. err=(%v)", er)
		return AtomosState(0)
	}

	return AtomosState(rsp.State)
}

func (e *ElementRemote) IdleTime() time.Duration {
	client, ctx, cancel, err := e.cosmos.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		e.cosmos.process.local.Log().Error("ElementRemote: IdleTime failed. err=(%v)", err)
		return 0
	}
	defer cancel()

	rsp, er := client.GetIDIdleTime(ctx, &CosmosRemoteGetIDIdleTimeReq{Id: e.context.info})
	if er != nil {
		e.cosmos.process.local.Log().Error("ElementRemote: IdleTime failed. err=(%v)", er)
		return 0
	}

	return time.Duration(rsp.IdleTime)
}

func (e *ElementRemote) SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error) {
	if callerID == nil {
		return nil, NewError(ErrFrameworkIncorrectUsage, "ElementRemote: SyncMessagingByName without fromID.").AddStack(nil)
	}

	var er error
	var arg *anypb.Any
	if in != nil {
		arg, er = anypb.New(in)
		if er != nil {
			return nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "ElementRemote: SyncMessagingByName arg error. err=(%v)", er).AddStack(nil)
		}
	}

	client, ctx, cancel, err := e.cosmos.getCurrentClientWithTimeout(timeout)
	if err != nil {
		return nil, err.AddStack(nil)
	}
	defer cancel()

	callerIdInfo := callerID.GetIDInfo()
	toIDInfo := e.context.info

	rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId: callerIdInfo,
		CallerContext: &IDContextInfo{
			IdChain: append(callerID.GetIDContext().FromCallChain(), callerID.GetIDInfo().Info()),
		},
		To:      toIDInfo,
		Timeout: int64(timeout),
		Message: name,
		Args:    arg,
	})
	if er != nil {
		return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: SyncMessagingByName response error. err=(%v)", er).AddStack(nil)
	}
	if rsp.Reply != nil {
		out, er = rsp.Reply.UnmarshalNew()
		if er != nil {
			return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: SyncMessagingByName reply unmarshal error. err=(%v)", er).AddStack(nil)
		}
	}
	if rsp.Error != nil {
		err = rsp.Error.AddStack(nil)
	}
	return out, err
}

func (e *ElementRemote) AsyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message, callback func(out proto.Message, err *Error)) {
	if callerID == nil {
		if callback != nil {
			callback(nil, NewError(ErrFrameworkIncorrectUsage, "ElementRemote: AsyncMessagingByName without fromID.").AddStack(nil))
		}
		e.cosmos.process.local.Log().Error("ElementRemote: AsyncMessagingByName without fromID.")
		return
	}

	var er error
	var arg *anypb.Any
	if in != nil {
		arg, er = anypb.New(in)
		if er != nil {
			if callback != nil {
				callback(nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "ElementRemote: AsyncMessagingByName arg error. err=(%v)", er).AddStack(nil))
			}
			e.cosmos.process.local.Log().Error("ElementRemote: AsyncMessagingByName arg error. err=(%v)", er)
			return
		}
	}

	client, ctx, cancel, err := e.cosmos.getCurrentClientWithTimeout(timeout)
	if err != nil {
		if callback != nil {
			callback(nil, err.AddStack(nil))
		}
		e.cosmos.process.local.Log().Error("ElementRemote: AsyncMessagingByName client error. err=(%v)", err)
		return
	}

	callerIdInfo := callerID.GetIDInfo()
	toIDInfo := e.context.info
	needReply := callback != nil

	e.cosmos.process.local.Parallel(func() {
		out, err := func() (out proto.Message, err *Error) {

			defer cancel()
			rsp, er := client.AsyncMessagingByName(ctx, &CosmosRemoteAsyncMessagingByNameReq{
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
			callerID.asyncCallback(callerID, name, out, err, callback)
		}
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

func (e *ElementRemote) Kill(_ SelfID, _ time.Duration) *Error {
	return NewError(ErrElementRemoteCannotKill, "ElementRemote: Cannot kill remote element.").AddStack(nil)
}

func (e *ElementRemote) SendWormhole(_ SelfID, _ time.Duration, _ AtomosWormhole) *Error {
	return NewErrorf(ErrElementRemoteCannotSendWormhole, "ElementRemote: Cannot send remote wormhole.").AddStack(nil)
}

func (e *ElementRemote) getIDTrackerManager() *atomosIDTracker {
	panic("ElementRemote: getIDTrackerManager should not be called.")
}

func (e *ElementRemote) getGoID() uint64 {
	//return e.info.GoId
	return 0
}

func (e *ElementRemote) asyncCallback(callerID SelfID, name string, reply proto.Message, err *Error, callback func(reply proto.Message, err *Error)) {
	if callback == nil {
		return
	}
	callback(reply, err)
}

// Implementation of Element

func (e *ElementRemote) GetAtomID(name string, _ *IDTrackerInfo, _ bool) (ID, *IDTracker, *Error) {
	client, ctx, cancel, err := e.cosmos.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	defer cancel()

	rsp, er := client.GetAtomID(ctx, &CosmosRemoteGetAtomIDReq{
		Element: e.context.info.Element,
		Atom:    name,
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
		a = newAtomRemote(e, rsp.Id, e.version)
		e.atoms[name] = a
	}
	e.lock.Unlock()

	return a, nil, nil
}

func (e *ElementRemote) GetAtomsNum() int {
	client, ctx, cancel, err := e.cosmos.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		e.cosmos.process.local.Log().Error("ElementRemote: GetAtomsNum failed. err=(%v)", err)
		return 0
	}
	defer cancel()

	rsp, er := client.GetElementInfo(ctx, &CosmosRemoteGetElementInfoReq{
		Element: e.context.info.Element,
	})
	if er != nil {
		e.cosmos.process.local.Log().Error("ElementRemote: GetAtomsNum failed. err=(%v)", er)
		return 0
	}

	return int(rsp.AtomsNum)
}

func (e *ElementRemote) GetActiveAtomsNum() int {
	client, ctx, cancel, err := e.cosmos.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		e.cosmos.process.local.Log().Error("ElementRemote: GetActiveAtomsNum failed. err=(%v)", err)
		return 0
	}
	defer cancel()

	rsp, er := client.GetElementInfo(ctx, &CosmosRemoteGetElementInfoReq{
		Element: e.context.info.Element,
	})
	if er != nil {
		e.cosmos.process.local.Log().Error("ElementRemote: GetActiveAtomsNum failed. err=(%v)", er)
		return 0
	}

	return int(rsp.ActiveAtomsNum)
}

func (e *ElementRemote) GetAllInactiveAtomsIDTrackerInfo() map[string]string {
	// Not Supported.
	return map[string]string{}
}

func (e *ElementRemote) SpawnAtom(callerID SelfID, name string, arg proto.Message, _ *IDTrackerInfo, _ bool) (ID, *IDTracker, *Error) {
	client, ctx, cancel, err := e.cosmos.getCurrentClientWithTimeout(0)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	defer cancel()

	var er error
	var anyArg *anypb.Any
	if arg != nil {
		anyArg, er = anypb.New(arg)
		if er != nil {
			return nil, nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "ElementRemote: SpawnAtom arg error. err=(%v)", er).AddStack(nil)
		}
	}
	rsp, er := client.SpawnAtom(ctx, &CosmosRemoteSpawnAtomReq{
		CallerId: callerID.GetIDInfo(),
		CallerContext: &IDContextInfo{
			IdChain: append(callerID.GetIDContext().FromCallChain(), callerID.GetIDInfo().Info()),
		},
		Element: e.context.info.Element,
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
		a = newAtomRemote(e, rsp.Id, e.version)
		e.atoms[name] = a
	}
	e.lock.Unlock()

	return a, nil, nil
}

func (e *ElementRemote) ScaleGetAtomID(callerID SelfID, name string, timeout time.Duration, in proto.Message, _ *IDTrackerInfo, _ bool) (ID, *IDTracker, *Error) {
	client, ctx, cancel, err := e.cosmos.getCurrentClientWithTimeout(timeout)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	defer cancel()

	var er error
	var arg *anypb.Any
	if in != nil {
		arg, er = anypb.New(in)
		if er != nil {
			return nil, nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "ElementRemote: ScaleGetAtomID arg error. err=(%v)", er).AddStack(nil)
		}
	}
	rsp, er := client.ScaleGetAtomID(ctx, &CosmosRemoteScaleGetAtomIDReq{
		CallerId: callerID.GetIDInfo(),
		CallerContext: &IDContextInfo{
			IdChain: append(callerID.GetIDContext().FromCallChain(), callerID.GetIDInfo().Info()),
		},
		To: &IDInfo{
			Type:    IDType_Atom,
			Cosmos:  e.context.info.Cosmos,
			Node:    e.context.info.Node,
			Element: e.context.info.Element,
			Atom:    "",
			Version: 0,
			//GoId:    0,
		},
		Timeout: int64(timeout),
		Message: name,
		Args:    arg,
	})
	if er != nil {
		return nil, nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "ElementRemote: ScaleGetAtomID reply error. err=(%v)", er).AddStack(nil)
	}
	if rsp.Error != nil {
		return nil, nil, rsp.Error.AddStack(nil)
	}

	e.lock.Lock()
	a, has := e.atoms[rsp.Id.Atom]
	if !has {
		a = newAtomRemote(e, rsp.Id, e.version)
		e.atoms[rsp.Id.Atom] = a
	}
	e.lock.Unlock()

	return a, nil, nil
}

// 内部实现
// INTERNAL

func (e *ElementRemote) setDisable() {
	e.enable = false
}

// remoteElementFakeSelfID is a fake SelfID for remote element.

type remoteElementFakeSelfID struct {
	*ElementRemote
	callerIDContext *IDContextInfo
}

func (e *ElementRemote) newRemoteElementFromCaller(callerID *IDInfo, callerIDContext *IDContextInfo) *remoteElementFakeSelfID {
	return &remoteElementFakeSelfID{
		ElementRemote:   e,
		callerIDContext: callerIDContext,
	}
}

func (r *remoteElementFakeSelfID) callerCounterRelease() {
}

func (r *remoteElementFakeSelfID) GetIDContext() IDContext {
	return r
}

func (r *remoteElementFakeSelfID) FromCallChain() []string {
	return r.callerIDContext.IdChain
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

func (r *remoteElementFakeSelfID) Parallel(_ func()) {
	panic("not supported, should not be called")
}

func (r *remoteElementFakeSelfID) Config() map[string][]byte {
	panic("not supported, should not be called")
}

func (r *remoteElementFakeSelfID) getAtomos() *BaseAtomos {
	panic("not supported, should not be called")
}
