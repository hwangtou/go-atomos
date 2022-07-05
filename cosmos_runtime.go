package go_atomos

// CHECKED!

import (
	"crypto/tls"
	"fmt"
	"runtime/debug"
	"sync"

	"google.golang.org/protobuf/proto"
)

// Local Cosmos Instance

type CosmosRuntime struct {
	mutex      sync.RWMutex
	cosmosSelf *CosmosSelf
	runnable   *CosmosRunnable
	loading    *CosmosRunnable
	elements   map[string]*ElementLocal
	//mainElem   *ElementLocal
	mainKillCh chan bool
	*mainAtom
}

// Life cycle

func newCosmosRuntime() *CosmosRuntime {
	return &CosmosRuntime{}
}

// 初始化Runnable。
// Initial Runnable.
func (c *CosmosRuntime) init(self *CosmosSelf, runnable *CosmosRunnable) error {
	self.logInfo("Cosmos.Init")

	c.cosmosSelf = self
	c.runnable = runnable
	c.loading = runnable
	c.elements = make(map[string]*ElementLocal, len(runnable.implements))
	c.mainElem = newMainElement(self)
	c.mainAtom = newMainAtom(c.mainElem)
	c.mainKillCh = make(chan bool)

	if errs := c.checkElements(runnable); len(errs) > 0 {
		c.rollback(false, errs)
		err := fmt.Errorf("runnable check elements failed")
		return err
	}
	// Enable Cert
	if self.config.EnableCert != nil {
		self.logInfo("Cosmos.Init: Enable Cert, cert=%s,key=%s",
			self.config.EnableCert.CertPath, self.config.EnableCert.KeyPath)
		pair, err := tls.LoadX509KeyPair(self.config.EnableCert.CertPath, self.config.EnableCert.KeyPath)
		if err != nil {
			return err
		}
		c.self.listenCert = &tls.Config{
			Certificates: []tls.Certificate{
				pair,
			},
		}
	}
	// Init remote to support remote.
	if err := self.remotes.init(); err != nil {
		self.logFatal("Cosmos.Init: Init remote error, err=%v", err)
		c.rollback(false, map[string]error{})
		return err
	}
	// Init telnet.
	if err := self.telnet.init(); err != nil {
		self.logFatal("Cosmos.Init: Init telnet error, err=%v", err)
		c.rollback(false, map[string]error{})
		return err
	}
	c.commit(false)

	// Load wormhole.
	c.daemon(false)

	c.loaded()
	return nil
}

// 执行Runnable。
// Run runnable.
func (c *CosmosRuntime) run(runnable *CosmosRunnable) error {
	//ma := c.mainAtom.instance.(MainId)
	c.cosmosSelf.logInfo("Cosmos.Run: NOW RUNNING!")
	runnable.mainScript(c.cosmosSelf, c.mainAtom, c.mainKillCh)
	return nil
}

// 升级
// Upgrade
func (c *CosmosRuntime) upgrade(runnable *CosmosRunnable, upgradeCount int) error {
	c.cosmosSelf.logInfo("Cosmos.Upgrade")
	c.mutex.Lock()
	if c.loading != nil {
		c.mutex.Unlock()
		err := fmt.Errorf("runnable is upgrading")
		return err
	}
	c.mutex.Unlock()
	if errs := c.checkElements(runnable); len(errs) > 0 {
		c.rollback(true, errs)
		err := fmt.Errorf("runnable check elements failed")
		return err
	}

	err := func(runnable *CosmosRunnable) error {
		defer c.cosmosSelf.deferRunnable()
		ma := c.mainAtom.instance.(MainId)
		c.cosmosSelf.logInfo("Cosmos.Upgrade: NOW UPGRADING!")
		runnable.upgradeScript(c.cosmosSelf, ma, c.mainKillCh)
		return nil
	}(runnable)

	if err != nil {
		c.rollback(true, map[string]error{})
		return err
	}
	c.commit(true)

	// Load wormhole.
	c.daemon(true)

	c.loaded()
	c.pushUpgrade(upgradeCount)

	return nil
}

func (c *CosmosRuntime) stop() bool {
	select {
	case c.mainKillCh <- true:
		return true
	default:
		c.cosmosSelf.logInfo("Cosmos.Daemon: Exit error, err=Runnable is blocking")
		return false
	}
}

// 退出Runnable。
// Exit runnable.
func (c *CosmosRuntime) close() {
	c.cosmosSelf.logInfo("Cosmos.Exit: NOW EXITING!")

	c.mutex.Lock()
	if c.runnable == nil {
		c.mutex.Unlock()
		c.cosmosSelf.logError("Cosmos.Exit: runnable was closed")
		return
	}
	runnable := c.runnable
	c.runnable = nil
	c.mutex.Unlock()

	// Unload local elements and its atomos.
	c.closeElement(runnable)
	// After Runnable Script terminated.
	// Close main.
	_ = c.mainAtom.pushKillMail(c.mainAtom, true)

	// Close remote.
	c.cosmosSelf.telnet.close()
	c.cosmosSelf.remotes.close()
	c.mainKillCh = nil
	c.mainAtom = nil
	c.mainElem = nil
	c.elements = nil
	c.cosmosSelf = nil
}

// Element Interface.

func (c *CosmosRuntime) getElement(name string) (elem *ElementLocal, err error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.runnable == nil || c.loading != nil {
		if c.runnable == nil {
			err = fmt.Errorf("no runnable")
		} else {
			err = fmt.Errorf("upgrading")
		}
		c.cosmosSelf.logError("Cosmos.Element: Get, Upgrading, name=%s", name)
		return nil, err
	}
	elem, has := c.elements[name]
	if !has {
		err = fmt.Errorf("local element not found, name=%s", name)
		c.cosmosSelf.logError("Cosmos.Element: Get, Cannot get element, name=%s", name)
		return nil, err
	}
	if !elem.avail {
		err = fmt.Errorf("local element is not avail, name=%s", name)
		c.cosmosSelf.logError("Cosmos.Element: Get, Not avail now, name=%s", name)
		return nil, err
	}
	return elem, nil
}

// Element Container Handlers.

func (c *CosmosRuntime) checkElements(runnable *CosmosRunnable) (errs map[string]error) {
	errs = map[string]error{}
	// Pre-initialize all local elements in the Runnable.
	for _, define := range runnable.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		if err := c.setElement(name, define); err != nil {
			c.cosmosSelf.logError("Cosmos.Check: Load element failed, element=%s,err=%v", name, err)
			errs[name] = err
			continue
		}
		//// Add the element.
		c.cosmosSelf.logInfo("Cosmos.Check: Load element succeed, element=%s", name)
	}
	return
}

func (c *CosmosRuntime) setElement(name string, define *ElementImplementation) error {
	defer func() {
		if r := recover(); r != nil {
			c.cosmosSelf.logFatal("Cosmos.Check: Check element panic, name=%s,err=%v,stack=%s",
				name, r, string(debug.Stack()))
		}
	}()
	c.mutex.Lock()
	elem, has := c.elements[name]
	if !has {
		elem = newElementLocal(c.cosmosSelf, int(define.Interface.Config.AtomInitNum))
		c.elements[name] = elem
	}
	c.mutex.Unlock()
	if !has {
		return elem.setInitDefine(define)
	} else {
		return elem.setUpgradeDefine(define)
	}
}

func (c *CosmosRuntime) daemon(isUpgrade bool) {
	for name, elem := range c.elements {
		go func(n string, e *ElementLocal) {
			defer func() {
				if r := recover(); r != nil {
					c.cosmosSelf.logFatal("Cosmos.Init: Daemon wormhole error, name=%s,err=%v", n, r)
				}
			}()
			if w, ok := e.current.Developer.(ElementWormholeDeveloper); ok {
				c.cosmosSelf.logInfo("Cosmos.Init: Daemon wormhole, name=%s", n)
				w.Daemon(isUpgrade)
			}
		}(name, elem)
	}
}

func (c *CosmosRuntime) rollback(isUpgrade bool, errs map[string]error) {
	for _, define := range c.loading.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		if elem, has := c.elements[name]; has {
			_, failed := errs[name]
			elem.rollback(isUpgrade, failed)
			c.cosmosSelf.logInfo("Cosmos.Check: Rollback, element=%s", name)
		}
	}
	c.mutex.Lock()
	if !isUpgrade {
		c.runnable = nil
	}
	c.loading = nil
	c.mutex.Unlock()
}

func (c *CosmosRuntime) commit(isUpgrade bool) {
	for _, define := range c.loading.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		if elem, has := c.elements[name]; has {
			elem.commit(isUpgrade)
			c.cosmosSelf.logInfo("Cosmos.Check: Commit, element=%s", name)
		}
	}
}

func (c *CosmosRuntime) loaded() {
	c.mutex.Lock()
	c.runnable = c.loading
	c.loading = nil
	c.mutex.Unlock()
}

func (c *CosmosRuntime) pushUpgrade(upgradeCount int) {
	for _, define := range c.runnable.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		if elem, has := c.elements[name]; has {
			elem.pushUpgrade(upgradeCount)
			c.cosmosSelf.logInfo("Cosmos.Check: Commit, element=%s", name)
		}
	}
}

func (c *CosmosRuntime) closeElement(runnable *CosmosRunnable) {
	wg := sync.WaitGroup{}
	for i := len(runnable.implementOrder) - 1; i >= 0; i -= 1 {
		name := runnable.implementOrder[i].Interface.Config.Name
		c.mutex.Lock()
		elem, has := c.elements[name]
		c.mutex.Unlock()
		if has {
			wg.Add(1)
			go func(name string) {
				defer func() {
					c.mutex.Lock()
					delete(c.elements, name)
					c.mutex.Unlock()
					wg.Done()
					if r := recover(); r != nil {
						c.cosmosSelf.logFatal("Cosmos.Close: Panic, name=%s,reason=%s", name, r)
					}
				}()
				elem.unload()
				c.cosmosSelf.logInfo("Cosmos.Close: Closed, element=%s", name)
			}(name)
		}
	}
	wg.Wait()
	c.cosmosSelf.logInfo("Cosmos.Close: Closed")
}

// Atom Interface.

func (c *CosmosRuntime) GetNodeName() string {
	return c.cosmosSelf.config.Node
}

func (c *CosmosRuntime) IsLocal() bool {
	return true
}

func (c *CosmosRuntime) GetAtomId(elemName, atomName string) (ID, error) {
	// Get element.
	e, err := c.getElement(elemName)
	if err != nil {
		return nil, err
	}
	// Get atom.
	return e.GetAtomId(atomName)
}

func (c *CosmosRuntime) SpawnAtom(elemName, atomName string, arg proto.Message) (ID, error) {
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

func (c *CosmosRuntime) MessageAtom(fromId, toId ID, message string, args proto.Message) (reply proto.Message, err error) {
	return toId.Element().MessagingAtom(fromId, toId, message, args)
}

func (c *CosmosRuntime) KillAtom(fromId, toId ID) error {
	return toId.Element().KillAtom(fromId, toId)
}
