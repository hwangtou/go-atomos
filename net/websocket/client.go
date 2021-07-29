package websocket

import (
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
)

type Client struct {
	*Conn
	ClientDelegate ClientDelegate
}

func (c *Client) Init(delegate ClientDelegate) (err error) {
	if delegate == nil {
		return ErrDelegateIsNil
	}
	c.Conn = &Conn{
		Delegate: delegate,
		sender:   make(chan *msg),
	}
	c.ClientDelegate = delegate
	return nil
}

func (c *Client) Connect() error {
	if c.running {
		c.Delegate.GetLogger().Printf("Client.Connect: Connected, name=%v,addr=%s",
			c.Delegate.GetName(), c.Delegate.GetAddr())
		return nil
	}
	c.Delegate.GetLogger().Printf("Client.Connect: Info, name=%v,addr=%s",
		c.Delegate.GetName(), c.Delegate.GetAddr())
	// Connect.
	u := url.URL{
		Scheme: WatchSchema,
		Host:   c.Delegate.GetAddr(),
		Path:   WatchUri,
	}
	tlsConfig, err := c.ClientDelegate.GetTLSConfig()
	if err != nil {
		return err
	}
	dialer := websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}
	c.conn, _, err = dialer.Dial(u.String(), http.Header{
		watchHeader: []string{c.Delegate.GetName()},
	})
	if err != nil {
		return err
	}

	c.run()

	if err = c.Delegate.Connected(); err != nil {
		if err := c.Stop(); err != nil {
			c.Delegate.GetLogger().Printf("Client.Connect: Stop connect error, err=%v", err)
		}
		return err
	}
	return nil
}
