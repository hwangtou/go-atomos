package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"sync"
	"time"
)

type ElementRemote struct {
	cosmos           *CosmosRemote
	name             string
	info             *ElementRemoteInfo
	elementInterface *ElementInterface
	state            AtomosState

	atoms map[string]*AtomRemote
	lock  sync.RWMutex

	callChain []ID
}

func newElementRemote(c *CosmosRemote, name string, info *ElementRemoteInfo, i *ElementInterface) *ElementRemote {
	return &ElementRemote{
		cosmos:           c,
		name:             name,
		info:             info,
		elementInterface: i,
		state:            0,
		atoms:            map[string]*AtomRemote{},
		lock:             sync.RWMutex{},
		callChain:        nil,
	}
}

// Implementation of ID

func (e *ElementRemote) GetIDInfo() *IDInfo {
	if e == nil || e.info == nil {
		return nil
	}
	return e.info.Id
}

func (e *ElementRemote) String() string {
	return e.GetIDInfo().Info()
}

func (e *ElementRemote) Release() {
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

func (e *ElementRemote) IdleDuration() time.Duration {
	return 0
}

func (e *ElementRemote) MessageByName(fromID ID, name string, in proto.Message) (proto.Message, *ErrorInfo) {
	return e.sendMessage(fromID, name, in)
}

func (e *ElementRemote) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	decoderFn, has := e.elementInterface.ElementDecoders[name]
	if !has {
		return nil, nil
	}
	return decoderFn.InDec, decoderFn.OutDec
}

func (e *ElementRemote) Kill(from ID) *ErrorInfo {
	return NewError(ErrMainCannotKill, "cannot kill remote element")
}

func (e *ElementRemote) SendWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo {
	return NewErrorf(ErrAtomosNotSupportWormhole, "remote element")
}

func (e *ElementRemote) getCallChain() []ID {
	return e.callChain
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

// Implementation of Element

func (e *ElementRemote) GetElementName() string {
	return e.GetIDInfo().Element
}

func (e *ElementRemote) GetAtomID(name string) (ID, *ErrorInfo) {
	return e.elementAtomGet(name)
}

func (e *ElementRemote) GetAtomsNum() int {
	return len(e.atoms)
}

func (e *ElementRemote) SpawnAtom(atomName string, arg proto.Message) (*AtomLocal, *ErrorInfo) {
	return nil, NewErrorf(ErrRemoteCannotRemoteSpawn, "").AutoStack(nil, nil)
}

func (e *ElementRemote) MessageElement(fromID, toID ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	if fromID == nil {
		return reply, NewErrorf(ErrAtomFromIDInvalid, "From ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args)
	}
	a := toID.getElementRemote()
	if a == nil {
		return reply, NewErrorf(ErrAtomToIDInvalid, "To ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args)
	}
	// PushProcessLog.
	return a.sendMessage(fromID, name, args)
}

func (e *ElementRemote) MessageAtom(fromID, toID ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	if fromID == nil {
		return reply, NewErrorf(ErrAtomFromIDInvalid, "From ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args)
	}
	a := toID.getAtomRemote()
	if a == nil {
		return reply, NewErrorf(ErrAtomToIDInvalid, "To ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)",
			fromID, toID, name, args)
	}
	// PushProcessLog.
	return a.sendMessage(fromID, name, args)
}

func (e *ElementRemote) ScaleGetAtomID(fromID ID, message string, args proto.Message) (ID, *ErrorInfo) {
	if fromID == nil {
		return nil, NewErrorf(ErrAtomFromIDInvalid, "From ID invalid, from=(%s),message=(%s),args=(%v)",
			fromID, message, args)
	}
	return e.getScaleID(fromID, message, args)
}

func (e *ElementRemote) KillAtom(fromID, toID ID) *ErrorInfo {
	return NewErrorf(ErrElementCannotKill, "").AutoStack(nil, nil)
}

// Internal

func (e *ElementRemote) elementAtomGet(name string) (ID, *ErrorInfo) {
	e.lock.RLock()
	a, has := e.atoms[name]
	e.lock.RUnlock()
	if has {
		return a, nil
	}
	url := e.cosmos.addr + RemoteAtomosGetAtom + "?element=" + e.info.Id.Element + "&atom=" + name
	info := &CosmosRemoteGetAtomIDRsp{}
	if err := e.cosmos.httpGet(url, info); err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	if info.Error != nil {
		return nil, info.Error.AutoStack(nil, nil)
	}
	if info.Info == nil {
		return nil, NewErrorf(ErrAtomNotExists, "").AutoStack(nil, nil)
	}
	atom := &AtomRemote{
		element:   e,
		info:      info.Info,
		state:     0,
		count:     0,
		callChain: nil,
	}
	e.lock.Lock()
	a, has = e.atoms[name]
	if has {
		a.info = atom.info
	} else {
		e.atoms[name] = atom
		a = atom
	}
	e.lock.Unlock()
	return a, nil
}

func (e *ElementRemote) sendMessage(fromID ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	// Send failed, how to handle? at least delete atom, and retry
	a, er := anypb.New(args)
	if er != nil {
		return reply, NewErrorf(ErrAtomMessageArgType, "marshal failed, err=(%v)", er)
	}
	if fromID != nil {
		if !e.checkCallChain(fromID.getCallChain()) {
			return reply, NewErrorf(ErrAtomCallDeadLock, "Call Dead Lock, chain=(%v),to(%s),name=(%s),args=(%v)",
				fromID.getCallChain(), e, name, args)
		}
		e.addCallChain(fromID.getCallChain())
		defer e.delCallChain()
	}
	req := &CosmosRemoteMessagingReq{
		From:      fromID.GetIDInfo(),
		FromAddr:  e.cosmos.main.getFromAddr(),
		To:        e.GetIDInfo(),
		CallChain: nil,
		Message:   name,
		Args:      a,
	}
	for _, id := range e.callChain {
		if id == nil {
			req.CallChain = append(req.CallChain, nil)
			continue
		}
		req.CallChain = append(req.CallChain, id.GetIDInfo())
	}
	rsp := &CosmosRemoteMessagingRsp{}
	if err = e.cosmos.httpPost(e.cosmos.addr+RemoteAtomosElementMessaging, req, rsp); err != nil {
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

func (e *ElementRemote) getScaleID(fromID ID, name string, args proto.Message) (id ID, err *ErrorInfo) {
	// Send failed, how to handle? at least delete atom, and retry
	a, er := anypb.New(args)
	if er != nil {
		return nil, NewErrorf(ErrAtomMessageArgType, "marshal failed, err=(%v)", er)
	}
	req := &CosmosRemoteScalingReq{
		From:     fromID.GetIDInfo(),
		FromAddr: e.cosmos.main.getFromAddr(),
		To:       e.GetIDInfo(),
		Message:  name,
		Args:     a,
	}
	rsp := &CosmosRemoteScalingRsp{}
	if err = e.cosmos.httpPost(e.cosmos.addr+RemoteAtomosElementScaling, req, rsp); err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	if rsp.Id != nil {
		e.lock.Lock()
		a, has := e.atoms[rsp.Id.Atomos]
		if !has {
			a = &AtomRemote{
				element: e,
				info: &AtomRemoteInfo{
					Id: rsp.Id,
				},
				state:     AtomosStateUnknown,
				count:     1,
				callChain: nil,
			}
		} else {
			a.count += 1
		}
		e.lock.Unlock()
		id = a
	}
	if rsp.Error != nil {
		err = rsp.Error
	}
	return id, err
}

func (e *ElementRemote) release(a *AtomRemote) {
	a.count -= 1
	// TODO: Remove
}

// Check chain.

func (e *ElementRemote) checkCallChain(fromIDList []ID) bool {
	for _, fromID := range fromIDList {
		if fromID.GetIDInfo().IsEqual(e.GetIDInfo()) {
			return false
		}
	}
	return true
}

func (e *ElementRemote) addCallChain(fromIDList []ID) {
	e.callChain = append(fromIDList, e)
}

func (e *ElementRemote) delCallChain() {
	e.callChain = nil
}
