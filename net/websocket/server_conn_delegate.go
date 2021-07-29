package websocket

import (
	"log"
	"net"
	"time"
)

// ServerConnDelegate

type ServerConnDelegate interface {
	ConnDelegate
	// Reconnect
	ReconnectKickOld()
	Reconnected() error
}

type ServerConnDelegateBase struct {
	name   string
	addr   net.Addr
	logger *log.Logger
}

func NewServerConnDelegate(name string, addr net.Addr, logger *log.Logger) *ServerConnDelegateBase {
	return &ServerConnDelegateBase{
		name:   name,
		addr:   addr,
		logger: logger,
	}
}

// Implementation of ConnDelegate.

func (s *ServerConnDelegateBase) GetName() string {
	return s.name
}

func (s *ServerConnDelegateBase) GetAddr() string {
	return s.addr.String()
}

func (s *ServerConnDelegateBase) GetLogger() *log.Logger {
	return s.logger
}

func (s *ServerConnDelegateBase) WriteTimeout() time.Duration {
	return writeWait
}

func (s *ServerConnDelegateBase) PingPeriod() time.Duration {
	return connPingPeriod
}

func (s *ServerConnDelegateBase) Receive(buf []byte) error {
	s.GetLogger().Printf("ServerConnDelegateBase.Receive: %s", string(buf))
	return nil
}

func (s *ServerConnDelegateBase) Connected() error {
	s.GetLogger().Printf("ServerConnDelegateBase.Connected")
	return nil
}

func (s *ServerConnDelegateBase) Disconnected() {
	s.GetLogger().Printf("ServerConnDelegateBase.Disconnected")
}

// Implementation of ServerConnDelegate.

func (s *ServerConnDelegateBase) ReconnectKickOld() {
	s.GetLogger().Printf("ServerConnDelegateBase: ReconnectKickOld")
}

func (s *ServerConnDelegateBase) Reconnected() error {
	s.GetLogger().Printf("ServerConnDelegateBase: Reconnected")
	return nil
}
