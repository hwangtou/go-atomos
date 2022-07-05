package go_atomos

// CHECKED!

import (
	"container/list"
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
	cosmos *CosmosSelf

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
	atoms    map[string]*AtomCore
	names    *list.List
	upgrades int
}

// 本地Element创建，用于本地Cosmos的创建过程。
// Create of the Local Element, uses in Local Cosmos creation.
func newElementLocal(cosmosSelf *CosmosSelf, atomInitNum int) *ElementLocal {
	elem := &ElementLocal{
		lock:       sync.RWMutex{},
		avail:      false,
		cosmos:     cosmosSelf,
		current:    nil,
		upgrading:  nil,
		implements: map[uint64]*ElementImplementation{},
		atoms:      make(map[string]*AtomCore, atomInitNum),
		names:      list.New(),
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
		return wh.Load(e.cosmos.runtime.mainAtom, false)
	}
	return nil
}

func (e *ElementLocal) setUpgradeDefine(define *ElementImplementation) error {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.avail = false
	e.upgrading = define
	if wh, ok := define.Developer.(ElementLoadable); ok {
		return wh.Load(e.cosmos.runtime.mainAtom, false)
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
					e.cosmos.logFatal("Element.Reload: Panic, name=%s,reason=%s", name, r)
				}
			}()
			e.lock.Lock()
			atom, has := e.atoms[name]
			e.lock.Unlock()
			if !has {
				return
			}
			e.cosmos.logInfo("Element.Reload: Reloading atom, name=%s", name)
			err := atom.pushReloadMail(e.current, upgradeCount)
			if err != nil {
				e.cosmos.logError("Element.Reload: Push reload failed, name=%s,err=%v", name, err)
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
					e.cosmos.logFatal("Element.Unload: Panic, name=%s,reason=%s", name, r)
				}
			}()
			e.lock.Lock()
			atom, has := e.atoms[name]
			e.lock.Unlock()
			if !has {
				return
			}
			e.cosmos.logInfo("Element.Unload: Kill atom, name=%s", name)
			err := atom.pushKillMail(e.cosmos.runtime.mainAtom, true)
			if err != nil {
				e.cosmos.logError("Element.Unload: Kill atom error, name=%s,err=%v", name, err)
			}
		}(name)
	}
	wg.Wait()
}

// Local implementations of Element type.

func (e *ElementLocal) GetElementName() string {
	return e.current.Interface.Config.Name
}

func (e *ElementLocal) GetAtomId(name string) (ID, error) {
	atom, err := e.elementGetAtom(name)
	if err != nil {
		return nil, err
	}
	return atom.id, nil
}

func (e *ElementLocal) SpawnAtom(name string, arg proto.Message) (*AtomCore, error) {
	return e.elementCreateAtom(name, arg)
}

func (e *ElementLocal) MessagingAtom(fromId, toId ID, message string, args proto.Message) (reply proto.Message, err error) {
	if fromId == nil {
		return reply, ErrFromNotFound
	}
	a, ok := toId.(*AtomCore)
	if !ok || a == nil {
		return reply, ErrAtomNotFound
	}
	return a.pushMessageMail(fromId, message, args)
}

func (e *ElementLocal) KillAtom(fromId, toId ID) error {
	if fromId == nil {
		return ErrFromNotFound
	}
	a, ok := toId.(*AtomCore)
	if !ok || a == nil {
		return ErrAtomNotFound
	}
	return a.pushKillMail(fromId, true)
}

// Internal

func (e *ElementLocal) elementGetAtom(name string) (*AtomCore, error) {
	e.lock.RLock()
	current := e.current
	atom, hasAtom := e.atoms[name]
	e.lock.RUnlock()
	if hasAtom && atom.state > AtomHalt {
		atom.count += 1
		return atom, nil
	}
	if current.Developer.Persistence() == nil {
		return nil, ErrAtomNotFound
	}
	return e.elementCreateAtom(name, nil)
}

func (e *ElementLocal) elementCreateAtom(name string, arg proto.Message) (*AtomCore, error) {
	e.lock.RLock()
	current, upgrade := e.current, e.upgrades
	e.lock.RUnlock()
	// Alloc an atom and try setting.
	atom := allocAtom()
	initAtom(atom, e, name, current, upgrade)
	// If not exist, lock and set an new one.
	e.lock.Lock()
	oldAtom, has := e.atoms[name]
	if !has {
		e.atoms[name] = atom
		atom.nameElement = e.names.PushBack(name)
	}
	e.lock.Unlock()
	// If exists and running, release new and return error.
	// 不用担心两个Atom同时创建的问题，因为Atom创建的时候就是AtomSpawning了，除非其中一个在极端短的时间内AtomHalt了
	if has && oldAtom.state > AtomHalt {
		deallocAtom(atom)
		return nil, ErrAtomExists
	}
	// If exists and not running, release new and use old.
	if has {
		deallocAtom(atom)
		atom = oldAtom
		atom.count += 1
	}
	// Get data and Spawning.
	var data proto.Message
	if p := current.Developer.Persistence(); p != nil {
		d, err := p.GetAtomData(name)
		if err != nil && arg == nil {
			atom.setHalt()
			e.elementReleaseAtom(atom)
			return nil, err
		}
		data = d
	}
	if err := e.elementSpawningAtom(atom, current, arg, data); err != nil {
		e.elementReleaseAtom(atom)
		return nil, err
	}
	return atom, nil
}

func (e *ElementLocal) elementReleaseAtom(atom *AtomCore) {
	atom.count -= 1
	if atom.state > AtomHalt {
		return
	}
	if atom.count > 0 {
		return
	}
	e.lock.Lock()
	_, has := e.atoms[atom.name]
	if has {
		delete(e.atoms, atom.name)
	}
	if atom.nameElement != nil {
		e.names.Remove(atom.nameElement)
		atom.nameElement = nil
	}
	e.lock.Unlock()
	releaseAtom(atom)
	deallocAtom(atom)
}

func (e *ElementLocal) atomsNum() int {
	e.lock.RLock()
	num := len(e.atoms)
	e.lock.RUnlock()
	return num
}

func (e *ElementLocal) elementSpawningAtom(a *AtomCore, impl *ElementImplementation, arg, data proto.Message) error {
	var err error
	initMailBox(a)
	a.mailbox.start()
	if err = impl.Interface.AtomSpawner(a, a.instance, arg, data); err != nil {
		a.setHalt()
		delMailBox(a.mailbox)
		a.mailbox.stop()
		return err
	}
	a.setWaiting()
	return nil
}
