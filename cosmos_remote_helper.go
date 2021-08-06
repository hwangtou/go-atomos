package go_atomos

import (
	"fmt"
	"log"
	"net/http"

	"github.com/hwangtou/go-atomos/net/websocket"
)

const (
	RemoteUriAtomId      = "/atom_id"
	RemoteUriAtomMessage = "/atom_msg"
)

// Cosmos Remote Helper

type cosmosRemotesHelper struct {
	self *CosmosSelf
	// Server
	server  websocket.Server
	started bool
}

func newCosmosRemoteHelper(s *CosmosSelf) *cosmosRemotesHelper {
	return &cosmosRemotesHelper{
		self:   s,
		server: websocket.Server{},
	}
}

func (h *cosmosRemotesHelper) init() error {
	config := h.self.config
	logger := log.New(h, "", 0)
	if config.EnableServer == nil {
		h.self.logInfo("cosmosRemotesHelper.init: Disable remote server")
		h.server = websocket.Server{
			ServerDelegate: &websocket.ServerDelegateBase{
				Mux:    map[string]func(http.ResponseWriter, *http.Request){},
				Logger: logger,
				Conn:   map[string]websocket.Connection{},
			},
		}
		return nil
	}
	delegate := &cosmosServerDelegate{
		helper: h,
		ServerDelegateBase: &websocket.ServerDelegateBase{
			Addr: fmt.Sprintf("%s:%d", config.EnableServer.Host, config.EnableServer.Port),
			Name: config.Node,
			Mux: map[string]func(http.ResponseWriter, *http.Request){
				RemoteUriAtomId:      h.handleAtomId,
				RemoteUriAtomMessage: h.handleAtomMsg,
				"/":                  h.handle404,
			},
			Logger: logger,
			Conn:   map[string]websocket.Connection{},
		},
	}
	if config.EnableCert != nil {
		delegate.ServerDelegateBase.CertFile = config.EnableCert.CertPath
		delegate.ServerDelegateBase.KeyFile = config.EnableCert.KeyPath
	}
	if err := h.server.Init(delegate); err != nil {
		return err
	}
	h.server.Start()
	h.started = true
	return nil
}

func (h *cosmosRemotesHelper) close() {
	if h.started {
		h.server.Stop()
		h.started = false
	}
}

func (h *cosmosRemotesHelper) Write(l []byte) (int, error) {
	var msg string
	if len(l) > 0 {
		msg = string(l[:len(l)-1])
	}
	h.self.pushCosmosLog(LogLevel_Info, msg)
	return 0, nil
}

// Client

func (h *cosmosRemotesHelper) getOrConnectRemote(name, addr string, cert *CertConfig) (*cosmosWatchRemote, error) {
	h.server.ServerDelegate.Lock()
	c, has := h.server.ServerDelegate.GetConn(name)
	if !has {
		r := &cosmosWatchRemote{
			name:         name,
			helper:       h,
			elements:     map[string]*ElementRemote{},
			initCh:       make(chan error, 1),
			isServerConn: false,
		}
		// Connect
		client := &websocket.Client{}
		certPath := ""
		if cert != nil {
			certPath = cert.CertPath
		}
		delegate := &cosmosClientDelegate{
			remote:             r,
			ClientDelegateBase: websocket.NewClientDelegate(name, addr, certPath, h.server.ServerDelegate.GetLogger()),
		}
		client.Init(delegate)
		r.Client = client
		h.server.ServerDelegate.SetConn(name, r)
		h.server.ServerDelegate.Unlock()
		// Try connect if offline
		if err := client.Connect(); err != nil {
			h.server.ServerDelegate.GetLogger().Printf("cosmosRemotesHelper.getOrConnectRemote: Connect failed, err=%v", err)
			return nil, err
		}
		return r, nil
	} else {
		h.server.ServerDelegate.Unlock()
		return c.(*cosmosWatchRemote), nil
	}
}

func (h *cosmosRemotesHelper) getRemote(name string) *cosmosWatchRemote {
	h.server.ServerDelegate.Lock()
	defer h.server.ServerDelegate.Unlock()
	c, has := h.server.ServerDelegate.GetConn(name)
	if !has {
		return nil
	}
	return c.(*cosmosWatchRemote)
}

// Implementation of websocket server delegate.

type cosmosServerDelegate struct {
	// Implement ServerDelegate base class to reuse.
	*websocket.ServerDelegateBase

	helper *cosmosRemotesHelper
}

// ServerDelegate Subclass

func (h *cosmosServerDelegate) NewServerConn(name string, conn *websocket.ServerConn) *websocket.ServerConn {
	conn.ServerConnDelegate = &cosmosServerConnDelegate{
		ServerConnDelegate: conn.ServerConnDelegate,
		remote: &cosmosWatchRemote{
			name:         name,
			helper:       h.helper,
			elements:     map[string]*ElementRemote{},
			initCh:       make(chan error, 1),
			isServerConn: true,
			ServerConn:   conn,
		},
	}
	return conn
}

func (h *cosmosServerDelegate) Started() {
}

func (h *cosmosServerDelegate) Stopped() {
	h.helper.self.logInfo("CosmosServer.Stopped: Stopping server, name=%s", h.helper.self.config.Node)
	for name, conn := range h.ServerDelegateBase.Conn {
		var err error
		h.helper.self.logInfo("CosmosServer.Stopped: Stop conn, name=%s,conn=%T", name, conn)
		if _, ok := conn.(*cosmosWatchRemote); !ok {
			h.helper.self.logInfo("CosmosServer.Stopped: Stop conn, stop failed")
		}
		// TODO: No two types of conn.
		switch c := conn.(type) {
		case *cosmosWatchRemote:
			h.helper.self.logInfo("CosmosServer.Stopped: Stop conn, cosmosRemote, server=%v,client=%v", c.Client, c.ServerConn)
			err = c.Client.Stop()
		case *websocket.ServerConn:
			h.helper.self.logInfo("CosmosServer.Stopped: Stop conn, ServerConn, conn=%v", c)
			err = c.Conn.Stop()
		default:
			h.helper.self.logInfo("CosmosServer.Stopped: Stop conn, unknown")
		}
		//switch c := conn.(type) {
		//case *websocket.ServerConn:
		//	err = c.Conn.Stop()
		//case *websocket.Client:
		//	err = c.Conn.Stop()
		//}
		if err != nil {
			h.helper.self.logInfo("CosmosServer.Stopped: Stop conn, name=%s,err=%v", name, err)
		}
	}
}
