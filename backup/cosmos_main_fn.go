package go_atomos

// CHECKED!

import (
	"crypto/tls"
	"crypto/x509"
	"github.com/hwangtou/go-atomos/core"
	"io/ioutil"
	"runtime/debug"
	"sync"

	"google.golang.org/protobuf/proto"
)

// Local Cosmos Instance

type CosmosMainFn struct {
	process  *CosmosProcess
	config   *Config
	runnable *CosmosRunnable
	loading  *CosmosRunnable

	elements map[string]*ElementLocal
	mutex    sync.RWMutex

	mainKillCh chan bool
	mainId     *core.BaseAtomos
	//*mainAtom

	// TLS if exists
	listenCert *tls.Config
	clientCert *tls.Config
	// Cosmos Server
	remoteServer *cosmosRemoteServer
}

func (c *CosmosMainFn) Description() string {
	return c.config.Node
}

func (c *CosmosMainFn) Halt(from core.ID, cancels map[uint64]core.CancelledTask) (save bool, data proto.Message) {
}

func (c *CosmosMainFn) Reload(newInstance core.Atomos) {
}

func (c *CosmosMainFn) OnMessaging(from core.ID, name string, args proto.Message) (reply proto.Message, err *core.ErrorInfo) {
}

func (c *CosmosMainFn) OnReloading(reload interface{}, reloads int) {
}

func (c *CosmosMainFn) OnStopping(from core.ID, cancelled map[uint64]core.CancelledTask) *core.ErrorInfo {
}

// Life cycle

func newCosmosMainFn() *CosmosMainFn {
	return &CosmosMainFn{}
}

// 初始化Runnable。
// Initial Runnable.
func (c *CosmosMainFn) loadRunnable(process *CosmosProcess, conf *Config, runnable *CosmosRunnable) *core.ErrorInfo {
	process.sharedLog.PushProcessLog(core.LogLevel_Info, "MainFn: Load runnable")

	id := &core.IDInfo{
		Type:    core.IDType_Main,
		Cosmos:  c.config.Node,
		Element: "",
		Atomos:  "",
	}

	c.process = process
	c.config = conf
	c.runnable = runnable
	c.loading = runnable
	c.elements = make(map[string]*ElementLocal, len(runnable.implements))
	c.mainKillCh = make(chan bool)
	c.mainId = core.NewBaseAtomos(id, process.sharedLog, runnable.mainLogLevel, c, c)

	// 加载TLS Cosmos Node支持，用于加密链接。
	if err := c.loadTlsCosmosNodeSupport(); err != nil {
		return err
	}
	// 加载远端Cosmos服务支持。
	if err := c.loadRemoteCosmosServerSupport(); err != nil {
		return err
	}

	// 事务式加载Elements。
	if errs := c.loadElementsTransaction(runnable); len(errs) > 0 {
		c.rollback(false, errs)
		return core.NewErrorf(core.ErrMainFnCheckElementFailed, "MainFn: Check element failed, errs=(%v)", errs)
	}
	//// Init telnet.
	//if err := process.telnet.init(); err != nil {
	//	process.logging(core.LogLevel_Fatal, "MainFn: Init telnet error, err=%v", err)
	//	c.rollback(false, map[string]*core.ErrorInfo{})
	//	return err
	//}
	c.commit(false)
	c.startRemoteCosmosServer()

	// Load wormhole.
	c.daemon(false)

	c.loaded()
	return nil
}

func (c *CosmosMainFn) loadRemoteCosmosServerSupport() *core.ErrorInfo {
	if c.config.EnableServer == nil {
		return nil
	}
	// Enable Server
	c.process.logging(core.LogLevel_Info, "MainFn: Enable Server, host=(%s),port=(%d)",
		c.config.EnableServer.Host, c.config.EnableServer.Port)
	c.remoteServer = newCosmosRemoteHelper(c)
	if err := c.remoteServer.init(); err != nil {
		return err
	}
	return nil
}

func (c *CosmosMainFn) startRemoteCosmosServer() {
	if server := c.remoteServer; server != nil {
		server.start()
	}
}

func (c *CosmosMainFn) loadTlsCosmosNodeSupport() *core.ErrorInfo {
	// Check enable Cert.
	if c.config.EnableCert == nil {
		return nil
	}
	cert := c.config.EnableCert
	if cert.CertPath == "" {
		return core.NewError(core.ErrCosmosCertConfigInvalid, "MainFn: Cert path is empty")
	}
	if cert.KeyPath == "" {
		return core.NewError(core.ErrCosmosCertConfigInvalid, "MainFn: Key path is empty")
	}
	// Load Key Pair.
	c.process.logging(core.LogLevel_Info, "MainFn: Enabling Cert, cert=(%s),key=(%s)", cert.CertPath, cert.KeyPath)
	pair, e := tls.LoadX509KeyPair(cert.CertPath, cert.KeyPath)
	if e != nil {
		err := core.NewErrorf(core.ErrMainFnLoadCertFailed, "MainFn: Load Key Pair failed, err=(%v)", e)
		c.process.logging(core.LogLevel_Fatal, err.Message)
		return err
	}
	c.listenCert = &tls.Config{
		Certificates: []tls.Certificate{
			pair,
		},
	}
	// Load Cert.
	caCert, e := ioutil.ReadFile(cert.CertPath)
	if e != nil {
		return core.NewErrorf(core.ErrCosmosCertConfigInvalid, "MainFn: Cert file read error, err=(%v)", e)
	}
	tlsConfig := &tls.Config{}
	if cert.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
		c.clientCert = tlsConfig
		return nil
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	// Create TLS configuration with the certificate of the server.
	tlsConfig.RootCAs = caCertPool
	c.clientCert = tlsConfig

	return nil
}

// 执行Runnable。
// Run runnable.
func (c *CosmosMainFn) run(runnable *CosmosRunnable) *core.ErrorInfo {
	//ma := c.mainAtom.instance.(MainId)
	c.process.logging(core.LogLevel_Info, "MainFn: NOW RUNNING!")
	runnable.mainScript(c.process, c.mainId, c.mainKillCh)
	return nil
}

// 升级
// Upgrade
func (c *CosmosMainFn) reload(runnable *CosmosRunnable, reloads int) *core.ErrorInfo {
	c.process.logging(core.LogLevel_Info, "MainFn: Reload")
	c.mutex.Lock()
	if c.loading != nil {
		c.mutex.Unlock()
		err := core.NewError(core.ErrMainFnIsReloading, "MainFn: Is reloading")
		c.process.logging(core.LogLevel_Fatal, err.Message)
		return err
	}
	c.mutex.Unlock()
	if errs := c.loadElementsTransaction(runnable); len(errs) > 0 {
		c.rollback(true, errs)
		err := core.NewErrorf(core.ErrMainFnReloadFailed, "MainFn: Reloading failed, errs=(%v)", errs)
		return err
	}

	err := func(runnable *CosmosRunnable) *core.ErrorInfo {
		defer c.process.deferRunnable()
		ma := c.mainAtom.instance.(MainId)
		c.process.logging(core.LogLevel_Info, "MainFn: NOW RELOADING!")
		runnable.reloadScript(c.process, ma, c.mainKillCh)
		return nil
	}(runnable)

	if err != nil {
		c.rollback(true, map[string]*core.ErrorInfo{})
		return err
	}
	c.commit(true)

	// Load wormhole.
	c.daemon(true)

	c.loaded()
	c.pushAtomosReload(reloads)

	return nil
}

func (c *CosmosMainFn) stop() bool {
	select {
	case c.mainKillCh <- true:
		return true
	default:
		c.process.logging(core.LogLevel_Info, "MainFn: Exit error, err=(Runnable is blocking)")
		return false
	}
}

// 退出Runnable。
// Exit runnable.
func (c *CosmosMainFn) close() {
	c.process.logging(core.LogLevel_Info, "MainFn: NOW EXITING!")

	c.mutex.Lock()
	if c.runnable == nil {
		c.mutex.Unlock()
		c.process.logging(core.LogLevel_Error, "Cosmos.Exit: Runnable is not running")
		return
	}
	runnable := c.runnable
	c.runnable = nil
	c.mutex.Unlock()

	// Unload local elements and its atomos.
	c.unloadElement(runnable)
	//// After Runnable Script terminated.
	//// Close main.
	//_ = c.mainAtom.pushKillMail(c.mainAtom, true)

	// Close remote.
	//c.process.telnet.close()
	c.process.remotes.close()
	c.mainKillCh = nil
	//c.mainAtom = nil
	//c.mainElem = nil
	c.elements = nil
	c.process = nil
}

// Element Interface.

func (c *CosmosMainFn) getElement(name string) (elem *ElementLocal, err *core.ErrorInfo) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.runnable == nil || c.loading != nil {
		if c.runnable == nil {
			err = core.NewError(core.ErrMainFnRunnableNotFound, "MainFn: Runnable not found")
		} else {
			err = core.NewError(core.ErrMainFnIsReloading, "MainFn: Runnable reloading")
		}
		c.process.logging(core.LogLevel_Error, err.Message)
		return nil, err
	}
	elem, has := c.elements[name]
	if !has {
		err = core.NewErrorf(core.ErrMainFnElementNotFound, "MainFn: Local element not found, name=(%s)", name)
		return nil, err
	}
	if !elem.avail {
		err = core.NewErrorf(core.ErrMainFnElementIsInvalid, "MainFn: Local element is invalid, name=(%s)", name)
		c.process.logging(core.LogLevel_Error, err.Message)
		return nil, err
	}
	return elem, nil
}

// Element Container Handlers.

func (c *CosmosMainFn) loadElementsTransaction(runnable *CosmosRunnable) (errs map[string]*core.ErrorInfo) {
	errs = map[string]*core.ErrorInfo{}
	// Pre-initialize all local elements in the Runnable.
	for _, define := range runnable.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		if err := c.loadElement(name, define); err != nil {
			c.process.logging(core.LogLevel_Fatal, "MainFn: Load element failed, element=%s,err=%v", name, err)
			errs[name] = err
			continue
		}
		//// Add the element.
		c.process.logging(core.LogLevel_Info, "MainFn: Load element succeed, element=%s", name)
	}
	return
}

func (c *CosmosMainFn) loadElement(name string, define *ElementImplementation) *core.ErrorInfo {
	defer func() {
		if r := recover(); r != nil {
			c.process.logging(core.LogLevel_Fatal, "MainFn: Check element panic, name=(%s),err=(%v),stack=(%s)",
				name, r, string(debug.Stack()))
		}
	}()
	c.mutex.Lock()
	elem, has := c.elements[name]
	if !has {
		elem = newElementLocal(c, define)
		c.elements[name] = elem
	}
	c.mutex.Unlock()
	return elem.loadElementSetDefine(define, elem)
}

func (c *CosmosMainFn) daemon(isReload bool) {
	for name, elem := range c.elements {
		go func(n string, e *ElementLocal) {
			defer func() {
				if r := recover(); r != nil {
					c.process.logging(core.LogLevel_Fatal, "MainFn: Start running PANIC, name=(%s),err=(%v)", n, r)
				}
			}()
			if w, ok := e.current.Developer.(ElementStartRunning); ok {
				c.process.logging(core.LogLevel_Info, "MainFn: Start running, name=(%s)", n)
				w.StartRunning(isReload)
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
			c.process.logging(core.LogLevel_Info, "MainFn: Rollback, element=(%s)", name)
		}
	}
	c.mutex.Lock()
	if !isReload {
		c.runnable = nil
	}
	c.loading = nil
	c.mutex.Unlock()
}

func (c *CosmosMainFn) commit(isReload bool) {
	for _, define := range c.loading.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		if elem, has := c.elements[name]; has {
			elem.commit(isReload)
			c.process.logging(core.LogLevel_Info, "MainFn: Commit, element=(%s)", name)
		}
	}
}

func (c *CosmosMainFn) loaded() {
	c.mutex.Lock()
	c.runnable = c.loading
	c.loading = nil
	c.mutex.Unlock()
}

func (c *CosmosMainFn) pushAtomosReload(reloads int) {
	for _, define := range c.runnable.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		if elem, has := c.elements[name]; has {
			elem.pushReload(reloads)
			c.process.logging(core.LogLevel_Info, "MainFn: Push atomos reloaded, element=(%s)", name)
		}
	}
}

func (c *CosmosMainFn) unloadElement(runnable *CosmosRunnable) {
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
						c.process.logging(core.LogLevel_Fatal, "MainFn: Close element PANIC, name=(%s),reason=(%s)", name, r)
					}
				}()
				elem.unload()
				c.process.logging(core.LogLevel_Info, "MainFn: Closed, element=(%s)", name)
			}(name)
		}
	}
	wg.Wait()
	c.process.logging(core.LogLevel_Info, "MainFn: Closed")
}

// Atom Interface.

func (c *CosmosMainFn) GetNodeName() string {
	return c.config.Node
}

func (c *CosmosMainFn) IsLocal() bool {
	return true
}

func (c *CosmosMainFn) GetAtomId(elemName, atomName string) (ID, *core.ErrorInfo) {
	// Get element.
	e, err := c.getElement(elemName)
	if err != nil {
		return nil, err
	}
	// Get atomos.
	return e.GetAtomId(atomName)
}

func (c *CosmosMainFn) SpawnAtom(elemName, atomName string, arg proto.Message) (ID, *core.ErrorInfo) {
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

func (c *CosmosMainFn) MessageAtom(fromId, toId ID, message string, args proto.Message) (reply proto.Message, err *core.ErrorInfo) {
	return toId.Element().MessagingAtom(fromId, toId, message, args)
}

func (c *CosmosMainFn) KillAtom(fromId, toId ID) *core.ErrorInfo {
	return toId.Element().KillAtom(fromId, toId)
}
