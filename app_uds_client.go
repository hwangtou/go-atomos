package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"net"
	"sort"
	"sync"
	"time"
)

type AppUDSClient struct {
	mutex   sync.Mutex
	connID  int32
	connMap map[int32]*AppUDSConn
	logging func(level LogLevel, format string, args ...interface{})
}

func NewAppUDSClient(logging func(level LogLevel, format string, args ...interface{})) *AppUDSClient {
	return &AppUDSClient{
		logging: logging,
		//address: addr,
		mutex:   sync.Mutex{},
		connID:  0,
		connMap: map[int32]*AppUDSConn{},
	}
}

func (c *AppUDSClient) Dial(addr string) (*AppUDSConn, *Error) {
	unixAddr, er := net.ResolveUnixAddr("unix", addr)
	if er != nil {
		return nil, NewErrorf(ErrAppUnixDomainSocketDialFailed, "App: UDS client cannot resolve unix address. address=(%s),err=(%v)", addr, er).AddStack(nil)
	}
	conn, er := net.DialUnix("unix", nil, unixAddr)
	if er != nil {
		return nil, NewErrorf(ErrAppUnixDomainSocketDialFailed, "App: UDS client cannot dial unix address. address==(%s),err=(%v)", addr, er).AddStack(nil)
	}

	c.mutex.Lock()
	c.connID += 1
	co := newAppUDSConn(c.connID, c, conn, c.logging, &appUDSClientSession{session: map[int64]AppUDSClientCallback{}})
	c.connMap[c.connID] = co
	c.mutex.Unlock()

	go co.Daemon()
	return co, nil
}

func (c *AppUDSClient) GetConn(id int32) *AppUDSConn {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.connMap[id]
}

func (c *AppUDSClient) Send(conn *AppUDSConn, command string, buf []byte, timeout time.Duration, callback AppUDSClientCallback) {
	session := conn.customize.(*appUDSClientSession)
	session.Lock()
	session.curID += 1
	sendPacket := UDSCommandPacket{
		SessionId: session.curID,
		Command:   command,
		Buf:       buf,
		Err:       nil,
	}
	session.Unlock()

	sendBuf, er := proto.Marshal(&sendPacket)
	if er != nil {
		conn.DaemonClose()
		err := NewErrorf(ErrAppUnixDomainSocketConnWriteFailed, "App: UDS client marshal packet failed. packet=(%v),err=(%v)", &sendPacket, er).AddStack(nil)
		callback(nil, err)
		c.logging(LogLevel_Err, "App: UDS client sends failed. err=(%v)", err)
		return
	}
	if err := conn.Send(sendBuf); err != nil {
		conn.DaemonClose()
		callback(nil, err.AddStack(nil))
		c.logging(LogLevel_Err, "App: UDS client sends packet failed. packet=(%v),err=(%v)", &sendPacket, err)
		return
	}

	session.Lock()
	session.session[session.curID] = callback
	session.Unlock()

	if timeout > 0 {
		go func() {
			<-time.After(timeout)
			session.Lock()
			_, has := session.session[session.curID]
			session.Unlock()
			if has {
				c.logging(LogLevel_Err, "App: UDS client waits for callback timeout. sessionID=(%d)", sendPacket.SessionId)
			}
		}()
	}
}

func (c *AppUDSClient) Close() {
	// Close conn.
	c.mutex.Lock()
	connList := make([]*AppUDSConn, 0, len(c.connMap))
	for id, conn := range c.connMap {
		connList = append(connList, conn)
		delete(c.connMap, id)
	}
	c.mutex.Unlock()
	sort.Slice(connList, func(i, j int) bool {
		return connList[i].id < connList[j].id
	})
	for _, conn := range connList {
		conn.DaemonClose()
	}
	c.logging(LogLevel_Info, "App: UDS listener has closed.")
}

func (c *AppUDSClient) onReceive(conn *AppUDSConn, readBuf []byte) {
	recvPacket := UDSCommandPacket{}
	if er := proto.Unmarshal(readBuf, &recvPacket); er != nil {
		c.logging(LogLevel_Err, "App: UDS client has received invalid packet. id=(%d),err=(%v)", conn.id, er)
		return
	}
	session := conn.customize.(*appUDSClientSession)
	session.Lock()
	callback, has := session.session[recvPacket.SessionId]
	delete(session.session, recvPacket.SessionId)
	session.Unlock()
	if !has {
		c.logging(LogLevel_Err, "App: UDS client has received unknown packet session. id=(%d),session=(%d)", conn.id, recvPacket.SessionId)
		return
	}
	callback(&recvPacket, nil)
}

func (c *AppUDSClient) onClose(conn *AppUDSConn) {
	c.mutex.Lock()
	delete(c.connMap, conn.id)
	c.mutex.Unlock()
}

type appUDSClientSession struct {
	sync.Mutex
	curID   int64
	session map[int64]AppUDSClientCallback
}
