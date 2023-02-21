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
	"os"
	"sync"
	"time"
)

type CosmosMain struct {
	process *CosmosProcess
	global  *CosmosGlobal

	// Config
	runnable *CosmosRunnable
	//mainKillCh chan bool

	atomos *BaseAtomos

	// Elements & Remote
	elements map[string]*ElementLocal

	remotes              map[string]*CosmosRemote
	remoteTrackers       map[uint64]*IDTracker
	remoteTrackerCounter uint64

	mutex sync.RWMutex

	// TLS if exists
	listenCert *tls.Config
	clientCert *tls.Config

	// Remote
	server   *http.Server
	listener net.Listener

	// 调用链
	// 调用链用于检测是否有循环调用，在处理message时把fromID的调用链加上自己之后
	callChain []ID
}

// 生命周期相关
// Life cycle

func initCosmosMain(process *CosmosProcess) *CosmosMain {
	SharedLogging().PushProcessLog(LogLevel_Info, "Cosmos: Loading. pid=(%d)", os.Getpid())
	id := &IDInfo{
		Type:    processIDType,
		Cosmos:  "",
		Element: "",
		Atomos:  "",
	}
	main := &CosmosMain{
		process:              process,
		runnable:             nil,
		atomos:               nil,
		elements:             map[string]*ElementLocal{},
		remotes:              map[string]*CosmosRemote{},
		remoteTrackers:       map[uint64]*IDTracker{},
		remoteTrackerCounter: 0,
		mutex:                sync.RWMutex{},
		listenCert:           nil,
		clientCert:           nil,
		callChain:            nil,
	}
	main.atomos = NewBaseAtomos(id, LogLevel_Debug, main, main)
	process.main = main
	_ = process.main.atomos.start(nil)
	return main
}

// Load Runnable

func (c *CosmosMain) loadOnce(runnable *CosmosRunnable) *Error {
	_, err := c.tryLoadRunnable(runnable)
	if err != nil {
		c.Log().Fatal("Cosmos: Once load failed. err=(%s)", err.Message)
		return err
	}
	return nil
}

func (c *CosmosMain) tryLoadRunnable(newRunnable *CosmosRunnable) (*runnableLoadingHelper, *Error) {
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
	return c.atomos.String()
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
	return c.atomos.GetState()
}

func (c *CosmosMain) IdleTime() time.Duration {
	return 0
}

func (c *CosmosMain) MessageByName(from ID, name string, timeout time.Duration, in proto.Message) (proto.Message, *Error) {
	return nil, NewError(ErrMainCannotMessage, "Cosmos: Cannot message main.").AddStack(c)
}

func (c *CosmosMain) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	return nil, nil
}

func (c *CosmosMain) Kill(from ID, timeout time.Duration) *Error {
	return NewError(ErrMainCannotKill, "Cosmos: Cannot kill main.").AddStack(c)
}

func (c *CosmosMain) SendWormhole(from ID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	return c.atomos.PushWormholeMailAndWaitReply(from, timeout, wormhole)
}

func (c *CosmosMain) getCallChain() []ID {
	c.atomos.mailbox.mutex.Lock()
	defer c.atomos.mailbox.mutex.Unlock()
	idList := make([]ID, 0, len(c.callChain)+1)
	for _, id := range c.callChain {
		idList = append(idList, id)
	}
	idList = append(idList, c)
	return idList
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

func (c *CosmosMain) getIDTrackerManager() *IDTrackerManager {
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

// KillSelf
// Atom kill itself from inner
func (c *CosmosMain) KillSelf() {
	c.Log().Info("Cosmos: Cannot KillSelf.")
}

func (c *CosmosMain) Parallel(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				err := NewErrorf(ErrFrameworkPanic, "Cosmos: Parallel recovers from panic.").AddPanicStack(c, 3, r)
				if ar, ok := c.atomos.instance.(AtomosRecover); ok {
					defer func() {
						recover()
						c.Log().Fatal("Cosmos: Parallel recovers from panic. err=(%v)", err)
					}()
					ar.ParallelRecover(err)
				} else {
					c.Log().Fatal("Cosmos: Parallel recovers from panic. err=(%v)", err)
				}
			}
		}()
		fn()
	}()
}

func (c *CosmosMain) Config() map[string][]byte {
	return c.runnable.config.Customize
}

func (c *CosmosMain) MessageSelfByName(from ID, name string, buf []byte, protoOrJSON bool) ([]byte, *Error) {
	return nil, NewError(ErrMainCannotMessage, "Cosmos: Cannot message.").AddStack(c)
}

// Implementation of CosmosNode

func (c *CosmosMain) GetNodeName() string {
	return c.GetIDInfo().Cosmos
}

func (c *CosmosMain) CosmosIsLocal() bool {
	return true
}

func (c *CosmosMain) CosmosGetElementID(elem string) (ID, *Error) {
	return c.getElement(elem)
}

func (c *CosmosMain) CosmosGetElementAtomID(elem, name string) (ID, *IDTracker, *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err.AddStack(c)
	}
	return element.GetAtomID(name, NewIDTrackerInfo(3))
}

func (c *CosmosMain) CosmosSpawnElementAtom(elem, name string, arg proto.Message) (ID, *IDTracker, *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err
	}
	return element.SpawnAtom(name, arg, NewIDTrackerInfo(3))
}

func (c *CosmosMain) CosmosMessageElement(fromID, toID ID, message string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	return toID.Element().MessageElement(fromID, toID, message, timeout, args)
}

func (c *CosmosMain) CosmosMessageAtom(fromID, toID ID, message string, timeout time.Duration, args proto.Message) (reply proto.Message, err *Error) {
	return toID.Element().MessageAtom(fromID, toID, message, timeout, args)
}

func (c *CosmosMain) CosmosScaleElementGetAtomID(fromID ID, elem, message string, timeout time.Duration, args proto.Message) (ID ID, tracker *IDTracker, err *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err.AddStack(c)
	}
	return element.ScaleGetAtomID(fromID, message, timeout, args, NewIDTrackerInfo(3))
}

// Implementation of AtomosUtilities

func (c *CosmosMain) Log() Logging {
	return c.atomos.Log()
}

func (c *CosmosMain) Task() Task {
	return c.atomos.Task()
}

// Main as an Atomos

func (c *CosmosMain) Halt(from ID, cancels map[uint64]CancelledTask) (save bool, data proto.Message) {
	c.Log().Fatal("Cosmos: Stopping of CosmosMain should not be called.")
	return false, nil
}

// 内部实现
// INTERNAL

// 邮箱控制器相关
// Mailbox Handler

func (c *CosmosMain) OnMessaging(from ID, name string, args proto.Message) (reply proto.Message, err *Error) {
	return nil, NewError(ErrMainCannotMessage, "Cosmos: Cannot send cosmos message.")
}

func (c *CosmosMain) pushKillMail(from ID, wait bool, timeout time.Duration) *Error {
	return c.atomos.PushKillMailAndWaitReply(from, wait, true, timeout)
}

func (c *CosmosMain) OnScaling(from ID, name string, arg proto.Message, tracker *IDTracker) (id ID, err *Error) {
	return nil, NewError(ErrMainCannotScale, "Cosmos: Cannot scale.").AddStack(c)
}

func (c *CosmosMain) OnWormhole(from ID, wormhole AtomosWormhole) *Error {
	holder, ok := c.atomos.instance.(AtomosAcceptWormhole)
	if !ok || holder == nil {
		err := NewErrorf(ErrAtomosNotSupportWormhole, "Cosmos: Not supported wormhole. type=(%T)", c.atomos.instance)
		c.Log().Error(err.Message)
		return err
	}
	return holder.AcceptWormhole(from, wormhole)
}

func (c *CosmosMain) OnStopping(from ID, cancelled map[uint64]CancelledTask) (err *Error) {
	c.Log().Info("Cosmos: Now exiting.")

	// Unload local elements and its atomos.
	for i := len(c.runnable.implementOrder) - 1; i >= 0; i -= 1 {
		name := c.runnable.implementOrder[i].Interface.Config.Name
		elem, has := c.elements[name]
		if !has {
			continue
		}
		if err := elem.pushKillMail(c, true, 0); err != nil {
			c.Log().Error("Cosmos: Exiting kill element error. element=(%s),err=(%v)", name, err.Message)
		}
	}
	c.callChain = nil
	c.clientCert = nil
	c.listenCert = nil
	c.elements = nil
	c.runnable = nil
	c.process = nil
	return nil
}

// Set & Unset

func (c *CosmosMain) Spawn() {}

func (c *CosmosMain) Set(message string) {}

func (c *CosmosMain) Unset(message string) {}

func (c *CosmosMain) Stopping() {}

func (c *CosmosMain) Halted() {}

// Element Inner

func (c *CosmosMain) getElement(name string) (elem *ElementLocal, err *Error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.runnable == nil {
		err = NewError(ErrMainRunnableNotFound, "Cosmos: It's not running.")
		return nil, err
	}
	elem, has := c.elements[name]
	if !has {
		err = NewErrorf(ErrMainElementNotFound, "Cosmos: Local element not found. name=(%s)", name).AddStack(c)
		return nil, err
	}
	return elem, nil
}

func (c *CosmosMain) trySpawningElements(helper *runnableLoadingHelper) (err *Error) {
	c.atomos.id.Cosmos = helper.newRunnable.config.Node
	c.atomos.log.level = helper.newRunnable.config.LogLevel
	// Spawn
	// TODO 有个问题，如果这里的Spawn逻辑需要用到新的helper里面的配置，那就会有问题，所以Spawn尽量不要做对其它Cosmos的操作，延后到Script。
	var loaded []*ElementLocal
	for _, impl := range helper.spawnElement {
		elem, e := c.cosmosElementSpawn(helper.newRunnable, impl)
		if e != nil {
			err = e.AddStack(c)
			c.Log().Fatal("Cosmos: Spawning element failed. name=(%s),err=(%s)", impl.Interface.Config.Name, err.Message)
			break
		}
		loaded = append(loaded, elem)
	}
	if err != nil {
		for _, elem := range loaded {
			if e := elem.pushKillMail(c, true, 0); e != nil {
				c.Log().Fatal("Cosmos: Spawning element failed, kill failed. name=(%s),err=(%v)", elem.GetName(), e.AddStack(c))
			}
		}
		c.Log().Fatal("Cosmos: Spawning element has rollback.")
		return
	}
	for _, elem := range loaded {
		s, ok := elem.atomos.instance.(ElementCustomizeStartRunning)
		if !ok || s == nil {
			continue
		}
		go func(s ElementCustomizeStartRunning) {
			defer func() {
				if r := recover(); r != nil {
					err := NewErrorf(ErrMainStartRunningPanic, "Cosmos: StartRunning recovers from panic.").AddPanicStack(c, 3, r)
					if ar, ok := c.atomos.instance.(AtomosRecover); ok {
						defer func() {
							recover()
							c.Log().Fatal("Cosmos: StartRunning recovers from panic. err=(%v)", err)
						}()
						ar.ParallelRecover(err)
					} else {
						c.Log().Fatal("Cosmos: StartRunning recovers from panic. err=(%v)", err)
					}
				}
			}()
			s.StartRunning()
		}(s)
	}
	return nil
}

func (c *CosmosMain) cosmosElementSpawn(r *CosmosRunnable, i *ElementImplementation) (elem *ElementLocal, err *Error) {
	defer func() {
		if r := recover(); r != nil {
			if err == nil {
				err = NewErrorf(ErrFrameworkPanic, "Element: Spawn Element recovers from panic.").AddPanicStack(c, 4, r)
				if ar, ok := c.atomos.instance.(AtomosRecover); ok {
					defer func() {
						recover()
						c.Log().Fatal("Element: Spawn recovers from panic. err=(%v)", err)
					}()
					ar.SpawnRecover(nil, err)
				} else {
					c.Log().Fatal("Element: Spawn recovers from panic. err=(%v)", err)
				}
			}
		}
	}()
	name := i.Interface.Config.Name

	elem = newElementLocal(c, r, i)

	c.mutex.Lock()
	_, has := c.elements[name]
	if !has {
		c.elements[name] = elem
	}
	c.mutex.Unlock()
	if has {
		return nil, NewErrorf(ErrElementLoaded, "Cosmos: Spawn Element exists. name=(%s)", name).AddStack(c)
	}

	// Element的Spawn逻辑。
	if err = elem.atomos.start(func() *Error {
		if err := elem.cosmosElementSpawn(r, i); err != nil {
			return err.AddStack(elem)
		}
		return nil
	}); err != nil {
		c.mutex.Lock()
		delete(c.elements, name)
		c.mutex.Unlock()
		return nil, err.AddStack(elem)
	}
	return elem, nil
}

// Runnable Loading Helper

type runnableLoadingHelper struct {
	newRunnable *CosmosRunnable

	spawnElement, reloadElement, delElement []*ElementImplementation

	// TLS if exists
	listenCert, clientCert *tls.Config
}

func (c *CosmosMain) newRunnableLoadingHelper(oldRunnable, newRunnable *CosmosRunnable) (*runnableLoadingHelper, *Error) {
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
			return nil, NewError(ErrCosmosConfigCertInvalid, "Cosmos: Cert path is empty.").AddStack(c)
		}
		if cert.KeyPath == "" {
			return nil, NewError(ErrCosmosConfigCertInvalid, "Cosmos: Key path is empty.").AddStack(c)
		}
		// Load Key Pair.
		c.Log().Info("Cosmos: Enabling Cert. cert=(%s),key=(%s)", cert.CertPath, cert.KeyPath)
		pair, e := tls.LoadX509KeyPair(cert.CertPath, cert.KeyPath)
		if e != nil {
			err := NewErrorf(ErrMainLoadCertFailed, "Cosmos: Load Key Pair failed. err=(%v)", e).AddStack(c)
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
			return nil, NewErrorf(ErrCosmosConfigCertInvalid, "Cosmos: Cert file read error. err=(%v)", e).AddStack(c)
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
		c.Log().Info("Cosmos: Enable Server. host=(%s),port=(%d)", server.Host, server.Port)
		if oldRunnable != nil && oldRunnable.config.EnableServer.IsEqual(newRunnable.config.EnableServer) {
			goto noServerReconnect
		}
		if err := c.listen(newRunnable, server.Host, server.Port); err != nil {
			return nil, err.AddStack(c)
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

func (c *CosmosMain) listen(runnable *CosmosRunnable, host string, port int32) *Error {
	config := runnable.config
	// Http
	listenAddr := fmt.Sprintf("%s:%d", host, port)

	mux := http.NewServeMux()
	mux.HandleFunc(RemoteAtomosConnect, c.remoteConnect)
	mux.HandleFunc(RemoteAtomosElementScaling, c.remoteElementScaling)
	mux.HandleFunc(RemoteAtomosGetAtom, c.remoteAtomGet)
	mux.HandleFunc(RemoteAtomosMessaging, c.remoteMessaging)
	mux.HandleFunc(RemoteAtomosIDRelease, c.remoteIDRelease)

	c.server = &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}
	if er := http2.ConfigureServer(c.server, &http2.Server{}); er != nil {
		return NewErrorf(ErrCosmosRemoteListenFailed, "Cosmos: Configure server failed. err=(%v)", er).AddStack(c)
	}

	// Listen the local port.
	if config.EnableCert != nil {
		cert, er := tls.LoadX509KeyPair(config.EnableCert.CertPath, config.EnableCert.KeyPath)
		if er != nil {
			return NewErrorf(ErrCosmosRemoteListenFailed, "Cosmos: Server cert invalid. err=(%v)", er).AddStack(c)
		}
		// Listen the local port.
		listener, er := tls.Listen("tcp", listenAddr, &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
		if er != nil {
			return NewErrorf(ErrCosmosRemoteListenFailed, "Cosmos: Loading server failed. err=(%v)", er).AddStack(c)
		}
		c.listener = listener
	} else {
		// Listen the local port.
		listener, er := net.Listen("tcp", listenAddr)
		if er != nil {
			return NewErrorf(ErrCosmosRemoteListenFailed, "Cosmos: Loading server failed. err=(%v)", er).AddStack(c)
		}
		c.listener = listener
	}
	go c.server.Serve(c.listener)
	return nil
}

func (c *CosmosMain) GetRemote(addr string) *CosmosRemote {
	c.mutex.RLock()
	remote, has := c.remotes[addr]
	c.mutex.RUnlock()
	if has {
		if err := remote.connect(); err != nil {
			c.Log().Fatal("Cosmos: Check remote connect failed. addr=(%s),err=(%v)", addr, err)
		}
		return remote
	}

	remote = newCosmosRemote(c, addr)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	oldRemote, has := c.remotes[addr]
	if !has {
		c.remotes[addr] = remote
		return remote
	} else {
		return oldRemote
	}
}

func (c *CosmosMain) remoteResponse(writer http.ResponseWriter, rsp proto.Message, err *Error) {
	if err != nil {
		buf, er := proto.Marshal(err)
		if er != nil {
			c.Log().Fatal("CosmosRemote: Marshal error failed. err=(%v),er=(%v)", err, er)
		}
		writer.WriteHeader(http.StatusInternalServerError)
		writer.Write(buf)
	} else {
		buf, er := proto.Marshal(rsp)
		if er != nil {
			c.Log().Fatal("CosmosRemote: Marshal response failed. err=(%v),er=(%v)", err, er)
			writer.WriteHeader(http.StatusInternalServerError)
		} else {
			writer.WriteHeader(http.StatusOK)
		}
		writer.Write(buf)
		return
	}
}

func (c *CosmosMain) remoteRequest(request *http.Request, req proto.Message) *Error {
	reqBuf, er := ioutil.ReadAll(request.Body)
	if er != nil {
		return NewErrorf(ErrCosmosRemoteResponseFailed, "CosmosRemote: Read failed. err=(%v)", er).AddStack(c)
	}
	er = proto.Unmarshal(reqBuf, req)
	if er != nil {
		return NewErrorf(ErrCosmosRemoteResponseFailed, "CosmosRemote: Unmarshal failed. err=(%v)", er).AddStack(c)
	}
	return nil
}

func (c *CosmosMain) remoteConnect(writer http.ResponseWriter, request *http.Request) {
	info := &CosmosRemoteInfo{
		Id:       c.GetIDInfo(),
		Elements: nil,
		Changed:  0,
	}
	c.mutex.RLock()
	for name, elem := range c.elements {
		info.Elements[name] = elem.GetIDInfo()
	}
	c.mutex.RUnlock()

	c.remoteResponse(writer, info, nil)
}

func (c *CosmosMain) remoteElementScaling(writer http.ResponseWriter, request *http.Request) {
	req := &CosmosRemoteScalingReq{}
	if err := c.remoteRequest(request, req); err != nil {
		c.remoteResponse(writer, nil, err.AddStack(c))
		return
	}
	if req.From == nil {
		req.From = &IDInfo{}
	}
	if req.To == nil {
		req.To = &IDInfo{}
	}
	if req.FromTracker == nil {
		req.FromTracker = &IDTrackerInfo{}
	}

	// FromID
	fromID, err := c.getOrCreateRemoteID(req.From, req.FromAddr)
	if err != nil {
		c.remoteResponse(writer, nil, err.AddStack(c))
		return
	}

	// ToID
	element, err := c.getElement(req.To.Element)
	if err != nil {
		c.remoteResponse(writer, nil, err.AddStack(c))
		return
	}

	// Args
	if req.Args == nil {
		c.remoteResponse(writer, nil, NewErrorf(ErrCosmosRemoteResponseFailed, "CosmosRemote: Args invalid.").AddStack(c))
		return
	}
	args, er := req.Args.UnmarshalNew()
	if er != nil {
		c.remoteResponse(writer, nil, NewErrorf(ErrCosmosRemoteResponseFailed, "CosmosRemote: Unmarshal failed. err=(%v)", er).AddStack(c))
		return
	}

	rsp := &CosmosRemoteScalingRsp{}
	scaleID, tracker, err := element.ScaleGetAtomID(fromID, req.Message, time.Duration(req.Timeout), args, req.FromTracker)
	if scaleID != nil {
		c.mutex.Lock()
		c.remoteTrackerCounter += 1
		c.remoteTrackers[c.remoteTrackerCounter] = tracker
		rsp.TrackerId = c.remoteTrackerCounter
		c.mutex.Unlock()
		rsp.Id = scaleID.GetIDInfo()
	}
	rsp.Error = err

	c.remoteResponse(writer, rsp, nil)
}

func (c *CosmosMain) remoteAtomGet(writer http.ResponseWriter, request *http.Request) {
	req := &CosmosRemoteGetIDReq{}
	if err := c.remoteRequest(request, req); err != nil {
		c.remoteResponse(writer, nil, err.AddStack(c))
		return
	}
	if req.FromTracker == nil {
		req.FromTracker = &IDTrackerInfo{}
	}

	//// FromID
	//fromID, err := c.getOrCreateRemoteID(req.From, req.FromAddr)
	//if err != nil {
	//	c.remoteResponse(writer, nil, err.AddStack(c))
	//	return
	//}

	// ToID
	element, err := c.getElement(req.Element)
	if err != nil {
		c.remoteResponse(writer, nil, err.AddStack(c))
		return
	}

	rsp := &CosmosRemoteGetIDRsp{}
	atom, tracker, err := element.GetAtomID(req.Atom, req.FromTracker)
	if atom != nil {
		c.mutex.Lock()
		c.remoteTrackerCounter += 1
		c.remoteTrackers[c.remoteTrackerCounter] = tracker
		rsp.TrackerId = c.remoteTrackerCounter
		c.mutex.Unlock()
		rsp.Id = atom.GetIDInfo()
	}
	rsp.Error = err

	c.remoteResponse(writer, rsp, nil)
}

func (c *CosmosMain) remoteMessaging(writer http.ResponseWriter, request *http.Request) {
	req := &CosmosRemoteMessagingReq{}
	if err := c.remoteRequest(request, req); err != nil {
		c.remoteResponse(writer, nil, err.AddStack(c))
		return
	}
	if req.From == nil {
		req.From = &IDInfo{}
	}
	if req.To == nil {
		req.To = &IDInfo{}
	}

	// FromID
	fromID, err := c.getOrCreateRemoteID(req.From, req.FromAddr)
	if err != nil {
		c.remoteResponse(writer, nil, err.AddStack(c))
		return
	}

	// ToID
	var toID ID
	element, err := c.getElement(req.To.Element)
	if err != nil {
		c.remoteResponse(writer, nil, err.AddStack(c))
		return
	}
	switch req.To.Type {
	case IDType_Element:
		toID = element
	case IDType_Atom:
		id, tracker, err := element.GetAtomID(req.To.Atomos, NewIDTrackerInfo(1))
		if err != nil {
			c.remoteResponse(writer, nil, err.AddStack(c))
			return
		}
		defer tracker.Release()
		toID = id
	default:
		c.remoteResponse(writer, nil, NewErrorf(ErrCosmosRemoteResponseFailed, "CosmosRemote: Invalid FromID type.").AddStack(c))
		return
	}

	// Args
	if req.Args == nil {
		c.remoteResponse(writer, nil, NewErrorf(ErrCosmosRemoteResponseFailed, "CosmosRemote: Args invalid.").AddStack(c))
		return
	}
	args, er := req.Args.UnmarshalNew()
	if er != nil {
		c.remoteResponse(writer, nil, NewErrorf(ErrCosmosRemoteResponseFailed, "CosmosRemote: Unmarshal failed. err=(%v)", er).AddStack(c))
		return
	}

	rsp := &CosmosRemoteMessagingRsp{}
	reply, err := toID.MessageByName(fromID, req.Message, time.Duration(req.Timeout), args)
	if reply != nil {
		rsp.Reply, er = anypb.New(reply)
		if er != nil {
			c.remoteResponse(writer, nil, NewErrorf(ErrCosmosRemoteResponseFailed, "CosmosRemote: Marshal reply failed. err=(%v)", er).AddStack(c))
			return
		}
	}
	if err != nil {
		rsp.Error = err
	}

	c.remoteResponse(writer, rsp, nil)
}

func (c *CosmosMain) remoteIDRelease(writer http.ResponseWriter, request *http.Request) {
	req := &CosmosRemoteReleaseIDReq{}
	if err := c.remoteRequest(request, req); err != nil {
		c.remoteResponse(writer, nil, err.AddStack(c))
		return
	}
	c.mutex.Lock()
	tracker, has := c.remoteTrackers[req.TrackerId]
	if has {
		delete(c.remoteTrackers, req.TrackerId)
	}
	c.mutex.Unlock()
	tracker.Release()

	rsp := &CosmosRemoteReleaseIDRsp{}
	c.remoteResponse(writer, rsp, nil)
}

func (c *CosmosMain) getOrCreateRemoteID(id *IDInfo, addr string) (ID, *Error) {
	if id == nil {
		return nil, NewErrorf(ErrAtomNotExists, "CosmosRemote: FromID is invalid.").AddStack(c)
	}
	// Get Cosmos Remote
	c.mutex.RLock()
	cr, has := c.remotes[addr]
	c.mutex.RUnlock()
	if !has {
		cr = newCosmosRemote(c, addr)
		c.mutex.Lock()
		cr, has = c.remotes[addr]
		if !has {
			c.remotes[addr] = cr
		}
		c.mutex.Unlock()
	}
	// Get Element
	cr.RLock()
	elem, has := cr.elements[id.Element]
	cr.RUnlock()
	if !has {
		return nil, NewErrorf(ErrCosmosRemoteElementNotFound, "CosmosRemote: FromID element not found. name=(%s)", id.Element).AddStack(c)
	}
	switch id.Type {
	case IDType_Element:
		return elem, nil
	case IDType_Atom:
		elem.lock.RLock()
		atom, has := elem.atoms[id.Atomos]
		elem.lock.RUnlock()
		if has {
			return atom, nil
		}
		elem.lock.Lock()
		atom, has = elem.atoms[id.Atomos]
		if !has {
			atom = newAtomRemote(elem, id)
			elem.atoms[id.Atomos] = atom
		}
		elem.lock.Unlock()
		return atom, nil
	}
	return nil, NewErrorf(ErrAtomNotExists, "CosmosRemote: FromID invalid type.").AddStack(c)
}

func (c *CosmosMain) getFromAddr() string {
	// TODO
	s := c.runnable.config.EnableServer
	if s == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}
