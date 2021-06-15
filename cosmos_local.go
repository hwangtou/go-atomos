package go_atomos

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"sync"
)

const (
	logCosmosLocalInitLoadElementNotImplemented   = "atomos.CosmosLocal.init: CosmosLocal init load element not implemented, name=%s"
	logCosmosLocalInitLoadElementImplementInvalid = "atomos.CosmosLocal.init: CosmosLocal init load element implement invalid, name=%s"
	logCosmosLocalInitCreateElementFailed         = "atomos.CosmosLocal.init: CosmosLocal create element failed, name=%s"
	logCosmosLocalInitLoadElementFailed           = "atomos.CosmosLocal.init: CosmosLocal load element failed, name=%s"
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
	config   *Config
	mutex    sync.RWMutex
	elements map[string]*ElementLocal
}

// Life cycle

func newCosmosLocal() *CosmosLocal {
	return &CosmosLocal{}
}

func (c *CosmosLocal) load(config *Config, self *CosmosSelf, defines map[string]*ElementDefine) error {
	// Configuration preparation.
	if err := config.check(); err != nil {
		return err
	}
	elements := make(map[string]*ElementLocal, len(config.CosmosLocal.Elements))
	for name, elemConf := range config.CosmosLocal.Elements {
		define, has := defines[name]
		if !has {
			err := fmt.Errorf(logCosmosLocalInitLoadElementNotImplemented, name)
			logErr(err.Error())
			return err
		}
		// TODO
		//if err := define.Check(); err != nil {
		//	err := fmt.Errorf(logCosmosLocalInitLoadElementImplementInvalid, name)
		//	logErr(err.Error())
		//	return err
		//}
		elem, err := elemConf.createElement(self, define)
		if err != nil {
			err := fmt.Errorf(logCosmosLocalInitCreateElementFailed, name)
			logErr(err.Error())
			return err
		}
		elements[name] = elem
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.elements != nil {
		err := fmt.Errorf(logCosmosLocalInitHasInitialized)
		logErr(err.Error())
		return err
	}
	c.config = config
	c.elements = elements

	// Load elements
	loaded := map[string]*ElementLocal{}
	// TODO: Parallels loading.
	for name, elem := range c.elements {
		if err := elem.load(); err != nil {
			err = fmt.Errorf(logCosmosLocalInitLoadElementFailed, name)
			logErr(err.Error())
			for loadedName, loadedElem := range loaded {
				err = loadedElem.unload()
				err = fmt.Errorf(logCosmosLocalInitLoadElementUnloadInfo, loadedName, err)
			}
			c.elements = nil
			return err
		}
		inf := fmt.Sprintf(logCosmosLocalInitLoadElementInfo, name)
		logInfo(inf)
	}
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
		logInfo(inf)
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
		logErr(err.Error())
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
		logErr(err.Error())
		return err
	}
	// Try load and set
	if err := elem.load(); err != nil {
		err := fmt.Errorf(logCosmosLocalAddElementLoadFailed, name, err)
		logErr(err.Error())
		return err
	}
	c.elements[name] = elem
	inf := fmt.Sprintf(logCosmosLocalInitLoadElementInfo, name)
	logInfo(inf)
	return nil
}

func (c *CosmosLocal) delElement(name string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check exists
	elem, has := c.elements[name]
	if !has {
		err := fmt.Errorf(logCosmosLocalDelElementNotFound, name)
		logErr(err.Error())
		return err
	}
	// Try unload and unset
	if err := elem.unload(); err != nil {
		err := fmt.Errorf(logCosmosLocalDelElementLoadFailed, name, err)
		logErr(err.Error())
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
	a, err := e.SpawnAtom(atomName, arg)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (c *CosmosLocal) CallAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
	return toId.Element().CallAtom(fromId, toId, message, args)
}

func (c *CosmosLocal) KillAtom(fromId, toId Id) error {
	return toId.Element().KillAtom(fromId, toId)
}
