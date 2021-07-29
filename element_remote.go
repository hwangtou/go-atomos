package go_atomos

import (
	"errors"
	"google.golang.org/protobuf/proto"
	"sync"
	"time"
)

// TODO: 远程Cosmos管理助手，在未来版本实现。
// TODO: Remote Cosmos Helper.

type ElementRemote struct {
	sync.RWMutex
	*ElementInterface
	cosmos   *cosmosWatchRemote
	name     string
	version  uint64
	cachedId map[string]*atomIdRemote
}

func (e *ElementRemote) GetName() string {
	return e.name
}

func (e *ElementRemote) GetAtomId(name string) (Id, error) {
	return e.ElementInterface.AtomIdConstructor(e.cosmos, name)
}

func (e *ElementRemote) getAtomId(name string) (Id, error) {
	e.RLock()
	id, has := e.cachedId[name]
	e.RUnlock()
	if !has {
		req := &CosmosRemoteGetAtomIdReq{
			Element: e.name,
			Name:    name,
		}
		resp := &CosmosRemoteGetAtomIdResp{}
		reqBuf, err := proto.Marshal(req)
		if err != nil {
			// TODO
			return nil, err
		}
		respBuf, err := e.cosmos.request("atom_id", reqBuf)
		err = proto.Unmarshal(respBuf, resp)
		if err != nil {
			// TODO
			return nil, err
		}
		if !resp.Has {
			return nil, ErrAtomNotFound
		}
		id = &atomIdRemote{
			cosmosNode: e.cosmos,
			element:    e,
			name:       name,
			version:    e.version,
		}
		e.Lock()
		e.cachedId[name] = id
		e.Unlock()
	}
	return id, nil
}

func (e *ElementRemote) SpawnAtom(_ string, _ proto.Message) (*AtomCore, error) {
	return nil, ErrAtomCannotSpawn
}

func (e *ElementRemote) MessagingAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
	// TODO: Check remote is nil
	req := &CosmosRemoteMessagingReq{
		From: &AtomId{
			Node:    fromId.Cosmos().GetNodeName(),
			Element: "element", // TODO fromId.Element().GetName(),
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
		// TODO
		return nil, err
	}
	respBuf, err := e.cosmos.request("atom_msg", reqBuf)
	err = proto.Unmarshal(respBuf, resp)
	if err != nil {
		// TODO
		return nil, err
	}
	if resp.Error != "" {
		err = errors.New(resp.Error)
	}
	reply, _ = resp.Reply.UnmarshalNew()
	return reply, err
}

func (e *ElementRemote) KillAtom(_, _ Id) error {
	// TODO
	return ErrAtomCannotKill
}

// Id

type atomIdRemote struct {
	cosmosNode *cosmosWatchRemote
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
