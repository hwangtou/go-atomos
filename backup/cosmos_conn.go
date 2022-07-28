package go_atomos

import (
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

// Interfaces

type Connection interface {
	Stop() error
}

type ConnDelegate interface {
	GetName() string
	GetAddr() string

	WriteTimeout() time.Duration
	PingPeriod() time.Duration
	Receive(conn ConnDelegate, msg []byte) error
	// State
	Connected(conn ConnDelegate) error
	Disconnected(conn ConnDelegate)
}

// Base Conn Struct

const (
	writeWait      = 2 * time.Second
	connPingPeriod = 10 * time.Second
)

type Conn struct {
	delegate ConnDelegate
	helper   *cosmosRemoteServer

	conn         *websocket.Conn
	sender       chan *msg
	runningMutex sync.Mutex
	running      bool
	reconnecting bool
	reconnectCh  chan bool
}

type msg struct {
	msgType int
	msgBuf  []byte
}

func newConn(h *cosmosRemoteServer, delegate ConnDelegate, conn *websocket.Conn) *Conn {
	return &Conn{
		delegate:     delegate,
		helper:       h,
		conn:         conn,
		sender:       make(chan *msg),
		runningMutex: sync.Mutex{},
		running:      false,
		reconnecting: false,
		reconnectCh:  make(chan bool, 1),
	}
}

func (c *Conn) run() {
	c.runningMutex.Lock()
	c.running = true
	c.runningMutex.Unlock()

	go c.writePump()
	go c.readPump()
}

// write content.
func (c *Conn) write(sc *msg) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
			c.helper.self.logError("Conn: Write Panic, err=%v", err)
		}
	}()

	c.runningMutex.Lock()
	sender := c.sender
	c.runningMutex.Unlock()
	sender <- sc
	return
}

func (c *Conn) Send(buf []byte) error {
	return c.write(&msg{
		msgType: websocket.BinaryMessage,
		msgBuf:  buf,
	})
}

// Stop.
func (c *Conn) Stop() (err error) {
	c.helper.self.logInfo("Conn: Stop Stopping")
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
			c.helper.self.logError("Conn: Stop Panic, err=%v", err)
		}
	}()
	c.runningMutex.Lock()
	sender := c.sender
	c.sender = nil
	c.runningMutex.Unlock()
	if sender != nil {
		close(sender)
		return
	} else {
		return errors.New("conn stopped")
	}
}

func (c *Conn) writePump() {
	ticker := time.NewTicker(connPingPeriod)
	defer func() {
		ticker.Stop()
		c.runningMutex.Lock()
		closed := c.running
		c.running = false
		c.runningMutex.Unlock()
		if !closed {
			if err := c.conn.Close(); err != nil {
				c.helper.self.logError("Conn: WritePump Close failed, err=%v", err)
			}
		}
		c.helper.self.logInfo("Conn: WritePump Closing End")
	}()
	for {
		c.runningMutex.Lock()
		sender := c.sender
		c.runningMutex.Unlock()
		if sender == nil {
			return
		}
		select {
		case msg, ok := <-sender:
			now := time.Now()
			if err := c.conn.SetWriteDeadline(now.Add(writeWait)); err != nil {
				c.helper.self.logError("Conn: WritePump SetWriteDeadline failed, err=%v", err)
			}
			if !ok {
				c.helper.self.logInfo("Conn: WritePump Closing Begin")
				if c.running {
					c.helper.self.logError("Conn: WritePump Close Message")
					if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
						c.helper.self.logError("Conn: WritePump Write CloseMessage failed, err=%v", err)
					}
				}
				c.runningMutex.Lock()
				c.sender = nil
				c.runningMutex.Unlock()
				return
			}
			switch msg.msgType {
			case websocket.BinaryMessage:
				w, err := c.conn.NextWriter(websocket.BinaryMessage)
				if err != nil {
					c.helper.self.logError("Conn: WritePump Writer invalid, err=%v", err)
					return
				}
				if _, err = w.Write(msg.msgBuf); err != nil {
					c.helper.self.logError("Conn: WritePump Write failed, err=%v", err)
					return
				}
				if err = w.Close(); err != nil {
					c.helper.self.logError("Conn: WritePump Writer close failed, err=%v", err)
					return
				}
			case websocket.PongMessage:
				if err := c.conn.WriteMessage(websocket.PongMessage, nil); err != nil {
					c.helper.self.logError("Conn: WritePump Writer pong failed, err=%v", err)
					return
				}
			}
		case <-ticker.C:
			now := time.Now()
			if err := c.conn.SetWriteDeadline(now.Add(writeWait)); err != nil {
				c.helper.self.logError("Conn: WritePump Ping SetWriteDeadline failed, err=%v", err)
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.helper.self.logError("Conn: WritePump Ping WriteMessage failed, err=%v", err)
				return
			}
		}
	}
}

func (c *Conn) readPump() {
	defer func() {
		c.helper.self.logInfo("Conn: ReadPump Closing Begin")
		c.runningMutex.Lock()
		closed, reconnecting, sender := c.running, c.reconnecting, c.sender
		c.running, c.reconnecting, c.sender = false, false, nil
		c.runningMutex.Unlock()
		if !closed {
			if err := c.conn.Close(); err != nil {
				c.helper.self.logError("Conn: ReadPump Close failed, err=%v", err)
			}
		}
		if sender != nil {
			close(sender)
		}
		c.helper.self.logInfo("Conn: ReadPump Closing End")
		if reconnecting {
			c.helper.self.logInfo("Conn: ReadPump Closing Send Reconnect")
			c.reconnectCh <- true
		}
	}()
	now := time.Now()
	pongWait := c.delegate.PingPeriod() + c.delegate.WriteTimeout()
	if err := c.conn.SetReadDeadline(now.Add(pongWait)); err != nil {
		c.helper.self.logError("Conn: ReadPump SetReadDeadline failed, err=%v", err)
	}
	c.conn.SetPingHandler(c.pingHandler)
	c.conn.SetPongHandler(c.pongHandler)
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err) {
				c.helper.self.logError("Conn: ReadPump Read failed, err=%v", err)
			}
			break
		}
		err = c.delegate.Receive(c.delegate, msg)
		if err != nil {
			c.helper.self.logError("Conn: ReadPump Handle read failed, err=%v", err)
			break
		}
	}
}

func (c *Conn) pingHandler(data string) error {
	//c.helper.self.logInfo("Conn: PingHandler: name=%s", c.delegate.GetName())
	now := time.Now()
	pongWait := c.delegate.PingPeriod() + c.delegate.WriteTimeout()
	if err := c.conn.SetReadDeadline(now.Add(pongWait)); err != nil {
		c.helper.self.logError("Conn: PingHandler SetReadDeadline failed, name=%s,err=%v", c.delegate.GetName(), err)
	}
	return c.write(&msg{websocket.PongMessage, []byte{}})
}

func (c *Conn) pongHandler(data string) error {
	//c.helper.self.logInfo("Conn: PongHandler name=%s", c.delegate.GetName())
	return nil
}
