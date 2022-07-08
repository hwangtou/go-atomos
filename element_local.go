package go_atomos

// CHECKED!

import (
	"container/list"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
)

// ElementLocal
// 本地Element实现。
// Implementation of local Element.
type ElementLocal struct {
	// Lock.
	lock sync.RWMutex

	// Available or Upgrading
	avail bool

	// CosmosSelf引用。
	// Reference to CosmosSelf.
	cosmos *CosmosProcess

	atomos *baseAtomos

	// 当前ElementImplementation的引用。
	// Reference to current in use ElementImplementation.
	current, upgrading *ElementImplementation

	// 所有添加过的不同版本的ElementImplementation的容器。
	// Container of all added versions of ElementImplementation.
	implements map[uint64]*ElementImplementation

	// 该Element所有Atom的容器。
	// Container of all atoms.
	// 思考：要考虑在频繁变动的情景下，迭代不全的问题。
	// 两种情景：更新&关闭。
	atoms    map[string]*AtomLocal
	names    *list.List
	upgrades int

	log      *loggingMailBox
	logLevel LogLevel
}

func (e *ElementLocal) Release() {
}

func (e *ElementLocal) Cosmos() CosmosNode {
	return e.cosmos.runtime
}

func (e *ElementLocal) Element() Element {
	return e
}

func (e *ElementLocal) GetName() string {
	return e.current.Interface.Name
}

func (e *ElementLocal) GetVersion() uint64 {
	return e.current.Interface.Config.Version
}

func (e *ElementLocal) Kill(from ID) *ErrorInfo {
	return NewError(ErrElementCannotKill, "Cannot kill element")
}

func (e *ElementLocal) String() string {
	return e.atomos.id.str()
}

func (e *ElementLocal) atomosHalt(a *baseAtomos) {
}

// 本地Element创建，用于本地Cosmos的创建过程。
// Create of the Local Element, uses in Local Cosmos creation.
func newElementLocal(cosmosSelf *CosmosSelf, define *ElementImplementation) *ElementLocal {
	id := &IDInfo{
		Type:    IDType_Element,
		Cosmos:  cosmosSelf.config.Node,
		Element: define.Interface.Name,
		Atomos:  "",
	}
	atomos := allocBaseAtomos()
	initBaseAtomos(atomos, id, cosmosSelf.log, define.Interface.Config.LogLevel, cosmosSelf, define.Developer.AtomConstructor())
	elem := &ElementLocal{
		lock:       sync.RWMutex{},
		avail:      false,
		cosmos:     cosmosSelf,
		atomos:     atomos,
		current:    nil,
		upgrading:  nil,
		implements: map[uint64]*ElementImplementation{},
		names:      list.New(),
	}
	if atomsInitNum, ok := define.Developer.(ElementCustomizeAtomsInitNum); ok {
		elem.atoms = make(map[string]*AtomLocal, atomsInitNum.GetElementAtomsInitNum())
	} else {
		elem.atoms = map[string]*AtomLocal{}
	}
	return elem
}

// 加载
func (e *ElementLocal) setInitDefine(define *ElementImplementation) error {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.avail = false
	e.current = define
	e.upgrading = nil
	e.implements[define.Interface.Config.Version] = define
	if wh, ok := define.Developer.(ElementLoadable); ok {
		return wh.Load(e.atomos, false)
	}
	return nil
}

func (e *ElementLocal) setUpgradeDefine(define *ElementImplementation) error {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.avail = false
	e.upgrading = define
	if wh, ok := define.Developer.(ElementLoadable); ok {
		return wh.Load(e.atomos, false)
	}
	return nil
}

func (e *ElementLocal) rollback(isUpgrade, loadFailed bool) {
	e.lock.Lock()
	defer e.lock.Unlock()
	if !loadFailed {
		var dev ElementDeveloper
		if e.upgrading != nil {
			dev = e.upgrading.Developer
		} else {
			dev = e.current.Developer
		}
		if wh, ok := dev.(ElementLoadable); ok {
			wh.Unload()
		}
	}
	if isUpgrade {
		e.avail = true
		e.upgrading = nil
	}
}

// For upgrading only.
func (e *ElementLocal) commit(isUpgrade bool) {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.avail = true
	if isUpgrade {
		e.current = e.upgrading
		e.upgrading = nil
		if _, has := e.implements[e.current.Interface.Config.Version]; !has {
			e.implements[e.current.Interface.Config.Version] = e.current
		}
	}
}

// 重载Element，需要指定一个版本的ElementImplementation。
// Reload element, specific version of ElementImplementation is needed.
func (e *ElementLocal) pushUpgrade(upgradeCount int) {
	e.lock.Lock()
	atomNameList := make([]string, 0, e.names.Len())
	for nameElem := e.names.Front(); nameElem != nil; nameElem = nameElem.Next() {
		atomNameList = append(atomNameList, nameElem.Value.(string))
	}
	e.upgrades = upgradeCount
	e.lock.Unlock()
	wg := sync.WaitGroup{}
	for _, name := range atomNameList {
		wg.Add(1)
		go func(name string) {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					e.atomos.log.Fatal("Element.Reload: Panic, name=%s,reason=%s", name, r)
				}
			}()
			e.lock.Lock()
			atom, has := e.atoms[name]
			e.lock.Unlock()
			if !has {
				return
			}
			e.atomos.log.Info("Element.Reload: Reloading atomos, name=%s", name)
			err := atom.pushReloadMail(e.current, upgradeCount)
			if err != nil {
				e.atomos.log.Error("Element.Reload: Push reload failed, name=%s,err=%v", name, err)
			}
		}(name)
	}
	wg.Wait()
}

func (e *ElementLocal) unload() {
	e.lock.Lock()
	atomNameList := make([]string, 0, e.names.Len())
	for nameElem := e.names.Front(); nameElem != nil; nameElem = nameElem.Next() {
		atomNameList = append(atomNameList, nameElem.Value.(string))
	}
	e.lock.Unlock()
	wg := sync.WaitGroup{}
	for _, name := range atomNameList {
		wg.Add(1)
		go func(name string) {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					e.atomos.log.Fatal("Element.Unload: Panic, name=%s,reason=%s", name, r)
				}
			}()
			e.lock.Lock()
			atom, has := e.atoms[name]
			e.lock.Unlock()
			if !has {
				return
			}
			e.atomos.log.Info("Element.Unload: Kill atomos, name=%s", name)
			err := atom.pushKillMail(e, true)
			if err != nil {
				e.atomos.log.Error("Element.Unload: Kill atomos error, name=%s,err=%v", name, err)
			}
		}(name)
	}
	wg.Wait()
}

// Local implementations of Element type.

func (e *ElementLocal) GetElementName() string {
	return e.current.Interface.Config.Name
}

func (e *ElementLocal) GetAtomId(name string) (ID, *ErrorInfo) {
	atom, err := e.elementGetAtom(name)
	if err != nil {
		return nil, err
	}
	return atom.id, nil
}

func (e *ElementLocal) SpawnAtom(name string, arg proto.Message) (*AtomLocal, *ErrorInfo) {
	return e.elementCreateAtom(name, arg)
}

func (e *ElementLocal) MessagingAtom(fromId, toId ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	if fromId == nil {
		return reply, NewError(ErrAtomFromIDInvalid, fmt.Sprintf("From ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)", fromId, toId, name, args))
	}
	a, ok := toId.(*AtomLocal)
	if !ok || a == nil {
		return reply, NewError(ErrAtomToIDInvalid, fmt.Sprintf("To ID invalid, from=(%s),to=(%s),name=(%s),args=(%v)", fromId, toId, name, args))
	}
	return a.pushMessageMail(fromId, name, args)
}

func (e *ElementLocal) KillAtom(fromId, toId ID) *ErrorInfo {
	if fromId == nil {
		return NewError(ErrAtomFromIDInvalid, fmt.Sprintf("From ID invalid, from=(%s),to=(%s)", fromId, toId))
	}
	a, ok := toId.(*AtomLocal)
	if !ok || a == nil {
		return NewError(ErrAtomToIDInvalid, fmt.Sprintf("To ID invalid, from=(%s),to=(%s)", fromId, toId))
	}
	return a.pushKillMail(fromId, true)
}

// Internal

func (e *ElementLocal) elementGetAtom(name string) (*AtomLocal, *ErrorInfo) {
	e.lock.RLock()
	current := e.current
	atom, hasAtom := e.atoms[name]
	e.lock.RUnlock()
	if hasAtom && atom.atomos.state > AtomosHalt {
		atom.atomos.refCount += 1
		return atom, nil
	}
	persistence, ok := current.Developer.(ElementCustomizeAutoDataPersistence)
	if !ok || persistence == nil {
		return nil, NewError(ErrAtomNotExists, fmt.Sprintf("Atom not exists, name=(%s)", name))
	}
	return e.elementCreateAtom(name, nil)
}

func (e *ElementLocal) elementCreateAtom(name string, arg proto.Message) (*AtomLocal, *ErrorInfo) {
	e.lock.RLock()
	//current, upgrade := e.current, e.upgrades
	current := e.current
	e.lock.RUnlock()
	// Alloc an atomos and try setting.
	atom := allocAtomLocal()
	initAtomLocal(name, atom, e, current, e.log, e.logLevel)
	// If not exist, lock and set an new one.
	e.lock.Lock()
	oldAtom, has := e.atoms[name]
	if !has {
		e.atoms[name] = atom
		atom.atomos.nameElement = e.names.PushBack(name)
	}
	e.lock.Unlock()
	// If exists and running, release new and return error.
	// 不用担心两个Atom同时创建的问题，因为Atom创建的时候就是AtomSpawning了，除非其中一个在极端短的时间内AtomHalt了
	if has && oldAtom.atomos.state > AtomosHalt {
		deallocAtomLocal(atom)
		return nil, NewError(ErrAtomExists, fmt.Sprintf("Atom exists, name=(%s),arg=(%v)", name, arg))
	}
	// If exists and not running, release new and use old.
	if has {
		deallocAtomLocal(atom)
		atom = oldAtom
		atom.atomos.refCount += 1
	}
	// Get data and Spawning.
	var data proto.Message
	if p, ok := current.Developer.(ElementCustomizeAutoDataPersistence); ok && p != nil {
		d, err := p.Persistence().GetAtomData(name)
		if err != nil {
			atom.atomos.setHalt()
			e.atomosRelease(atom.atomos)
			return nil, err
		}
		if d == nil && arg == nil {
			atom.atomos.setHalt()
			e.atomosRelease(atom.atomos)
			return nil, NewError(ErrAtomSpawnArgInvalid, fmt.Sprintf("Spawn atom without arg, name=(%s)", name))
		}
		data = d
	}
	if err := e.elementSpawningAtom(atom, current, arg, data); err != nil {
		e.atomosRelease(atom.atomos)
		return nil, err
	}
	return atom, nil
}

func (e *ElementLocal) atomosRelease(atom *baseAtomos) {
	atom.refCount -= 1
	if atom.state > AtomosHalt {
		return
	}
	if atom.refCount > 0 {
		return
	}
	e.lock.Lock()
	a, has := e.atoms[atom.id.Atomos]
	if has {
		delete(e.atoms, atom.id.Atomos)
	}
	if atom.nameElement != nil {
		e.names.Remove(atom.nameElement)
		atom.nameElement = nil
	}
	e.lock.Unlock()
	releaseAtomLocal(a)
	deallocAtomLocal(a)
}

func (e *ElementLocal) atomsNum() int {
	e.lock.RLock()
	num := len(e.atoms)
	e.lock.RUnlock()
	return num
}

func (e *ElementLocal) elementSpawningAtom(a *AtomLocal, impl *ElementImplementation, arg, data proto.Message) *ErrorInfo {
	//initMailBox(a.atomos)
	a.atomos.mailbox.start()
	if err := impl.Interface.AtomSpawner(a, a.atomos.instance, arg, data); err != nil {
		a.atomos.setHalt()
		//delMailBox(a.mailbox)
		a.atomos.mailbox.stop()
		return err
	}
	a.atomos.setWaiting()
	return nil
}
