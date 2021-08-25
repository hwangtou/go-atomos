package go_atomos

// CHECKED!

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
)

// Local Cosmos Instance

type CosmosLocal struct {
	mutex      sync.RWMutex
	config     *Config
	cosmosSelf *CosmosSelf
	elements   map[string]*ElementLocal
	interfaces map[string]*ElementInterface
	mainElem   *ElementLocal
	mainAtom   *mainAtom
	mainKillCh chan bool
}

// Life cycle

func newCosmosLocal() *CosmosLocal {
	return &CosmosLocal{}
}

// 初始化Runnable。
// Initial Runnable.
func (c *CosmosLocal) initRunnable(self *CosmosSelf, runnable CosmosRunnable) error {
	self.logInfo("CosmosLocal.initRunnable")

	// Check runnable.
	if err := runnable.Check(); err != nil {
		return err
	}

	c.config = self.config
	c.mainElem = newMainElement(self)
	c.mainAtom = newMainAtom(c.mainElem)
	c.cosmosSelf = self
	// Pre-initialize all local elements in the Runnable.
	loadedElements := map[string]*ElementLocal{}
	elements := make(map[string]*ElementLocal, len(runnable.implementations))
	for name, define := range runnable.implementations {
		// Create local element .
		elem := newElementLocal(self, define)
		// Load the element.
		if err := elem.load(); err != nil {
			self.logFatal("CosmosLocal.initRunnable: Load local element failed, element=%s,err=%s", name, err)
			for loadedName, loadedElem := range loadedElements {
				err = loadedElem.unload()
				if err != nil {
					self.logInfo("CosmosLocal.initRunnable: Unload loaded element, element=%s,err=%v",
						loadedName, err)
				} else {
					self.logInfo("CosmosLocal.initRunnable: Unload loaded element, element=%s", loadedName)
				}
			}
			c.config = nil
			c.mainElem = nil
			c.mainAtom = nil
			c.cosmosSelf = nil
			return err
		}
		// Add the element.
		elements[name] = elem
		self.logInfo("CosmosLocal.initRunnable: Load local element succeed, element=%s", name)
	}
	// Pre-initialize all elements interface in the runnable.
	elementInterfaces := make(map[string]*ElementInterface)
	for elementName, elementInterface := range runnable.interfaces {
		elementInterfaces[elementName] = elementInterface
	}

	// Lock, set elements, and unlock.
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.elements != nil {
		err := fmt.Errorf("local cosmos has been initialized")
		self.logFatal("CosmosLocal.initRunnable: Init runtime error, err=%v", err)
		return err
	}
	c.elements = elements
	c.interfaces = elementInterfaces
	c.mainKillCh = make(chan bool)

	for _, define := range c.elements {
		define.current.Developer.Load(c.mainAtom)
	}

	// Init remote to support remote.
	if err := self.remotes.init(); err != nil {
		self.logFatal("CosmosLocal.initRunnable: Init remote error, err=%v", err)
		return err
	}

	return nil
}

// 执行Runnable。
// Run runnable.
func (c *CosmosLocal) runRunnable(runnable CosmosRunnable) error {
	c.mutex.Lock()
	cosmos := c.cosmosSelf
	c.mutex.Unlock()
	if cosmos == nil {
		err := fmt.Errorf("local cosmos has not been initialized")
		c.cosmosSelf.logFatal("CosmosLocal.runRunnable: Framework PANIC, err=%v", err.Error())
		return err
	}

	ma := c.mainAtom.instance.(MainId)
	c.cosmosSelf.logInfo("CosmosLocal.runRunnable: Runnable is now RUNNING")
	runnable.script(cosmos, ma, c.mainKillCh)

	// Close main.
	if err := c.mainAtom.Kill(c.mainAtom); err != nil {
		c.cosmosSelf.logError("CosmosLocal.runRunnable: Kill main atom error, err=%v", err)
	}
	return nil
}

// 退出Runnable。
// Exit runnable.
func (c *CosmosLocal) exitRunnable() {
	c.cosmosSelf.logInfo("CosmosLocal.exitRunnable: Runnable is now EXITING")

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Close remote.
	c.cosmosSelf.remotes.close()
	// Unload local element interfaces.
	for elemName := range c.interfaces {
		delete(c.elements, elemName)
	}
	// Unload local elements.
	for elemName, elem := range c.elements {
		err := elem.unload()
		if err != nil {
			c.cosmosSelf.logInfo("CosmosLocal.exitRunnable: Unload local element, element=%s,err=%v",
				elemName, err)
		} else {
			c.cosmosSelf.logInfo("CosmosLocal.exitRunnable: Unload local element, element=%s", elemName)
		}
		elem.current.Developer.Unload()
		delete(c.elements, elemName)
	}
	c.mainKillCh = nil
	c.mainAtom = nil
	c.mainElem = nil
	c.interfaces = nil
	c.elements = nil
	c.cosmosSelf = nil
	c.config = nil
}

// Element Interface.

func (c *CosmosLocal) addElement(name string, elem *ElementLocal) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check exists
	if _, has := c.elements[name]; has {
		err := fmt.Errorf("local element exists, name=%s", name)
		c.cosmosSelf.logFatal("CosmosLocal.addElement: Element exists, name=%s", name)
		return err
	}
	// Try load and set
	if err := elem.load(); err != nil {
		err = fmt.Errorf("local element loads failed, name=%s", name)
		c.cosmosSelf.logFatal("CosmosLocal.addElement: Element loads failed, err=%v", err.Error())
		return err
	}
	c.elements[name] = elem

	c.cosmosSelf.logInfo("CosmosLocal.addElement: Load local element succeed, element=%s", name)
	return nil
}

func (c *CosmosLocal) getElement(name string) (*ElementLocal, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	elem, has := c.elements[name]
	if !has {
		err := fmt.Errorf("local element not found, name=%s", name)
		c.cosmosSelf.logError("CosmosLocal.getElement: Cannot get element, name=%s", name)
		return nil, err
	}
	return elem, nil
}

func (c *CosmosLocal) delElement(name string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check exists
	elem, has := c.elements[name]
	if !has {
		err := fmt.Errorf("local element not found, name=%s", name)
		c.cosmosSelf.logFatal("CosmosLocal.delElement: Cannot delete element, err=%s", err.Error())
		return err
	}
	// Try unload and unset
	if err := elem.unload(); err != nil {
		err = fmt.Errorf("local element unloads failed, name=%s,err=%v", name, err)
		c.cosmosSelf.logFatal("CosmosLocal.delElement: Cannot unload element, err=%s", err.Error())
		return err
	}
	delete(c.elements, name)
	return nil
}

// Atom Interface.

func (c *CosmosLocal) GetNodeName() string {
	return c.config.Node
}

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
		err = fmt.Errorf("element has not registered, name=%s", elemName)
		return nil, err
	}

	// Try spawning.
	i, err := e.SpawnAtom(atomName, arg)
	if err != nil {
		return nil, err
	}
	return i, nil
}

func (c *CosmosLocal) MessageAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
	return toId.Element().MessagingAtom(fromId, toId, message, args)
}

func (c *CosmosLocal) KillAtom(fromId, toId Id) error {
	return toId.Element().KillAtom(fromId, toId)
}
