package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"sync"
)

// 本地Element实现。
// Implementation of local Element.
type ElementLocal struct {
	// Lock.
	mutex sync.RWMutex

	// CosmosSelf引用。
	// Reference to CosmosSelf.
	cosmos *CosmosSelf

	// 当前ElementDefine的引用。
	// Reference to current in use ElementDefine.
	current *ElementDefine

	// 所有添加过的不同版本的ElementDefine的容器。
	// Container of all added versions of ElementDefine.
	define map[uint64]*ElementDefine

	// 该Element所有Atom的容器。
	// Container of all atoms.
	atoms map[string]*AtomCore

	// Is loaded.
	loaded bool
}

// 本地Element创建，用于本地Cosmos的创建过程。
// Create of the Local Element, uses in Local Cosmos creation.
func newElementLocal(cosmosSelf *CosmosSelf, define *ElementDefine) (*ElementLocal, error) {
	elem := &ElementLocal{}
	elem.cosmos = cosmosSelf
	elem.current = define
	elem.define = map[uint64]*ElementDefine{
		define.Config.Version: define,
	}
	elem.atoms = make(map[string]*AtomCore, define.Config.AtomInitNum)
	return elem, nil
}

// 重载Element，需要指定一个版本的ElementDefine。
// Reload element, specific version of ElementDefine is needed.
func (e *ElementLocal) reload(newDefine *ElementDefine) error {
	e.current = newDefine
	e.define[newDefine.Config.Version] = newDefine
	for _, atom := range e.atoms {
		err := atom.pushReloadMail(newDefine.Config.Version)
		if err != nil {
			// TODO
		}
	}
	return nil
}

// 加载
func (e *ElementLocal) load() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.loaded = true
	return nil
}

func (e *ElementLocal) unload() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.loaded = false
	for atomName, atom := range e.atoms {
		if atom.getState() != AtomHalt {
			// todo: Exit and store and tell user.
		}
		delete(e.atoms, atomName)
	}
	e.cosmos = nil
	e.define = nil
	return nil
}

// Local implementation of Element type.

func (e *ElementLocal) GetName() string {
	return e.current.Config.Name
}

func (e *ElementLocal) GetAtomId(name string) (Id, error) {
	return e.current.AtomIdConstructor(e.cosmos.local, name)
}

func (e *ElementLocal) SpawnAtom(atomName string, arg proto.Message) (*AtomCore, error) {
	inst := e.current.AtomConstructor()
	// Alloc an atom and try setting.
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
	a.state = AtomSpawning
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

func (e *ElementLocal) MessagingAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
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

// Internal

// NOTICE: No concurrency of element.
func (e *ElementLocal) spawningAtom(a *AtomCore, arg proto.Message) (*AtomCore, error) {
	initMailBox(a)
	a.mailbox.Start()
	if err := a.instance.Spawn(a, arg); err != nil {
		a.state = AtomHalt
		a.mailbox.Stop()
		DelMailBox(a.mailbox)
		return nil, err
	}
	a.state = AtomWaiting
	return a, nil
}

func (e *ElementLocal) getCall(name string, version uint64) *ElementAtomMessage {
	e.mutex.RLock()
	if !e.loaded {
		e.mutex.RUnlock()
		return nil
	}
	c, has := e.define[version].AtomCalls[name]
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
