package go_atomos

const (
	lCRIWSNotInit       = "cosmosRemote.initWatchServer: Not initialized."
	lCRIWSInitFailed    = "cosmosRemote.initWatchServer: Init failed, err=%v"
	lCRIWSInit          = "cosmosRemote.initWatchServer: Has initialized, port=%d"
	lCRGENotFound       = "cosmosRemote.getElement: Element not found, nodeName=%s"
	lCRSElementUpdate   = "cosmosRemote.setElement: Update exists element, nodeName=%s"
	lCRSElementAdd      = "cosmosRemote.setElement: Add element, nodeName=%s"
	lCRDElementNotFound = "cosmosRemote.delElement: Element not found, nodeName=%s"
	lCRDElementDel      = "cosmosRemote.delElement: Delete element, nodeName=%s"
	lCRSAFailed         = "cosmosRemote.SpawnAtom: Cannot spawn remote atom"
)

// TODO: 远程Cosmos管理助手，在未来版本实现。
// TODO: Remote Cosmos Helper.

//type CosmosClusterHelper struct {
//	sync.RWMutex
//	self    *CosmosSelf
//	server  *cosmosServer
//	remotes map[string]*cosmosRemoteStateMachine
//}

//func newCosmosClusterHelper(s *CosmosSelf) *CosmosClusterHelper {
//	h := &CosmosClusterHelper{
//		RWMutex: sync.RWMutex{},
//		self:    s,
//		remotes: map[string]*cosmosRemoteStateMachine{},
//	}
//	h.server = &cosmosServer{
//		helper: h,
//	}
//	return h
//}
//
//func (h *CosmosClusterHelper) init() (err error) {
//	if err = h.initWatchServer(); err != nil {
//		return
//	}
//	return nil
//}

//func (h *CosmosClusterHelper) initWatchServer() error {
//	rCfg := h.self.config.EnableRemoteServer
//	if rCfg == nil {
//		h.self.logInfo(lCRIWSNotInit)
//		return nil
//	}
//	err := h.server.init(rCfg.Port, rCfg.CertPath, rCfg.KeyPath)
//	if err != nil {
//		h.self.logFatal(lCRIWSInitFailed, err)
//		return err
//	}
//	h.self.logInfo(lCRIWSInit, rCfg.Port)
//	h.server.start()
//	return nil
//}

//func (h *CosmosClusterHelper) close() {
//	for _, remote := range h.remotes {
//		remote.disconnectWatch()
//	}
//}

//func (h *CosmosClusterHelper) packInfo() ([]byte, error) {
//	initInfo := &CosmosRemoteConnectInit{
//		Config: &CosmosLocalConfig{
//			EnableRemote: h.self.config.EnableRemoteServer != nil,
//			NodeName:     h.self.config.Node,
//			Elements:     map[string]*ElementConfig{},
//		},
//	}
//	if initInfo.Config.EnableRemote {
//		for name, elem := range h.self.local.elements {
//			initInfo.Config.Elements[name] = &ElementConfig{
//				Name:    name,
//				Version: elem.current.ElementInterface.Config.Version,
//			}
//		}
//	}
//	buf, err := proto.Marshal(initInfo)
//	if err != nil {
//		h.self.logFatal("CosmosClusterHelper.packInfo: Marshal failed, err=%v", err)
//		return nil, err
//	}
//	return buf, nil
//}

// Outgoing connection.

//func (h *CosmosClusterHelper) connectRemote(nodeName, nodeAddr string) (*cosmosRemote, error) {
//	remote := h.getOrCreateRemote(nodeName, nodeAddr)
//	if err := remote.connectWatcher(); err != nil {
//		h.delRemote(nodeName)
//		return nil, err
//	}
//	return remote, nil
//}
//
//func (h *CosmosClusterHelper) acceptRemote(nodeName string, conn *websocket.Conn) error {
//	remote := h.getOrCreateRemote(nodeName, conn.RemoteAddr().String())
//	if err := remote.acceptWatcher(conn); err != nil {
//		h.delRemote(nodeName)
//		return err
//	}
//	return nil
//}

//func (h *CosmosClusterHelper) getOrCreateRemote(nodeName, nodeAddr string) *cosmosRemoteStateMachine {
//	h.Lock()
//	c, has := h.remotes[nodeName]
//	if has {
//		h.Unlock()
//		return c
//	}
//	c = &cosmosRemoteStateMachine{
//		cosmosRemote: &cosmosRemote{
//			nodeName: nodeName,
//			nodeAddr: nodeAddr,
//			helper:   h,
//			elements: map[string]*ElementRemote{},
//			//watcher:   cosmosWatcher{},
//			//requester: nil,
//		},
//		state:   CosmosRemoteDisconnected,
//		stateCh: make(chan *cosmosRemoteState),
//	}
//	c.watcher.remote = c
//	h.remotes[nodeName] = c
//	h.Unlock()
//	return c
//}

//func (h *CosmosClusterHelper) getRemote(nodeName string) *cosmosRemoteStateMachine {
//	h.RLock()
//	defer h.RUnlock()
//	return h.remotes[nodeName]
//}

// TODO Handle Remote Close

// Cosmos Remote

//type cosmosRemote struct {
//	nodeName  string
//	nodeAddr  string
//	watcher   cosmosWatcher
//	requester *http.Client
//	helper    *CosmosClusterHelper
//	elements  map[string]*ElementRemote
//	//initCh    chan error
//}

// Message

//func (c *cosmosRemote) requestMessage(t string, jsonBuf io.Reader) (body []byte, err error) {
//	// Connect.
//	resp, err := c.requester.Post("https://"+c.nodeAddr+"/"+t, "application/json", jsonBuf)
//	if err != nil {
//		return nil, err
//	}
//	defer resp.Body.Close()
//	body, err = ioutil.ReadAll(resp.Body)
//	if err != nil {
//		return nil, err
//	}
//	return body, nil
//}

//func (c *cosmosRemote) haltWatch() {
//	c.state = CosmosRemoteHalt
//}
//
//func (c *cosmosRemote) connectWatcher() error {
//	// Connect.
//	u := url.URL{
//		Scheme: cosmosWatchSchema,
//		Host:   c.nodeAddr,
//		Path:   cosmosWatchUri,
//	}
//	caCert, err := ioutil.ReadFile(c.helper.self.config.EnableRemoteServer.CertPath)
//	if err != nil {
//		c.helper.self.logFatal("cosmosRemote.connectWatcher: Read cert failed, err=%v", err)
//		return err
//	}
//	tlsConfig := &tls.Config{}
//	if skipVerify {
//		tlsConfig.InsecureSkipVerify = true
//	} else {
//		caCertPool := x509.NewCertPool()
//		caCertPool.AppendCertsFromPEM(caCert)
//		// Create TLS configuration with the certificate of the server.
//		tlsConfig.RootCAs = caCertPool
//	}
//	dialer := websocket.Dialer{
//		TLSClientConfig: tlsConfig,
//	}
//	c.watcher.conn, _, err = dialer.Dial(u.String(), http.Header{
//		cosmosWatchHeader: []string{c.helper.self.config.Node},
//	})
//	if err != nil {
//		return err
//	}
//	if err := c.watcherInit(); err != nil {
//		c.helper.self.logFatal("cosmosRemote.connectWatcher: Init failed, err=%v", err)
//		return err
//	}
//	return nil
//}
//
//func (c *cosmosRemote) acceptWatcher(conn *websocket.Conn) error {
//	c.watcher.conn = conn
//	if err := c.watcherInit(); err != nil {
//		c.helper.self.logFatal("cosmosRemote.acceptWatcher: Init failed, err=%v", err)
//		return err
//	}
//	return nil
//}

//func (c *cosmosRemote) watcherInit() error {
//	// Init http2 requester.
//	caCert, err := ioutil.ReadFile(c.helper.self.config.EnableRemoteServer.CertPath)
//	if err != nil {
//		c.helper.self.logFatal("cosmosRemote.watcherInit: Read cert failed, err=%v", err)
//		return err
//	}
//	tlsConfig := &tls.Config{}
//	if skipVerify {
//		tlsConfig.InsecureSkipVerify = true
//	} else {
//		caCertPool := x509.NewCertPool()
//		caCertPool.AppendCertsFromPEM(caCert)
//		// Create TLS configuration with the certificate of the server.
//		tlsConfig.RootCAs = caCertPool
//	}
//	c.requester = &http.Client{
//		Transport: &http2.Transport{
//			TLSClientConfig: tlsConfig,
//		},
//	}
//
//	// Init websocket.
//	c.watcher.reader = c.watcherRead
//	c.watcher.sender = make(chan *sendContent)
//	go c.watcher.writePump()
//	go c.watcher.readPump()
//	go func() {
//		if err = c.watchConnSendAllInfo(); err != nil {
//			c.initCh <- err
//		}
//	}()
//
//	// Waiting for reply.
//	select {
//	case err = <-c.initCh:
//		return err
//	case <-time.After(initWait):
//		err = errors.New("request timeout")
//		c.helper.self.logFatal("cosmosWatcher.connectWatcher: Watch failed, err=%v", err)
//		return err
//	}
//}

//func (c *cosmosRemote) reconnect() error {
//
//}

//func (c *cosmosRemote) watcherRead(buf []byte) error {
//	switch c.state {
//	case CosmosRemoteInitializing:
//		if err := c.watchConnReadAllInfo(buf); err != nil {
//			c.initCh <- err
//			return err
//		}
//		c.state = CosmosRemoteRunning
//		c.initCh <- nil
//	case CosmosRemoteRunning:
//		updateInfo := CosmosRemoteConnectUpdate{}
//		if err := proto.Unmarshal(buf, &updateInfo); err != nil {
//			return err
//		}
//		switch updateInfo.Type {
//		case CosmosRemoteUpdateType_Create:
//			c.elements[updateInfo.Name] = &ElementRemote{
//				name:     updateInfo.Name,
//				version:  updateInfo.Config.Version,
//				cachedId: map[string]*atomIdRemote{},
//			}
//		case CosmosRemoteUpdateType_Update:
//			e := c.elements[updateInfo.Name]
//			e.version = updateInfo.Config.Version
//		case CosmosRemoteUpdateType_Delete:
//			delete(c.elements, updateInfo.Name)
//		}
//	}
//	return nil
//}

//func (c *cosmosRemote) disconnectWatch() {
//	err := c.watcher.stop()
//	c.helper.self.logInfo("cosmosRemote.disconnectWatch: Disconnect, name=%s,err=%v", c.nodeName, err)
//}

//func (c *cosmosRemote) watchConnReadAllInfo(buf []byte) error {
//	initInfo := &CosmosRemoteConnectInit{}
//	if err := proto.Unmarshal(buf, initInfo); err != nil {
//		c.initCh <- err
//		return err
//	}
//	if !debugMode && c.nodeName != initInfo.Config.NodeName {
//		err := errors.New("cosmosRemote name not match")
//		c.initCh <- err
//		return err
//	}
//	for name, elem := range initInfo.Config.Elements {
//		ei, has := c.helper.self.local.elementInterfaces[name]
//		if !has {
//			c.helper.self.logFatal("cosmosRemote.watchConnReadAllInfo: Element not supported, element=%s", name)
//			continue
//		}
//		c.elements[name] = &ElementRemote{
//			ElementInterface: ei,
//			cosmos:           c,
//			name:             name,
//			version:          elem.Version,
//			cachedId:         map[string]*atomIdRemote{},
//		}
//	}
//	return nil
//}

//func (c *cosmosRemote) watchConnSendAllInfo() error {
//	buf, err := c.helper.packInfo()
//	if err != nil {
//		return err
//	}
//	if err = c.watcher.send(&sendContent{websocket.BinaryMessage, buf}); err != nil {
//		c.helper.self.logFatal("cosmosRemote.watchConnSendAllInfo: write failed, err=%v", err)
//		return err
//	}
//	return nil
//}
