package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"net"
	"os"
	"path"
	"sort"
	"sync"
)

type appUDSServer struct {
	config  *Config
	logging *appLogging

	addr     *net.UnixAddr
	listener *net.UnixListener

	mutex   sync.Mutex
	connID  int32
	connMap map[int32]*AppUDSConn

	commandHandler map[string]AppUDSCommandFn
}

func (s *appUDSServer) check() *Error {
	runSocketPath := path.Join(s.config.RunPath, s.config.Node+".socket")
	if _, er := os.Stat(runSocketPath); er != nil && !os.IsNotExist(er) {
		return NewErrorf(ErrAppUnixDomainSocketFileInvalid, "App: UDS server checks socket file failed, address=(%s),err=(%v)", runSocketPath, er).AddStack(nil)
	} else if er == nil {
		if er = os.Remove(runSocketPath); er != nil {
			return NewErrorf(ErrAppUnixDomainSocketFileInvalid, "App: UDS server removes old socket file failed, address=(%s),err=(%v)", runSocketPath, er).AddStack(nil)
		}
	}
	unixAddr, er := net.ResolveUnixAddr("unix", runSocketPath)
	if er != nil {
		return NewErrorf(ErrAppUnixDomainSocketListenFailed, "App: UDS server resolves address failed. address=(%s),err=(%v)", runSocketPath, er).AddStack(nil)
	}
	s.addr = unixAddr
	return nil
}

func (s *appUDSServer) daemon() *Error {
	l, er := net.ListenUnix("unix", s.addr)
	if er != nil {
		return NewErrorf(ErrAppUnixDomainSocketListenFailed, "App: UDS server listens failed. address=(%s),err=(%v)", s.addr.Name, er).AddStack(nil)
	}
	s.listener = l

	go func() {
		defer func() {
			if r := recover(); r != nil {
				SharedLogging().PushProcessLog(LogLevel_Err, "App: UDS server has recovered and exit. reason=(%v)", r)
			}
		}()
		for {
			conn, er := s.listener.AcceptUnix()
			if er != nil {
				if !isCloseError(er) {
					SharedLogging().PushProcessLog(LogLevel_Err, "App: UDS server has exited. err=(%v)", er)
				}
				return
			}
			s.mutex.Lock()
			s.connID += 1
			c := newAppUDSConn(s.connID, s, conn, SharedLogging().PushProcessLog, nil)
			s.connMap[s.connID] = c
			s.mutex.Unlock()
			SharedLogging().PushProcessLog(LogLevel_Info, "App: UDS server accepts connection. conn=(%d)", c.id)
			go c.Daemon()
		}
	}()
	return nil
}

func (s *appUDSServer) close() {
	// Listener close.
	if er := s.listener.Close(); er != nil {
		SharedLogging().PushProcessLog(LogLevel_Err, "App: UDS server closes error. err=(%v)", er)
	}
	// Remove.
	runSocketPath := path.Join(s.config.RunPath, s.config.Node+".socket")
	er := os.Remove(runSocketPath)
	if er != nil && !os.IsNotExist(er) {
		SharedLogging().PushProcessLog(LogLevel_Err, "App: UDS server closes error. err=(%v)", er)
	}
	// Close conn.
	s.mutex.Lock()
	connList := make([]*AppUDSConn, 0, len(s.connMap))
	for id, conn := range s.connMap {
		connList = append(connList, conn)
		delete(s.connMap, id)
	}
	s.mutex.Unlock()
	sort.Slice(connList, func(i, j int) bool {
		return connList[i].id < connList[j].id
	})
	for _, conn := range connList {
		conn.DaemonClose()
	}
	SharedLogging().PushProcessLog(LogLevel_Info, "App: UDS server has closed.")
}

func (s *appUDSServer) onReceive(conn *AppUDSConn, readBuf []byte) {
	recvPacket, sendPacket := UDSCommandPacket{}, UDSCommandPacket{}
	var handlerFn AppUDSCommandFn
	var has bool
	if er := proto.Unmarshal(readBuf, &recvPacket); er != nil {
		SharedLogging().PushProcessLog(LogLevel_Err, "App: UDS server connection has received invalid packet. buf=(%v),err=(%v)", readBuf, er)
		sendPacket.Command = UDSInvalidPacket
		goto send
	}
	sendPacket.SessionId = recvPacket.SessionId
	handlerFn, has = s.commandHandler[recvPacket.Command]
	if !has {
		sendPacket.Command = UDSInvalidCommand
		goto send
	}
	sendPacket.Buf, sendPacket.Err = handlerFn(recvPacket.Buf)
send:
	sendBuf, er := proto.Marshal(&sendPacket)
	if er != nil {
		conn.DaemonClose()
		SharedLogging().PushProcessLog(LogLevel_Err, "App: UDS server connection would send invalid packet. buf=(%v),err=(%v)", &sendPacket, er)
		return
	}
	if err := conn.Send(sendBuf); err != nil {
		conn.DaemonClose()
		SharedLogging().PushProcessLog(LogLevel_Err, "App: UDS server connection sends packet failed. buf=(%v),err=(%v)", &sendPacket, err)
		return
	}
}

func (s *appUDSServer) onClose(conn *AppUDSConn) {
	s.mutex.Lock()
	delete(s.connMap, conn.id)
	s.mutex.Unlock()
	SharedLogging().PushProcessLog(LogLevel_Info, "App: UDS server connection has closed. id=(%d)", conn.id)
}

func (s *appUDSServer) log() *appLogging {
	return s.logging
}
