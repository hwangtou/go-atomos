package go_atomos

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	RemoteUriAtomId      = "/atom_id"
	RemoteUriAtomMessage = "/atom_msg"
)

// Cosmos Remote Helper

type cosmosRemotesHelper struct {
	self  *CosmosSelf
	mutex sync.Mutex
	// Server
	server  Server
	enabled bool
	// State
	started bool
	// Conn
	conns map[string]cosmosConn
}

func newCosmosRemoteHelper(s *CosmosSelf) *cosmosRemotesHelper {
	helper := &cosmosRemotesHelper{
		self:   s,
		server: Server{},
		conns:  map[string]cosmosConn{},
	}
	helper.server.helper = helper
	return helper
}

func (h *cosmosRemotesHelper) delConn(name string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if _, has := h.conns[name]; has {
		delete(h.conns, name)
	}
}

type cosmosConn interface {
	Connection
}

type incomingConn struct {
	name  string
	conn  *Conn
	watch *cosmosWatchRemote
}

func (h *cosmosRemotesHelper) newIncomingConn(remoteName string, conn *websocket.Conn) (ic *incomingConn, hasOld bool) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	c, has := h.conns[remoteName]
	if has {
		ic, ok := c.(*incomingConn)
		if !ok {
			return nil, true
		}
		return ic, true
	}
	ic = &incomingConn{}
	ic.name = remoteName
	ic.conn = newConn(h, ic, conn)
	ic.watch = newCosmosWatchRemote(h, ic)
	h.conns[remoteName] = ic
	return ic, false
}

func (i incomingConn) GetName() string {
	return i.name
}

func (i incomingConn) GetAddr() string {
	// Remote listen address
	if i.watch.enableRemote == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", i.watch.enableRemote.Host, i.watch.enableRemote.Port)
}

func (i incomingConn) WriteTimeout() time.Duration {
	return writeWait
}

func (i incomingConn) PingPeriod() time.Duration {
	return connPingPeriod
}

func (i incomingConn) Receive(conn ConnDelegate, msg []byte) error {
	return i.watch.Receive(conn, msg)
}

func (i incomingConn) Stop() error {
	return i.conn.Stop()
}

func (i incomingConn) Connected(conn ConnDelegate) error {
	return i.watch.Connected(conn)
}

func (i incomingConn) Reconnected(conn ConnDelegate) error {
	return i.watch.Reconnected(conn)
}

func (i incomingConn) Disconnected(conn ConnDelegate) {
	i.watch.Disconnected(conn)
}

type outgoingConn struct {
	name, addr string
	conn       *Conn
	watch      *cosmosWatchRemote
}

func (h *cosmosRemotesHelper) newOutgoingConn(remoteName, remoteAddr string) (oc *outgoingConn, ic *incomingConn, hasOld bool) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	c, has := h.conns[remoteName]
	if has {
		switch conn := c.(type) {
		case *outgoingConn:
			return conn, nil, true
		case *incomingConn:
			return nil, conn, true
		}
	}
	oc = &outgoingConn{}
	oc.name = remoteName
	oc.addr = remoteAddr
	oc.conn = newConn(h, oc, nil)
	oc.watch = newCosmosWatchRemote(h, oc)
	h.conns[remoteName] = oc
	return oc, nil, false
}

func (o *outgoingConn) connect() (err error) {
	cert := o.watch.helper.self.clientCert
	// Connect.
	u := url.URL{
		Host: o.addr,
		Path: WatchUri,
	}
	dialer := websocket.Dialer{}
	if cert == nil {
		u.Scheme = WatchWsSchema
	} else {
		u.Scheme = WatchWssSchema
		dialer.TLSClientConfig = cert
	}
	o.conn.conn, _, err = dialer.Dial(u.String(), http.Header{
		WatchHeader: []string{o.watch.helper.self.config.Node},
	})
	if err != nil {
		return err
	}
	if err = o.watch.initRequester(); err != nil {
		return err
	}

	o.conn.run()

	if err = o.Connected(o); err != nil {
		if err = o.conn.Stop(); err != nil {
			o.watch.helper.self.logError("Remote.Conn: Stop connect error, err=%v", err)
		}
		return err
	}
	return nil
}

func (o outgoingConn) GetName() string {
	return o.name
}

func (o outgoingConn) GetAddr() string {
	return o.addr
}

func (o outgoingConn) WriteTimeout() time.Duration {
	return writeWait
}

func (o outgoingConn) PingPeriod() time.Duration {
	return connPingPeriod
}

func (o outgoingConn) Receive(conn ConnDelegate, msg []byte) error {
	return o.watch.Receive(conn, msg)
}

func (o outgoingConn) Stop() error {
	return o.conn.Stop()
}

func (o outgoingConn) Connected(conn ConnDelegate) error {
	return o.watch.Connected(conn)
}

func (o outgoingConn) Disconnected(conn ConnDelegate) {
	o.watch.Disconnected(conn)
}

func (h *cosmosRemotesHelper) init() (err error) {
	// Config shortcut
	config := h.self.config
	// Enable Cert
	if config.EnableCert != nil {
		h.self.logInfo("Cosmos.Init: Enable Cert, cert=%s,key=%s",
			config.EnableCert.CertPath, config.EnableCert.KeyPath)
		pair, err := tls.LoadX509KeyPair(config.EnableCert.CertPath, config.EnableCert.KeyPath)
		if err != nil {
			return err
		}
		h.server.tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{
				pair,
			},
		}
	}
	// Enable Server
	if config.EnableServer != nil {
		h.self.logInfo("Cosmos.Init: Enable Server, host=%s,port=%d",
			config.EnableServer.Host, config.EnableServer.Port)
		h.enabled = true
		if err = h.server.init(config.EnableServer.Host, config.EnableServer.Port); err != nil {
			return err
		}
		h.started = true
		h.server.start()
	}
	h.started = true
	return nil
}

func (h *cosmosRemotesHelper) close() {
	if h.started {
		h.server.stop()
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

func (h *cosmosRemotesHelper) getOrConnectRemote(name, addr string) (*cosmosWatchRemote, error) {
	oc, ic, has := h.newOutgoingConn(name, addr)
	if has {
		if oc != nil {
			return oc.watch, nil
		}
		if ic != nil {
			return ic.watch, nil
		}
		return nil, errors.New("connection invalid")
	}
	// Connect
	if err := oc.connect(); err != nil {
		h.delConn(name)
		return nil, err
	}
	// Try connect if offline
	return oc.watch, nil
}

func (h *cosmosRemotesHelper) getRemote(name string) *cosmosWatchRemote {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	conn, has := h.conns[name]
	if !has {
		return nil
	}
	switch c := conn.(type) {
	case *incomingConn:
		return c.watch
	case *outgoingConn:
		return c.watch
	}
	return nil
}

func (h *cosmosRemotesHelper) serverStarted() {
}

func (h *cosmosRemotesHelper) serverStopped() {
}
