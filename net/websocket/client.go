package websocket

import (
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
)

type Client struct {
	*Conn
	delegate ClientDelegate
}

func (c *Client) Init(delegate *clientDelegate) (err error) {
	if delegate == nil {
		return ErrDelegateIsNil
	}
	c.delegate = delegate
	c.Conn = &Conn{
		delegate: delegate,
		sender:   make(chan *msg),
	}
	return nil
}

func (c *Client) Connect() error {
	c.delegate.GetLogger().Printf("Client.Connect: Info, name=%v,addr=%s",
		c.delegate.GetName(), c.delegate.GetAddr())
	// Connect.
	u := url.URL{
		Scheme: WatchSchema,
		Host:   c.delegate.GetAddr(),
		Path:   WatchUri,
	}
	tlsConfig, err := c.delegate.GetTLSConfig()
	if err != nil {
		return err
	}
	dialer := websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}
	c.conn, _, err = dialer.Dial(u.String(), http.Header{
		watchHeader: []string{c.delegate.GetName()},
	})
	if err != nil {
		return err
	}

	c.run()

	if err = c.delegate.Connected(); err != nil {
		if err := c.Stop(); err != nil {
			c.delegate.GetLogger().Printf("Client.Connect: Stop connect error, err=%v", err)
		}
		return err
	}
	return nil
}
