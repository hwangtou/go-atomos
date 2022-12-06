package go_atomos

import (
	"flag"
	"gopkg.in/yaml.v2"
	"os"
)

func (x *Config) Check() *Error {
	if x == nil {
		return NewError(ErrCosmosConfigInvalid, "No configuration")
	}
	//if x.Node == "" {
	//	return NewError(ErrCosmosConfigNodeNameInvalid, "Node name is empty")
	//}
	if x.LogPath == "" {
		return NewError(ErrCosmosConfigLogPathInvalid, "Log path is empty")
	}
	return nil
}

//type yamlConfig struct {
//	Cosmos string `yaml:"cosmos"`
//	Node   string `yaml:"node"`
//
//	LogLevel string `yaml:"log-level"`
//	LogPath  string `yaml:"log-path"`
//	RunPath  string `yaml:"run-path"`
//
//	Managers     []*yamlManager          `yaml:"managers"`
//	UseCert      *yamlCertConfig         `yaml:"use-cert"`
//	EnableServer *yamlRemoteServerConfig `yaml:"enable-server"`
//	Customize    map[string]string       `yaml:"customize"`
//}

type yamlManager struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type yamlCertConfig struct {
	CertPath           string `yaml:"cert-path"`
	KeyPath            string `yaml:"key-path"`
	InsecureSkipVerify bool   `yaml:"insecure-skip-verify"`
}

type yamlRemoteServerConfig struct {
	Port int32 `yaml:"port"`
}

func NewNodeConfigFromYamlFileArgument() (*Config, *Error) {
	// Config
	configPath := flag.String("config", "config/config.yaml", "yaml config path")
	flag.Parse()

	// Load config.
	return NodeConfigFromYaml(*configPath)
}

func NodeConfigFromYaml(filepath string) (*Config, *Error) {
	dat, err := os.ReadFile(filepath)
	if err != nil {
		return nil, NewErrorf(ErrCosmosConfigInvalid, "Read failed, err=(%v)", err)
	}
	y := &NodeYAMLConfig{}
	if err = yaml.Unmarshal(dat, y); err != nil {
		return nil, NewErrorf(ErrCosmosConfigInvalid, "Unmarshal failed, err=(%v)", err)
	}
	logLevel := LogLevel_DEBUG
	if lv, ok := LogLevel_value[y.LogLevel]; ok {
		logLevel = LogLevel(lv)
	}
	conf := &Config{
		Cosmos:       y.Cosmos,
		Node:         y.Node,
		ReporterUrl:  y.ReporterUrl,
		ConfigerUrl:  y.ConfigerUrl,
		LogLevel:     logLevel,
		LogPath:      y.LogPath,
		RunPath:      y.RunPath,
		EtcPath:      y.EtcPath,
		EnableCert:   nil,
		EnableServer: nil,
		Customize:    map[string][]byte{},
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
			Host: "",
			Port: server.Port,
		}
	}
	if custom := y.CustomizeConfig; custom != nil {
		for key, value := range custom {
			conf.Customize[key] = value
		}
	}
	return conf, nil
}

func SupervisorConfigFromYaml(filepath string) (*Config, *Error) {
	dat, err := os.ReadFile(filepath)
	if err != nil {
		return nil, NewErrorf(ErrCosmosConfigInvalid, "Read failed, err=(%v)", err)
	}
	y := &SupervisorYAMLConfig{}
	if err = yaml.Unmarshal(dat, y); err != nil {
		return nil, NewErrorf(ErrCosmosConfigInvalid, "Unmarshal failed, err=(%v)", err)
	}
	logLevel := LogLevel_DEBUG
	if lv, ok := LogLevel_value[y.LogLevel]; ok {
		logLevel = LogLevel(lv)
	}
	conf := &Config{
		Cosmos:            y.Cosmos,
		NodeList:          y.NodeList,
		KeepaliveNodeList: y.KeepaliveNodeList,
		ReporterUrl:       y.ReporterUrl,
		ConfigerUrl:       y.ConfigerUrl,
		LogLevel:          logLevel,
		LogPath:           y.LogPath,
		RunPath:           y.RunPath,
		EtcPath:           y.EtcPath,
	}
	return conf, nil
}

// Config

type SupervisorYAMLConfig struct {
	Cosmos string `yaml:"cosmos"`

	NodeList          []string `yaml:"node-list"`
	KeepaliveNodeList []string `yaml:"keepalive-node-list"`

	ReporterUrl string `yaml:"reporter-url"`
	ConfigerUrl string `yaml:"configer-url"`

	LogLevel string `yaml:"log-level"`
	LogPath  string `yaml:"log-path"`

	RunPath string `yaml:"run-path"`
	EtcPath string `yaml:"etc-path"`
}

type NodeYAMLConfig struct {
	Cosmos string `yaml:"cosmos"`
	Node   string `yaml:"node"`

	ReporterUrl string `yaml:"reporter-url"`
	ConfigerUrl string `yaml:"configer-url"`

	LogLevel string `yaml:"log-level"`
	LogPath  string `yaml:"log-path"`

	BuildPath string `yaml:"build-path"`
	BinPath   string `yaml:"bin-path"`
	RunPath   string `yaml:"run-path"`
	EtcPath   string `yaml:"etc-path"`

	EnableCert   *CertConfig         `yaml:"enable-cert"`
	EnableServer *RemoteServerConfig `yaml:"enable-server"`

	CustomizeConfig map[string][]byte `yaml:"customize"`
}

//type CertConfig struct {
//	CertPath           string `yaml:"cert_path"`
//	KeyPath            string `yaml:"key_path"`
//	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
//}

//type RemoteServerConfig struct {
//	Host string `yaml:"host"`
//	Port int32  `yaml:"port"`
//}
