package go_atomos

// CHECKED!

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
)

// Config Error

var (
	ErrConfigIsNil           = errors.New("config not found")
	ErrConfigNodeInvalid     = errors.New("config node name is invalid")
	ErrConfigLogPathInvalid  = errors.New("config log path is invalid")
	ErrConfigCertPathInvalid = errors.New("config cert path is invalid")
	ErrConfigKeyPathInvalid  = errors.New("config key path is invalid")
)

func (x *Config) check(c *CosmosSelf) error {
	if x == nil {
		return ErrConfigIsNil
	}
	if x.Node == "" {
		return ErrConfigNodeInvalid
	}
	if x.LogPath == "" {
		return ErrConfigLogPathInvalid
	}
	// TODO: Try open log file
	return nil
}

func (x *Config) getClientCertConfig() (tlsConfig *tls.Config, err error) {
	cert := x.EnableCert
	if cert == nil {
		return nil, nil
	}
	if cert.CertPath == "" {
		return nil, ErrConfigCertPathInvalid
	}
	caCert, err := ioutil.ReadFile(cert.CertPath)
	if err != nil {
		return nil, err
	}
	tlsConfig = &tls.Config{}
	if cert.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
		return tlsConfig, nil
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	// Create TLS configuration with the certificate of the server.
	tlsConfig.RootCAs = caCertPool
	return tlsConfig, nil
}

func (x *Config) getListenCertConfig() (tlsConfig *tls.Config, err error) {
	cert := x.EnableCert
	if cert == nil {
		return nil, nil
	}
	if cert.CertPath == "" {
		return nil, ErrConfigCertPathInvalid
	}
	if cert.KeyPath == "" {
		return nil, ErrConfigKeyPathInvalid
	}
	tlsConfig = &tls.Config{
		Certificates: make([]tls.Certificate, 1),
	}
	tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(cert.CertPath, cert.KeyPath)
	if err != nil {
		return nil, err
	}
	return
}

func (x *AtomId) str() string {
	return fmt.Sprintf("%s::%s::%s", x.Node, x.Element, x.Name)
}
