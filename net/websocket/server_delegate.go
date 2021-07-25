package websocket

import (
	"crypto/tls"
	"github.com/gorilla/websocket"
	"log"
	"net"
	"net/http"
)

type ServerDelegate interface {
	// Info.
	GetListenAddr() string
	GetListenTLSConfig() (*tls.Config, error)
	GetServerMux() map[string]func(http.ResponseWriter, *http.Request)
	NewServerConn(info string, addr net.Addr) ConnDelegate
	GetLogger() *log.Logger

	// State Changes.
	Started()
	FatalError()
	Stopped()

	// Accept.
	AcceptHeaderInfo(header http.Header) (string, error)
	AcceptNewConnect(info string, serverConn *ServerConn) error
	AcceptReconnect(info string, serverConn *ServerConn, conn *websocket.Conn) error

	// Reconnect.
	ReconnectKickOld()
	Reconnected() error
}

type serverDelegate struct {
	addr, name        string
	certFile, keyFile string
	mux               map[string]func(http.ResponseWriter, *http.Request)
	logger            *log.Logger
}

const (
	watchHeader = "name"
)

func (s *serverDelegate) GetListenAddr() string {
	return s.addr
}

func (s *serverDelegate) GetListenTLSConfig() (conf *tls.Config, err error) {
	if s.certFile == "" && s.keyFile == "" {
		return nil, nil
	}
	conf = &tls.Config{
		Certificates: make([]tls.Certificate, 1),
	}
	conf.Certificates[0], err = tls.LoadX509KeyPair(s.certFile, s.keyFile)
	if err != nil {
		return nil, err
	}
	return
}

func (s *serverDelegate) GetServerMux() map[string]func(http.ResponseWriter, *http.Request) {
	return s.mux
}

func (s *serverDelegate) NewServerConn(name string, addr net.Addr) ConnDelegate {
	return &ServerConnDelegate{
		name:   name,
		addr:   addr,
		logger: s.logger,
	}
}

func (s *serverDelegate) GetLogger() *log.Logger {
	return s.logger
}

// State Changes.

func (s *serverDelegate) Started() {
	s.GetLogger().Printf("Server: Started")
}

func (s *serverDelegate) FatalError() {
	s.GetLogger().Printf("Server: FatalError")
}

func (s *serverDelegate) Stopped() {
	s.GetLogger().Printf("Server: Stopped")
}

// Watcher

func (s *serverDelegate) AcceptHeaderInfo(header http.Header) (string, error) {
	// Check header.
	nodeName := header.Get(watchHeader)
	if nodeName == "" {
		return "", ErrHeaderInvalid
	}
	return nodeName, nil
}

func (s *serverDelegate) AcceptNewConnect(name string, sc *ServerConn) error {
	sc.run()

	if err := sc.delegate.Connected(); err != nil {
		if err := sc.Stop(); err != nil {
			s.GetLogger().Printf("Server: Stop accept error, err=%v", err)
		}
		return err
	}
	return nil
}

func (s *serverDelegate) AcceptReconnect(info string, sc *ServerConn, conn *websocket.Conn) error {
	sc.runningMutex.Lock()
	sc.reconnecting = true
	sc.runningMutex.Unlock()
	if err := sc.Stop(); err != nil {
		s.GetLogger().Printf("ServerDelegate: Accept reconnect stop old connect error, err=%v", err)
	}
	<-sc.reconnectCh

	sc.conn = conn
	sc.sender = make(chan *msg)
	sc.run()
	if err := s.Reconnected(); err != nil {
		if err := sc.Stop(); err != nil {
			s.GetLogger().Printf("ServerDelegate: Accept reconnect error, err=%v", err)
		}
		return err
	}
	return nil
}

func (s *serverDelegate) ReconnectKickOld() {
	s.GetLogger().Printf("ServerDelegate: ReconnectKickOld")
}

func (s *serverDelegate) Reconnected() error {
	s.GetLogger().Printf("ServerDelegate: Reconnected")
	return nil
}
