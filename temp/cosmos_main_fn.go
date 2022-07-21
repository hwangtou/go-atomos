package temp

// CHECKED!

import (
	"crypto/tls"
	"sync"
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

//// 初始化Runnable。
//// Initial Runnable.
//func (c *CosmosMainFn) initCosmosMainFn(conf *Config, runnable *CosmosRunnable) *ErrorInfo {
//
//	// 事务式加载Elements。
//	c.loadElementsTransaction(runnable) //, false); len(errs) > 0 {
//	//	//c.loadElementsTransactionRollback(false, errs)
//	//	return NewErrorf(ErrMainFnCheckElementFailed, "MainFn: Check element failed, errs=(%v)", errs)
//	//}
//	c.loadElementsTransactionCommit(false)
//	c.listenCert, c.clientCert = listenCert, clientCert
//	c.startRemoteCosmosServer()
//
//	// Set loaded.
//	c.mutex.Lock()
//	c.runnable = c.loading
//	c.loading = nil
//	c.mutex.Unlock()
//
//	// Send spawn.
//	for _, impl := range runnable.implementOrder {
//		name := impl.Interface.Config.Name
//		c.mutex.RLock()
//		elem, has := c.elements[name]
//		c.mutex.RUnlock()
//		if !has {
//			continue
//		}
//		go func(n string, i *ElementImplementation, e *ElementLocal) {
//			defer func() {
//				if r := recover(); r != nil {
//					c.process.logging(LogLevel_Fatal, "MainFn: Start running PANIC, name=(%s),err=(%v)", n, r)
//				}
//			}()
//			if err := i.Interface.ElementSpawner(a, a.atomos.instance, arg, data); err != nil {
//				return err
//			}
//			if w, ok := e.current.Developer.(ElementStartRunning); ok {
//				c.process.logging(LogLevel_Info, "MainFn: Start running, name=(%s)", n)
//				w.StartRunning(isReload)
//			}
//		}(name, impl, elem)
//	}
//	return nil
//}

func (c *CosmosMainFn) deleteCosmosMainFn() {
}

//// Load elements.
//
//func (c *CosmosMainFn) loadElementsTransaction(runnable *CosmosRunnable) { //, reload bool) (errs map[string]*ErrorInfo) {
//	//errs = map[string]*ErrorInfo{}
//	// Pre-initialize all local elements in the Runnable.
//	for _, define := range runnable.implementOrder {
//		// Create local element.
//		name := define.Interface.Config.Name
//		// loadElement
//		func(name string, define *ElementImplementation) {
//			defer func() {
//				if r := recover(); r != nil {
//					c.Log().Fatal("MainFn: Check element panic, name=(%s),err=(%v),stack=(%s)",
//						name, r, string(debug.Stack()))
//				}
//			}()
//			c.mutex.Lock()
//			elem, has := c.elements[name]
//			if !has {
//				elem = newElementLocal(c, define)
//				c.elements[name] = elem
//			}
//			c.mutex.Unlock()
//			if !has {
//				elem.initElementLocal(define, c.atomos.reloads)
//			} else {
//				elem.pushReloadMail(c, define, c.atomos.reloads)
//			}
//		}(name, define) //; err != nil {
//		//	c.process.logging(LogLevel_Fatal, "MainFn: Load element failed, element=(%s),err=(%v)", name, err)
//		//	errs[name] = err
//		//	continue
//		//}
//
//		//// Add the element.
//		c.process.logging(LogLevel_Info, "MainFn: Load element succeed, element=%s", name)
//	}
//	return
//}

// Loading

func (c *CosmosMainFn) startRemoteCosmosServer() {
	if server := c.remoteServer; server != nil {
		server.start()
	}
}
