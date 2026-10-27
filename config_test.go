package atomos

import (
	"os"
	"path/filepath"
	"testing"
)

// Config loading and validation tests.
//
// Audit B3 was "config validation was a no-op" — the validators below now
// return real errors; these tests pin the behavior they promised.

func Test_checkCosmosName(t *testing.T) {
	valid := []string{"a", "Cosmos.1", "node_x", "0"}
	for _, s := range valid {
		if !checkCosmosName(s) {
			t.Errorf("checkCosmosName(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "a-b", "a b", "中", "a/b"}
	for _, s := range invalid {
		if checkCosmosName(s) {
			t.Errorf("checkCosmosName(%q) = true, want false", s)
		}
	}
}

func Test_checkNodeName(t *testing.T) {
	if !checkNodeName("node.1") {
		t.Error(`checkNodeName("node.1") = false, want true`)
	}
	if checkNodeName("") || checkNodeName("node-1") {
		t.Error("checkNodeName accepted empty or dashed name")
	}
}

func Test_ValidateCosmosNodeConfig(t *testing.T) {
	cases := []struct {
		name   string
		conf   *Config
		wantOK bool
	}{
		{"nil config", nil, false},
		{"valid", &Config{Cosmos: "cosmos", Node: "node"}, true},
		{"empty cosmos", &Config{Cosmos: "", Node: "node"}, false},
		{"bad cosmos char", &Config{Cosmos: "cos mos", Node: "node"}, false},
		{"empty node", &Config{Cosmos: "cosmos", Node: ""}, false},
		{"bad node char", &Config{Cosmos: "cosmos", Node: "node!"}, false},
	}
	for _, c := range cases {
		err := c.conf.ValidateCosmosNodeConfig()
		if c.wantOK && err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		}
		if !c.wantOK {
			if err == nil {
				t.Errorf("%s: expected error, got nil", c.name)
			} else if code := CodeOf(err); code != ErrCodeCosmosConfigInvalid {
				t.Errorf("%s: code=%d, want ErrCodeCosmosConfigInvalid", c.name, code)
			}
		}
	}
}

func Test_ValidateSupervisorConfig(t *testing.T) {
	cases := []struct {
		name   string
		conf   *Config
		wantOK bool
	}{
		{"nil config", nil, false},
		{"cluster nil", &Config{Cosmos: "c", Node: "n"}, true},
		{"cluster disabled", &Config{Cosmos: "c", Node: "n", EnableCluster: &CosmosClusterConfig{Enable: false}}, true},
		{"cluster enabled with endpoints", &Config{Cosmos: "c", Node: "n", EnableCluster: &CosmosClusterConfig{Enable: true, EtcdEndpoints: []string{"localhost:2379"}}}, true},
		{"cluster enabled without endpoints", &Config{Cosmos: "c", Node: "n", EnableCluster: &CosmosClusterConfig{Enable: true}}, false},
		{"secure file TLS complete", &Config{Cosmos: "c", Node: "n", EnableCluster: &CosmosClusterConfig{Enable: true, EtcdEndpoints: []string{"localhost:2379"}, EnableCert: &CertConfig{CertPath: "s.crt", KeyPath: "s.key", ClientCertPath: "c.crt", ClientKeyPath: "c.key", ClientCaPath: "ca.crt"}}}, true},
		{"secure file TLS incomplete", &Config{Cosmos: "c", Node: "n", EnableCluster: &CosmosClusterConfig{Enable: true, EtcdEndpoints: []string{"localhost:2379"}, EnableCert: &CertConfig{CertPath: "s.crt", KeyPath: "s.key"}}}, false},
		{"secure file TLS skip verify", &Config{Cosmos: "c", Node: "n", EnableCluster: &CosmosClusterConfig{Enable: true, EtcdEndpoints: []string{"localhost:2379"}, EnableCert: &CertConfig{CertPath: "s.crt", KeyPath: "s.key", ClientCertPath: "c.crt", ClientKeyPath: "c.key", ClientCaPath: "ca.crt", InsecureSkipVerify: true}}}, false},
		{"invalid admin node", &Config{Cosmos: "c", Node: "n", EnableCluster: &CosmosClusterConfig{Enable: true, EtcdEndpoints: []string{"localhost:2379"}, AdminNodes: []string{"bad/node"}}}, false},
	}
	for _, c := range cases {
		err := c.conf.ValidateSupervisorConfig()
		if c.wantOK && err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		}
		if !c.wantOK {
			if err == nil {
				t.Errorf("%s: expected error, got nil", c.name)
			} else if code := CodeOf(err); code != ErrCodeCosmosConfigInvalid {
				t.Errorf("%s: code=%d, want ErrCodeCosmosConfigInvalid", c.name, code)
			}
		}
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Pin the ATOMOS_* environment empty so a developer's shell cannot change
// what these tests parse.
func clearAtomosEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ATOMOS_COSMOS", "ATOMOS_NODE", "ATOMOS_LOG_LEVEL", "ATOMOS_LOG_PATH",
		"ATOMOS_LOG_STDOUT", "ATOMOS_ETCD_ENDPOINTS", "ATOMOS_GRPC_PORTS",
	} {
		t.Setenv(k, "")
	}
}

func Test_NewCosmosNodeConfigFromYamlPath(t *testing.T) {
	clearAtomosEnv(t)

	path := writeConfig(t, `
cosmos: yamlCosmos
node: yamlNode
log-level: warn
log-std: true
log-path: /tmp/logs
enable-cluster:
  enable: true
  etcd-endpoints:
    - localhost:2379
    - localhost:2380
  optional-ports:
    - 50200
customize:
  my-key: my-value
`)
	conf, err := NewCosmosNodeConfigFromYamlPath(path, &CosmosRunnable{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if conf.Cosmos != "yamlCosmos" || conf.Node != "yamlNode" {
		t.Fatalf("names: %+v", conf)
	}
	if conf.LogLevel != LogLevel_Warn {
		t.Errorf("log level=%v, want Warn", conf.LogLevel)
	}
	if string(conf.Customize[configKeyLogStdout]) != "1" {
		t.Error("log-std true should set stdout customize key")
	}
	if conf.EnableCluster == nil || !conf.EnableCluster.Enable {
		t.Fatal("cluster section not parsed")
	}
	if len(conf.EnableCluster.EtcdEndpoints) != 2 {
		t.Errorf("endpoints=%v", conf.EnableCluster.EtcdEndpoints)
	}
	if len(conf.EnableCluster.OptionalPorts) != 1 || conf.EnableCluster.OptionalPorts[0] != 50200 {
		t.Errorf("optional ports=%v", conf.EnableCluster.OptionalPorts)
	}
	if string(conf.Customize["my-key"]) != "my-value" {
		t.Errorf("customize my-key=%q", conf.Customize["my-key"])
	}
}

func Test_NewCosmosNodeConfigFromYamlPath_DisabledClusterIsStandalone(t *testing.T) {
	clearAtomosEnv(t)
	path := writeConfig(t, `
cosmos: c
node: n
enable-cluster:
  enable: false
  etcd-endpoints: [localhost:2379]
  optional-ports: [50051]
`)
	conf, err := NewCosmosNodeConfigFromYamlPath(path, &CosmosRunnable{})
	if err != nil {
		t.Fatal(err)
	}
	if conf.EnableCluster != nil {
		t.Fatalf("disabled cluster must remain standalone, got %+v", conf.EnableCluster)
	}
}

func Test_NewCosmosNodeConfigFromYamlPath_TLS(t *testing.T) {
	clearAtomosEnv(t)
	path := writeConfig(t, `
cosmos: c
node: n
enable-cluster:
  enable: true
  etcd-endpoints: [localhost:2379]
  admin-nodes: [opsNode]
  enable-cert:
    cert-path: /tmp/cluster.crt
    key-path: /tmp/cluster.key
    insecure-skip-verify: true
    client-cert-path: /tmp/client.crt
    client-key-path: /tmp/client.key
    client-ca-path: /tmp/client-ca.crt
`)
	conf, err := NewCosmosNodeConfigFromYamlPath(path, &CosmosRunnable{})
	if err != nil {
		t.Fatal(err)
	}
	cert := conf.EnableCluster.EnableCert
	if cert == nil || cert.CertPath != "/tmp/cluster.crt" || cert.KeyPath != "/tmp/cluster.key" || !cert.InsecureSkipVerify {
		t.Fatalf("TLS config was not preserved: %+v", cert)
	}
	if cert.ClientCertPath != "/tmp/client.crt" || cert.ClientKeyPath != "/tmp/client.key" || cert.ClientCaPath != "/tmp/client-ca.crt" {
		t.Fatalf("client TLS config was not preserved: %+v", cert)
	}
	if len(conf.EnableCluster.AdminNodes) != 1 || conf.EnableCluster.AdminNodes[0] != "opsNode" {
		t.Fatalf("admin nodes were not preserved: %v", conf.EnableCluster.AdminNodes)
	}
}

func Test_NewCosmosNodeConfigFromYamlPath_LogLevelAliases(t *testing.T) {
	clearAtomosEnv(t)
	cases := map[string]LogLevel{
		"fatal": LogLevel_Fatal,
		"error": LogLevel_Err,
		"err":   LogLevel_Err,
		"warn":  LogLevel_Warn,
		"info":  LogLevel_Info,
		"inf":   LogLevel_Info,
		// Absent or unrecognized falls back to Debug.
		"":         LogLevel_Debug,
		"nonsense": LogLevel_Debug,
	}
	for in, want := range cases {
		content := "cosmos: c\nnode: n\n"
		if in != "" {
			content += "log-level: " + in + "\n"
		}
		conf, err := NewCosmosNodeConfigFromYamlPath(writeConfig(t, content), &CosmosRunnable{})
		if err != nil {
			t.Fatalf("level %q: %v", in, err)
		}
		if conf.LogLevel != want {
			t.Errorf("log-level %q parsed as %v, want %v", in, conf.LogLevel, want)
		}
	}
}

func Test_NewCosmosNodeConfigFromYamlPath_Errors(t *testing.T) {
	clearAtomosEnv(t)

	// Missing file.
	if _, err := NewCosmosNodeConfigFromYamlPath(filepath.Join(t.TempDir(), "nope.yaml"), &CosmosRunnable{}); CodeOf(err) != ErrCodeCosmosConfigReadFailed {
		t.Errorf("missing file: err=%v, want ErrCodeCosmosConfigReadFailed", err)
	}

	// Malformed YAML.
	if _, err := NewCosmosNodeConfigFromYamlPath(writeConfig(t, "cosmos: [unclosed"), &CosmosRunnable{}); CodeOf(err) != ErrCodeCosmosConfigInvalid {
		t.Errorf("bad yaml: err=%v, want ErrCodeCosmosConfigInvalid", err)
	}

	// enable-elements naming an element the runnable does not implement.
	path := writeConfig(t, "cosmos: c\nnode: n\nenable-elements:\n  - NoSuchElement\n")
	_, err := NewCosmosNodeConfigFromYamlPath(path, &CosmosRunnable{})
	if CodeOf(err) != ErrCodeCosmosConfigInvalid {
		t.Errorf("unknown element: err=%v, want ErrCodeCosmosConfigInvalid", err)
	}
}

func Test_applyEnvOverrides(t *testing.T) {
	clearAtomosEnv(t)

	conf := &Config{
		Cosmos:   "yamlCosmos",
		Node:     "yamlNode",
		LogLevel: LogLevel_Debug,
		Customize: map[string][]byte{
			configKeyLogStdout: []byte("0"),
		},
	}

	t.Setenv("ATOMOS_COSMOS", "envCosmos")
	t.Setenv("ATOMOS_NODE", "envNode")
	t.Setenv("ATOMOS_LOG_LEVEL", "ERR")
	t.Setenv("ATOMOS_LOG_PATH", "/env/logs")
	t.Setenv("ATOMOS_LOG_STDOUT", "true")
	t.Setenv("ATOMOS_ETCD_ENDPOINTS", "localhost:2379,localhost:2380")
	// Invalid entries and out-of-range ports must be skipped, not fatal.
	t.Setenv("ATOMOS_GRPC_PORTS", "23107, not-a-port, 70000, 0")

	applyEnvOverrides(conf)

	if conf.Cosmos != "envCosmos" || conf.Node != "envNode" {
		t.Errorf("names: %+v", conf)
	}
	if conf.LogLevel != LogLevel_Err {
		t.Errorf("log level=%v, want Err", conf.LogLevel)
	}
	if conf.LogPath != "/env/logs" {
		t.Errorf("log path=%q", conf.LogPath)
	}
	if string(conf.Customize[configKeyLogStdout]) != "1" {
		t.Error("ATOMOS_LOG_STDOUT=true should enable stdout key")
	}
	if conf.EnableCluster == nil || len(conf.EnableCluster.EtcdEndpoints) != 2 {
		t.Errorf("etcd endpoints=%v", conf.EnableCluster)
	}
	ports := conf.EnableCluster.OptionalPorts
	if len(ports) != 1 || ports[0] != 23107 {
		t.Errorf("grpc ports=%v, want [23107]", ports)
	}
}
