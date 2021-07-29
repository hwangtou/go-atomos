package websocket

import (
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"sync"
	"time"
)

type Conn struct {
	Delegate ConnDelegate

	conn         *websocket.Conn
	sender       chan *msg
	runningMutex sync.Mutex
	running      bool
	reconnecting bool
	reconnectCh  chan bool
}

type Connection interface {
	Stop() error
}

type ConnDelegate interface {
	GetName() string
	GetAddr() string
	GetLogger() *log.Logger

	WriteTimeout() time.Duration
	PingPeriod() time.Duration
	Receive(msg []byte) error
	// State
	Connected() error
	Disconnected()
}

const (
	writeWait      = 2 * time.Second
	connPingPeriod = 10 * time.Second
)

type msg struct {
	msgType int
	msgBuf  []byte
}

func (c *Conn) run() {
	c.runningMutex.Lock()
	c.running = true
	c.runningMutex.Unlock()

	go c.writePump()
	go c.readPump()
}

func (c *Conn) logger(format string, args ...interface{}) {
	c.Delegate.GetLogger().Printf(format, args...)
}

// write content.
func (c *Conn) write(sc *msg) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
			c.logger("Conn.write: Panic, err=%v", err)
		}
	}()

	c.runningMutex.Lock()
	defer c.runningMutex.Unlock()
	c.sender <- sc
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
	c.logger("Conn.Stop: Stopping")
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
			c.logger("Conn.Stop: Panic, err=%v", err)
		}
	}()
	c.runningMutex.Lock()
	defer c.runningMutex.Unlock()
	if c.sender != nil {
		close(c.sender)
		c.sender = nil
	}
	return
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
				c.logger("Conn.writePump: Close failed, err=%v", err)
			}
		}
		c.logger("Conn.writePump: Closing End")
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
				c.logger("Conn.writePump: SetWriteDeadline failed, err=%v", err)
			}
			if !ok {
				c.logger("Conn.writePump: Closing Begin")
				if c.running {
					c.logger("Conn.writePump: Close Message")
					if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
						c.logger("Conn.writePump: Write CloseMessage failed, err=%v", err)
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
					c.logger("Conn.writePump: Writer invalid, err=%v", err)
					return
				}
				if _, err = w.Write(msg.msgBuf); err != nil {
					c.logger("Conn.writePump: Write failed, err=%v", err)
					return
				}
				if err = w.Close(); err != nil {
					c.logger("Conn.writePump: Writer close failed, err=%v", err)
					return
				}
			case websocket.PongMessage:
				if err := c.conn.WriteMessage(websocket.PongMessage, nil); err != nil {
					c.logger("Conn.writePump: Writer pong failed, err=%v", err)
					return
				}
			}
		case <-ticker.C:
			now := time.Now()
			if err := c.conn.SetWriteDeadline(now.Add(writeWait)); err != nil {
				c.logger("Conn.writePump: Ping SetWriteDeadline failed, err=%v", err)
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.logger("Conn.writePump: Ping WriteMessage failed, err=%v", err)
				return
			}
		}
	}
}

func (c *Conn) readPump() {
	defer func() {
		c.logger("Conn.readPump: Closing Begin")
		c.runningMutex.Lock()
		closed, reconnecting, sender := c.running, c.reconnecting, c.sender
		c.running, c.reconnecting, c.sender = false, false, nil
		c.runningMutex.Unlock()
		if !closed {
			if err := c.conn.Close(); err != nil {
				c.logger("Conn.readPump: Close failed, err=%v", err)
			}
		}
		if sender != nil {
			close(sender)
		}
		c.logger("Conn.readPump: Closing End")
		if reconnecting {
			c.logger("Conn.readPump: Closing Send Reconnect")
			c.reconnectCh <- true
		}
	}()
	now := time.Now()
	pongWait := c.Delegate.PingPeriod() + c.Delegate.WriteTimeout()
	if err := c.conn.SetReadDeadline(now.Add(pongWait)); err != nil {
		c.logger("Conn.readPump: SetReadDeadline failed, err=%v", err)
	}
	c.conn.SetPingHandler(c.pingHandler)
	c.conn.SetPongHandler(c.pongHandler)
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err) {
				c.logger("Conn.readPump: Read failed, err=%v", err)
			}
			break
		}
		err = c.Delegate.Receive(msg)
		if err != nil {
			c.logger("Conn.readPump: Handle read failed, err=%v", err)
			break
		}
	}
}

func (c *Conn) pingHandler(data string) error {
	c.logger("Conn.pingHandler: name=%s", c.Delegate.GetName())
	now := time.Now()
	pongWait := c.Delegate.PingPeriod() + c.Delegate.WriteTimeout()
	if err := c.conn.SetReadDeadline(now.Add(pongWait)); err != nil {
		c.logger("Conn.pingHandler: SetReadDeadline failed, name=%s,err=%v", c.Delegate.GetName(), err)
	}
	return c.write(&msg{websocket.PongMessage, []byte{}})
}

func (c *Conn) pongHandler(data string) error {
	c.logger("Conn.pongHandler: name=%s", c.Delegate.GetName())
	return nil
}
