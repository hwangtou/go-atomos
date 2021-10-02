package go_atomos

// CHECKED!

import (
	"fmt"
	"runtime/debug"
	"sync"

	"google.golang.org/protobuf/proto"
)

// Local Cosmos Instance

type CosmosLocal struct {
	mutex         sync.RWMutex
	config        *Config
	cosmosSelf    *CosmosSelf
	elements      map[string]*ElementLocal
	elementsOrder []*ElementImplementation
	interfaces    map[string]*ElementInterface
	mainElem      *ElementLocal
	mainAtom      *mainAtom
	mainKillCh    chan bool
}

// Life cycle

func newCosmosLocal() *CosmosLocal {
	return &CosmosLocal{}
}

// 初始化Runnable。
// Initial Runnable.
func (c *CosmosLocal) initRunnable(self *CosmosSelf, runnable CosmosRunnable) error {
	self.logInfo("Cosmos.Init")

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

	elemName := ""
	defer func() {
		if r := recover(); r != nil {
			self.logFatal("Cosmos.Init: Load element error, name=%s,err=%v,stack=%s",
				elemName, r, string(debug.Stack()))
		}
	}()
	for _, define := range runnable.implementOrder {
		// Create local element .
		elemName = define.Interface.Config.Name
		elem := newElementLocal(self, define)
		// Load the element.
		if err := elem.load(); err != nil {
			self.logFatal("Cosmos.Init: Load element failed, element=%s,err=%s", elemName, err)
			for loadedName, loadedElem := range loadedElements {
				err = loadedElem.unload()
				if err != nil {
					self.logInfo("Cosmos.Init: Unload loaded element, element=%s,err=%v",
						loadedName, err)
				} else {
					self.logInfo("Cosmos.Init: Unload loaded element, element=%s", loadedName)
				}
			}
			c.config = nil
			c.mainElem = nil
			c.mainAtom = nil
			c.cosmosSelf = nil
			return err
		}
		// Add the element.
		elements[elemName] = elem
		self.logInfo("Cosmos.Init: Load element succeed, element=%s", elemName)
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
		self.logFatal("Cosmos.Init: Init runtime error, err=%v", err)
		return err
	}
	c.elements = elements
	c.elementsOrder = runnable.implementOrder
	c.interfaces = elementInterfaces
	c.mainKillCh = make(chan bool)

	// Init remote to support remote.
	if err := self.remotes.init(); err != nil {
		self.logFatal("Cosmos.Init: Init remote error, err=%v", err)
		return err
	}

	// Load wormhole.
	for name, elem := range c.elements {
		go func(n string, e *ElementLocal) {
			defer func() {
				if r := recover(); r != nil {
					self.logFatal("Cosmos.Init: Daemon wormhole error, name=%s,err=%v", n, r)
				}
			}()
			if w, ok := e.current.Developer.(ElementWormholeDeveloper); ok {
				self.logInfo("Cosmos.Init: Daemon wormhole, name=%s", n)
				w.Daemon()
			}
		}(name, elem)
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
		c.cosmosSelf.logFatal("Cosmos.Run: Framework PANIC, err=%v", err.Error())
		return err
	}

	ma := c.mainAtom.instance.(MainId)
	c.cosmosSelf.logInfo("Cosmos.Run: NOW RUNNING!")
	runnable.script(cosmos, ma, c.mainKillCh)

	// Close main.
	if err := c.mainAtom.Kill(c.mainAtom); err != nil {
		c.cosmosSelf.logError("Cosmos.Run: Kill main atom error, err=%v", err)
	}
	return nil
}

// 退出Runnable。
// Exit runnable.
func (c *CosmosLocal) exitRunnable() {
	c.cosmosSelf.logInfo("Cosmos.Exit: NOW EXITING!")

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Close remote.
	c.cosmosSelf.remotes.close()
	// Unload local element interfaces.
	for elemName := range c.interfaces {
		delete(c.interfaces, elemName)
	}
	// Unload local elements.
	elemName := ""
	defer func() {
		if r := recover(); r != nil {
			c.cosmosSelf.logFatal("Cosmos.Exit: Unload element error, name=%s,err=%v,stack=%s",
				elemName, r, string(debug.Stack()))
		}
	}()
	for i := len(c.elementsOrder) - 1; i >= 0; i -= 1 {
		elemName = c.elementsOrder[i].Interface.Config.Name
		define, has := c.elements[elemName]
		if !has {
			c.cosmosSelf.logInfo("Cosmos.Exit: Unload runtime error, element=%s,err=not found", elemName)
			continue
		}
		err := define.unload()
		if err != nil {
			c.cosmosSelf.logInfo("Cosmos.Exit: Unload local element, element=%s,err=%v", elemName, err)
		} else {
			c.cosmosSelf.logInfo("Cosmos.Exit: Unload local element, element=%s", elemName)
		}
		define.current.Developer.Unload()
		delete(c.elements, elemName)
	}
	c.mainKillCh = nil
	c.mainAtom = nil
	c.mainElem = nil
	c.interfaces = nil
	c.elements = nil
	c.elementsOrder = nil
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
		c.cosmosSelf.logFatal("Cosmos.Element: Add, element exists, name=%s", name)
		return err
	}
	// Try load and set
	if err := elem.load(); err != nil {
		err = fmt.Errorf("local element loads failed, name=%s", name)
		c.cosmosSelf.logFatal("Cosmos.Element: Add, element loads failed, err=%v", err.Error())
		return err
	}
	c.elements[name] = elem

	c.cosmosSelf.logInfo("Cosmos.Element: Add, Load element succeed, element=%s", name)
	return nil
}

func (c *CosmosLocal) getElement(name string) (*ElementLocal, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	elem, has := c.elements[name]
	if !has {
		err := fmt.Errorf("local element not found, name=%s", name)
		c.cosmosSelf.logError("Cosmos.Element: Get, Cannot get element, name=%s", name)
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
		c.cosmosSelf.logFatal("Cosmos.Element: Del, Cannot delete element, err=%s", err.Error())
		return err
	}
	// Try unload and unset
	if err := elem.unload(); err != nil {
		err = fmt.Errorf("local element unloads failed, name=%s,err=%v", name, err)
		c.cosmosSelf.logFatal("Cosmos.Element: Del, Cannot unload element, err=%s", err.Error())
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
