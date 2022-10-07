package go_atomos

import (
	"gopkg.in/yaml.v2"
	"os"
)

func (x *Config) Check() *ErrorInfo {
	if x == nil {
		return NewError(ErrCosmosConfigInvalid, "No configuration")
	}
	if x.Node == "" {
		return NewError(ErrCosmosConfigNodeNameInvalid, "Node name is empty")
	}
	if x.LogPath == "" {
		return NewError(ErrCosmosConfigLogPathInvalid, "Log path is empty")
	}
	// TODO: Try open log file
	return nil
}

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
	Host  string `yaml:"host"`
	Port  int32  `yaml:"port"`
	Token string `yaml:"token"`
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

func ConfigFromYaml(filepath string) (*Config, *ErrorInfo) {
	dat, err := os.ReadFile(filepath)
	if err != nil {
		return nil, NewErrorf(ErrCosmosConfigInvalid, "Read failed, err=(%v)", err)
	}
	y := &yamlConfig{}
	if err = yaml.Unmarshal(dat, y); err != nil {
		return nil, NewErrorf(ErrCosmosConfigInvalid, "Unmarshal failed, err=(%v)", err)
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
