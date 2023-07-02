package go_atomos

import (
	"gopkg.in/yaml.v2"
	"os"
)

func NewCosmosNodeConfigFromYamlPath(filepath string) (*Config, *Error) {
	dat, err := os.ReadFile(filepath)
	if err != nil {
		return nil, NewErrorf(ErrCosmosConfigInvalid, "Read failed, err=(%v)", err).AddStack(nil)
	}
	y := &NodeYAMLConfig{}
	if err = yaml.Unmarshal(dat, y); err != nil {
		return nil, NewErrorf(ErrCosmosConfigInvalid, "Unmarshal failed, err=(%v)", err).AddStack(nil)
	}
	logLevel := LogLevel_Debug
	if lv, ok := LogLevel_value[y.LogLevel]; ok {
		logLevel = LogLevel(lv)
	}
	conf := &Config{
		Cosmos:            y.Cosmos,
		Node:              y.Node,
		NodeList:          nil,
		KeepaliveNodeList: nil,
		ReporterUrl:       y.ReporterUrl,
		ConfigerUrl:       y.ConfigerUrl,
		LogLevel:          logLevel,
		LogPath:           y.LogPath,
		LogMaxSize:        int64(y.LogMaxSize),
		BuildPath:         y.BuildPath,
		BinPath:           y.BinPath,
		RunPath:           y.RunPath,
		EtcPath:           y.EtcPath,
		EnableCluster:     nil,
		Customize:         map[string][]byte{},
	}
	if cluster := y.EnableCluster; cluster != nil {
		conf.EnableCluster = &CosmosClusterConfig{
			Enable:        cluster.Enable,
			EtcdEndpoints: cluster.EtcdEndpoints,
			OptionalPorts: cluster.OptionalPorts,
			EnableCert:    nil,
		}
		if cert := cluster.EnableCert; cert != nil {
			conf.EnableCluster.EnableCert = &CertConfig{
				CertPath:           cert.CertPath,
				KeyPath:            cert.KeyPath,
				InsecureSkipVerify: cert.InsecureSkipVerify,
			}
		}
	}
	if custom := y.CustomizeConfig; custom != nil {
		for key, value := range custom {
			conf.Customize[key] = []byte(value)
		}
	}
	return conf, nil
}

func NewSupervisorConfigFromYaml(filepath string) (*Config, *Error) {
	dat, err := os.ReadFile(filepath)
	if err != nil {
		return nil, NewErrorf(ErrCosmosConfigInvalid, "Read failed, err=(%v)", err).AddStack(nil)
	}
	y := &SupervisorYAMLConfig{}
	if err = yaml.Unmarshal(dat, y); err != nil {
		return nil, NewErrorf(ErrCosmosConfigInvalid, "Unmarshal failed, err=(%v)", err).AddStack(nil)
	}
	logLevel := LogLevel_Debug
	if lv, ok := LogLevel_value[y.LogLevel]; ok {
		logLevel = LogLevel(lv)
	}
	conf := &Config{
		Cosmos:            y.Cosmos,
		Node:              "supervisor",
		NodeList:          y.NodeList,
		KeepaliveNodeList: y.KeepaliveNodeList,
		ReporterUrl:       y.ReporterUrl,
		ConfigerUrl:       y.ConfigerUrl,
		LogLevel:          logLevel,
		LogPath:           y.LogPath,
		LogMaxSize:        int64(y.LogMaxSize),
		RunPath:           y.RunPath,
		EtcPath:           y.EtcPath,
	}
	return conf, nil
}

func (x *Config) ValidateSupervisorConfig() *Error {
	// TODO
	return nil
}

func (x *Config) ValidateCosmosNodeConfig() *Error {
	// TODO
	return nil
}

// Checkers

func CheckCosmosName(cosmosName string) bool {
	if cosmosName == "" {
		return false
	}
	for _, c := range cosmosName {
		if !('0' <= c && c <= '9' || 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || c == '.' || c == '_') {
			return false
		}
	}
	return true
}

func CheckNodeName(nodeName string) bool {
	if nodeName == "" {
		return false
	}
	for _, c := range nodeName {
		if !('0' <= c && c <= '9' || 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || c == '.' || c == '_') {
			return false
		}
	}
	return true
}

// Config

type SupervisorYAMLConfig struct {
	Cosmos            string   `yaml:"cosmos"`
	NodeList          []string `yaml:"node-list"`
	KeepaliveNodeList []string `yaml:"keepalive-node-list"`

	ReporterUrl string `yaml:"reporter-url"`
	ConfigerUrl string `yaml:"configer-url"`

	LogLevel   string `yaml:"log-level"`
	LogPath    string `yaml:"log-path"`
	LogMaxSize int    `yaml:"log-max-size"`

	RunPath string `yaml:"run-path"`
	EtcPath string `yaml:"etc-path"`
}

type NodeYAMLConfig struct {
	Cosmos string `yaml:"cosmos"`
	Node   string `yaml:"node"`

	ReporterUrl string `yaml:"reporter-url"`
	ConfigerUrl string `yaml:"configer-url"`

	LogLevel   string `yaml:"log-level"`
	LogPath    string `yaml:"log-path"`
	LogMaxSize int    `yaml:"log-max-size"`

	BuildPath string `yaml:"build-path"`
	BinPath   string `yaml:"bin-path"`
	RunPath   string `yaml:"run-path"`
	EtcPath   string `yaml:"etc-path"`

	EnableCluster *ClusterYAMLConfig `yaml:"enable-cluster"`

	CustomizeConfig map[string]string `yaml:"customize"`
}

type ClusterYAMLConfig struct {
	Enable        bool        `yaml:"enable"`
	EtcdEndpoints []string    `yaml:"etcd-endpoints"`
	OptionalPorts []int32     `yaml:"optional-ports"`
	EnableCert    *CertConfig `yaml:"enable-cert"`
}

type CertYAMLConfig struct {
	CertPath string `yaml:"cert-path"`
	KeyPath  string `yaml:"key-path"`
	Insecure bool   `yaml:"insecure"`
}
