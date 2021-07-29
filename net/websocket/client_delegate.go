package websocket

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"log"
	"time"
)

type ClientDelegate interface {
	ConnDelegate
	GetTLSConfig() (*tls.Config, error)
	ShouldSkipVerify() bool
}

type ClientDelegateBase struct {
	name     string
	addr     string
	certFile string
	logger   *log.Logger
}

func NewClientDelegate(name, addr, certFile string, logger *log.Logger) *ClientDelegateBase {
	return &ClientDelegateBase{
		name:     name,
		addr:     addr,
		certFile: certFile,
		logger:   logger,
	}
}

// Implementation of ConnDelegate.

func (c *ClientDelegateBase) GetName() string {
	return c.name
}

func (c *ClientDelegateBase) GetAddr() string {
	return c.addr
}

func (c *ClientDelegateBase) GetLogger() *log.Logger {
	return c.logger
}

func (c *ClientDelegateBase) WriteTimeout() time.Duration {
	return writeWait
}

func (c *ClientDelegateBase) PingPeriod() time.Duration {
	return connPingPeriod
}

func (c *ClientDelegateBase) Receive(msg []byte) error {
	c.GetLogger().Printf("ClientDelegate: Receive, msg=%s", string(msg))
	return nil
}

func (c *ClientDelegateBase) Connected() error {
	c.GetLogger().Printf("ClientDelegate: Connected")
	return nil
}

func (c *ClientDelegateBase) Disconnected() {
	c.GetLogger().Printf("ClientDelegate: Disconnected")
}

// Implementation of ClientDelegate.

func (c *ClientDelegateBase) GetTLSConfig() (conf *tls.Config, err error) {
	if c.certFile == "" {
		return nil, nil
	}
	caCert, err := ioutil.ReadFile(c.certFile)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{}
	if c.ShouldSkipVerify() {
		tlsConfig.InsecureSkipVerify = true
	} else {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		// Create TLS configuration with the certificate of the server.
		tlsConfig.RootCAs = caCertPool
	}
	return tlsConfig, nil
}

func (c *ClientDelegateBase) ShouldSkipVerify() bool {
	return true
}
