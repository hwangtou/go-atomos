package go_atomos

// CHECKED!

import (
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// 远程Element实现。
// Implementation of remote Element.
type ElementRemote struct {
	// Lock.
	sync.RWMutex

	// CosmosRemote引用。
	// Reference to cosmosRemote.
	cosmos *cosmosRemote

	// 当前ElementInterface的引用。
	// Reference to current in use ElementInterface.
	elemInter *ElementInterface

	// 该Element所有Id的缓存容器。
	// Container of all cached Id.
	cachedId map[string]Id
}

func newPrivateElementInterface(name string) *ElementInterface {
	return &ElementInterface{
		Config: &ElementConfig{
			Name:        name,
			Version:     0,
			LogLevel:    0,
			AtomInitNum: 0,
			Messages:    nil,
		},
		AtomMessages: map[string]*ElementAtomMessage{},
	}
}

// Remote implementations of Element type.

func (e *ElementRemote) GetName() string {
	return e.elemInter.Config.Name
}

func (e *ElementRemote) GetAtomId(name string) (Id, error) {
	if !e.enableRemote() {
		return nil, ErrRemoteNotAllowed
	}
	e.RLock()
	id, has := e.cachedId[name]
	e.RUnlock()
	if !has {
		req := &CosmosRemoteGetAtomIdReq{
			Element: e.elemInter.Config.Name,
			Name:    name,
		}
		resp := &CosmosRemoteGetAtomIdResp{}
		reqBuf, err := proto.Marshal(req)
		if err != nil {
			e.cosmos.helper.self.logFatal("Element.Remote: GetAtomId Protobuf marshal error, req=%+v,err=%v",
				req, err)
			return nil, err
		}
		respBuf, err := e.cosmos.request(RemoteUriAtomId, reqBuf)
		if err != nil {
			e.cosmos.helper.self.logFatal("Element.Remote: GetAtomId Request error, req=%+v,err=%v",
				req, err)
			return nil, err
		}
		if err = proto.Unmarshal(respBuf, resp); err != nil {
			e.cosmos.helper.self.logFatal("Element.Remote: GetAtomId Protobuf unmarshal error, req=%+v,err=%v",
				req, err)
			return nil, err
		}
		if !resp.Has {
			return nil, ErrAtomNotFound
		}
		id = e.getOrCreateAtomId(name)
	}
	return id, nil
}

func (e *ElementRemote) SpawnAtom(_ string, _ proto.Message) (*AtomCore, error) {
	return nil, ErrAtomCannotSpawn
}

func (e *ElementRemote) MessagingAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
	if fromId == nil {
		return nil, ErrFromNotFound
	}
	if toId == nil {
		return nil, ErrAtomNotFound
	}
	if !e.enableRemote() {
		return nil, ErrRemoteNotAllowed
	}
	req := &CosmosRemoteMessagingReq{
		From: &AtomId{
			Node:    fromId.Cosmos().GetNodeName(),
			Element: fromId.Element().GetName(),
			Name:    fromId.Name(),
		},
		To: &AtomId{
			Node:    toId.Cosmos().GetNodeName(),
			Element: toId.Element().GetName(),
			Name:    toId.Name(),
		},
		Message: message,
		Args:    MessageToAny(args),
	}
	resp := &CosmosRemoteMessagingResp{}
	reqBuf, err := proto.Marshal(req)
	if err != nil {
		e.cosmos.helper.self.logFatal("Element.Remote: MessagingAtom Protobuf marshal error, req=%+v,err=%v",
			req, err)
		return nil, err
	}
	respBuf, err := e.cosmos.request(RemoteUriAtomMessage, reqBuf)
	if err != nil {
		e.cosmos.helper.self.logFatal("Element.Remote: MessagingAtom Request error, req=%+v,err=%v",
			req, err)
	}
	if err = proto.Unmarshal(respBuf, resp); err != nil {
		e.cosmos.helper.self.logFatal("Element.Remote: MessagingAtom Protobuf unmarshal error, req=%+v,err=%v",
			req, err)
		return nil, err
	}
	if resp.Error != "" {
		err = errors.New(resp.Error)
	}
	reply, _ = resp.Reply.UnmarshalNew()
	return reply, err
}

func (e *ElementRemote) KillAtom(_, _ Id) error {
	return ErrAtomCannotKill
}

func (e *ElementRemote) enableRemote() bool {
	return e.cosmos.enableRemote != nil
}

func (e *ElementRemote) getOrCreateAtomId(name string) Id {
	e.Lock()
	defer e.Unlock()
	id, has := e.cachedId[name]
	if has {
		return id
	}
	id = &atomIdRemote{
		cosmosNode: e.cosmos,
		element:    e,
		name:       name,
		version:    e.elemInter.Config.Version,
		created:    time.Now(),
	}
	if e.elemInter != nil {
		id = e.elemInter.AtomIdConstructor(id)
	}
	e.cachedId[name] = id
	return id
}

// Remote implementations of Id type.

type atomIdRemote struct {
	cosmosNode *cosmosRemote
	element    *ElementRemote
	name       string
	version    uint64
	created    time.Time
}

func (a *atomIdRemote) Cosmos() CosmosNode {
	return a.cosmosNode
}

func (a *atomIdRemote) Element() Element {
	return a.element
}

func (a *atomIdRemote) Name() string {
	return a.name
}

func (a *atomIdRemote) Version() uint64 {
	return a.version
}

func (a *atomIdRemote) Kill(from Id) error {
	return ErrAtomCannotKill
}

func (a *atomIdRemote) getLocalAtom() *AtomCore {
	return nil
}
