package atomos

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
)

type Messenger[E ID, A ID, AT Atomos, IN, OUT proto.Message] struct {
	ElementID E
	AtomID    A

	Name string
}

type MessengerType interface {
	Decoder() *IOMessageDecoder
}

func (m Messenger[E, A, AT, IN, OUT]) Decoder(i IN, o OUT) *IOMessageDecoder {
	return &IOMessageDecoder{
		InDec: func(buf []byte, isProtoOrJson bool) (proto.Message, *Error) {
			ci := proto.Clone(i)
			if isProtoOrJson {
				if err := proto.Unmarshal(buf, ci); err != nil {
					return nil, NewErrorf(ErrAtomMessageArgType, "Argument unmarshal failed, err=(%v)", err).AddStack(nil)
				}
			} else {
				if err := json.Unmarshal(buf, ci); err != nil {
					return nil, NewErrorf(ErrAtomMessageArgType, "Argument unmarshal failed, err=(%v)", err).AddStack(nil)
				}
			}
			return ci, nil
		},
		OutDec: func(buf []byte, isProtoOrJson bool) (proto.Message, *Error) {
			ci := proto.Clone(i)
			if isProtoOrJson {
				if err := proto.Unmarshal(buf, ci); err != nil {
					return nil, NewErrorf(ErrAtomMessageArgType, "Argument unmarshal failed, err=(%v)", err).AddStack(nil)
				}
			} else {
				if err := json.Unmarshal(buf, ci); err != nil {
					return nil, NewErrorf(ErrAtomMessageArgType, "Argument unmarshal failed, err=(%v)", err).AddStack(nil)
				}
			}
			return ci, nil
		},
	}
}

func (m Messenger[E, A, AT, IN, OUT]) SyncElement(e E, callerID SelfID, in IN, ext ...ArgsForBaseAtomos) (OUT, *Error) {
	// Check arguments.
	var nilID OUT
	if callerID == nil {
		return nilID, NewErrorf(ErrAtomFromIDInvalid, "Messenger: callerID is nil").AddStack(nil)
	}
	m.ElementID = e

	return m.handleReply(m.ElementID.SyncMessagingByName(callerID, m.Name, in, ext))
}

func (m Messenger[E, A, AT, IN, OUT]) AsyncElement(e E, callerID SelfID, in IN, callback func(OUT, *Error), ext ...ArgsForBaseAtomos) *Error {
	// Check arguments.
	if callerID == nil {
		var nilID OUT
		callback(nilID, NewErrorf(ErrAtomFromIDInvalid, "Messenger: callerID is nil").AddStack(nil))
	}
	m.ElementID = e

	if callback == nil {
		return m.ElementID.AsyncMessagingByName(callerID, m.Name, in, nil, ext)
	} else {
		return m.ElementID.AsyncMessagingByName(callerID, m.Name, in, func(message proto.Message, err *Error) {
			callback(m.handleReply(message, err))
		}, ext)
	}
}

func (m Messenger[E, A, AT, IN, OUT]) SyncAtom(a A, callerID SelfID, in IN, ext ...ArgsForBaseAtomos) (OUT, *Error) {
	// Check arguments.
	var nilID OUT
	if callerID == nil {
		return nilID, NewErrorf(ErrAtomFromIDInvalid, "Messenger: callerID is nil").AddStack(nil)
	}
	m.AtomID = a

	return m.handleReply(m.AtomID.SyncMessagingByName(callerID, m.Name, in, ext))
}

func (m Messenger[E, A, AT, IN, OUT]) AsyncAtom(a A, callerID SelfID, in IN, callback func(OUT, *Error), ext ...ArgsForBaseAtomos) *Error {
	// Check arguments.
	if callerID == nil {
		var nilID OUT
		callback(nilID, NewErrorf(ErrAtomFromIDInvalid, "Messenger: callerID is nil").AddStack(nil))
	}
	m.AtomID = a

	if callback == nil {
		return m.AtomID.AsyncMessagingByName(callerID, m.Name, in, nil, ext)
	} else {
		return m.AtomID.AsyncMessagingByName(callerID, m.Name, in, func(message proto.Message, err *Error) {
			callback(m.handleReply(message, err.AddStack(nil)))
		}, ext)
	}
}

func (m Messenger[E, A, AT, IN, OUT]) ExecuteAtom(to Atomos, in proto.Message) (AT, IN, *Error) {
	i, ok := in.(IN)
	if !ok {
		var nilAT AT
		var nilIN IN
		return nilAT, nilIN, NewErrorf(ErrAtomMessageArgType, "Arg type=(%T)", in).AddStack(nil)
	}
	a, ok := to.(AT)
	if !ok {
		var nilAT AT
		var nilIN IN
		return nilAT, nilIN, NewErrorf(ErrAtomMessageAtomType, "Atom type=(%T)", to).AddStack(nil)
	}
	return a, i, nil
}

func (m Messenger[E, A, AT, IN, OUT]) handleReply(rsp proto.Message, err *Error) (OUT, *Error) {
	if rsp == nil {
		var nilID OUT
		return nilID, err.AddStack(nil)
	}
	reply, ok := rsp.(OUT)
	if !ok {
		var nilID OUT
		return nilID, NewErrorf(ErrAtomMessageReplyType, "Reply type invalid. name=(%s),type=(%T)", m.Name, rsp).AddStack(nil)
	}
	return reply, err.AddStack(nil)
}
