package go_atomos

import (
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/hwangtou/go-atomos/core"
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

type cosmosRemoteServer struct {
	self  *CosmosMainFn
	mutex sync.Mutex
	// Server
	server Server
	//enabled bool
	// State
	started bool
	// Conn
	conn map[string]cosmosConn
}

func newCosmosRemoteHelper(s *CosmosMainFn) *cosmosRemoteServer {
	helper := &cosmosRemoteServer{
		self:   s,
		server: Server{},
		conn:   map[string]cosmosConn{},
	}
	helper.server.helper = helper
	return helper
}

func (h *cosmosRemoteServer) delConn(name string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if _, has := h.conn[name]; has {
		delete(h.conn, name)
	}
}

type cosmosConn interface {
	Connection
}

type incomingConn struct {
	name  string
	conn  *Conn
	watch *cosmosRemote
}

func (h *cosmosRemoteServer) newIncomingConn(remoteName string, conn *websocket.Conn) (ic *incomingConn, hasOld bool) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	c, has := h.conn[remoteName]
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
	ic.watch = newCosmosRemote(h, ic)
	h.conn[remoteName] = ic
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
	watch      *cosmosRemote
}

func (h *cosmosRemoteServer) newOutgoingConn(remoteName, remoteAddr string) (oc *outgoingConn, ic *incomingConn, hasOld bool) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	c, has := h.conn[remoteName]
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
	oc.watch = newCosmosRemote(h, oc)
	h.conn[remoteName] = oc
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

func (h *cosmosRemoteServer) init() (err *core.ErrorInfo) {
	// Config shortcut
	config := h.self.config.EnableServer
	// Enable Server
	//h.enabled = true
	if err = h.server.init(config.Host, config.Port); err != nil {
		return err
	}
	//h.started = true
	//h.server.start()
	//h.started = true
	return nil
}

func (h *cosmosRemoteServer) start() {
	h.server.start()
	h.started = true
}

func (h *cosmosRemoteServer) close() {
	if h.started {
		h.server.stop()
		h.started = false
	}
}

func (h *cosmosRemoteServer) Write(l []byte) (int, error) {
	var msg string
	if len(l) > 0 {
		msg = string(l[:len(l)-1])
	}
	h.self.pushCosmosLog(LogLevel_Info, msg)
	return 0, nil
}

// Client

func (h *cosmosRemoteServer) getOrConnectRemote(name, addr string) (*cosmosRemote, error) {
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

func (h *cosmosRemoteServer) getRemote(name string) *cosmosRemote {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	conn, has := h.conn[name]
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

func (h *cosmosRemoteServer) serverStarted() {
}

func (h *cosmosRemoteServer) serverStopped() {
}
