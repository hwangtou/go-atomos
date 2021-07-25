package websocket

import (
	"errors"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"sync"
)

type Server struct {
	delegate ServerDelegate
	server   *http.Server
	listener net.Listener
	upgrade  websocket.Upgrader

	connMutex sync.Mutex
	conn      map[string]*ServerConn
}

const (
	WatchSchema = "wss"
	WatchUri    = "/watch"
)

var (
	ErrDelegateIsNil              = errors.New("websocket server delegate is nil")
	ErrDelegateMuxIsNil           = errors.New("websocket server delegate mux is nil")
	ErrDelegateMuxConflictWatcher = errors.New("websocket server delegate mux conflict with watcher")
	ErrDelegateLoggerIsNil        = errors.New("websocket server delegate logger is nil")

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
	s.delegate = delegate
	s.conn = map[string]*ServerConn{}
	s.server = &http.Server{
		Addr:      delegate.GetListenAddr(),
		Handler:   mux,
		TLSConfig: tlsConfig,
		ErrorLog:  logger,
	}
	// Listen the local port.
	s.listener, err = net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) Start() {
	go s.server.ServeTLS(s.listener, "", "")
	s.delegate.Started()
}

func (s *Server) Stop() {
	if err := s.listener.Close(); err != nil {
		s.delegate.GetLogger().Printf("Server.Stop: Close listener failed, err=%v", err)
	}
	if err := s.server.Close(); err != nil {
		s.delegate.GetLogger().Printf("Server.Stop: Close server failed, err=%v", err)
	}
	s.delegate.Stopped()
}

func (s *Server) handleWatch(writer http.ResponseWriter, request *http.Request) {
	s.delegate.GetLogger().Printf("Server.handleWatch: Begin")
	// Check header.
	name, err := s.delegate.AcceptHeaderInfo(request.Header)
	if err != nil {
		s.delegate.GetLogger().Printf("Server.handleWatch: Header name invalid, err=%v", err)
		return
	}
	// Upgrade.
	conn, err := s.upgrade.Upgrade(writer, request, nil)
	if err != nil {
		s.delegate.GetLogger().Printf("Server.handleWatch: Upgrade, err=%v", err)
		return
	}
	// Check duplicated.
	s.connMutex.Lock()
	c, hasOld := s.conn[name]
	if !hasOld {
		c = &ServerConn{
			name: name,
			Conn: &Conn{
				delegate:     s.delegate.NewServerConn(name, conn.RemoteAddr()),
				conn:         conn,
				sender:       make(chan *msg),
				running:      false,
				runningMutex: sync.Mutex{},
				reconnecting: false,
				reconnectCh:  make(chan bool, 1),
			},
		}
	}
	s.connMutex.Unlock()
	// Init.
	if !hasOld {
		s.delegate.GetLogger().Printf("Server.handleWatch: Accept new conn Begin")
		if err = s.delegate.AcceptNewConnect(name, c); err != nil {
			s.delegate.GetLogger().Printf("Server.handleWatch: Accept new conn failed, err=%v", err)
			if err = c.Stop(); err != nil {
				s.delegate.GetLogger().Printf("Server.handleWatch: Accept new conn stop error, err=%v", err)
			}
			s.connMutex.Lock()
			if _, has := s.conn[name]; has {
				delete(s.conn, name)
			}
			s.connMutex.Unlock()
			return
		}
		s.connMutex.Lock()
		s.conn[name] = c
		s.connMutex.Unlock()
	} else {
		s.delegate.GetLogger().Printf("Server.handleWatch: Accept reconnect Begin")
		if err = s.delegate.AcceptReconnect(name, c, conn); err != nil {
			s.delegate.GetLogger().Printf("Server.handleWatch: Accept reconnect failed, err=%v", err)
			//if err = c.Stop(); err != nil {
			//	s.delegate.GetLogger().Printf("Server.handleWatch: Accept reconnect stop error, err=%v", err)
			//}
			s.connMutex.Lock()
			if _, has := s.conn[name]; has {
				delete(s.conn, name)
			}
			s.connMutex.Unlock()
			return
		}
	}
}
