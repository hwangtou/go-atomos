package go_atomos

import (
	"bufio"
	"io"
	"net"
	"strings"
	"sync"
)

type appUDSDelegate interface {
	onReceive(*AppUDSConn, []byte)
	onClose(conn *AppUDSConn)

	//log() *appLogging
}

type AppUDSConn struct {
	id   int32
	conn *net.UnixConn

	delegate appUDSDelegate

	closeCh chan bool
	closed  bool
	mutex   sync.Mutex

	logging   func(level LogLevel, format string, args ...interface{})
	customize interface{}
}

func newAppUDSConn(id int32, delegate appUDSDelegate, conn *net.UnixConn, logging func(level LogLevel, format string, args ...interface{}), customize interface{}) *AppUDSConn {
	return &AppUDSConn{
		id:        id,
		conn:      conn,
		delegate:  delegate,
		closeCh:   make(chan bool, 1),
		closed:    false,
		mutex:     sync.Mutex{},
		logging:   logging,
		customize: customize,
	}
}

func (c *AppUDSConn) Daemon() {
	defer c.DaemonClose()
	defer func() {
		if r := recover(); r != nil {
			c.logging(LogLevel_Err, "App: UDS connection has recovered and exited. id=(%d),reason=(%v)", c.id, r)
		}
	}()

	reader := bufio.NewReaderSize(c.conn, udsConnReadBufSize)
	readBuf := make([]byte, 0, udsConnReadBufSize)
	for {
		buf, isPrefix, er := reader.ReadLine()
		if er == nil {
			readBuf = append(readBuf, buf...)
			if isPrefix {
				continue
			}
			c.delegate.onReceive(c, readBuf)
			readBuf = make([]byte, 0, udsConnReadBufSize)
			continue
		}

		if !isCloseError(er) {
			c.logging(LogLevel_Err, "App: UDS connection has closed with error. id=(%d),err=(%v)", c.id, er)
		}
		return
	}
}

func (c *AppUDSConn) DaemonClose() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return
	}
	c.closed = true
	if er := c.conn.Close(); er != nil {
		c.logging(LogLevel_Err, "App: UDS connection has closed with error. id=(%d),err=(%v)", c.id, er)
	}

	c.delegate.onClose(c)
	c.closeCh <- true
}

func (c *AppUDSConn) Send(buf []byte) *Error {
	_, er := c.conn.Write(append(buf, []byte("\n")...))
	if er != nil {
		return NewErrorf(ErrAppUnixDomainSocketConnWriteFailed, "App: UDS connection sends error. id=(%d),err=(%v)", c.id, er)
	}
	return nil
}

func (c *AppUDSConn) IsClosed() bool {
	return c.closed
}

func (c *AppUDSConn) CloseCh() chan bool {
	return c.closeCh
}

func isCloseError(er error) bool {
	if er == nil {
		return false
	}
	if er == io.EOF {
		return true
	}
	if strings.Contains(er.Error(), "use of closed network connection") {
		return true
	}
	return false
}
