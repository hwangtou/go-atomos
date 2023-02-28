package go_atomos

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"sync"
	"sync/atomic"
	"time"
)

type ElementRemote struct {
	cosmos *CosmosRemote
	name   string
	info   *IDInfo
	intf   *ElementInterface
	state  AtomosState

	atoms map[string]*AtomRemote
	lock  sync.RWMutex

	callIDCounter uint64
	idTracker     *IDTrackerManager
}

type ElementRemoteID struct {
	*ElementRemote
	callChain string
}

func newElementRemote(c *CosmosRemote, name string, info *IDInfo, i *ElementInterface) *ElementRemote {
	e := &ElementRemote{
		cosmos:    c,
		name:      name,
		info:      info,
		intf:      i,
		state:     0,
		atoms:     map[string]*AtomRemote{},
		lock:      sync.RWMutex{},
		idTracker: nil,
	}
	e.idTracker = NewIDTrackerManager(e)
	return e
}

func (e *ElementRemote) newElementRemoteID(callChain string) *ElementRemoteID {
	return &ElementRemoteID{
		ElementRemote: e,
		callChain:     callChain,
	}
}

func (e *ElementRemote) Release(tracker *IDTracker) {
	// TODO
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
	return e.sendElementMessage(fromID, name, timeout, in)
}

func (e *ElementRemote) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := e.intf.ElementDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

func (e *ElementRemote) Kill(from ID, timeout time.Duration) *Error {
	return NewError(ErrElementRemoteCannotKill, "ElementRemoteID: Cannot kill remote element.").AddStack(e.cosmos.main)
}

func (e *ElementRemote) SendWormhole(from ID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	return NewErrorf(ErrElementRemoteCannotSendWormhole, "ElementRemoteID: Cannot send remote wormhole.").AddStack(e.cosmos.main)
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
	return ""
}

func (e *ElementRemote) First() ID {
	callChainID := atomic.AddUint64(&e.callIDCounter, 1)
	return &ElementRemoteID{
		ElementRemote: e,
		callChain:     fmt.Sprintf("%s:%d", e.info, callChainID),
	}
}

func (e *ElementRemoteID) getCurCallChain() string {
	return e.callChain
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
	panic("todo")
}

func (e *ElementRemote) MessageElement(fromID, toID ID, name string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	if fromID == nil {
		return reply, NewErrorf(ErrElementFromIDInvalid, "ElementRemoteID: MessageElement, FromID invalid. from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args).AddStack(e.cosmos.main)
	}
	elem := toID.getElementRemote()
	if elem == nil {
		return reply, NewErrorf(ErrElementToIDInvalid, "ElementRemoteID: MessageElement, ToID invalid. from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args).AddStack(e.cosmos.main)
	}
	return e.sendElementMessage(fromID, name, timeout, args)
}

func (e *ElementRemote) MessageAtom(fromID, toID ID, name string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	if fromID == nil {
		return reply, NewErrorf(ErrAtomFromIDInvalid, "ElementRemoteID: MessageAtom, FromID invalid. from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args).AddStack(e.cosmos.main)
	}
	a := toID.getAtomRemote()
	if a == nil {
		return reply, NewErrorf(ErrAtomToIDInvalid, "ElementRemoteID: MessageAtom, ToID invalid. from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args).AddStack(e.cosmos.main)
	}
	return e.sendAtomMessage(fromID, toID, name, timeout, args)
}

func (e *ElementRemote) ScaleGetAtomID(fromID ID, name string, timeout time.Duration, args proto.Message, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	return e.getElementScaleID(fromID, name, timeout, args, tracker)
}

func (e *ElementRemote) KillAtom(fromID, toID ID, timeout time.Duration) *Error {
	return NewErrorf(ErrElementCannotKill, "").AddStack(e.cosmos.main)
}

// Internal

func (e *ElementRemote) getElementScaleID(fromID ID, name string, timeout time.Duration, args proto.Message, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	// Send failed, how to handle? at least delete atom, and retry
	argProto, er := anypb.New(args)
	if er != nil {
		return nil, nil, NewErrorf(ErrAtomMessageArgType, "marshal failed, err=(%v)", er).AddStack(e.cosmos.main)
	}
	if fromID == nil {
		return nil, nil, NewErrorf(ErrAtomFromIDInvalid, "ElementRemoteID: ScaleGetAtomID, FromID invalid. from=(%s),name=(%s),args=(%v)",
			fromID, name, args).AddStack(e.cosmos.main)
	}
	var fromCallChain string
	f, ok := fromID.(*FirstID)
	if ok {
		fromCallChain = f.getFirstIDCallChain()
	} else {
		fromCallChain = fromID.getCurCallChain()
	}
	req := &CosmosRemoteScalingReq{
		From:          fromID.GetIDInfo(),
		FromCallChain: fromCallChain,
		FromTracker:   tracker,
		To:            e.GetIDInfo(),
		Timeout:       float32(timeout / time.Second),
		Message:       name,
		Args:          argProto,
	}
	rsp := &CosmosRemoteScalingRsp{}
	if err := e.cosmos.httpPost(e.cosmos.addr+RemoteAtomosElementScaling, req, rsp, timeout); err != nil {
		return nil, nil, err.AddStack(e.cosmos.main)
	}
	if rsp.Error != nil {
		return nil, nil, rsp.Error.AddStack(e.cosmos.main)
	}
	e.lock.Lock()
	a, has := e.atoms[name]
	if has {
		a.info = rsp.Id
	} else {
		a = newAtomRemote(e, rsp.Id)
		e.atoms[name] = a
	}
	e.lock.Unlock()
	return a.newAtomRemoteID(fromCallChain), newIDTrackerWithTrackerID(tracker, e, rsp.TrackerId), nil
}

func (e *ElementRemote) sendElementMessage(fromID ID, name string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	// Send failed, how to handle? at least delete atom, and retry
	a, er := anypb.New(args)
	if er != nil {
		return reply, NewErrorf(ErrElementMessageArgType, "ElementRemoteID: Marshal failed, err=(%v)", er)
	}
	if fromID == nil {
		return nil, NewErrorf(ErrAtomosCallDeadLock, "ElementRemoteID: FromID is nil. to=(%v),name=(%s),arg=(%v)",
			e, name, args).AddStack(e.cosmos.main)
	}

	var fromCallChain string
	f, ok := fromID.(*FirstID)
	if ok {
		fromCallChain = f.getFirstIDCallChain()
	} else {
		fromCallChain = fromID.getCurCallChain()
	}
	req := &CosmosRemoteMessagingReq{
		From:          fromID.GetIDInfo(),
		FromCallChain: fromCallChain,
		To:            e.GetIDInfo(),
		Timeout:       float32(timeout),
		Message:       name,
		Args:          a,
	}
	rsp := &CosmosRemoteMessagingRsp{}
	if err = e.cosmos.httpPost(e.cosmos.addr+RemoteAtomosElementMessaging, req, rsp, timeout); err != nil {
		return nil, err.AddStack(e.cosmos.main)
	}
	if rsp.Reply != nil {
		reply, er = rsp.Reply.UnmarshalNew()
		if er != nil && rsp.Error == nil {
			err = NewErrorf(ErrElementMessageReplyType, "unmarshal failed, err=(%v)", er).AddStack(e.cosmos.main)
		}
	}
	if rsp.Error != nil {
		err = rsp.Error.AddStack(e.cosmos.main)
	}
	return reply, err
}

func (e *ElementRemote) spawnAtom() {
	// TODO
}

func (e *ElementRemote) elementAtomGet(name string, tracker *IDTrackerInfo) (ID, *IDTracker, *Error) {
	req := &CosmosRemoteGetIDReq{
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
	e.lock.Lock()
	a, has := e.atoms[name]
	if has {
		a.info = rsp.Id
	} else {
		a = newAtomRemote(e, rsp.Id)
		e.atoms[name] = a
	}
	e.lock.Unlock()
	return a.newAtomRemoteID(e.getCurCallChain()), newIDTrackerWithTrackerID(tracker, e, rsp.TrackerId), nil
}

func (e *ElementRemote) elementAtomRelease(tracker *IDTracker) {
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

func (e *ElementRemote) sendAtomMessage(fromID, to ID, name string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	// Send failed, how to handle? at least delete atom, and retry
	a, er := anypb.New(args)
	if er != nil {
		return reply, NewErrorf(ErrAtomMessageArgType, "ElementRemoteID: Marshal failed, err=(%v)", er).AddStack(e.cosmos.main)
	}
	if fromID == nil {
		return nil, NewErrorf(ErrAtomosCallDeadLock, "ElementRemoteID: FromID is nil. to=(%v),name=(%s),arg=(%v)",
			e, name, args).AddStack(e.cosmos.main)
	}

	var fromCallChain string
	f, ok := fromID.(*FirstID)
	if ok {
		fromCallChain = f.getFirstIDCallChain()
	} else {
		fromCallChain = fromID.getCurCallChain()
	}
	req := &CosmosRemoteMessagingReq{
		From:          fromID.GetIDInfo(),
		FromCallChain: fromCallChain,
		To:            to.GetIDInfo(),
		Timeout:       float32(timeout),
		Message:       name,
		Args:          a,
	}
	rsp := &CosmosRemoteMessagingRsp{}
	if err = e.cosmos.httpPost(e.cosmos.addr+RemoteAtomosAtomMessaging, req, rsp, timeout); err != nil {
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

func (e *ElementRemote) elementAtomState(a *AtomRemote) AtomosState {
	// TODO
	return AtomosHalt
}

func (e *ElementRemote) elementAtomIdleTime(a *AtomRemote) time.Duration {
	// TODO
	return 0
}
