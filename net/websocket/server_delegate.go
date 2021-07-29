package websocket

import (
	"crypto/tls"
	"log"
	"net/http"
	"sync"
)

type ServerDelegate interface {
	// Info.
	GetListenAddr() string
	GetListenTLSConfig() (*tls.Config, error)
	GetServerMux() map[string]func(http.ResponseWriter, *http.Request)
	NewServerConn(info string, conn *ServerConn) *ServerConn
	GetLogger() *log.Logger

	// Connection Management.
	Lock()
	Unlock()
	GetConn(name string) (conn Connection, has bool)
	SetConn(name string, conn Connection)
	DelConn(name string)

	// State Changes.
	Started()
	FatalError()
	Stopped()
}

type ServerDelegateBase struct {
	sync.Mutex
	Addr     string
	Name     string
	CertFile string
	KeyFile  string
	Mux      map[string]func(http.ResponseWriter, *http.Request)
	Logger   *log.Logger

	Conn map[string]Connection
}

const (
	watchHeader = "name"
)

// Implementation of ServerDelegate

func (s *ServerDelegateBase) GetListenAddr() string {
	return s.Addr
}

func (s *ServerDelegateBase) GetListenTLSConfig() (conf *tls.Config, err error) {
	if s.CertFile == "" && s.KeyFile == "" {
		return nil, nil
	}
	conf = &tls.Config{
		Certificates: make([]tls.Certificate, 1),
	}
	conf.Certificates[0], err = tls.LoadX509KeyPair(s.CertFile, s.KeyFile)
	if err != nil {
		return nil, err
	}
	return
}

func (s *ServerDelegateBase) GetServerMux() map[string]func(http.ResponseWriter, *http.Request) {
	return s.Mux
}

func (s *ServerDelegateBase) NewServerConn(name string, conn *ServerConn) *ServerConn {
	return conn
}

func (s *ServerDelegateBase) GetLogger() *log.Logger {
	return s.Logger
}

// Connection Management.

func (s *ServerDelegateBase) GetConn(name string) (conn Connection, has bool) {
	conn, has = s.Conn[name]
	return
}

func (s *ServerDelegateBase) SetConn(name string, conn Connection) {
	s.Conn[name] = conn
}

func (s *ServerDelegateBase) DelConn(name string) {
	if _, has := s.Conn[name]; has {
		delete(s.Conn, name)
	}
}

// State Changes.

func (s *ServerDelegateBase) Started() {
	s.GetLogger().Printf("Server: Started")
}

func (s *ServerDelegateBase) FatalError() {
	s.GetLogger().Printf("Server: FatalError")
}

func (s *ServerDelegateBase) Stopped() {
	s.GetLogger().Printf("Server: Stopped")
	s.Lock()
	defer s.Unlock()
	for name, conn := range s.Conn {
		c := conn.(Connection)
		err := c.Stop()
		if err != nil {
			s.GetLogger().Printf("Server: Stopping conn error, err=%v", err)
		}
		delete(s.Conn, name)
	}
}
