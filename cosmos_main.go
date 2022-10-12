package go_atomos

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"google.golang.org/protobuf/proto"
	"io/ioutil"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

type CosmosMain struct {
	process *CosmosProcess

	// Config
	runnable          *CosmosRunnable
	waitProcessExitCh chan bool

	atomos *BaseAtomos

	// Elements
	elements map[string]*ElementLocal
	mutex    sync.RWMutex

	// TLS if exists
	listenCert *tls.Config
	clientCert *tls.Config
	//// Cosmos Server
	//remoteServer *cosmosRemoteServer

	// 调用链
	// 调用链用于检测是否有循环调用，在处理message时把fromID的调用链加上自己之后
	callChain []ID
}

// 生命周期相关
// Life cycle

func newCosmosMain(process *CosmosProcess, runnable *CosmosRunnable) *CosmosMain {
	id := &IDInfo{
		Type:    IDType_Main,
		Cosmos:  runnable.config.Node,
		Element: MainElementName,
		Atomos:  "",
	}
	mainFn := &CosmosMain{
		process:           process,
		runnable:          nil,
		atomos:            nil,
		waitProcessExitCh: make(chan bool, 1),
		elements:          make(map[string]*ElementLocal, len(runnable.implements)),
		mutex:             sync.RWMutex{},
		listenCert:        nil,
		clientCert:        nil,
		callChain:         nil,
	}
	mainFn.atomos = NewBaseAtomos(id, process.sharedLog, runnable.mainLogLevel, mainFn, mainFn, 0)
	return mainFn
}

// Load Runnable

func (c *CosmosMain) onceLoad(runnable *CosmosRunnable) *ErrorInfo {
	_, err := c.tryLoadRunnable(runnable)
	if err != nil {
		c.Log().Fatal("CosmosMain: Once load failed, err=(%v)", err)
		return err
	}
	return nil
}

func (c *CosmosMain) tryLoadRunnable(newRunnable *CosmosRunnable) (*runnableLoadingHelper, *ErrorInfo) {
	oldRunnable := c.runnable
	c.runnable = newRunnable
	helper, err := c.newRunnableLoadingHelper(oldRunnable, newRunnable)
	if err != nil {
		c.runnable = oldRunnable
		c.Log().Fatal(err.Message)
		return nil, err
	}

	if err = c.trySpawningElements(helper); err != nil {
		c.runnable = oldRunnable
		c.Log().Fatal(err.Message)
		return nil, err
	}

	// NOTICE: Spawning might fail, it might cause reloads count increase, but actually reload failed. TODO
	// NOTICE: After spawning, reload won't stop, no matter what error happens.

	c.listenCert = helper.listenCert
	c.clientCert = helper.clientCert
	//c.remote = helper.remote
	return helper, nil
}

// Implementation of ID

func (c *CosmosMain) GetIDInfo() *IDInfo {
	if c == nil {
		return nil
	}
	return c.atomos.GetIDInfo()
}

func (c *CosmosMain) String() string {
	return c.atomos.Description()
}

func (c *CosmosMain) Release() {
}

func (c *CosmosMain) Cosmos() CosmosNode {
	return c
}

func (c *CosmosMain) Element() Element {
	return nil
}

func (c *CosmosMain) GetName() string {
	return c.GetIDInfo().Cosmos
}

func (c *CosmosMain) State() AtomosState {
	return c.atomos.state
}

func (c *CosmosMain) IdleDuration() time.Duration {
	if c.atomos.state != AtomosWaiting {
		return 0
	}
	return time.Now().Sub(c.atomos.lastWait)
}

//func (c *CosmosMain) MessageByName(from ID, name string, buf []byte, protoOrJSON bool) ([]byte, *ErrorInfo) {
//	return nil, NewError(ErrMainCannotMessage, "Cannot message main")
//}

func (c *CosmosMain) MessageByName(from ID, name string, in proto.Message) (proto.Message, *ErrorInfo) {
	return nil, NewError(ErrMainCannotMessage, "Cannot message main")
}

func (c *CosmosMain) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	return nil, nil
}

func (c *CosmosMain) Kill(from ID) *ErrorInfo {
	return NewError(ErrMainCannotKill, "Cannot kill main")
}

func (c *CosmosMain) SendWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo {
	return c.atomos.PushWormholeMailAndWaitReply(from, wormhole)
}

func (c *CosmosMain) getCallChain() []ID {
	return c.callChain
}

func (c *CosmosMain) getElementLocal() *ElementLocal {
	return nil
}

func (c *CosmosMain) getAtomLocal() *AtomLocal {
	return nil
}

// Implementation of atomos.SelfID
// Implementation of atomos.ParallelSelf
//
// SelfID，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// SelfID, a concept that provide Atom resource access to inner Atom.
// With SelfID, Atom can access its self-main with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provides Log and Tasks method to inner Atom.

func (c *CosmosMain) CosmosMain() *CosmosMain {
	return c
}

//func (e *CosmosMain) ElementLocal() *ElementLocal {
//	return e
//}

// KillSelf
// Atom kill itself from inner
func (c *CosmosMain) KillSelf() {
	if err := c.pushKillMail(c, false); err != nil {
		c.Log().Error("KillSelf error, err=%v", err)
		return
	}
	c.Log().Info("KillSelf")
}

func (c *CosmosMain) Parallel(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				c.Log().Fatal("Parallel PANIC, stack=(%s)", string(stack))
			}
		}()
		fn()
	}()
}

func (c *CosmosMain) Config() map[string]string {
	return c.runnable.config.Customize
}

func (c *CosmosMain) MessageSelfByName(from ID, name string, buf []byte, protoOrJSON bool) ([]byte, *ErrorInfo) {
	return nil, NewError(ErrMainCannotMessage, "CosmosMain: Cannot message.")
}

func (c *CosmosMain) OnScaling(from ID, name string, args proto.Message) (id ID, err *ErrorInfo) {
	return nil, NewError(ErrMainCannotScale, "CosmosMain: Cannot scale.")
}

// Check chain.

func (c *CosmosMain) checkCallChain(fromIDList []ID) bool {
	for _, fromID := range fromIDList {
		if fromID.GetIDInfo().IsEqual(c.GetIDInfo()) {
			return false
		}
	}
	return true
}

func (c *CosmosMain) addCallChain(fromIDList []ID) {
	c.callChain = append(fromIDList, c)
}

func (c *CosmosMain) delCallChain() {
	c.callChain = nil
}

// Implementation of CosmosNode

func (c *CosmosMain) GetNodeName() string {
	return c.GetIDInfo().Cosmos
}

func (c *CosmosMain) CosmosIsLocal() bool {
	return true
}

func (c *CosmosMain) CosmosGetElementID(elem string) (ID, *ErrorInfo) {
	return c.getElement(elem)
}

func (c *CosmosMain) CosmosGetElementAtomID(elem, name string) (ID, *ErrorInfo) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, err
	}
	return element.GetAtomID(name)
}

func (c *CosmosMain) CosmosSpawnElementAtom(elem, name string, arg proto.Message) (ID, *ErrorInfo) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, err
	}
	return element.SpawnAtom(name, arg)
}

func (c *CosmosMain) CosmosMessageElement(fromID, toID ID, message string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	return toID.Element().MessageElement(fromID, toID, message, args)
}

func (c *CosmosMain) CosmosMessageAtom(fromID, toID ID, message string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	return toID.Element().MessageAtom(fromID, toID, message, args)
}

func (c *CosmosMain) CosmosScaleElementGetAtomID(fromID ID, elem, message string, args proto.Message) (ID ID, err *ErrorInfo) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, err
	}
	return element.ScaleGetAtomID(fromID, message, args)
}

// Implementation of AtomosUtilities

func (c *CosmosMain) Log() Logging {
	return c.atomos.Log()
}

func (c *CosmosMain) Task() Task {
	return c.atomos.Task()
}

// Main as an Atomos

func (c *CosmosMain) Description() string {
	return c.atomos.String()
}

func (c *CosmosMain) Halt(from ID, cancels map[uint64]CancelledTask) (save bool, data proto.Message) {
	c.Log().Fatal("CosmosMain: Halt of CosmosMain should not be called.")
	return false, nil
}

func (c *CosmosMain) Reload(newInstance Atomos) {
	c.Log().Fatal("CosmosMain: Reload of CosmosMain should not be called.")
}

// 内部实现
// INTERNAL

// 邮箱控制器相关
// Mailbox Handler
// TODO: Performance tracer.

//func (e *CosmosMain) pushMessageMail(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
//	return e.atomos.PushMessageMailAndWaitReply(from, name, args)
//}

func (c *CosmosMain) OnMessaging(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	return nil, NewError(ErrMainCannotMessage, "CosmosMain: Cannot send message.")
}

func (c *CosmosMain) pushKillMail(from ID, wait bool) *ErrorInfo {
	return c.atomos.PushKillMailAndWaitReply(from, wait)
}

func (c *CosmosMain) OnStopping(from ID, cancelled map[uint64]CancelledTask) (err *ErrorInfo) {
	c.Log().Info("CosmosMain: NOW EXITING!")

	// Unload local elements and its atomos.
	for i := len(c.runnable.implementOrder) - 1; i >= 0; i -= 1 {
		name := c.runnable.implementOrder[i].Interface.Config.Name
		elem, has := c.elements[name]
		if !has {
			continue
		}
		err := elem.pushKillMail(c, true)
		if err != nil {
			c.Log().Error("CosmosMain: Exiting kill element error, element=(%s),err=(%v)", name, err.Message)
		}
	}

	//// After Runnable Script terminated.

	//c.process.remotes.close()
	c.callChain = nil
	c.clientCert = nil
	c.listenCert = nil
	c.waitProcessExitCh = nil
	c.elements = nil
	c.runnable = nil
	c.process = nil
	return nil
}

func (c *CosmosMain) pushReloadMail(from ID, impl *CosmosRunnable, reloads int) *ErrorInfo {
	return c.atomos.PushReloadMailAndWaitReply(from, impl, reloads)
}

func (c *CosmosMain) OnReloading(oldElement Atomos, reloadObject AtomosReloadable) (newElement Atomos) {
	// Actually, atomos of CosmosMain has nothing to change.
	// Here will return oldElement directly, and OnReload function of CosmosMain won't be executed.
	newElement = oldElement
	// 如果没有新的Element，就用旧的Element。
	// Use old Element if there is no new Element.
	newRunnable, ok := reloadObject.(*CosmosRunnable)
	if !ok || newRunnable == nil {
		err := NewErrorf(ErrElementReloadInvalid, "Reload is invalid, newRunnable=(%v),reloads=(%d)", newRunnable, c.atomos.reloads)
		c.Log().Fatal(err.Message)
		return
	}
	c.Log().Info("CosmosMain: NOW RELOADING!")

	helper, err := c.tryLoadRunnable(newRunnable)
	if err != nil {
		c.Log().Info("CosmosMain: Reload failed, err=(%v)", err)
		return
	}

	// Reload
	for _, define := range helper.reloadElement {
		// Create local element.
		name := define.Interface.Config.Name
		c.mutex.RLock()
		elem, has := c.elements[name]
		c.mutex.RUnlock()
		if !has {
			c.Log().Fatal("CosmosMain: Reload is reloading not exists element, name=(%s)", name)
			continue
		}
		err := elem.pushReloadMail(c, define, c.atomos.reloads)
		if err != nil {
			c.Log().Fatal("CosmosMain: Reload sent reload failed, name=(%s),err=(%v)", name, err)
		}
	}

	// Halt
	for _, define := range helper.delElement {
		// Create local element.
		name := define.Interface.Config.Name
		c.mutex.RLock()
		elem, has := c.elements[name]
		c.mutex.RUnlock()
		if !has {
			c.Log().Fatal("CosmosMain: Reload is halting not exists element, name=(%s)", name)
			continue
		}
		err := elem.pushKillMail(c, true)
		if err != nil {
			c.Log().Fatal("CosmosMain: Reload sent halt failed, name=(%s),err=(%v)", name, err)
		}
		c.mutex.Lock()
		delete(c.elements, name)
		c.mutex.Unlock()
	}

	// Execute newRunnable script after reloading all elements and atomos.
	if err := func(runnable *CosmosRunnable) (err *ErrorInfo) {
		defer func() {
			if r := recover(); r != nil {
				err = NewErrorf(ErrMainReloadFailed, "CosmosMain: Reload script CRASH! reason=(%s)", r)
				c.Log().Fatal(err.Message)
			}
		}()
		runnable.reloadScript(c)
		return
	}(newRunnable); err != nil {
		c.Log().Fatal("CosmosMain: Reload failed, err=(%v)", err)
	}

	c.Log().Info("CosmosMain: RELOADED!")
	return
}

func (c *CosmosMain) OnWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo {
	holder, ok := c.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		err := NewErrorf(ErrAtomosNotSupportWormhole, "CosmosCosmosMain: Not supported wormhole, type=(%T)", c.atomos.instance)
		c.Log().Error(err.Message)
		return err
	}
	return holder.AcceptWormhole(from, wormhole)
}

// Element Inner

func (c *CosmosMain) getElement(name string) (elem *ElementLocal, err *ErrorInfo) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.runnable == nil {
		err = NewError(ErrMainRunnableNotFound, "CosmosMain: It's not running.")
		return nil, err
	}
	elem, has := c.elements[name]
	if !has {
		err = NewErrorf(ErrMainElementNotFound, "CosmosMain: Local element not found, name=(%s)", name)
		return nil, err
	}
	return elem, nil
}

func (c *CosmosMain) trySpawningElements(helper *runnableLoadingHelper) (err *ErrorInfo) {
	// Spawn
	// TODO 有个问题，如果这里的Spawn逻辑需要用到新的helper里面的配置，那就会有问题，所以Spawn尽量不要做对其它Cosmos的操作，延后到Script。
	var loaded []*ElementLocal
	for _, impl := range helper.spawnElement {
		elem, e := c.cosmosElementSpawn(helper.newRunnable, impl)
		if e != nil {
			err = e
			c.Log().Fatal("CosmosMain: Spawning element failed, name=(%s),err=(%v)", impl.Interface.Config.Name, err)
			break
		}
		loaded = append(loaded, elem)
	}
	if err != nil {
		for _, elem := range loaded {
			if err := elem.pushKillMail(c, true); err != nil {
				c.Log().Fatal("CosmosMain: Spawning element failed, kill error, name=(%s),err=(%v)", elem.GetName(), err)
			}
		}
		c.Log().Fatal("CosmosMain: Spawning element has rollback")
		return
	}
	for _, elem := range loaded {
		s, ok := elem.atomos.instance.(ElementCustomizeStartRunning)
		if !ok || s == nil {
			continue
		}
		// TODO
		go func(s ElementCustomizeStartRunning) {
			defer func() {
				if r := recover(); r != nil {
					err = NewErrorf(ErrMainStartRunningPanic, "CosmosMain: Running PANIC! reason=(%s)", r)
					c.Log().Fatal(err.Message)
				}
			}()
			s.StartRunning()
		}(s)
	}
	return nil
}

func (c *CosmosMain) cosmosElementSpawn(r *CosmosRunnable, i *ElementImplementation) (elem *ElementLocal, err *ErrorInfo) {
	name := i.Interface.Config.Name
	defer func() {
		//var stack []byte
		if re := recover(); re != nil {
			_, file, line, ok := runtime.Caller(4)
			if !ok {
				file, line = "???", 0
			}
			if err == nil {
				err = NewErrorf(ErrFrameworkPanic, "SpawnElement, Recover from panic, reason=(%s),file=(%s),line=(%d)", re, file, line)
			}
			err.AddStack(c, file, fmt.Sprintf("%v", re), line, nil)
		}
		//if len(stack) != 0 {
		//	err = NewErrorf(ErrFrameworkPanic, "Spawn new element PANIC, name=(%s),err=(%v)", name, r).
		//		AddStack(c.GetIDInfo(), stack)
		//} else if err != nil && len(err.Stacks) > 0 {
		//	err = err.AddStack(c.GetIDInfo(), debug.Stack())
		//}
		//if err != nil {
		//	c.Log().Fatal("CosmosMain: %s, err=(%v)", name, err)
		//}
	}()

	elem = newElementLocal(c, r, i)

	c.mutex.Lock()
	_, has := c.elements[name]
	if !has {
		c.elements[name] = elem
	}
	c.mutex.Unlock()
	if has {
		return nil, NewErrorf(ErrElementLoaded, "ElementSpawner: Element exists, name=(%s)", name)
	}

	elem.atomos.setSpawning()
	err = elem.cosmosElementSpawn(r, i)
	if err != nil {
		elem.atomos.setHalt()
		c.mutex.Lock()
		delete(c.elements, name)
		c.mutex.Unlock()
		return nil, err
	}
	elem.atomos.setWaiting()
	return elem, nil
}

// Runnable Loading Helper

type runnableLoadingHelper struct {
	newRunnable *CosmosRunnable

	spawnElement, reloadElement, delElement []*ElementImplementation

	// TLS if exists
	listenCert, clientCert *tls.Config
	//// Cosmos Server
	//remoteServer *cosmosRemoteServer
}

func (c *CosmosMain) newRunnableLoadingHelper(oldRunnable, newRunnable *CosmosRunnable) (*runnableLoadingHelper, *ErrorInfo) {
	helper := &runnableLoadingHelper{
		newRunnable:   newRunnable,
		spawnElement:  nil,
		reloadElement: nil,
		delElement:    nil,
		listenCert:    c.listenCert,
		clientCert:    c.clientCert,
	}

	// Element
	if oldRunnable == nil {
		// All are cosmosElementSpawn elements.
		for _, impl := range newRunnable.implementOrder {
			helper.spawnElement = append(helper.spawnElement, impl)
		}
	} else {
		for _, newImpl := range newRunnable.implementOrder {
			_, has := oldRunnable.implements[newImpl.Interface.Config.Name]
			if has {
				helper.reloadElement = append(helper.reloadElement, newImpl)
			} else {
				helper.spawnElement = append(helper.spawnElement, newImpl)
			}
		}
		for _, oldImpl := range oldRunnable.implementOrder {
			_, has := newRunnable.implements[oldImpl.Interface.Config.Name]
			if !has {
				helper.delElement = append(helper.delElement, oldImpl)
			}
		}
	}

	// 加载TLS Cosmos Node支持，用于加密链接。
	// loadTlsCosmosNodeSupport()
	// Check enable Cert.
	if cert := newRunnable.config.UseCert; cert != nil {
		if cert.CertPath == "" {
			return nil, NewError(ErrCosmosCertConfigInvalid, "CosmosMain: Cert path is empty")
		}
		if cert.KeyPath == "" {
			return nil, NewError(ErrCosmosCertConfigInvalid, "CosmosMain: Key path is empty")
		}
		// Load Key Pair.
		c.Log().Info("CosmosMain: Enabling Cert, cert=(%s),key=(%s)", cert.CertPath, cert.KeyPath)
		pair, e := tls.LoadX509KeyPair(cert.CertPath, cert.KeyPath)
		if e != nil {
			err := NewErrorf(ErrMainLoadCertFailed, "CosmosMain: Load Key Pair failed, err=(%v)", e)
			c.Log().Fatal(err.Message)
			return nil, err
		}
		helper.listenCert = &tls.Config{
			Certificates: []tls.Certificate{
				pair,
			},
		}
		// Load Cert.
		caCert, e := ioutil.ReadFile(cert.CertPath)
		if e != nil {
			return nil, NewErrorf(ErrCosmosCertConfigInvalid, "CosmosMain: Cert file read error, err=(%v)", e)
		}
		tlsConfig := &tls.Config{}
		if cert.InsecureSkipVerify {
			tlsConfig.InsecureSkipVerify = true
			helper.clientCert = tlsConfig
		} else {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			// Create TLS configuration with the certificate of the server.
			tlsConfig.RootCAs = caCertPool
			helper.clientCert = tlsConfig
		}
	} else {
		helper.listenCert = nil
		helper.clientCert = nil
	}

	// 加载远端Cosmos服务支持。
	// loadRemoteCosmosServerSupport
	if server := newRunnable.config.EnableServer; server != nil {
		// Enable Server
		c.Log().Info("CosmosMain: Enable Server, host=(%s),port=(%d)", server.Host, server.Port)
		if oldRunnable != nil && oldRunnable.config.EnableServer.IsEqual(newRunnable.config.EnableServer) {
			goto noServerReconnect
		}
		//c.remoteServer = newCosmosRemoteHelper(a)
		//if err := c.remoteServer.init(); err != nil {
		//	return nil, err
		//}
	noServerReconnect:
		// TODO: UpdateLogic
	}

	return helper, nil
}
