package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"time"
)

type AtomRemote struct {
	element *ElementRemote
	info    *AtomRemoteInfo

	state     AtomosState
	count     int
	callChain []ID
}

func newAtomRemote(e *ElementRemote, info *AtomRemoteInfo) *AtomRemote {
	return &AtomRemote{
		element:   e,
		info:      info,
		state:     AtomosStateUnknown,
		count:     1,
		callChain: nil,
	}
}

//
// Implementation of ID
//

func (a *AtomRemote) GetIDInfo() *IDInfo {
	if a == nil || a.info == nil {
		return nil
	}
	return a.info.Id
}

func (a *AtomRemote) String() string {
	return a.GetIDInfo().Info()
}

func (a *AtomRemote) Release() {
	a.count -= 1
	if a.count <= 0 {
		a.element.release(a)
	}
}

func (a *AtomRemote) Cosmos() CosmosNode {
	return a.element.cosmos
}

func (a *AtomRemote) Element() Element {
	return a.element
}

func (a *AtomRemote) GetName() string {
	return a.GetIDInfo().Atomos
}

func (a *AtomRemote) State() AtomosState {
	return a.state
}

func (a *AtomRemote) IdleDuration() time.Duration {
	return 0
}

func (a *AtomRemote) MessageByName(fromID ID, name string, in proto.Message) (proto.Message, *ErrorInfo) {
	return a.sendMessage(fromID, name, in)
}

func (a *AtomRemote) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := a.element.elementInterface.AtomDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

func (a *AtomRemote) Kill(from ID) *ErrorInfo {
	return NewError(ErrMainCannotKill, "cannot kill remote atom")
}

func (a *AtomRemote) SendWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo {
	return NewErrorf(ErrAtomosNotSupportWormhole, "remote atom")
}

func (a *AtomRemote) getCallChain() []ID {
	return a.callChain
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

func (a *AtomRemote) sendMessage(fromID ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	// Send failed, how to handle? at least delete atom, and retry
	ap, er := anypb.New(args)
	if er != nil {
		return reply, NewErrorf(ErrAtomMessageArgType, "marshal failed, err=(%v)", er)
	}
	if fromID != nil {
		if !a.checkCallChain(fromID.getCallChain()) {
			return reply, NewErrorf(ErrAtomCallDeadLock, "Call Dead Lock, chain=(%v),to(%s),name=(%s),args=(%v)",
				fromID.getCallChain(), a, name, args)
		}
		a.addCallChain(fromID.getCallChain())
		defer a.delCallChain()
	}
	req := &CosmosRemoteMessagingReq{
		From:      fromID.GetIDInfo(),
		FromAddr:  a.element.cosmos.main.getFromAddr(),
		To:        a.GetIDInfo(),
		CallChain: nil,
		Message:   name,
		Args:      ap,
	}
	for _, id := range a.callChain {
		if id == nil {
			req.CallChain = append(req.CallChain, nil)
			continue
		}
		req.CallChain = append(req.CallChain, id.GetIDInfo())
	}
	rsp := &CosmosRemoteMessagingRsp{}
	if err = a.element.cosmos.httpPost(a.element.cosmos.addr+RemoteAtomosAtomMessaging+"?element="+name, req, rsp); err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	if rsp.Reply != nil {
		reply, er = rsp.Reply.UnmarshalNew()
		if er != nil && rsp.Error == nil {
			err = NewErrorf(ErrAtomMessageReplyType, "unmarshal failed, err=(%v)", er).AutoStack(nil, nil)
		}
	}
	if rsp.Error != nil {
		err = rsp.Error
	}
	return reply, err
}

// Check chain.

func (a *AtomRemote) checkCallChain(fromIDList []ID) bool {
	for _, fromID := range fromIDList {
		if fromID.GetIDInfo().IsEqual(a.GetIDInfo()) {
			return false
		}
	}
	return true
}

func (a *AtomRemote) addCallChain(fromIDList []ID) {
	a.callChain = append(fromIDList, a)
}

func (a *AtomRemote) delCallChain() {
	a.callChain = nil
}
