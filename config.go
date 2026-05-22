package atomos

import (
	"gopkg.in/yaml.v2"
	"os"
	"strings"
)

func NewCosmosNodeConfigFromYamlPath(filepath string, runnable *CosmosRunnable) (*Config, *Error) {
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
	switch strings.ToLower(y.LogLevel) {
	case "fatal":
		logLevel = LogLevel_Fatal
	case "error", "err":
		logLevel = LogLevel_Err
	case "warn":
		logLevel = LogLevel_Warn
	case "info", "inf":
		logLevel = LogLevel_Info
	case "debug":
		fallthrough
	default:
		logLevel = LogLevel_Debug
	}
	conf := &Config{
		Cosmos:         y.Cosmos,
		Node:           y.Node,
		LogLevel:       logLevel,
		LogPath:        y.LogPath,
		LogMaxSize:     int64(y.LogMaxSize),
		BuildPath:      y.BuildPath,
		BinPath:        y.BinPath,
		RunPath:        y.RunPath,
		EtcPath:        y.EtcPath,
		EnableCluster:  nil,
		EnableElements: nil,
		Customize:      map[string][]byte{},
	}
	if strings.ToLower(y.LogSTD) == "true" {
		conf.Customize[ConfigKeyLogStdout] = []byte("1")
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
	for _, element := range y.EnableElements {
		impl, has := runnable.implements[element]
		if !has {
			return nil, NewErrorf(ErrCosmosConfigInvalid, "Element not found. element=(%s)", element).AddStack(nil)
		}
		runnable.SetElementSpawn(impl.Interface.Config.Name)
	}
	if custom := y.CustomizeConfig; custom != nil {
		for key, value := range custom {
			conf.Customize[key] = []byte(value)
		}
	}
	applyEnvOverrides(conf)
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

type NodeYAMLConfig struct {
	Cosmos string `yaml:"cosmos"`
	Node   string `yaml:"node"`

	LogLevel   string `yaml:"log-level"`
	LogPath    string `yaml:"log-path"`
	LogMaxSize int    `yaml:"log-max-size"`
	LogSTD     string `yaml:"log-std"`

	BuildPath string `yaml:"build-path"`
	BinPath   string `yaml:"bin-path"`
	RunPath   string `yaml:"run-path"`
	EtcPath   string `yaml:"etc-path"`

	EnableCluster  *ClusterYAMLConfig `yaml:"enable-cluster"`
	EnableElements []string           `yaml:"enable-elements"`

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

// applyEnvOverrides overlays environment variables onto the Config struct.
// Environment variables take precedence over YAML values.
// This is a no-op when no ATOMOS_* env vars are set.
func applyEnvOverrides(conf *Config) {
	if v := os.Getenv("ATOMOS_COSMOS"); v != "" {
		conf.Cosmos = v
	}
	if v := os.Getenv("ATOMOS_NODE"); v != "" {
		conf.Node = v
	}
	if v := os.Getenv("ATOMOS_LOG_LEVEL"); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			conf.LogLevel = LogLevel_Debug
		case "info", "inf":
			conf.LogLevel = LogLevel_Info
		case "warn":
			conf.LogLevel = LogLevel_Warn
		case "error", "err":
			conf.LogLevel = LogLevel_Err
		case "fatal":
			conf.LogLevel = LogLevel_Fatal
		}
	}
	if v := os.Getenv("ATOMOS_LOG_PATH"); v != "" {
		conf.LogPath = v
	}
	if v := os.Getenv("ATOMOS_LOG_STDOUT"); v == "true" || v == "1" {
		conf.Customize[ConfigKeyLogStdout] = []byte("1")
	}
	if v := os.Getenv("ATOMOS_ETCD_ENDPOINTS"); v != "" {
		if conf.EnableCluster == nil {
			conf.EnableCluster = &CosmosClusterConfig{}
		}
		conf.EnableCluster.EtcdEndpoints = strings.Split(v, ",")
	}
}
