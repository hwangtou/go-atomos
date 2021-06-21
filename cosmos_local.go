package go_atomos

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"sync"
)

const (
	logCosmosLocalInitLoadElementNotImplemented   = "atomos.CosmosLocal.init: CosmosLocal init load element not implemented, name=%s"
	logCosmosLocalInitLoadElementImplementInvalid = "atomos.CosmosLocal.init: CosmosLocal init load element implement invalid, name=%s"
	logCosmosLocalInitCreateElementFailed         = "atomos.CosmosLocal.init: CosmosLocal create element failed, name=%s,err=%s"
	logCosmosLocalInitLoadElementFailed           = "atomos.CosmosLocal.init: CosmosLocal load element failed, name=%s,err=%s"
	logCosmosLocalInitLoadElementInfo             = "atomos.CosmosLocal.init: CosmosLocal load element, name=%s"
	logCosmosLocalInitLoadElementUnloadInfo       = "atomos.CosmosLocal.init: CosmosLocal load element unload, name=%s,err=%v"
	logCosmosLocalUnloadElementInfo               = "atomos.CosmosLocal.init: CosmosLocal unload element, name=%s,err=%v"
	logCosmosLocalInitHasInitialized              = "atomos.CosmosLocal.init: CosmosLocal has been initialized"
	logCosmosLocalGetElementNotFound              = "atomos.CosmosLocal.getElement: Element not found, name=%s"
	logCosmosLocalAddElementNotFound              = "atomos.CosmosLocal.addElement: Element exists, name=%s"
	logCosmosLocalAddElementLoadFailed            = "atomos.CosmosLocal.addElement: Element load failed, name=%s,err=%v"
	logCosmosLocalDelElementNotFound              = "atomos.CosmosLocal.delElement: Element not found, name=%s"
	logCosmosLocalDelElementLoadFailed            = "atomos.CosmosLocal.delElement: Element unload failed, name=%s,err=%v"
)

type CosmosLocal struct {
	mutex        sync.RWMutex
	config       *Config
	cosmos       *CosmosSelf
	mainId       *idMain
	elements     map[string]*ElementLocal
	killNoticeCh chan bool
}

// Life cycle

func newCosmosLocal() *CosmosLocal {
	return &CosmosLocal{}
}

func (c *CosmosLocal) exitRunnable() {
	c.cosmos.Info("daemon exit runnable")
	// todo: remove all actors of elements, then remove all element. Save actor data and warning if actor still run.
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.cosmos.cluster.close()
	c.close()
}

func (c *CosmosLocal) close() {
	c.config = nil
	c.cosmos = nil
	c.mainId = nil
	for elemName, elem := range c.elements {
		if err := elem.unload(); err != nil {
			// todo
		}
		delete(c.elements, elemName)
	}
	c.elements = nil
	c.killNoticeCh = nil
}

// TODO
func (c *CosmosLocal) initRunnable(self *CosmosSelf, runnable CosmosRunnable) error {
	if err := runnable.Check(); err != nil {
		return err
	}
	// Initial all locals elements.
	// TODO: Parallels loading.
	loaded := map[string]*ElementLocal{}
	elements := make(map[string]*ElementLocal, len(runnable.defines))
	for name, define := range runnable.defines {
		elem, err := createElement(self, define)
		if err != nil {
			err = fmt.Errorf(logCosmosLocalInitCreateElementFailed, name, err)
			self.Fatal("%s", err.Error())
			return err
		}
		if err = elem.load(); err != nil {
			err = fmt.Errorf(logCosmosLocalInitLoadElementFailed, name, err)
			self.Fatal("%s", err.Error())
			for loadedName, loadedElem := range loaded {
				err = loadedElem.unload()
				err = fmt.Errorf(logCosmosLocalInitLoadElementUnloadInfo, loadedName, err)
			}
			return err
		}
		elements[name] = elem
		inf := fmt.Sprintf(logCosmosLocalInitLoadElementInfo, name)
		self.Info("%s", inf)
	}

	// Lock, set elements, and unlock.
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.config != nil || c.elements != nil {
		err := fmt.Errorf(logCosmosLocalInitHasInitialized)
		self.Fatal("%s", err.Error())
		return err
	}
	c.config = self.config
	c.cosmos = self
	c.elements = elements
	c.mainId = &idMain{}
	c.killNoticeCh = make(chan bool)

	return nil
}

func (c *CosmosLocal) runRunnable(runnable CosmosRunnable) error {
	c.mutex.Lock()
	cosmos := c.cosmos
	c.mutex.Unlock()
	if cosmos == nil {
		// todo
		err := fmt.Errorf("not inited")
		c.cosmos.Fatal("%s", err.Error())
		return err
	}
	runnable.script(cosmos, &idMain{}, c.killNoticeCh)
	return nil
}

func (c *CosmosLocal) unload() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.elements == nil {
		return
	}
	// TODO: Parallels unloading
	for name, elem := range c.elements {
		err := elem.unload()
		inf := fmt.Sprintf(logCosmosLocalUnloadElementInfo, name, err)
		c.cosmos.Info("%s", inf)
	}
	c.config = nil
	c.elements = nil
}

// Element

func (c *CosmosLocal) getElement(name string) (*ElementLocal, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	elem, has := c.elements[name]
	if !has {
		err := fmt.Errorf(logCosmosLocalGetElementNotFound, name)
		c.cosmos.Fatal("%s", err.Error())
		return nil, err
	}
	return elem, nil
}

func (c *CosmosLocal) addElement(name string, elem *ElementLocal) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check exists
	if _, has := c.elements[name]; has {
		err := fmt.Errorf(logCosmosLocalAddElementNotFound, name)
		c.cosmos.Fatal("%s", err.Error())
		return err
	}
	// Try load and set
	if err := elem.load(); err != nil {
		err := fmt.Errorf(logCosmosLocalAddElementLoadFailed, name, err)
		c.cosmos.Fatal("%s", err.Error())
		return err
	}
	c.elements[name] = elem
	inf := fmt.Sprintf(logCosmosLocalInitLoadElementInfo, name)
	c.cosmos.Info("%s", inf)
	return nil
}

func (c *CosmosLocal) delElement(name string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check exists
	elem, has := c.elements[name]
	if !has {
		err := fmt.Errorf(logCosmosLocalDelElementNotFound, name)
		c.cosmos.Fatal("%s", err.Error())
		return err
	}
	// Try unload and unset
	if err := elem.unload(); err != nil {
		err := fmt.Errorf(logCosmosLocalDelElementLoadFailed, name, err)
		c.cosmos.Fatal("%s", err.Error())
		return err
	}
	delete(c.elements, name)
	return nil
}

// Atom

func (c *CosmosLocal) IsLocal() bool {
	return true
}

func (c *CosmosLocal) GetAtomId(elemName, atomName string) (Id, error) {
	// Get element.
	e, err := c.getElement(elemName)
	if err != nil {
		return nil, err
	}
	// Get atom.
	return e.GetAtomId(atomName)
}

func (c *CosmosLocal) SpawnAtom(elemName, atomName string, arg proto.Message) (Id, error) {
	// Get element.
	e, err := c.getElement(elemName)
	if err != nil {
		return nil, err
	}
	// Try spawning.
	i, err := e.SpawnAtom(atomName, arg)
	if err != nil {
		return nil, err
	}
	return i, nil
}

func (c *CosmosLocal) CallAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
	return toId.Element().CallAtom(fromId, toId, message, args)
}

func (c *CosmosLocal) KillAtom(fromId, toId Id) error {
	return toId.Element().KillAtom(fromId, toId)
}
