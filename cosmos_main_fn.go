package go_atomos

// CHECKED!

import (
	"crypto/tls"
	"fmt"
	"github.com/hwangtou/go-atomos/core"
	"runtime/debug"
	"sync"

	"google.golang.org/protobuf/proto"
)

// Local Cosmos Instance

type CosmosMainFn struct {
	mutex         sync.RWMutex
	cosmosProcess *CosmosProcess
	config        *Config
	runnable      *CosmosRunnable
	loading       *CosmosRunnable
	elements      map[string]*ElementLocal
	//mainElem   *ElementLocal
	mainKillCh chan bool
	mainId     *core.BaseAtomos
	//*mainAtom
	// TLS if exists
	listenCert *tls.Config
	clientCert *tls.Config
}

func (c *CosmosMainFn) OnMessaging(from core.ID, name string, args proto.Message) (reply proto.Message, err *core.ErrorInfo) {
	//TODO implement me
	panic("implement me")
}

func (c *CosmosMainFn) OnReloading(reload interface{}, reloads int) {
	//TODO implement me
	panic("implement me")
}

func (c *CosmosMainFn) OnStopping(from core.ID, cancelled map[uint64]core.CancelledTask) *core.ErrorInfo {
	//TODO implement me
	panic("implement me")
}

// Life cycle

func newCosmosMainFn() *CosmosMainFn {
	return &CosmosMainFn{}
}

func mainConstructor() core.Atomos {
}

// 初始化Runnable。
// Initial Runnable.
func (c *CosmosMainFn) loadRunnable(process *CosmosProcess, conf *Config, runnable *CosmosRunnable) *core.ErrorInfo {
	process.sharedLog.PushProcessLog(core.LogLevel_Info, "Cosmos.Init")

	id := &core.IDInfo{
		Type:    core.IDType_Main,
		Cosmos:  process.config.Node,
		Element: "",
		Atomos:  "",
	}

	c.cosmosProcess = process
	c.config = conf
	c.runnable = runnable
	c.loading = runnable
	c.elements = make(map[string]*ElementLocal, len(runnable.implements))
	//c.mainElem = newMainElement(process)
	//c.mainAtom = newMainAtom(c.mainElem)
	c.mainKillCh = make(chan bool)
	c.mainId = core.NewBaseAtomos(id, process.sharedLog, runnable.mainLogLevel, c, mainConstructor())

	if err := c.loadTlsCosmosNodeConfig(process); err != nil {
		return err
	}

	// Check for elements.
	if errs := c.setElementsTransaction(runnable); len(errs) > 0 {
		c.rollback(false, errs)
		return core.NewErrorf(core.ErrMainFnCheckElementFailed, "Check element failed, errs=(%v)", errs)
	}
	// Init remote to support remote.
	if err := process.remotes.init(); err != nil {
		process.logging(core.LogLevel_Fatal, "Cosmos.Init: Init remote error, err=%v", err)
		c.rollback(false, map[string]*core.ErrorInfo{})
		return err
	}
	// Init telnet.
	if err := process.telnet.init(); err != nil {
		process.logFatal("Cosmos.Init: Init telnet error, err=%v", err)
		c.rollback(false, map[string]error{})
		return err
	}
	c.commit(false)

	// Load wormhole.
	c.daemon(false)

	c.loaded()
	return nil
}

func (c *CosmosMainFn) loadTlsCosmosNodeConfig(process *CosmosProcess) *core.ErrorInfo {
	// Enable Cert
	if c.config.EnableCert == nil {
		return nil
	}
	cert := c.config.EnableCert
	process.logging(core.LogLevel_Info, "MainFn: Enabling Cert, cert=(%s),key=(%s)", cert.CertPath, cert.KeyPath)
	pair, e := tls.LoadX509KeyPair(cert.CertPath, cert.KeyPath)
	if e != nil {
		err := core.NewErrorf(core.ErrMainFnLoadCertFailed, "MainFn: Load Cert failed, err=(%v)", e)
		process.logging(core.LogLevel_Fatal, err.Message)
		return err
	}
	c.listenCert = &tls.Config{
		Certificates: []tls.Certificate{
			pair,
		},
	}
	if c.clientCert, err = conf.getClientCertConfig(); err != nil {
		return err
	}
	if c.listenCert, err = conf.getListenCertConfig(); err != nil {
		return err
	}

	return nil
}

// 执行Runnable。
// Run runnable.
func (c *CosmosMainFn) run(runnable *CosmosRunnable) *ErrorInfo {
	//ma := c.mainAtom.instance.(MainId)
	c.cosmosProcess.logInfo("Cosmos.Run: NOW RUNNING!")
	runnable.mainScript(c.cosmosProcess, c.mainAtom, c.mainKillCh)
	return nil
}

// 升级
// Upgrade
func (c *CosmosMainFn) reload(runnable *CosmosRunnable, upgradeCount int) *ErrorInfo {
	c.cosmosProcess.logInfo("Cosmos.Upgrade")
	c.mutex.Lock()
	if c.loading != nil {
		c.mutex.Unlock()
		err := fmt.Errorf("runnable is reloading")
		return err
	}
	c.mutex.Unlock()
	if errs := c.setElementsTransaction(runnable); len(errs) > 0 {
		c.rollback(true, errs)
		err := fmt.Errorf("runnable check elements failed")
		return err
	}

	err := func(runnable *CosmosRunnable) error {
		defer c.cosmosProcess.deferRunnable()
		ma := c.mainAtom.instance.(MainId)
		c.cosmosProcess.logInfo("Cosmos.Upgrade: NOW UPGRADING!")
		runnable.reloadScript(c.cosmosProcess, ma, c.mainKillCh)
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

func (c *CosmosMainFn) stop() bool {
	select {
	case c.mainKillCh <- true:
		return true
	default:
		c.cosmosProcess.logInfo("Cosmos.Daemon: Exit error, err=Runnable is blocking")
		return false
	}
}

// 退出Runnable。
// Exit runnable.
func (c *CosmosMainFn) close() {
	c.cosmosProcess.logInfo("Cosmos.Exit: NOW EXITING!")

	c.mutex.Lock()
	if c.runnable == nil {
		c.mutex.Unlock()
		c.cosmosProcess.logError("Cosmos.Exit: runnable was closed")
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
	c.cosmosProcess.telnet.close()
	c.cosmosProcess.remotes.close()
	c.mainKillCh = nil
	c.mainAtom = nil
	c.mainElem = nil
	c.elements = nil
	c.cosmosProcess = nil
}

// Element Interface.

func (c *CosmosMainFn) getElement(name string) (elem *ElementLocal, err error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.runnable == nil || c.loading != nil {
		if c.runnable == nil {
			err = fmt.Errorf("no runnable")
		} else {
			err = fmt.Errorf("reloading")
		}
		c.cosmosProcess.logError("Cosmos.Element: Get, Upgrading, name=%s", name)
		return nil, err
	}
	elem, has := c.elements[name]
	if !has {
		err = fmt.Errorf("local element not found, name=%s", name)
		c.cosmosProcess.logError("Cosmos.Element: Get, Cannot get element, name=%s", name)
		return nil, err
	}
	if !elem.avail {
		err = fmt.Errorf("local element is not avail, name=%s", name)
		c.cosmosProcess.logError("Cosmos.Element: Get, Not avail now, name=%s", name)
		return nil, err
	}
	return elem, nil
}

// Element Container Handlers.

func (c *CosmosMainFn) setElementsTransaction(runnable *CosmosRunnable) (errs map[string]*core.ErrorInfo) {
	errs = map[string]error{}
	// Pre-initialize all local elements in the Runnable.
	for _, define := range runnable.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		if err := c.setElement(name, define); err != nil {
			c.cosmosProcess.logging(core.LogLevel_Fatal, "MainFn: Load element failed, element=%s,err=%v", name, err)
			errs[name] = err
			continue
		}
		//// Add the element.
		c.cosmosProcess.logging(core.LogLevel_Info, "MainFn: Load element succeed, element=%s", name)
	}
	return
}

func (c *CosmosMainFn) setElement(name string, define *ElementImplementation) error {
	defer func() {
		if r := recover(); r != nil {
			c.cosmosProcess.logging(core.LogLevel_Fatal, "MainFn: Check element panic, name=(%s),err=(%v),stack=(%s)",
				name, r, string(debug.Stack()))
		}
	}()
	c.mutex.Lock()
	elem, has := c.elements[name]
	if !has {
		elem = newElementLocal(c.cosmosProcess, define)
		c.elements[name] = elem
	}
	c.mutex.Unlock()
	return elem.setElementSetDefine(define, has)
}

func (c *CosmosMainFn) daemon(isUpgrade bool) {
	for name, elem := range c.elements {
		go func(n string, e *ElementLocal) {
			defer func() {
				if r := recover(); r != nil {
					c.cosmosProcess.logFatal("Cosmos.Init: Daemon wormhole error, name=%s,err=%v", n, r)
				}
			}()
			if w, ok := e.current.Developer.(ElementWormholeDeveloper); ok {
				c.cosmosProcess.logInfo("Cosmos.Init: Daemon wormhole, name=%s", n)
				w.Daemon(isUpgrade)
			}
		}(name, elem)
	}
}

func (c *CosmosMainFn) rollback(isReload bool, errs map[string]*core.ErrorInfo) {
	for _, define := range c.loading.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		if elem, has := c.elements[name]; has {
			_, failed := errs[name]
			elem.rollback(isReload, failed)
			c.cosmosProcess.logging(core.LogLevel_Info, "MainFn: Rollback, element=(%s)", name)
		}
	}
	c.mutex.Lock()
	if !isReload {
		c.runnable = nil
	}
	c.loading = nil
	c.mutex.Unlock()
}

func (c *CosmosMainFn) commit(isUpgrade bool) {
	for _, define := range c.loading.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		if elem, has := c.elements[name]; has {
			elem.commit(isUpgrade)
			c.cosmosProcess.logInfo("Cosmos.Check: Commit, element=%s", name)
		}
	}
}

func (c *CosmosMainFn) loaded() {
	c.mutex.Lock()
	c.runnable = c.loading
	c.loading = nil
	c.mutex.Unlock()
}

func (c *CosmosMainFn) pushUpgrade(upgradeCount int) {
	for _, define := range c.runnable.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		if elem, has := c.elements[name]; has {
			elem.pushUpgrade(upgradeCount)
			c.cosmosProcess.logInfo("Cosmos.Check: Commit, element=%s", name)
		}
	}
}

func (c *CosmosMainFn) closeElement(runnable *CosmosRunnable) {
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
						c.cosmosProcess.logFatal("Cosmos.Close: Panic, name=%s,reason=%s", name, r)
					}
				}()
				elem.unload()
				c.cosmosProcess.logInfo("Cosmos.Close: Closed, element=%s", name)
			}(name)
		}
	}
	wg.Wait()
	c.cosmosProcess.logInfo("Cosmos.Close: Closed")
}

// Atom Interface.

func (c *CosmosMainFn) GetNodeName() string {
	return c.cosmosProcess.config.Node
}

func (c *CosmosMainFn) IsLocal() bool {
	return true
}

func (c *CosmosMainFn) GetAtomId(elemName, atomName string) (ID, error) {
	// Get element.
	e, err := c.getElement(elemName)
	if err != nil {
		return nil, err
	}
	// Get atomos.
	return e.GetAtomId(atomName)
}

func (c *CosmosMainFn) SpawnAtom(elemName, atomName string, arg proto.Message) (ID, error) {
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

func (c *CosmosMainFn) MessageAtom(fromId, toId ID, message string, args proto.Message) (reply proto.Message, err error) {
	return toId.Element().MessagingAtom(fromId, toId, message, args)
}

func (c *CosmosMainFn) KillAtom(fromId, toId ID) error {
	return toId.Element().KillAtom(fromId, toId)
}
