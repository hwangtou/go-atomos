package go_atomos

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"reflect"
	"time"
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

func (m Messenger[E, A, AT, IN, OUT]) GetScaleID(e E, callerID SelfID, elemName string, in IN, ext ...interface{}) (ID, *IDTracker, *Error) {
	// Check arguments.
	if callerID == nil {
		return nil, nil, NewErrorf(ErrAtomFromIDInvalid, "Messenger: callerID is nil").AddStack(nil)
	}
	m.ElementID = e
	timeout := m.handleExt(ext...)

	return m.ElementID.Cosmos().CosmosGetScaleAtomID(callerID, elemName, m.Name, timeout, in)
}

func (m Messenger[E, A, AT, IN, OUT]) SyncElement(e E, callerID SelfID, in IN, ext ...interface{}) (OUT, *Error) {
	// Check arguments.
	var nilID OUT
	if callerID == nil {
		return nilID, NewErrorf(ErrAtomFromIDInvalid, "Messenger: callerID is nil").AddStack(nil)
	}
	m.ElementID = e
	if m.isElementNil() {
		return nilID, NewErrorf(ErrAtomNotExists, "Messenger: elementID is nil").AddStack(nil)
	}
	timeout := m.handleExt(ext...)

	return m.handleReply(m.ElementID.SyncMessagingByName(callerID, m.Name, timeout, in))
}

func (m Messenger[E, A, AT, IN, OUT]) AsyncElement(e E, callerID SelfID, in IN, callback func(OUT, *Error), ext ...interface{}) {
	// Check arguments.
	if callerID == nil {
		var nilID OUT
		if callback != nil {
			callback(nilID, NewErrorf(ErrAtomFromIDInvalid, "Messenger: callerID is nil").AddStack(nil))
		} else {
			callerID.Log().Fatal("Messenger: callerID is nil.")
		}
		return
	}
	// If element is nil, it will be handled by the Atomos.
	m.ElementID = e
	if m.isElementNil() {
		var nilID OUT
		if callback != nil {
			callback(nilID, NewErrorf(ErrAtomNotExists, "Messenger: elementID is nil").AddStack(nil))
		} else {
			callerID.Log().Fatal("Messenger: elementID is nil.")
		}
		return
	}
	timeout := m.handleExt(ext...)

	if callback != nil {
		m.ElementID.AsyncMessagingByName(callerID, m.Name, timeout, in, func(message proto.Message, err *Error) {
			callback(m.handleReply(message, err))
		})
	} else {
		m.ElementID.AsyncMessagingByName(callerID, m.Name, timeout, in, nil)
	}
}

func (m Messenger[E, A, AT, IN, OUT]) SyncAtom(a A, callerID SelfID, in IN, ext ...interface{}) (OUT, *Error) {
	// Check arguments.
	var nilID OUT
	if callerID == nil {
		return nilID, NewErrorf(ErrAtomFromIDInvalid, "Messenger: callerID is nil").AddStack(nil)
	}
	m.AtomID = a
	if m.isAtomNil() {
		return nilID, NewErrorf(ErrAtomNotExists, "Messenger: atomID is nil").AddStack(nil)
	}
	timeout := m.handleExt(ext...)

	return m.handleReply(m.AtomID.SyncMessagingByName(callerID, m.Name, timeout, in))
}

func (m Messenger[E, A, AT, IN, OUT]) AsyncAtom(a A, callerID SelfID, in IN, callback func(OUT, *Error), ext ...interface{}) {
	// Check arguments.
	if callerID == nil {
		var nilID OUT
		if callback != nil {
			callback(nilID, NewErrorf(ErrAtomFromIDInvalid, "Messenger: callerID is nil").AddStack(nil))
		} else {
			callerID.Log().Fatal("Messenger: callerID is nil.")
		}
		return
	}
	m.AtomID = a
	if m.isAtomNil() {
		var nilID OUT
		if callback != nil {
			callback(nilID, NewErrorf(ErrAtomNotExists, "Messenger: atomID is nil").AddStack(nil))
		} else {
			callerID.Log().Fatal("Messenger: atomID is nil.")
		}
		return
	}
	timeout := m.handleExt(ext...)

	if callback != nil {
		m.AtomID.AsyncMessagingByName(callerID, m.Name, timeout, in, func(message proto.Message, err *Error) {
			callback(m.handleReply(message, err.AddStack(nil)))
		})
	} else {
		m.AtomID.AsyncMessagingByName(callerID, m.Name, timeout, in, nil)
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

func (m Messenger[E, A, AT, IN, OUT]) ExecuteScale(to Atomos, in proto.Message) (AT, IN, *Error) {
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

// Handle extended arguments.
func (m Messenger[E, A, AT, IN, OUT]) handleExt(ext ...interface{}) (timeout time.Duration) {
	for _, e := range ext {
		switch arg := e.(type) {
		case time.Duration:
			timeout = arg
		}
	}
	return
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

func (m Messenger[E, A, AT, IN, OUT]) isElementNil() bool {
	return reflect.ValueOf(m.ElementID).IsNil()
}

func (m Messenger[E, A, AT, IN, OUT]) isAtomNil() bool {
	return reflect.ValueOf(m.AtomID).IsNil()
}
