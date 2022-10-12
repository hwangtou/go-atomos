package go_atomos

import (
	"flag"
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
	return nil
}

type yamlConfig struct {
	Node         string                  `yaml:"node"`
	LogPath      string                  `yaml:"log-path"`
	LogLevel     string                  `yaml:"log-level"`
	ManagePort   int                     `yaml:"manage-port"`
	Managers     []*yamlManager          `yaml:"managers"`
	UseCert      *yamlCertConfig         `yaml:"use-cert"`
	EnableServer *yamlRemoteServerConfig `yaml:"enable-server"`
	Customize    map[string]string       `yaml:"customize"`
}

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

func NewConfigFromYamlFileArgument() (*Config, *ErrorInfo) {
	// Config
	configPath := flag.String("config", "config/config.yaml", "yaml config path")
	flag.Parse()

	// Load config.
	return ConfigFromYaml(*configPath)
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
	logLevel := LogLevel_Debug
	if lv, ok := LogLevel_value[y.LogLevel]; ok {
		logLevel = LogLevel(lv)
	}
	conf := &Config{
		LogPath:      y.LogPath,
		LogLevel:     logLevel,
		ManagePort:   uint32(y.ManagePort),
		Managers:     nil,
		Node:         y.Node,
		UseCert:      nil,
		EnableServer: nil,
		Customize:    map[string]string{},
	}
	for _, manager := range y.Managers {
		conf.Managers = append(conf.Managers, &Manager{
			Username: manager.Username,
			Password: manager.Password,
		})
	}
	if cert := y.UseCert; cert != nil {
		conf.UseCert = &CertConfig{
			CertPath:           cert.CertPath,
			KeyPath:            cert.KeyPath,
			InsecureSkipVerify: cert.InsecureSkipVerify,
		}
	}
	if server := y.EnableServer; server != nil {
		conf.EnableServer = &RemoteServerConfig{
			Port: server.Port,
		}
	}
	if custom := y.Customize; custom != nil {
		for key, value := range custom {
			conf.Customize[key] = value
		}
	}
	return conf, nil
}
