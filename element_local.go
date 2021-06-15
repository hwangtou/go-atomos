package go_atomos

import (
	"github.com/golang/protobuf/proto"
	"sync"
)

type ElementLocal struct {
	mutex  sync.RWMutex
	cosmos *CosmosSelf
	define *ElementDefine
	log    *MailBox
	atoms  map[string]*AtomCore
	loaded bool
}

func (x *ElementConfig) createElement(cosmosSelf *CosmosSelf, define *ElementDefine) (*ElementLocal, error) {
	elem := &ElementLocal{}
	elem.cosmos = cosmosSelf
	elem.define = define
	elem.log = NewMailBox(MailBoxHandler{
		OnReceive: elem.onLogMessage,
		OnPanic:   elem.onLogPanic,
		OnStop:    elem.onLogStop,
	})
	elem.atoms = make(map[string]*AtomCore, x.AtomInitNum)
	return elem, nil
}

func (e *ElementLocal) load() error {
	e.log.Start()
	return nil
}

func (e *ElementLocal) unload() error {
	e.log.Stop()
	return nil
}

func (e *ElementLocal) GetName() string {
	return e.define.Name
}

func (e *ElementLocal) GetAtomId(name string) (Id, error) {
	return e.define.AtomIdFactory(e.cosmos.local, name)
}

func (e *ElementLocal) SpawnAtom(atomName string, arg proto.Message) (*AtomCore, error) {
	inst := e.define.AtomCreator()
	// Alloc atom and try setting.
	a := allocAtom()
	initAtom(a, e, atomName, inst)
	e.mutex.Lock()
	if !e.loaded {
		e.mutex.Unlock()
		deallocAtom(a)
		return nil, ErrElementNotLoaded
	}
	if _, has := e.atoms[atomName]; has {
		e.mutex.Unlock()
		deallocAtom(a)
		return nil, ErrAtomExists
	}
	a.state = Spawning
	e.atoms[atomName] = a
	e.mutex.Unlock()
	// Try spawning.
	ac, err := e.spawningAtom(a, arg)
	if err != nil {
		e.mutex.Lock()
		delete(e.atoms, atomName)
		e.mutex.Unlock()
	}
	return ac, nil
}

// NOTICE: No concurrency of element.
func (e *ElementLocal) spawningAtom(a *AtomCore, arg proto.Message) (*AtomCore, error) {
	initMailBox(a)
	a.mailbox.Start()
	if err := a.instance.Spawn(a, arg); err != nil {
		a.state = Halt
		a.mailbox.Stop()
		DelMailBox(a.mailbox)
		return nil, err
	}
	a.state = Waiting
	return a, nil
}

// NOTICE: No concurrency of element.
// TODO: dead-lock loop checking
func (e *ElementLocal) CallAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
	if fromId == nil {
		return reply, ErrFromNotFound
	}
	a := toId.getLocalAtom()
	if a == nil {
		return reply, ErrAtomNotFound
	}
	return a.pushMessageMail(fromId, message, args)
}

func (e *ElementLocal) KillAtom(fromId, toId Id) error {
	if fromId == nil {
		return ErrFromNotFound
	}
	a := toId.getLocalAtom()
	if a == nil {
		return ErrAtomNotFound
	}
	return a.pushKillMail(fromId)
}

func (e *ElementLocal) getCall(name string) *ElementAtomCall {
	e.mutex.RLock()
	if !e.loaded {
		e.mutex.RUnlock()
		return nil
	}
	c, has := e.define.AtomCalls[name]
	e.mutex.RUnlock()
	if !has {
		return nil
	}
	return c
}

func (e *ElementLocal) getAtomId(name string) (Id, error) {
	e.mutex.RLock()
	if !e.loaded {
		e.mutex.RUnlock()
		return nil, ErrElementNotLoaded
	}
	a, has := e.atoms[name]
	e.mutex.RUnlock()
	if !has {
		return nil, ErrAtomNotFound
	}
	return a, nil
}
