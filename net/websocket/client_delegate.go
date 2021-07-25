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

type clientDelegate struct {
	name     string
	addr     string
	certFile string
	logger   *log.Logger
}

// Implementation of ConnDelegate.

func (c *clientDelegate) GetName() string {
	return c.name
}

func (c *clientDelegate) GetAddr() string {
	return c.addr
}

func (c *clientDelegate) GetLogger() *log.Logger {
	return c.logger
}

func (c *clientDelegate) WriteTimeout() time.Duration {
	return writeWait
}

func (c *clientDelegate) PingPeriod() time.Duration {
	return connPingPeriod
}

func (c *clientDelegate) Receive(msg []byte) error {
	c.GetLogger().Printf("ClientDelegate: Receive, msg=%s", string(msg))
	return nil
}

func (c *clientDelegate) Connected() error {
	c.GetLogger().Printf("ClientDelegate: Connected")
	return nil
}

func (c *clientDelegate) Disconnected() {
	c.GetLogger().Printf("ClientDelegate: Disconnected")
}

func (c *clientDelegate) GetTLSConfig() (conf *tls.Config, err error) {
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

func (c *clientDelegate) ShouldSkipVerify() bool {
	return true
}
