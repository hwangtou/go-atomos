package go_atomos

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"io/ioutil"
	"net"
	"net/http"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"time"
)

type CosmosMain struct {
	process *CosmosProcess

	// Config
	runnable   *CosmosRunnable
	mainKillCh chan bool

	atomos *BaseAtomos

	// Elements
	elements map[string]*ElementLocal
	mutex    sync.RWMutex

	// TLS if exists
	listenCert *tls.Config
	clientCert *tls.Config

	// Cosmos Server
	server       *http.Server
	listener     net.Listener
	remotes      map[string]*CosmosRemote
	remotesMutex sync.RWMutex

	//remoteServer *cosmosRemoteServer

	// 调用链
	// 调用链用于检测是否有循环调用，在处理message时把fromID的调用链加上自己之后
	callChain []ID
}

// 生命周期相关
// Life cycle

func newCosmosMain(process *CosmosProcess, runnable *CosmosRunnable) *CosmosMain {
	process.sharedLog.PushProcessLog(LogLevel_Info, "Main: Loading")
	id := &IDInfo{
		Type:    IDType_Main,
		Cosmos:  runnable.config.Node,
		Element: MainElementName,
		Atomos:  "",
	}
	mainFn := &CosmosMain{
		process:    process,
		runnable:   nil,
		atomos:     nil,
		mainKillCh: make(chan bool),
		elements:   make(map[string]*ElementLocal, len(runnable.implements)),
		mutex:      sync.RWMutex{},
		listenCert: nil,
		clientCert: nil,
		//callChain:  nil,
	}
	mainFn.atomos = NewBaseAtomos(id, process.sharedLog, runnable.mainLogLevel, mainFn, mainFn, 0)
	return mainFn
}

// Load Runnable

func (c *CosmosMain) onceLoad(runnable *CosmosRunnable) *ErrorInfo {
	_, err := c.tryLoadRunnable(runnable)
	if err != nil {
		c.Log().Fatal("Main: Once load failed, err=(%v)", err)
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
		return nil, err.AutoStack(nil, nil)
	}

	// NOTICE: Spawning might fail, it might cause reloads count increase, but actually reload failed. TODO
	// NOTICE: After spawning, reload won't stop, no matter what error happens.

	c.listenCert = helper.listenCert
	c.clientCert = helper.clientCert
	c.remotes = map[string]*CosmosRemote{}
	c.remotesMutex = sync.RWMutex{}
	if err = c.listen(); err != nil {
		return nil, err.AutoStack(nil, nil)
	}
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
	//if c.atomos.state != AtomosWaiting {
	//	return 0
	//}
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

func (c *CosmosMain) getElementRemote() *ElementRemote {
	return nil
}

func (c *CosmosMain) getAtomRemote() *AtomRemote {
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
	return nil, NewError(ErrMainCannotMessage, "Main: Cannot message.")
}

func (c *CosmosMain) OnScaling(from ID, name string, args proto.Message) (id ID, err *ErrorInfo) {
	return nil, NewError(ErrMainCannotScale, "Main: Cannot scale.")
}

// Check chain.

func (c *CosmosMain) checkCallChain(fromIDList []ID) bool {
	//for _, fromID := range fromIDList {
	//	if fromID.GetIDInfo().IsEqual(c.GetIDInfo()) {
	//		return false
	//	}
	//}
	//return true
	panic("not supported")
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
	c.Log().Fatal("Main: Halt of CosmosMain should not be called.")
	return false, nil
}

func (c *CosmosMain) Reload(newInstance Atomos) {
	c.Log().Fatal("Main: Reload of CosmosMain should not be called.")
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
	return nil, NewError(ErrMainCannotMessage, "Main: Cannot send message.")
}

func (c *CosmosMain) pushKillMail(from ID, wait bool) *ErrorInfo {
	return c.atomos.PushKillMailAndWaitReply(from, wait)
}

func (c *CosmosMain) OnStopping(from ID, cancelled map[uint64]CancelledTask) (err *ErrorInfo) {
	c.Log().Info("Main: NOW EXITING!")

	// Unload local elements and its atomos.
	for i := len(c.runnable.implementOrder) - 1; i >= 0; i -= 1 {
		name := c.runnable.implementOrder[i].Interface.Config.Name
		elem, has := c.elements[name]
		if !has {
			continue
		}
		err := elem.pushKillMail(c, true)
		if err != nil {
			c.Log().Error("Main: Exiting kill element error, element=(%s),err=(%v)", name, err.Message)
		}
	}

	//// After Runnable Script terminated.

	//c.process.remotes.close()
	c.callChain = nil
	c.clientCert = nil
	c.listenCert = nil
	c.mainKillCh = nil
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
	c.Log().Info("Main: NOW RELOADING!")

	helper, err := c.tryLoadRunnable(newRunnable)
	if err != nil {
		c.Log().Info("Main: Reload failed, err=(%v)", err)
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
			c.Log().Fatal("Main: Reload is reloading not exists element, name=(%s)", name)
			continue
		}
		err := elem.pushReloadMail(c, define, c.atomos.reloads)
		if err != nil {
			c.Log().Fatal("Main: Reload sent reload failed, name=(%s),err=(%v)", name, err)
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
			c.Log().Fatal("Main: Reload is halting not exists element, name=(%s)", name)
			continue
		}
		err := elem.pushKillMail(c, true)
		if err != nil {
			c.Log().Fatal("Main: Reload sent halt failed, name=(%s),err=(%v)", name, err)
		}
		c.mutex.Lock()
		delete(c.elements, name)
		c.mutex.Unlock()
	}

	// Execute newRunnable script after reloading all elements and atomos.
	if err := func(runnable *CosmosRunnable) (err *ErrorInfo) {
		defer func() {
			if r := recover(); r != nil {
				err = NewErrorf(ErrMainReloadFailed, "Main: Reload script CRASH! reason=(%s)", r)
				c.Log().Fatal(err.Message)
			}
		}()
		runnable.reloadScript(c)
		return
	}(newRunnable); err != nil {
		c.Log().Fatal("Main: Reload failed, err=(%v)", err)
	}

	c.Log().Info("Main: RELOADED!")
	return
}

func (c *CosmosMain) OnWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo {
	holder, ok := c.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		err := NewErrorf(ErrAtomosNotSupportWormhole, "CosmosMain: Not supported wormhole, type=(%T)", c.atomos.instance)
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
		err = NewError(ErrMainRunnableNotFound, "Main: It's not running.")
		return nil, err
	}
	elem, has := c.elements[name]
	if !has {
		err = NewErrorf(ErrMainElementNotFound, "Main: Local element not found, name=(%s)", name)
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
			c.Log().Fatal("Main: Spawning element failed, name=(%s),err=(%v)", impl.Interface.Config.Name, err)
			break
		}
		loaded = append(loaded, elem)
	}
	if err != nil {
		for _, elem := range loaded {
			if err := elem.pushKillMail(c, true); err != nil {
				c.Log().Fatal("Main: Spawning element failed, kill error, name=(%s),err=(%v)", elem.GetName(), err)
			}
		}
		c.Log().Fatal("Main: Spawning element has rollback")
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
					err = NewErrorf(ErrMainStartRunningPanic, "Main: Running PANIC! reason=(%s)", r)
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
		//	c.Log().Fatal("Main: %s, err=(%v)", name, err)
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
	if cert := newRunnable.config.EnableCert; cert != nil {
		if cert.CertPath == "" {
			return nil, NewError(ErrCosmosCertConfigInvalid, "Main: Cert path is empty")
		}
		if cert.KeyPath == "" {
			return nil, NewError(ErrCosmosCertConfigInvalid, "Main: Key path is empty")
		}
		// Load Key Pair.
		c.Log().Info("Main: Enabling Cert, cert=(%s),key=(%s)", cert.CertPath, cert.KeyPath)
		pair, e := tls.LoadX509KeyPair(cert.CertPath, cert.KeyPath)
		if e != nil {
			err := NewErrorf(ErrMainLoadCertFailed, "Main: Load Key Pair failed, err=(%v)", e)
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
			return nil, NewErrorf(ErrCosmosCertConfigInvalid, "Main: Cert file read error, err=(%v)", e)
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
		c.Log().Info("Main: Enable Server, host=(%s),port=(%d)", server.Host, server.Port)
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

// Remote

func (c *CosmosMain) listen() *ErrorInfo {
	if c.runnable == nil {
		return nil
	}
	config := c.runnable.config
	if config.EnableCert == nil || config.EnableServer == nil {
		return nil
	}
	// Http
	host := config.EnableServer.Host
	port := config.EnableServer.Port
	listenAddr := fmt.Sprintf("%s:%d", host, port)

	mux := http.NewServeMux()
	mux.HandleFunc(RemoteAtomosConnect, c.remoteConnect)
	mux.HandleFunc(RemoteAtomosElementMessaging, c.remoteElementMessaging)
	mux.HandleFunc(RemoteAtomosElementScaling, c.remoteElementScaling)
	mux.HandleFunc(RemoteAtomosGetAtom, c.remoteGetAtom)
	mux.HandleFunc(RemoteAtomosAtomMessaging, c.remoteMessageAtom)

	c.server = &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}
	if er := http2.ConfigureServer(c.server, &http2.Server{}); er != nil {
		err := NewErrorf(ErrRemoteListenerLoad, "Loading server failed, err=(%v)", er).AutoStack(nil, nil)
		return err.AutoStack(nil, nil)
	}

	// Listen the local port.
	cert, er := tls.LoadX509KeyPair(config.EnableCert.CertPath, config.EnableCert.KeyPath)
	if er != nil {
		return NewErrorf(ErrCosmosConfigInvalid, "cert invalid, err=(%v)", er).AutoStack(nil, nil)
	}
	// Listen the local port.
	listener, er := tls.Listen("tcp", listenAddr, &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	if er != nil {
		err := NewErrorf(ErrRemoteListenerListen, "Loading server failed, err=(%v)", er).AutoStack(nil, nil)
		return err.AutoStack(nil, nil)
	}
	c.listener = listener
	go c.server.Serve(c.listener)
	return nil
}

func (c *CosmosMain) GetRemote(addr string) *CosmosRemote {
	c.remotesMutex.RLock()
	remote, has := c.remotes[addr]
	c.remotesMutex.RUnlock()
	if has {
		if err := remote.connect(); err != nil {
			// TODO
		}
		return remote
	}

	c.remotesMutex.Lock()
	remote = newCosmosRemote(c, addr)
	c.remotes[addr] = remote
	c.remotesMutex.Unlock()

	if err := remote.connect(); err != nil {
		// TODO
	}

	return remote
}

func (c *CosmosMain) remoteConnect(writer http.ResponseWriter, request *http.Request) {
	info := &CosmosRemoteInfo{
		Id:       c.GetIDInfo(),
		Elements: nil,
		Changed:  0,
	}
	c.mutex.RLock()
	for name, elem := range c.elements {
		info.Elements[name] = &ElementRemoteInfo{
			Id: elem.GetIDInfo(),
		}
	}
	c.mutex.RUnlock()

	rspBuf, er := proto.Marshal(info)
	if er != nil {
		// TODO
		return
	}
	_, er = writer.Write(rspBuf)
	if er != nil {
		// TODO
		return
	}
}

func (c *CosmosMain) remoteElementMessaging(writer http.ResponseWriter, request *http.Request) {
	req := &CosmosRemoteMessagingReq{}
	reqBuf, er := ioutil.ReadAll(request.Body)
	if er != nil {
		// TODO
		return
	}
	er = proto.Unmarshal(reqBuf, req)
	if er != nil {
		// TODO
		return
	}

	// FromID
	fromID, err := c.getOrCreateRemoteID(req.From, req.FromAddr)
	if err != nil {
		// TODO
		return
	}

	// ToID
	element, err := c.remoteGetElementID(req.To)
	if err != nil {
		// TODO
		return
	}

	// Args
	args, er := req.Args.UnmarshalNew()
	if er != nil {
		// TODO
		return
	}

	rsp := &CosmosRemoteMessagingRsp{}
	reply, err := element.MessageElement(fromID, element, req.Message, args)
	rsp.Reply, er = anypb.New(reply)
	if er != nil {
		// TODO
	}
	rsp.Error = err

	rspBuf, er := proto.Marshal(rsp)
	if er != nil {
		// TODO
		return
	}
	_, er = writer.Write(rspBuf)
	if er != nil {
		// TODO
		return
	}
}

func (c *CosmosMain) remoteMessageAtom(writer http.ResponseWriter, request *http.Request) {
	req := &CosmosRemoteMessagingReq{}
	reqBuf, er := ioutil.ReadAll(request.Body)
	if er != nil {
		// TODO
		return
	}
	er = proto.Unmarshal(reqBuf, req)
	if er != nil {
		// TODO
		return
	}

	// FromID
	fromID, err := c.getOrCreateRemoteID(req.From, req.FromAddr)
	if err != nil {
		// TODO
		return
	}

	// ToID
	element, atom, err := c.remoteGetAtomID(req.To)
	if err != nil {
		// TODO
		return
	}

	// Args
	args, er := req.Args.UnmarshalNew()
	if er != nil {
		// TODO
		return
	}

	rsp := &CosmosRemoteMessagingRsp{}
	reply, err := element.MessageElement(fromID, atom, req.Message, args)
	rsp.Reply, er = anypb.New(reply)
	if er != nil {
		// TODO
	}
	rsp.Error = err

	rspBuf, er := proto.Marshal(rsp)
	if er != nil {
		// TODO
		return
	}
	_, er = writer.Write(rspBuf)
	if er != nil {
		// TODO
		return
	}
}

func (c *CosmosMain) remoteGetAtom(writer http.ResponseWriter, request *http.Request) {
	q := request.URL.Query()
	elemName, atomName := q.Get("element"), q.Get("atom")
	id, err := c.CosmosGetElementAtomID(elemName, atomName)
	rsp := &CosmosRemoteGetAtomIDRsp{}
	if id != nil {
		rsp.Info = &AtomRemoteInfo{Id: id.GetIDInfo()}
	}
	rsp.Error = err

	rspBuf, er := proto.Marshal(rsp)
	if er != nil {
		// TODO
		return
	}
	_, er = writer.Write(rspBuf)
	if er != nil {
		// TODO
		return
	}
}

func (c *CosmosMain) remoteElementScaling(writer http.ResponseWriter, request *http.Request) {
	req := &CosmosRemoteScalingReq{}
	reqBuf, er := ioutil.ReadAll(request.Body)
	if er != nil {
		// TODO
		return
	}
	er = proto.Unmarshal(reqBuf, req)
	if er != nil {
		// TODO
		return
	}

	// FromID
	fromID, err := c.getOrCreateRemoteID(req.From, req.FromAddr)
	if err != nil {
		// TODO
		return
	}

	// ToID
	element, err := c.remoteGetElementID(req.To)
	if err != nil {
		// TODO
		return
	}

	// Args
	args, er := req.Args.UnmarshalNew()
	if er != nil {
		// TODO
		return
	}

	rsp := &CosmosRemoteScalingRsp{}
	scaleID, err := element.ScaleGetAtomID(fromID, req.Message, args)
	if scaleID != nil {
		rsp.Id = scaleID.GetIDInfo()
	}
	rsp.Error = err

	rspBuf, er := proto.Marshal(rsp)
	if er != nil {
		// TODO
		return
	}
	_, er = writer.Write(rspBuf)
	if er != nil {
		// TODO
		return
	}
}

func (c *CosmosMain) getOrCreateRemoteID(id *IDInfo, addr string) (ID, *ErrorInfo) {
	if id == nil {
		return nil, NewErrorf(ErrAtomNotExists, "")
	}
	// Get Cosmos Remote
	c.remotesMutex.RLock()
	cr, has := c.remotes[id.Element]
	c.remotesMutex.RUnlock()
	if !has {
		c.remotesMutex.Lock()
		cr, has = c.remotes[id.Element]
		if !has {
			cr = newCosmosRemote(c, addr)
			c.remotes[id.Element] = cr
		}
		c.remotesMutex.Unlock()

		if err := cr.connect(); err != nil {
			// TODO
			return nil, err
		}
	}
	// Get Element
	elem, has := cr.elements[id.Element]
	if !has {
		return nil, NewErrorf(ErrMainElementNotFound, "")
	}
	switch id.Type {
	case IDType_Element:
		return elem, nil
	case IDType_Atomos:
		elem.lock.RLock()
		atom, has := elem.atoms[id.Atomos]
		elem.lock.RUnlock()
		if has {
			return atom, nil
		}
		elem.lock.Lock()
		atom, has = elem.atoms[id.Atomos]
		if !has {
			atom = newAtomRemote(elem, &AtomRemoteInfo{Id: id})
			elem.atoms[id.Atomos] = atom
		}
		elem.lock.Unlock()
		return atom, nil
	}
	return nil, NewErrorf(ErrAtomNotExists, "")
}

func (c *CosmosMain) remoteGetElementID(to *IDInfo) (*ElementLocal, *ErrorInfo) {
	return c.getElement(to.Element)
}

func (c *CosmosMain) remoteGetAtomID(to *IDInfo) (*ElementLocal, *AtomLocal, *ErrorInfo) {
	elem, err := c.getElement(to.Element)
	if err != nil {
		return nil, nil, err
	}
	atom, err := elem.elementAtomGet(to.Atomos, true)
	if err != nil {
		return nil, nil, err
	}
	return elem, atom, nil
}

func (c *CosmosMain) getFromAddr() string {
	server := c.runnable.config.EnableServer
	if server == nil {
		return ""
	}
	return server.Host + strconv.FormatInt(int64(server.Port), 10)
}
