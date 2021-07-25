package websocket

import (
	"log"
	"net"
	"time"
)

type ServerConn struct {
	*Conn
	name string
}

type ServerConnDelegate struct {
	name   string
	addr   net.Addr
	logger *log.Logger
}

// Implementation of ConnDelegate.

func (s *ServerConnDelegate) GetName() string {
	return s.name
}

func (s *ServerConnDelegate) GetAddr() string {
	return s.addr.String()
}

func (s *ServerConnDelegate) GetLogger() *log.Logger {
	return s.logger
}

func (s *ServerConnDelegate) WriteTimeout() time.Duration {
	return writeWait
}

func (s *ServerConnDelegate) PingPeriod() time.Duration {
	return connPingPeriod
}

func (s *ServerConnDelegate) Receive(buf []byte) error {
	s.GetLogger().Printf("ServerConnDelegate.Receive: %s", string(buf))
	return nil
}

func (s *ServerConnDelegate) Connected() error {
	s.GetLogger().Printf("ServerConnDelegate.Connected")
	return nil
}

func (s *ServerConnDelegate) Disconnected() {
	s.GetLogger().Printf("ServerConnDelegate.Disconnected")
}
