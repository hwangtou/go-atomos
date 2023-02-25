package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"sync"
	"time"
)

type ElementRemote struct {
	cosmos *CosmosRemote
	name   string
	info   *IDInfo
	impl   *ElementImplementation
	state  AtomosState

	atoms map[string]*AtomRemote
	lock  sync.RWMutex

	//callChain []ID
	callChains map[string]bool

	idTracker *IDTrackerManager
}

func newElementRemote(c *CosmosRemote, name string, info *IDInfo, i *ElementImplementation) *ElementRemote {
	e := &ElementRemote{
		cosmos:     c,
		name:       name,
		info:       info,
		impl:       i,
		state:      0,
		atoms:      map[string]*AtomRemote{},
		lock:       sync.RWMutex{},
		callChains: map[string]bool{},
		idTracker:  nil,
	}
	e.idTracker = NewIDTrackerManager(e)
	return e
}

// Implementation of ID

func (e *ElementRemote) GetIDInfo() *IDInfo {
	if e == nil || e.info == nil {
		return nil
	}
	return e.info
}

func (e *ElementRemote) String() string {
	return e.GetIDInfo().Info()
}

func (e *ElementRemote) Release(id *IDTracker) {
	e.idTracker.Release(id)
}

func (e *ElementRemote) Cosmos() CosmosNode {
	return e.cosmos
}

func (e *ElementRemote) Element() Element {
	return e
}

func (e *ElementRemote) GetName() string {
	return e.GetIDInfo().Element
}

func (e *ElementRemote) State() AtomosState {
	return e.state
}

func (e *ElementRemote) IdleTime() time.Duration {
	return 0
}

func (e *ElementRemote) MessageByName(fromID ID, name string, timeout time.Duration, in proto.Message) (proto.Message, *Error) {
	return e.sendMessage(fromID, name, timeout, in)
}

func (e *ElementRemote) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := e.impl.ElementDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

func (e *ElementRemote) Kill(from ID, timeout time.Duration) *Error {
	return NewError(ErrElementRemoteCannotKill, "ElementRemote: Cannot kill remote element.").AddStack(e.cosmos.main)
}

func (e *ElementRemote) SendWormhole(from ID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	return NewErrorf(ErrElementRemoteCannotSendWormhole, "ElementRemote: Cannot send remote wormhole.").AddStack(e.cosmos.main)
}

func (e *ElementRemote) getElementLocal() *ElementLocal {
	return nil
}

func (e *ElementRemote) getAtomLocal() *AtomLocal {
	return nil
}

func (e *ElementRemote) getElementRemote() *ElementRemote {
	return e
}

func (e *ElementRemote) getAtomRemote() *AtomRemote {
	return nil
}

func (e *ElementRemote) getIDTrackerManager() *IDTrackerManager {
	return nil
}

func (e *ElementRemote) getCurCallChain() string {
	//TODO implement me
	panic("implement me")
}

func (e *ElementRemote) First() ID {
	//TODO implement me
	panic("implement me")
}

// Implementation of Element

func (e *ElementRemote) GetElementName() string {
	return e.GetIDInfo().Element
}

func (e *ElementRemote) GetAtomID(name string, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	return e.elementAtomGet(name, tracker)
}

func (e *ElementRemote) GetAtomsNum() int {
	// TODO
	return 0
}

func (e *ElementRemote) GetActiveAtomsNum() int {
	// TODO
	return 0
}

func (e *ElementRemote) GetAllInactiveAtomsIDTrackerInfo() map[string]string {
	// TODO
	return map[string]string{}
}

func (e *ElementRemote) SpawnAtom(name string, arg proto.Message, tracker *IDTrackerInfo) (*AtomLocal, *IDTracker, *Error) {
	return nil, nil, NewErrorf(ErrCosmosRemoteCannotSpawn, "").AddStack(e.cosmos.main)
}

func (e *ElementRemote) MessageElement(fromID, toID ID, name string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	if fromID == nil {
		return reply, NewErrorf(ErrAtomFromIDInvalid, "ElementRemote: MessageElement, FromID invalid. from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args).AddStack(e.cosmos.main)
	}
	elem := toID.getElementRemote()
	if elem == nil {
		return reply, NewErrorf(ErrAtomToIDInvalid, "ElementRemote: MessageElement, ToID invalid. from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args).AddStack(e.cosmos.main)
	}
	// PushProcessLog.
	return e.sendMessage(fromID, name, timeout, args)
}

func (e *ElementRemote) MessageAtom(fromID, toID ID, name string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	if fromID == nil {
		return reply, NewErrorf(ErrAtomFromIDInvalid, "ElementRemote: MessageAtom, FromID invalid. from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args).AddStack(e.cosmos.main)
	}
	a := toID.getAtomRemote()
	if a == nil {
		return reply, NewErrorf(ErrAtomToIDInvalid, "ElementRemote: MessageAtom, ToID invalid. from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args).AddStack(e.cosmos.main)
	}
	// PushProcessLog.
	return e.sendMessage(fromID, name, timeout, args)
}

func (e *ElementRemote) ScaleGetAtomID(fromID ID, name string, timeout time.Duration, args proto.Message, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	if fromID == nil {
		return nil, nil, NewErrorf(ErrAtomFromIDInvalid, "ElementRemote: ScaleGetAtomID, FromID invalid. from=(%s),name=(%s),args=(%v)",
			fromID, name, args).AddStack(e.cosmos.main)
	}
	return e.getScaleID(fromID, name, timeout, args, tracker)
}

func (e *ElementRemote) KillAtom(fromID, toID ID, timeout time.Duration) *Error {
	return NewErrorf(ErrElementCannotKill, "").AddStack(e.cosmos.main)
}

// Internal

func (e *ElementRemote) elementAtomGet(name string, tracker *IDTrackerInfo) (*AtomRemote, *IDTracker, *Error) {
	e.lock.RLock()
	a, has := e.atoms[name]
	e.lock.RUnlock()
	if has {
		return a, a.idTracker.NewIDTracker(tracker), nil
	}

	req := &CosmosRemoteGetIDReq{
		FromAddr:    e.cosmos.addr,
		FromTracker: tracker,
		Element:     e.name,
		Atom:        name,
	}
	rsp := &CosmosRemoteGetIDRsp{}
	if err := e.cosmos.httpPost(e.cosmos.addr+RemoteAtomosGetAtom, req, rsp, 0); err != nil {
		return nil, nil, err.AddStack(e.cosmos.main)
	}
	if rsp.Error != nil {
		return nil, nil, rsp.Error.AddStack(e.cosmos.main)
	}
	a = newAtomRemote(e, rsp.Id)
	e.lock.Lock()
	a, has = e.atoms[name]
	if has {
		a.info = rsp.Id
	} else {
		e.atoms[name] = a
	}
	e.lock.Unlock()
	return a, a.idTracker.NewIDTracker(tracker), nil
}

func (e *ElementRemote) sendMessage(fromID ID, name string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	// Send failed, how to handle? at least delete atom, and retry
	a, er := anypb.New(args)
	if er != nil {
		return reply, NewErrorf(ErrAtomMessageArgType, "marshal failed, err=(%v)", er)
	}
	if fromID != nil {
		//if !e.checkCallChain(fromID.getCallChain()) {
		//	return reply, NewErrorf(ErrAtomosCallDeadLock, "Call Dead Lock, chain=(%v),to(%s),name=(%s),args=(%v)",
		//		fromID.getCallChain(), e, name, args)
		//}
		//e.addCallChain(fromID.getCallChain())
		//defer e.delCallChain()
	}

	req := &CosmosRemoteMessagingReq{
		From:      fromID.GetIDInfo(),
		FromAddr:  e.cosmos.main.getFromAddr(),
		To:        e.GetIDInfo(),
		CallChain: nil,
		Timeout:   float32(timeout),
		Message:   name,
		Args:      a,
	}
	//for _, id := range e.callChain {
	//	if id == nil {
	//		req.CallChain = append(req.CallChain, nil)
	//		continue
	//	}
	//	req.CallChain = append(req.CallChain, id.GetIDInfo())
	//}
	rsp := &CosmosRemoteMessagingRsp{}
	if err = e.cosmos.httpPost(e.cosmos.addr+RemoteAtomosMessaging, req, rsp, timeout); err != nil {
		return nil, err.AddStack(e.cosmos.main)
	}
	if rsp.Reply != nil {
		reply, er = rsp.Reply.UnmarshalNew()
		if er != nil && rsp.Error == nil {
			err = NewErrorf(ErrAtomMessageReplyType, "unmarshal failed, err=(%v)", er).AddStack(e.cosmos.main)
		}
	}
	if rsp.Error != nil {
		err = rsp.Error.AddStack(e.cosmos.main)
	}
	return reply, err
}

func (e *ElementRemote) getScaleID(fromID ID, name string, timeout time.Duration, args proto.Message, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	// Send failed, how to handle? at least delete atom, and retry
	argProto, er := anypb.New(args)
	if er != nil {
		return nil, nil, NewErrorf(ErrAtomMessageArgType, "marshal failed, err=(%v)", er)
	}
	req := &CosmosRemoteScalingReq{
		From:     fromID.GetIDInfo(),
		FromAddr: e.cosmos.main.getFromAddr(),
		To:       e.GetIDInfo(),
		Message:  name,
		Args:     argProto,
	}
	rsp := &CosmosRemoteScalingRsp{}
	if err := e.cosmos.httpPost(e.cosmos.addr+RemoteAtomosElementScaling, req, rsp, timeout); err != nil {
		return nil, nil, err.AddStack(e.cosmos.main)
	}
	if rsp.Error != nil {
		return nil, nil, rsp.Error.AddStack(e.cosmos.main)
	}
	a := newAtomRemote(e, rsp.Id)
	e.lock.Lock()
	a, has := e.atoms[name]
	if has {
		a.info = rsp.Id
	} else {
		e.atoms[name] = a
	}
	e.lock.Unlock()
	return a, a.idTracker.NewIDTracker(tracker), nil
	//if rsp.Id != nil {
	//	e.lock.Lock()
	//	a, has := e.atoms[rsp.Id.Atomos]
	//	if !has {
	//		a = &AtomRemote{
	//			element: e,
	//			info: &AtomRemoteInfo{
	//				Id: rsp.Id,
	//			},
	//			state:     AtomosStateUnknown,
	//			count:     1,
	//			callChain: nil,
	//		}
	//	} else {
	//		a.count += 1
	//	}
	//	e.lock.Unlock()
	//	id = a
	//}
	//if rsp.Error != nil {
	//	err = rsp.Error
	//}
	//return id, tracker, err
}

//func (e *ElementRemote) release(a *AtomRemote) {
//	a.count -= 1
//	// TODO: Remove
//}

func (e *ElementRemote) elementAtomRelease(a *AtomRemote, tracker *IDTracker) {
	req := &CosmosRemoteReleaseIDReq{
		TrackerId: uint64(tracker.id),
	}
	rsp := &CosmosRemoteReleaseIDRsp{}
	if err := e.cosmos.httpPost(e.cosmos.addr+RemoteAtomosIDRelease, req, rsp, 0); err != nil {
		// TODO
	}
	if rsp.Error != nil {
		// TODO
	}
}

func (e *ElementRemote) elementAtomState(a *AtomRemote) AtomosState {
	// TODO
	return AtomosHalt
}

func (e *ElementRemote) elementAtomIdleTime(a *AtomRemote) time.Duration {
	// TODO
	return 0
}
