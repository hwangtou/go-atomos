package websocket

import (
	"errors"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"sync"
)

type Server struct {
	ServerDelegate ServerDelegate
	server         *http.Server
	listener       net.Listener
	upgrade        websocket.Upgrader
}

const (
	WatchSchema = "wss"
	WatchUri    = "/watch"
)

var (
	ErrDelegateIsNil              = errors.New("websocket server delegate is nil")
	ErrDelegateMuxIsNil           = errors.New("websocket server delegate mux is nil")
	ErrDelegateMuxConflictWatcher = errors.New("websocket server delegate mux conflict with watcher")
	ErrDelegateLoggerIsNil        = errors.New("websocket server delegate Logger is nil")

	ErrHeaderInvalid = errors.New("websocket server accept header invalid")
)

func (s *Server) Init(delegate ServerDelegate) (err error) {
	if delegate == nil {
		// TODO
		return ErrDelegateIsNil
	}
	// TLS Check.
	tlsConfig, err := delegate.GetListenTLSConfig()
	if err != nil {
		return err
	}
	// Mux Check.
	muxMap := delegate.GetServerMux()
	if muxMap == nil {
		return ErrDelegateMuxIsNil
	}
	if _, has := muxMap[WatchUri]; has {
		return ErrDelegateMuxConflictWatcher
	}
	// Log Check.
	logger := delegate.GetLogger()
	if logger == nil {
		return ErrDelegateLoggerIsNil
	}
	// Server Mux.
	mux := http.NewServeMux()
	mux.HandleFunc(WatchUri, s.handleWatch)
	for uri, handler := range muxMap {
		mux.HandleFunc(uri, handler)
	}
	s.server = &http.Server{
		Addr:      delegate.GetListenAddr(),
		Handler:   mux,
		TLSConfig: tlsConfig,
		ErrorLog:  logger,
	}
	s.ServerDelegate = delegate
	// Listen the local port.
	s.listener, err = net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) Start() {
	go s.server.ServeTLS(s.listener, "", "")
	s.ServerDelegate.Started()
}

func (s *Server) Stop() {
	if err := s.listener.Close(); err != nil {
		s.ServerDelegate.GetLogger().Printf("Server.Stop: Close listener failed, err=%v", err)
	}
	if err := s.server.Close(); err != nil {
		s.ServerDelegate.GetLogger().Printf("Server.Stop: Close server failed, err=%v", err)
	}
	s.ServerDelegate.Stopped()
}

func (s *Server) handleWatch(writer http.ResponseWriter, request *http.Request) {
	s.ServerDelegate.GetLogger().Printf("Server.handleWatch: Begin")
	// Check header.
	name, err := s.acceptHeaderInfo(request.Header)
	if err != nil {
		s.ServerDelegate.GetLogger().Printf("Server.handleWatch: Header name invalid, err=%v", err)
		return
	}
	// Upgrade.
	conn, err := s.upgrade.Upgrade(writer, request, nil)
	if err != nil {
		s.ServerDelegate.GetLogger().Printf("Server.handleWatch: Upgrade, err=%v", err)
		return
	}
	// Check duplicated.
	s.ServerDelegate.Lock()
	var newServerConn *ServerConn
	c, hasOld := s.ServerDelegate.GetConn(name)
	if !hasOld {
		newServerConn = &ServerConn{
			Conn: &Conn{
				Delegate:     nil,
				conn:         conn,
				sender:       make(chan *msg),
				runningMutex: sync.Mutex{},
				running:      false,
				reconnecting: false,
				reconnectCh:  make(chan bool, 1),
			},
			ServerConnDelegate: &ServerConnDelegateBase{
				name:   name,
				addr:   conn.RemoteAddr(),
				logger: s.ServerDelegate.GetLogger(),
			},
		}
		sc := s.ServerDelegate.NewServerConn(name, newServerConn)
		sc.Conn.Delegate = sc.ServerConnDelegate
		c = newServerConn
		s.ServerDelegate.SetConn(name, c)
	}
	s.ServerDelegate.Unlock()
	// Accept new connection.
	if newServerConn != nil {
		s.ServerDelegate.GetLogger().Printf("Server.handleWatch: Accept new conn Begin")
		if err = s.acceptNewConnect(name, newServerConn); err != nil {
			s.ServerDelegate.GetLogger().Printf("Server.handleWatch: Accept new conn failed, err=%v", err)
			if err = newServerConn.Stop(); err != nil {
				s.ServerDelegate.GetLogger().Printf("Server.handleWatch: Accept new conn stop error, err=%v", err)
			}
			s.ServerDelegate.Lock()
			s.ServerDelegate.DelConn(name)
			s.ServerDelegate.Unlock()
		}
		return
	}
	// Accept connection exist.
	switch c := c.(type) {
	case *ServerConn:
		// Accepted connection reconnected.
		s.ServerDelegate.GetLogger().Printf("Server.handleWatch: Accept reconnect Begin")
		if err = s.acceptReconnect(name, c, conn); err != nil {
			s.ServerDelegate.GetLogger().Printf("Server.handleWatch: Accept reconnect failed, err=%v", err)
			if err = c.Stop(); err != nil {
				s.ServerDelegate.GetLogger().Printf("Server.handleWatch: Accept reconnect stop error, err=%v", err)
			}
			s.ServerDelegate.Lock()
			s.ServerDelegate.DelConn(name)
			s.ServerDelegate.Unlock()
			return
		}
	case *Client:
		// Currently not supported.
		s.ServerDelegate.GetLogger().Printf("Server.handleWatch: Currently not support server conn reconnect as client.")
		return
	}
}

func (s *Server) acceptHeaderInfo(header http.Header) (string, error) {
	// Check header.
	nodeName := header.Get(watchHeader)
	if nodeName == "" {
		return "", ErrHeaderInvalid
	}
	return nodeName, nil
}

func (s *Server) acceptNewConnect(name string, sc *ServerConn) error {
	sc.run()

	if err := sc.ServerConnDelegate.Connected(); err != nil {
		if err := sc.Stop(); err != nil {
			s.ServerDelegate.GetLogger().Printf("Server: Stop accept error, err=%v", err)
		}
		return err
	}
	return nil
}

func (s *Server) acceptReconnect(info string, sc *ServerConn, conn *websocket.Conn) error {
	sc.runningMutex.Lock()
	sc.reconnecting = true
	sc.runningMutex.Unlock()
	if err := sc.Stop(); err != nil {
		s.ServerDelegate.GetLogger().Printf("ServerDelegate: Accept reconnect stop old connect error, err=%v", err)
	}
	<-sc.reconnectCh

	sc.conn = conn
	sc.sender = make(chan *msg)
	sc.run()
	if err := sc.ServerConnDelegate.Reconnected(); err != nil {
		if err := sc.Stop(); err != nil {
			s.ServerDelegate.GetLogger().Printf("ServerDelegate: Accept reconnect error, err=%v", err)
		}
		return err
	}
	return nil
}
