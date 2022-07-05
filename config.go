package go_atomos

// CHECKED!

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v3"
)

// Config Error

var (
	ErrConfigIsNil           = errors.New("config not found")
	ErrConfigNodeInvalid     = errors.New("config node name is invalid")
	ErrConfigLogPathInvalid  = errors.New("config log path is invalid")
	ErrConfigCertPathInvalid = errors.New("config cert path is invalid")
	ErrConfigKeyPathInvalid  = errors.New("config key path is invalid")
)

type yamlConfig struct {
	Node         string                  `yaml:"node"`
	LogPath      string                  `yaml:"log_path"`
	LogLevel     int                     `yaml:"log_level"`
	EnableCert   *yamlCertConfig         `yaml:"enable_cert"`
	EnableServer *yamlRemoteServerConfig `yaml:"enable_server"`
	EnableTelnet *yamlTelnetConfig       `yaml:"enable_telnet"`
	Customize    map[string]string       `yaml:"customize"`
}

type yamlCertConfig struct {
	CertPath           string `yaml:"cert_path"`
	KeyPath            string `yaml:"key_path"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type yamlRemoteServerConfig struct {
	Host string `yaml:"host"`
	Port int32  `yaml:"port"`
}

type yamlTelnetConfig struct {
	Network string           `yaml:"network"`
	Address string           `yaml:"address"`
	Admin   *yamlTelnetAdmin `yaml:"admin"`
}

type yamlTelnetAdmin struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

func ConfigFromYaml(filepath string) (*Config, error) {
	dat, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	y := &yamlConfig{}
	if err = yaml.Unmarshal(dat, y); err != nil {
		return nil, err
	}
	conf := &Config{
		Node:      y.Node,
		LogPath:   y.LogPath,
		LogLevel:  LogLevel(y.LogLevel),
		Customize: map[string]string{},
	}
	if cert := y.EnableCert; cert != nil {
		conf.EnableCert = &CertConfig{
			CertPath:           cert.CertPath,
			KeyPath:            cert.KeyPath,
			InsecureSkipVerify: cert.InsecureSkipVerify,
		}
	}
	if server := y.EnableServer; server != nil {
		conf.EnableServer = &RemoteServerConfig{
			Host: server.Host,
			Port: server.Port,
		}
	}
	if telnet := y.EnableTelnet; telnet != nil {
		conf.EnableTelnet = &TelnetServerConfig{
			Network: telnet.Network,
			Address: telnet.Address,
		}
		if telnetAdmin := telnet.Admin; telnetAdmin != nil {
			conf.EnableTelnet.Admin = &TelnetAdminConfig{
				Username: telnetAdmin.Username,
				Password: telnetAdmin.Password,
			}
		}
	}
	if custom := y.Customize; custom != nil {
		for key, value := range custom {
			conf.Customize[key] = value
		}
	}
	return conf, nil
}

func (x *Config) Check() error {
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

func (x *IDInfo) str() string {
	if x == nil {
		return "InvalidAtomId"
	}
	switch x.Type {
	case IDType_Atomos:
		return fmt.Sprintf("%s::%s::%s", x.Cosmos, x.Element, x.Atomos)
	case IDType_Element:
		return fmt.Sprintf("%s::%s", x.Cosmos, x.Element)
	case IDType_Cosmos:
		fallthrough
	default:
		return fmt.Sprintf("%s", x.Cosmos)
	}
}
