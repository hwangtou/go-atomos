package go_atomos

// CHECKED!

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

	// 当前ElementImplementation的引用。
	// Reference to current in use ElementImplementation.
	current *ElementImplementation

	// 所有添加过的不同版本的ElementImplementation的容器。
	// Container of all added versions of ElementImplementation.
	implements map[uint64]*ElementImplementation

	// 该Element所有Atom的容器。
	// Container of all atoms.
	atoms map[string]*AtomCore

	// Is loaded.
	loaded bool
}

// 本地Element创建，用于本地Cosmos的创建过程。
// Create of the Local Element, uses in Local Cosmos creation.
func newElementLocal(cosmosSelf *CosmosSelf, define *ElementImplementation) *ElementLocal {
	elem := &ElementLocal{}
	elem.cosmos = cosmosSelf
	elem.current = define
	elem.implements = map[uint64]*ElementImplementation{
		define.Interface.Config.Version: define,
	}
	elem.atoms = make(map[string]*AtomCore, define.Interface.Config.AtomInitNum)
	return elem
}

// 重载Element，需要指定一个版本的ElementImplementation。
// Reload element, specific version of ElementImplementation is needed.
func (e *ElementLocal) reload(newDefine *ElementImplementation) error {
	e.current = newDefine
	e.implements[newDefine.Interface.Config.Version] = newDefine
	for name, atom := range e.atoms {
		err := atom.pushReloadMail(newDefine.Interface.Config.Version)
		if err != nil {
			e.cosmos.logError("Element.Reload: Push reload failed, name=%s,err=%v", name, err)
		}
	}
	return nil
}

// 加载
func (e *ElementLocal) load() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if err := e.current.Developer.Load(e.cosmos.local.mainAtom); err != nil {
		return err
	}
	e.loaded = true
	return nil
}

func (e *ElementLocal) unload() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.loaded = false
	wg := sync.WaitGroup{}
	for atomName, atom := range e.atoms {
		go func(atomName string, atom *AtomCore) {
			wg.Add(1)
			defer func() {
				wg.Done()
			}()
			defer func() {
				if r := recover(); r != nil {
					e.cosmos.logFatal("Element.Unload: Panic, name=%s,reason=%s", atomName, r)
				}
			}()
			e.cosmos.logInfo("Element.Unload: Kill atom, name=%s", atomName)
			err := atom.Kill(e.cosmos.local.mainAtom)
			if err != nil {
				e.cosmos.logError("Element.Unload: Kill atom error, name=%s,err=%v", atomName, err)
			}
		}(atomName, atom)
	}
	wg.Wait()

	//e.cosmos = nil
	//e.implements = nil
	return nil
}

// Local implementations of Element type.

func (e *ElementLocal) GetName() string {
	return e.current.Interface.Config.Name
}

func (e *ElementLocal) GetAtomId(name string) (Id, error) {
	id, err := e.getAtomId(name)
	if err == nil {
		return id, nil
	}
	if err != ErrAtomNotFound {
		return nil, err
	}
	// Try to recover.
	data, err := e.getAtomData(e.current, name)
	if err != nil {
		return nil, err
	}
	a, err := e.lockAtomName(name)
	if err != nil {
		return nil, err
	}
	// Try spawning.
	ac, err := e.spawningAtomMailbox(a, nil, data)
	if err != nil {
		e.mutex.Lock()
		delete(e.atoms, name)
		e.mutex.Unlock()
	}
	return ac, nil
}

func (e *ElementLocal) SpawnAtom(name string, arg proto.Message) (*AtomCore, error) {
	a, err := e.lockAtomName(name)
	if err != nil {
		return nil, err
	}
	// Try spawning.
	data, err := e.getAtomData(a.element.implements[a.version], name)
	if err != nil && err != ErrAtomNotFound {
		return nil, err
	}
	ac, err := e.spawningAtomMailbox(a, arg, data)
	if err != nil {
		e.mutex.Lock()
		delete(e.atoms, name)
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
	return a.pushKillMail(fromId, true)
}

// Internal

func (e *ElementLocal) lockAtomName(name string) (*AtomCore, error) {
	inst := e.current.Developer.AtomConstructor()
	// Alloc an atom and try setting.
	a := allocAtom()
	initAtom(a, e, name, inst)
	e.mutex.Lock()
	if !e.loaded {
		e.mutex.Unlock()
		deallocAtom(a)
		return nil, ErrElementNotLoaded
	}
	if _, has := e.atoms[name]; has {
		e.mutex.Unlock()
		deallocAtom(a)
		return nil, ErrAtomExists
	}
	a.state = AtomSpawning
	e.atoms[name] = a
	e.mutex.Unlock()
	return a, nil
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
	return e.implements[a.version].Interface.AtomIdConstructor(a), nil
}

func (e *ElementLocal) spawningAtomMailbox(a *AtomCore, arg, data proto.Message) (*AtomCore, error) {
	var err error
	initMailBox(a)
	a.mailbox.Start()
	impl := a.element.implements[a.version]
	if err = impl.Interface.AtomSpawner(a, a.instance, arg, data); err != nil {
		a.setHalt()
		DelMailBox(a.mailbox)
		a.mailbox.Stop()
		return nil, err
	}
	a.setWaiting()
	return a, nil
}

func (e *ElementLocal) getAtomData(impl *ElementImplementation, name string) (proto.Message, error) {
	p := impl.Developer.Persistence()
	if p == nil {
		return nil, ErrAtomNotFound
	}
	data, err := p.GetAtomData(name)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (e *ElementLocal) getMessageHandler(name string, version uint64) MessageHandler {
	e.mutex.RLock()
	if !e.loaded {
		e.mutex.RUnlock()
		return nil
	}
	c, has := e.implements[version].AtomHandlers[name]
	e.mutex.RUnlock()
	if !has {
		return nil
	}
	return c
}
