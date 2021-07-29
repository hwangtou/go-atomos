package websocket

// ServerConn

type ServerConn struct {
	*Conn
	ServerConnDelegate ServerConnDelegate
}

func (s *ServerConn) Stop() error {
	return s.Conn.Stop()
}
