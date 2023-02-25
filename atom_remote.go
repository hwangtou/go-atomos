package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"sync"
	"time"
)

type AtomRemote struct {
	element *ElementRemote
	info    *IDInfo

	mutex sync.RWMutex

	idTracker *IDTrackerManager
}

func (a *AtomRemote) getCurCallChain() string {
	//TODO implement me
	panic("implement me")
}

func (a *AtomRemote) First() ID {
	//TODO implement me
	panic("implement me")
}

func newAtomRemote(e *ElementRemote, info *IDInfo) *AtomRemote {
	a := &AtomRemote{
		element:   e,
		info:      info,
		mutex:     sync.RWMutex{},
		idTracker: nil,
	}
	a.idTracker = NewIDTrackerManager(a)
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

func (a *AtomRemote) Release(tracker *IDTracker) {
	a.element.elementAtomRelease(a, tracker)
}

func (a *AtomRemote) Cosmos() CosmosNode {
	return a.element.cosmos
}

func (a *AtomRemote) Element() Element {
	return a.element
}

func (a *AtomRemote) GetName() string {
	return a.GetIDInfo().Atom
}

func (a *AtomRemote) State() AtomosState {
	return a.element.elementAtomState(a)
}

func (a *AtomRemote) IdleTime() time.Duration {
	return a.element.elementAtomIdleTime(a)
}

func (a *AtomRemote) MessageByName(from ID, name string, timeout time.Duration, in proto.Message) (proto.Message, *Error) {
	return a.element.sendMessage(from, name, timeout, in)
}

func (a *AtomRemote) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := a.element.impl.AtomDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

func (a *AtomRemote) Kill(from ID, timeout time.Duration) *Error {
	return NewError(ErrMainCannotKill, "cannot kill remote atom")
}

func (a *AtomRemote) SendWormhole(from ID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	return NewErrorf(ErrAtomosNotSupportWormhole, "remote atom")
}

func (a *AtomRemote) getElementLocal() *ElementLocal {
	return nil
}

func (a *AtomRemote) getAtomLocal() *AtomLocal {
	return nil
}

func (a *AtomRemote) getElementRemote() *ElementRemote {
	return nil
}

func (a *AtomRemote) getAtomRemote() *AtomRemote {
	return a
}

func (a *AtomRemote) getIDTrackerManager() *IDTrackerManager {
	return a.idTracker
}

//func (a *AtomRemote) sendMessage(fromID ID, name string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
//	// Send failed, how to handle? at least delete atom, and retry
//	ap, er := anypb.New(args)
//	if er != nil {
//		return reply, NewErrorf(ErrAtomMessageArgType, "marshal failed, err=(%v)", er)
//	}
//	if fromID != nil {
//		if !a.checkCallChain(fromID.getCallChain()) {
//			return reply, NewErrorf(ErrAtomCallDeadLock, "Call Dead Lock, chain=(%v),to(%s),name=(%s),args=(%v)",
//				fromID.getCallChain(), a, name, args)
//		}
//		a.addCallChain(fromID.getCallChain())
//		defer a.delCallChain()
//	}
//	req := &CosmosRemoteMessagingReq{
//		From:      fromID.GetIDInfo(),
//		FromAddr:  a.element.cosmos.main.getFromAddr(),
//		To:        a.GetIDInfo(),
//		CallChain: nil,
//		Message:   name,
//		Args:      ap,
//	}
//	for _, id := range a.callChain {
//		if id == nil {
//			req.CallChain = append(req.CallChain, nil)
//			continue
//		}
//		req.CallChain = append(req.CallChain, id.GetIDInfo())
//	}
//	rsp := &CosmosRemoteMessagingRsp{}
//	if err = a.element.cosmos.httpPost(a.element.cosmos.addr+RemoteAtomosAtomMessaging+"?element="+name, req, rsp); err != nil {
//		return nil, err.AutoStack(nil, nil)
//	}
//	if rsp.Reply != nil {
//		reply, er = rsp.Reply.UnmarshalNew()
//		if er != nil && rsp.Error == nil {
//			err = NewErrorf(ErrAtomMessageReplyType, "unmarshal failed, err=(%v)", er).AutoStack(nil, nil)
//		}
//	}
//	if rsp.Error != nil {
//		err = rsp.Error
//	}
//	return reply, err
//}

// Check chain.

//func (a *AtomRemote) checkCallChain(fromIDList []ID) bool {
//	for _, fromID := range fromIDList {
//		if fromID.GetIDInfo().IsEqual(a.GetIDInfo()) {
//			return false
//		}
//	}
//	return true
//}
//
//func (a *AtomRemote) addCallChain(fromIDList []ID) {
//	a.callChain = append(fromIDList, a)
//}
//
//func (a *AtomRemote) delCallChain() {
//	a.callChain = nil
//}
