package atomos

// Debug server end-to-end coverage: start a process with the _debug_server
// customize key, spawn an atom, and assert the introspection endpoints
// (index, /elements, /metrics, /healthz, /debug/pprof/) report the element
// and atom inventory; then assert Stop closes the listener.

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

const debugElemName = "debugElem"
const debugAtomName = "debugAtom"

type debugDev struct{}

func (d *debugDev) ElementConstructor() Atomos         { return &debugAtomDev{name: "elem"} }
func (d *debugDev) AtomConstructor(name string) Atomos { return &debugAtomDev{name: name} }
func (d *debugDev) AtomAutoData() AtomAutoData         { return nil }
func (d *debugDev) ElementAutoData() ElementAutoData   { return nil }

type debugAtomDev struct {
	name string
}

func (a *debugAtomDev) String() string { return "debugAtom:" + a.name }
func (a *debugAtomDev) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

type debugMainScript struct{}

func (s *debugMainScript) OnBoot(local *CosmosProcess) error    { return nil }
func (s *debugMainScript) OnStartUp(local *CosmosProcess) error { return nil }
func (s *debugMainScript) OnShutdown() error                    { return nil }

func httpGet(t *testing.T, url string) (string, int) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s body failed: %v", url, err)
	}
	return string(body), resp.StatusCode
}

func TestDebugServerEndpoints(t *testing.T) {
	// Reserve a port, then release it for the debug server to bind.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port failed: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("release port failed: %v", err)
	}

	logger := &reproLogger{}
	process, cerr := newCosmosProcess("debugCosmos", "debugNode", logger)
	if cerr != nil {
		t.Fatalf("newCosmosProcess failed: %v", cerr.Message)
	}

	dev := &debugDev{}
	elemImpl := NewImplementationFromDeveloper(dev)
	elemImpl.Interface = NewInterfaceFromDeveloper(debugElemName, dev)
	elemImpl.Interface.ElementSpawner = func(s ElementSelfID, a Atomos, data proto.Message, args ...ArgsForBaseAtomos) error {
		return nil
	}
	elemImpl.Interface.AtomSpawner = func(s AtomSelfID, a Atomos, arg, data proto.Message, args ...ArgsForBaseAtomos) error {
		return nil
	}
	elemImpl.ElementHandlers = map[string]MessageHandler{}
	elemImpl.AtomHandlers = map[string]MessageHandler{}

	runnable := (&CosmosRunnable{}).
		SetConfig(&Config{
			Cosmos:         "debugCosmos",
			Node:           "debugNode",
			LogLevel:       LogLevel_Debug,
			EnableElements: []string{debugElemName},
			Customize: map[string][]byte{
				configKeyDebugServer: []byte(addr),
			},
		}).
		SetMainScript(&debugMainScript{}).
		AddElementImplementationWithSpawn(elemImpl)

	if err := process.Start(runnable); err != nil {
		for _, e := range logger.errs {
			t.Logf("captured error log: %s", e)
		}
		t.Fatalf("process start failed: %v", err)
	}
	defer func() { _ = process.Stop() }()

	// The debug server must be up once Start returns.
	if process.debugServer == nil || process.debugServer.addr() == "" {
		t.Fatalf("debug server not started")
	}

	// Spawn an atom so the inventory has something to report.
	elem, gerr := process.local.getLocalElement(debugElemName)
	if gerr != nil {
		t.Fatalf("get element failed: %v", gerr.Message)
	}
	atomID, tracker, serr := elem.SpawnAtom(process.local, debugAtomName, nil, nil)
	if serr != nil {
		t.Fatalf("spawn atom failed: %v", serr)
	}
	defer tracker.Release()
	if atomID.GetIDInfo().Atom != debugAtomName {
		t.Fatalf("spawned atom name mismatch: %s", atomID.GetIDInfo().Atom)
	}

	base := "http://" + addr

	t.Run("Index", func(t *testing.T) {
		body, code := httpGet(t, base+"/")
		if code != http.StatusOK {
			t.Fatalf("index status = %d", code)
		}
		for _, want := range []string{"/elements", "/metrics", "/healthz", "/debug/pprof/"} {
			if !strings.Contains(body, want) {
				t.Errorf("index missing link %q", want)
			}
		}
	})

	t.Run("Elements", func(t *testing.T) {
		body, code := httpGet(t, base+"/elements")
		if code != http.StatusOK {
			t.Fatalf("elements status = %d", code)
		}
		for _, want := range []string{
			"atomos debugCosmos / node debugNode",
			debugElemName,
			debugAtomName,
			"sync wait graph",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("elements page missing %q", want)
			}
		}
	})

	t.Run("Metrics", func(t *testing.T) {
		body, code := httpGet(t, base+"/metrics")
		if code != http.StatusOK {
			t.Fatalf("metrics status = %d", code)
		}
		for _, want := range []string{
			`atomos_process_info{cosmos="debugCosmos",node="debugNode"} 1`,
			"atomos_elements 1",
			`atomos_element_atoms{element="` + debugElemName + `"} 1`,
			`atomos_element_atoms_active{element="` + debugElemName + `"} 1`,
			`atomos_atom_mails{element="` + debugElemName + `",atom="` + debugAtomName + `"}`,
			`atomos_atom_idle_seconds{element="` + debugElemName + `",atom="` + debugAtomName + `"}`,
			"atomos_sync_wait_edges",
			"atomos_sync_chains",
			"go_goroutines",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("metrics missing %q", want)
			}
		}
	})

	t.Run("Healthz", func(t *testing.T) {
		body, code := httpGet(t, base+"/healthz")
		if code != http.StatusOK || strings.TrimSpace(body) != "ok" {
			t.Fatalf("healthz status = %d body = %q", code, body)
		}
	})

	t.Run("Pprof", func(t *testing.T) {
		body, code := httpGet(t, base+"/debug/pprof/")
		if code != http.StatusOK {
			t.Fatalf("pprof index status = %d", code)
		}
		if !strings.Contains(body, "Types of profiles available") {
			t.Errorf("pprof index body unexpected: %.200s", body)
		}
	})

	t.Run("UnknownPath404", func(t *testing.T) {
		_, code := httpGet(t, base+"/nope")
		if code != http.StatusNotFound {
			t.Fatalf("unknown path status = %d, want 404", code)
		}
	})

	// Stop must close the listener.
	if err := process.Stop(); err != nil {
		t.Fatalf("process stop failed: %v", err)
	}
	client := &http.Client{Timeout: 500 * time.Millisecond}
	if resp, err := client.Get(fmt.Sprintf("http://%s/healthz", addr)); err == nil {
		_ = resp.Body.Close()
		t.Fatalf("debug server still serving after Stop")
	}
}

func TestDebugServerAddrParsing(t *testing.T) {
	cases := []struct {
		customize map[string][]byte
		want      string
	}{
		{nil, ""},
		{map[string][]byte{}, ""},
		{map[string][]byte{configKeyDebugServer: []byte("")}, ""},
		{map[string][]byte{configKeyDebugServer: []byte("  ")}, ""},
		{map[string][]byte{configKeyDebugServer: []byte("127.0.0.1:9099")}, "127.0.0.1:9099"},
		{map[string][]byte{configKeyDebugServer: []byte(" :9099 ")}, ":9099"},
		{map[string][]byte{configKeyDebugServer: []byte("9099")}, ":9099"},
		{map[string][]byte{configKeyLogStdout: []byte("1")}, ""},
	}
	for _, c := range cases {
		if got := debugServerAddr(c.customize); got != c.want {
			t.Errorf("debugServerAddr(%v) = %q, want %q", c.customize, got, c.want)
		}
	}
}

func TestDebugServerDisabledByDefault(t *testing.T) {
	logger := &reproLogger{}
	process, cerr := newCosmosProcess("debugCosmosOff", "debugNodeOff", logger)
	if cerr != nil {
		t.Fatalf("newCosmosProcess failed: %v", cerr.Message)
	}

	dev := &debugDev{}
	elemImpl := NewImplementationFromDeveloper(dev)
	elemImpl.Interface = NewInterfaceFromDeveloper(debugElemName, dev)
	elemImpl.Interface.ElementSpawner = func(s ElementSelfID, a Atomos, data proto.Message, args ...ArgsForBaseAtomos) error {
		return nil
	}
	elemImpl.Interface.AtomSpawner = func(s AtomSelfID, a Atomos, arg, data proto.Message, args ...ArgsForBaseAtomos) error {
		return nil
	}
	elemImpl.ElementHandlers = map[string]MessageHandler{}
	elemImpl.AtomHandlers = map[string]MessageHandler{}

	runnable := (&CosmosRunnable{}).
		SetConfig(&Config{
			Cosmos:         "debugCosmosOff",
			Node:           "debugNodeOff",
			LogLevel:       LogLevel_Debug,
			EnableElements: []string{debugElemName},
		}).
		SetMainScript(&debugMainScript{}).
		AddElementImplementationWithSpawn(elemImpl)

	if err := process.Start(runnable); err != nil {
		t.Fatalf("process start failed: %v", err)
	}
	defer func() { _ = process.Stop() }()

	if process.debugServer != nil {
		t.Fatalf("debug server started without config")
	}
	if addr := debugServerAddr(runnable.config.Customize); addr != "" {
		t.Fatalf("unexpected addr %q", addr)
	}
}
