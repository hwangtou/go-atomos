package go_atomos

// CHECKED!

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"runtime/debug"
	"sync"

	"google.golang.org/protobuf/proto"
)

const (
	MainFnElementName = "Main"
)

// Local Cosmos Instance

type CosmosMainFn struct {
	process  *CosmosProcess
	config   *Config
	runnable *CosmosRunnable
	loading  *CosmosRunnable

	atomos *BaseAtomos
	//id     ID

	elements map[string]*ElementLocal
	mutex    sync.RWMutex

	mainKillCh chan bool
	//mainId     *BaseAtomos
	//*mainAtom

	// TLS if exists
	listenCert *tls.Config
	clientCert *tls.Config
	//// Cosmos Server
	//remoteServer *cosmosRemoteServer

	// 调用链
	// 调用链用于检测是否有循环调用，在处理message时把fromId的调用链加上自己之后
	callChain []ID
}

// 生命周期相关
// Life cycle

func newCosmosMainFn(process *CosmosProcess, conf *Config, runnable *CosmosRunnable) *CosmosMainFn {
	process.sharedLog.PushProcessLog(LogLevel_Info, "MainFn: Loading")
	id := &IDInfo{
		Type:    IDType_Main,
		Cosmos:  conf.Node,
		Element: MainFnElementName,
		Atomos:  "",
	}
	mainFn := &CosmosMainFn{
		process:    process,
		config:     conf,
		runnable:   runnable,
		loading:    runnable,
		atomos:     nil,
		elements:   make(map[string]*ElementLocal, len(runnable.implements)),
		mutex:      sync.RWMutex{},
		mainKillCh: make(chan bool),
		listenCert: nil,
		clientCert: nil,
		callChain:  nil,
	}
	mainFn.atomos = NewBaseAtomos(id, process.sharedLog, runnable.mainLogLevel, mainFn, mainFn, 0)
	return mainFn
}

// 初始化Runnable。
// Initial Runnable.
func (c *CosmosMainFn) initCosmosMainFn(conf *Config, runnable *CosmosRunnable) *ErrorInfo {
	// 加载TLS Cosmos Node支持，用于加密链接。
	// loadTlsCosmosNodeSupport()
	// Check enable Cert.
	var listenCert, clientCert *tls.Config
	if cert := conf.EnableCert; cert != nil {
		if cert.CertPath == "" {
			return NewError(ErrCosmosCertConfigInvalid, "MainFn: Cert path is empty")
		}
		if cert.KeyPath == "" {
			return NewError(ErrCosmosCertConfigInvalid, "MainFn: Key path is empty")
		}
		// Load Key Pair.
		c.Log().Info("MainFn: Enabling Cert, cert=(%s),key=(%s)", cert.CertPath, cert.KeyPath)
		pair, e := tls.LoadX509KeyPair(cert.CertPath, cert.KeyPath)
		if e != nil {
			err := NewErrorf(ErrMainFnLoadCertFailed, "MainFn: Load Key Pair failed, err=(%v)", e)
			c.Log().Fatal(err.Message)
			return err
		}
		listenCert = &tls.Config{
			Certificates: []tls.Certificate{
				pair,
			},
		}
		// Load Cert.
		caCert, e := ioutil.ReadFile(cert.CertPath)
		if e != nil {
			return NewErrorf(ErrCosmosCertConfigInvalid, "MainFn: Cert file read error, err=(%v)", e)
		}
		tlsConfig := &tls.Config{}
		if cert.InsecureSkipVerify {
			tlsConfig.InsecureSkipVerify = true
			clientCert = tlsConfig
		} else {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			// Create TLS configuration with the certificate of the server.
			tlsConfig.RootCAs = caCertPool
			clientCert = tlsConfig
		}
	}

	//// 加载远端Cosmos服务支持。
	//// loadRemoteCosmosServerSupport
	//if server := conf.EnableServer; server != nil {
	//	// Enable Server
	//	c.Log().Info("MainFn: Enable Server, host=(%s),port=(%d)", server.Host, server.Port)
	//	c.remoteServer = newCosmosRemoteHelper(a)
	//	if err := c.remoteServer.init(); err != nil {
	//		return nil, err
	//	}
	//}

	// 事务式加载Elements。
	c.loadElementsTransaction(runnable) //, false); len(errs) > 0 {
	//	//c.loadElementsTransactionRollback(false, errs)
	//	return NewErrorf(ErrMainFnCheckElementFailed, "MainFn: Check element failed, errs=(%v)", errs)
	//}
	c.loadElementsTransactionCommit(false)
	c.listenCert, c.clientCert = listenCert, clientCert
	c.startRemoteCosmosServer()

	// Set loaded.
	c.mutex.Lock()
	c.runnable = c.loading
	c.loading = nil
	c.mutex.Unlock()

	// Send spawn.
	for _, impl := range runnable.implementOrder {
		name := impl.Interface.Config.Name
		c.mutex.RLock()
		elem, has := c.elements[name]
		c.mutex.RUnlock()
		if !has {
			continue
		}
		go func(n string, i *ElementImplementation, e *ElementLocal) {
			defer func() {
				if r := recover(); r != nil {
					c.process.logging(LogLevel_Fatal, "MainFn: Start running PANIC, name=(%s),err=(%v)", n, r)
				}
			}()
			if err := i.Interface.ElementSpawner(a, a.atomos.instance, arg, data); err != nil {
				return err
			}
			if w, ok := e.current.Developer.(ElementStartRunning); ok {
				c.process.logging(LogLevel_Info, "MainFn: Start running, name=(%s)", n)
				w.StartRunning(isReload)
			}
		}(name, impl, elem)
	}
	return nil
}

// emon(isReload bool) {
//	for name, elem := range c.elements {
//		go func(n string, e *ElementLocal) {
//			defer func() {
//				if r := recover(); r != nil {
//					c.process.logging(LogLevel_Fatal, "MainFn: Start running PANIC, name=(%s),err=(%v)", n, r)
//				}
//			}()
//			if w, ok := e.current.Developer.(ElementStartRunning); ok {
//				c.process.logging(LogLevel_Info, "MainFn: Start running, name=(%s)", n)
//				w.StartRunning(isReload)
//			}
//		}(name, elem)
//	}
//}

func (c *CosmosMainFn) deleteCosmosMainFn() {
}

// Implementation of ID

func (c *CosmosMainFn) GetIDInfo() *IDInfo {
	return c.atomos.GetIDInfo()
}

func (c *CosmosMainFn) getCallChain() []ID {
	return c.callChain
}

func (c *CosmosMainFn) Release() {
}

func (c *CosmosMainFn) Cosmos() CosmosNode {
	return c
}

func (c *CosmosMainFn) Element() Element {
	return c
}

func (c *CosmosMainFn) GetName() string {
	return c.GetIDInfo().Element
}

func (c *CosmosMainFn) Kill(from ID) *ErrorInfo {
	return NewError(ErrMainFnCannotKill, "Cannot kill main")
}

func (c *CosmosMainFn) String() string {
	return c.atomos.Description()
}

// Implementation of atomos.SelfID
// Implementation of atomos.ParallelSelf
//
// SelfID，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// SelfID, a concept that provide Atom resource access to inner Atom.
// With SelfID, Atom can access its self-mainFn with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provides Log and Tasks method to inner Atom.

func (e *CosmosMainFn) CosmosMainFn() *CosmosMainFn {
	return e
}

//func (e *CosmosMainFn) ElementLocal() *ElementLocal {
//	return e
//}

// KillSelf
// Atom kill itself from inner
func (e *CosmosMainFn) KillSelf() {
	if err := e.pushKillMail(e, false); err != nil {
		e.Log().Error("KillSelf error, err=%v", err)
		return
	}
	e.Log().Info("KillSelf")
}

// Check chain.

func (e *CosmosMainFn) checkCallChain(fromIdList []ID) bool {
	for _, fromId := range fromIdList {
		if fromId.GetIDInfo().IsEqual(e.GetIDInfo()) {
			return false
		}
	}
	return true
}

func (e *CosmosMainFn) addCallChain(fromIdList []ID) {
	e.callChain = append(fromIdList, e)
}

func (e *CosmosMainFn) delCallChain() {
	e.callChain = nil
}

// Implementation of CosmosNode

func (c *CosmosMainFn) GetNodeName() string {
	return c.GetIDInfo().Cosmos
}

func (c *CosmosMainFn) IsLocal() bool {
	return true
}

func (c *CosmosMainFn) GetElementAtomId(elem, name string) (ID, *ErrorInfo) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, err
	}
	return element.GetAtomId(name)
}

func (c *CosmosMainFn) SpawnElementAtom(elem, name string, arg proto.Message) (ID, *ErrorInfo) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, err
	}
	return element.SpawnAtom(name, arg)
}

// Implementation of Element

func (c *CosmosMainFn) GetElementName() string {
	return MainFnElementName
}

func (c *CosmosMainFn) GetAtomId(_ string) (ID, *ErrorInfo) {
	return c, nil
}

func (c *CosmosMainFn) GetAtomsNum() int {
	return 1
}

func (c *CosmosMainFn) SpawnAtom(_ string, _ proto.Message) (*AtomLocal, *ErrorInfo) {
	return nil, NewError(ErrMainFnCannotSpawn, "Cannot spawn main")
}

func (c *CosmosMainFn) MessageAtom(fromId, toId ID, message string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	return toId.Element().MessageAtom(fromId, toId, message, args)
}

func (c *CosmosMainFn) KillAtom(fromId, toId ID) *ErrorInfo {
	return toId.Element().KillAtom(fromId, toId)
}

// Implementation of AtomosUtilities

func (e *CosmosMainFn) Log() Logging {
	return e.atomos.Log()
}

func (e *CosmosMainFn) Task() Task {
	return e.atomos.Task()
}

// MainFn as an Atomos

func (c *CosmosMainFn) Description() string {
	return c.GetIDInfo().str()
}

func (c *CosmosMainFn) Halt(from ID, cancels map[uint64]CancelledTask) (save bool, data proto.Message) {
}

func (c *CosmosMainFn) Reload(newInstance Atomos) {
}

// 内部实现
// INTERNAL

// 邮箱控制器相关
// Mailbox Handler
// TODO: Performance tracer.

//func (e *CosmosMainFn) pushMessageMail(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
//	return e.atomos.PushMessageMailAndWaitReply(from, name, args)
//}

func (e *CosmosMainFn) OnMessaging(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	return nil, NewError(ErrMainFnCannotMessage, "MainFn: Cannot send message.")
}

func (e *CosmosMainFn) pushKillMail(from ID, wait bool) *ErrorInfo {
	return e.atomos.PushKillMailAndWaitReply(from, wait)
}

func (c *CosmosMainFn) OnStopping(from ID, cancelled map[uint64]CancelledTask) (err *ErrorInfo) {
	c.process.logging(LogLevel_Info, "MainFn: NOW EXITING!")

	c.mutex.Lock()
	if c.runnable == nil {
		c.mutex.Unlock()
		c.process.logging(LogLevel_Error, "MainFn: Runnable is not running")
		return
	}
	runnable := c.runnable
	c.runnable = nil
	c.mutex.Unlock()

	// Unload local elements and its atomos.
	c.unloadElement(runnable)
	//// After Runnable Script terminated.

	c.process.remotes.close()
	c.callChain = nil
	c.clientCert = nil
	c.listenCert = nil
	c.mainKillCh = nil
	c.elements = nil
	c.config = nil
	c.process = nil
	return nil
}

func (e *CosmosMainFn) pushReloadMail(from ID, impl *CosmosRunnable, reloads int) *ErrorInfo {
	return e.atomos.PushReloadMailAndWaitReply(from, impl, reloads)
}

func (c *CosmosMainFn) OnReloading(oldElement Atomos, reloadObject AtomosReloadable) (newElement Atomos) {
	// 如果没有新的Element，就用旧的Element。
	// Use old Element if there is no new Element.
	newElement = oldElement
	reload, ok := reloadObject.(*CosmosRunnable)
	if !ok || reload == nil {
		err := NewErrorf(ErrElementReloadInvalid, "Reload is invalid, reload=(%v),reloads=(%d)", reload, c.atomos.reloads)
		c.Log().Fatal(err.Message)
		return
	}
	c.process.logging(LogLevel_Info, "MainFn: Reload")
	c.mutex.Lock()
	if c.loading != nil {
		c.mutex.Unlock()
		err := NewError(ErrMainFnIsReloading, "MainFn: Is reloading")
		c.process.logging(LogLevel_Fatal, err.Message)
		return
	}
	c.mutex.Unlock()
	if errs := c.loadElementsTransaction(reload, true); len(errs) > 0 {
		c.loadElementsTransactionRollback(true, errs)
		err := NewErrorf(ErrMainFnReloadFailed, "MainFn: Reloading failed, errs=(%v)", errs)
		c.Log().Fatal(err.Message)
		return
	}
	c.loadElementsTransactionCommit(true)

	// Set loaded.
	c.mutex.Lock()
	c.runnable = c.loading
	c.loading = nil
	c.mutex.Unlock()

	// Load wormhole.
	c.daemon(true)

	// Push reload to atomos.
	for _, define := range c.runnable.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		c.mutex.RLock()
		elem, has := c.elements[name]
		c.mutex.RUnlock()
		if has {
			// 重载Element，需要指定一个版本的ElementImplementation。
			// Reload element, specific version of ElementImplementation is needed.
			elem.lock.Lock()
			atomNameList := make([]string, 0, elem.names.Len())
			for nameElem := elem.names.Front(); nameElem != nil; nameElem = nameElem.Next() {
				atomNameList = append(atomNameList, nameElem.Value.(string))
			}
			elem.lock.Unlock()
			wg := sync.WaitGroup{}
			for _, name := range atomNameList {
				wg.Add(1)
				go func(name string) {
					defer func() {
						wg.Done()
						if r := recover(); r != nil {
							elem.Log().Fatal("MainFn: Reloading atom PANIC, name=(%s),reason=(%s)", name, r)
						}
					}()
					elem.lock.Lock()
					atom, has := elem.atoms[name]
					elem.lock.Unlock()
					if !has {
						return
					}
					elem.Log().Info("MainFn: Reloading atomos, name=(%s)", name)
					err := atom.pushReloadMail(elem, elem.current, c.atomos.reloads)
					if err != nil {
						elem.Log().Error("MainFn: Reloading atomos failed, name=(%s),err=(%v)", name, err)
					}
				}(name)
			}
			wg.Wait()
			c.process.logging(LogLevel_Info, "MainFn: Push atomos reloaded, element=(%s)", name)
		}
	}

	// Execute reload script after reloading all elements and atomos.
	if err := func(runnable *CosmosRunnable) (err *ErrorInfo) {
		defer func() {
			if r := recover(); r != nil {
				err = NewErrorf(ErrMainFnReloadFailed, "MainFn: Reload script CRASH! reason=(%s)", r)
				c.Log().Fatal(err.Message)
			}
		}()
		c.process.logging(LogLevel_Info, "MainFn: NOW RELOADING!")
		runnable.reloadScript(c)
		return
	}(reload); err != nil {
		c.process.logging(LogLevel_Info, "MainFn: Reloading!")
		return
	}

	return
}

// Load elements.

func (c *CosmosMainFn) loadElementsTransaction(runnable *CosmosRunnable) { //, reload bool) (errs map[string]*ErrorInfo) {
	//errs = map[string]*ErrorInfo{}
	// Pre-initialize all local elements in the Runnable.
	for _, define := range runnable.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		// loadElement
		func(name string, define *ElementImplementation) {
			defer func() {
				if r := recover(); r != nil {
					c.Log().Fatal("MainFn: Check element panic, name=(%s),err=(%v),stack=(%s)",
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
			if !has {
				elem.initElementLocal(define, c.atomos.reloads)
			} else {
				elem.pushReloadMail(c, define, c.atomos.reloads)
			}
		}(name, define) //; err != nil {
		//	c.process.logging(LogLevel_Fatal, "MainFn: Load element failed, element=(%s),err=(%v)", name, err)
		//	errs[name] = err
		//	continue
		//}

		//// Add the element.
		c.process.logging(LogLevel_Info, "MainFn: Load element succeed, element=%s", name)
	}
	return
}

//func (c *CosmosMainFn) loadElementsTransactionRollback(isReload bool, errs map[string]*ErrorInfo) {
//	for _, define := range c.loading.implementOrder {
//		// Create local element.
//		name := define.Interface.Config.Name
//		c.mutex.RLock()
//		elem, has := c.elements[name]
//		c.mutex.RUnlock()
//		if has {
//			_, failed := errs[name]
//			elem.rollback(isReload, failed)
//			c.process.logging(LogLevel_Info, "MainFn: Rollback, element=(%s)", name)
//		}
//	}
//	c.mutex.Lock()
//	if !isReload {
//		c.runnable = nil
//	}
//	c.loading = nil
//	c.mutex.Unlock()
//}

func (c *CosmosMainFn) loadElementsTransactionCommit(isReload bool) {
	for _, define := range c.loading.implementOrder {
		// Create local element.
		name := define.Interface.Config.Name
		c.mutex.RLock()
		elem, has := c.elements[name]
		c.mutex.RUnlock()
		if has {
			elem.lock.Lock()
			elem.avail = true
			if isReload {
				elem.current = elem.reloading
				elem.reloading = nil
			}
			elem.lock.Unlock()
			c.process.logging(LogLevel_Info, "MainFn: Commit, element=(%s)", name)
		}
	}
}

// Get element

func (c *CosmosMainFn) getElement(name string) (elem *ElementLocal, err *ErrorInfo) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.runnable == nil || c.loading != nil {
		if c.runnable == nil {
			err = NewError(ErrMainFnRunnableNotFound, "MainFn: Runnable not found")
		} else {
			err = NewError(ErrMainFnIsReloading, "MainFn: Runnable reloading")
		}
		c.process.logging(LogLevel_Error, err.Message)
		return nil, err
	}
	elem, has := c.elements[name]
	if !has {
		err = NewErrorf(ErrMainFnElementNotFound, "MainFn: Local element not found, name=(%s)", name)
		return nil, err
	}
	if !elem.avail {
		err = NewErrorf(ErrMainFnElementIsInvalid, "MainFn: Local element is invalid, name=(%s)", name)
		c.process.logging(LogLevel_Error, err.Message)
		return nil, err
	}
	return elem, nil
}

//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//

// Loading

func (c *CosmosMainFn) startRemoteCosmosServer() {
	if server := c.remoteServer; server != nil {
		server.start()
	}
}

// Element Interface.

//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//

func (c *CosmosMainFn) daemon(isReload bool) {
	for name, elem := range c.elements {
		go func(n string, e *ElementLocal) {
			defer func() {
				if r := recover(); r != nil {
					c.process.logging(LogLevel_Fatal, "MainFn: Start running PANIC, name=(%s),err=(%v)", n, r)
				}
			}()
			if w, ok := e.current.Developer.(ElementStartRunning); ok {
				c.process.logging(LogLevel_Info, "MainFn: Start running, name=(%s)", n)
				w.StartRunning(isReload)
			}
		}(name, elem)
	}
}

func (c *CosmosMainFn) unloadElement(runnable *CosmosRunnable) {
	wg := sync.WaitGroup{}
	for i := len(runnable.implementOrder) - 1; i >= 0; i -= 1 {
		name := runnable.implementOrder[i].Interface.Config.Name
		c.mutex.RLock()
		elem, has := c.elements[name]
		c.mutex.RUnlock()
		if has {
			wg.Add(1)
			go func(name string) {
				defer func() {
					c.mutex.Lock()
					delete(c.elements, name)
					c.mutex.Unlock()
					wg.Done()
					if r := recover(); r != nil {
						c.process.logging(LogLevel_Fatal, "MainFn: Close element PANIC, name=(%s),reason=(%s)", name, r)
					}
				}()
				elem.lock.Lock()
				atomNameList := make([]string, 0, elem.names.Len())
				for nameElem := elem.names.Front(); nameElem != nil; nameElem = nameElem.Next() {
					atomNameList = append(atomNameList, nameElem.Value.(string))
				}
				elem.lock.Unlock()
				wg := sync.WaitGroup{}
				for _, name := range atomNameList {
					wg.Add(1)
					go func(name string) {
						defer func() {
							wg.Done()
							if r := recover(); r != nil {
								elem.Log().Fatal("Element.Unload: Panic, name=%s,reason=%s", name, r)
							}
						}()
						elem.lock.Lock()
						atom, has := elem.atoms[name]
						elem.lock.Unlock()
						if !has {
							return
						}
						elem.Log().Info("Element.Unload: Kill atomos, name=%s", name)
						err := atom.pushKillMail(elem, true)
						if err != nil {
							elem.Log().Error("Element.Unload: Kill atomos error, name=%s,err=%v", name, err)
						}
					}(name)
				}
				wg.Wait()
				c.process.logging(LogLevel_Info, "MainFn: Closed, element=(%s)", name)
			}(name)
		}
	}
	wg.Wait()
	c.process.logging(LogLevel_Info, "MainFn: Closed")
}
