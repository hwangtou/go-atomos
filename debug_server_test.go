package atomos

// Debug server end-to-end coverage: start a process with the _debug_server
// customize key, spawn an atom, and assert the introspection endpoints
// (index, /elements, /metrics, /healthz, /debug/pprof/) report the element
// and atom inventory; then assert Stop closes the listener.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
			EnableElements: []string{debugElemName, ForTestAtomosName},
			Customize: map[string][]byte{
				configKeyDebugServer: []byte(addr),
			},
		}).
		SetMainScript(&debugMainScript{}).
		AddElementImplementationWithSpawn(elemImpl).
		// The ForTest element carries decoders, exercising the schema
		// metadata surface (Config.Messages) on /schema and /metrics.
		AddElementImplementationWithSpawn(GetForTestAtomosImplement(&testRunnableDev{}))

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
		// Seed one recorded RPC so the histogram block is exercised.
		process.observeRPC("SyncMessaging", time.Now().Add(-20*time.Millisecond))
		body, code := httpGet(t, base+"/metrics")
		if code != http.StatusOK {
			t.Fatalf("metrics status = %d", code)
		}
		for _, want := range []string{
			`atomos_process_info{cosmos="debugCosmos",node="debugNode"} 1`,
			"atomos_elements 2",
			`atomos_element_atoms{element="` + debugElemName + `"} 1`,
			`atomos_element_atoms_active{element="` + debugElemName + `"} 1`,
			`atomos_element_messages{element="` + ForTestAtomosName + `"} 2`,
			`atomos_atom_mails{element="` + debugElemName + `",atom="` + debugAtomName + `"}`,
			`atomos_atom_idle_seconds{element="` + debugElemName + `",atom="` + debugAtomName + `"}`,
			"atomos_sync_wait_edges",
			"atomos_sync_chains",
			"atomos_rpc_latency_seconds_bucket{le=\"+Inf\"} 1",
			"atomos_rpc_latency_seconds_count 1",
			`atomos_rpc_calls_total{method="SyncMessaging"} 1`,
			"atomos_remote_connections 0",
			"go_goroutines",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("metrics missing %q", want)
			}
		}
	})

	t.Run("Schema", func(t *testing.T) {
		body, code := httpGet(t, base+"/schema")
		if code != http.StatusOK {
			t.Fatalf("schema status = %d", code)
		}
		for _, want := range []string{
			`"cosmos": "debugCosmos"`,
			`"name": "` + debugElemName + `"`,
			`"Greeting"`,
			`"in": "type.googleapis.com/atomos.ForTestGreetingI"`,
			`"out": "type.googleapis.com/atomos.ForTestGreetingO"`,
			`"ElementSayHello"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("schema missing %q", want)
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

func writeDebugServerCertificate(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "atomos-debug-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath = filepath.Join(dir, "debug.crt")
	keyPath = filepath.Join(dir, "debug.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func startDebugServerTestProcess(t *testing.T, addr, token string, secure ...bool) (*CosmosProcess, *reproLogger) {
	t.Helper()
	logger := &reproLogger{}
	process, cerr := newCosmosProcess("debugSecurityCosmos", "debugSecurityNode", logger)
	if cerr != nil {
		t.Fatalf("newCosmosProcess failed: %v", cerr.Message)
	}
	runnable := newTestCosmosRunnable(&IDInfo{
		Type:   IDType_Cosmos,
		Cosmos: "debugSecurityCosmos",
		Node:   "debugSecurityNode",
	})
	runnable.config.Customize = map[string][]byte{configKeyDebugServer: []byte(addr)}
	if token != "" {
		runnable.config.Customize[configKeyDebugServerToken] = []byte(token)
	}
	if len(secure) > 0 && secure[0] {
		certPath, keyPath := writeDebugServerCertificate(t)
		runnable.config.Customize[configKeyDebugServerCert] = []byte(certPath)
		runnable.config.Customize[configKeyDebugServerKey] = []byte(keyPath)
	}
	if err := process.Start(runnable); err != nil {
		t.Fatalf("process start failed: %v", err)
	}
	t.Cleanup(func() { _ = process.Stop() })
	return process, logger
}

func httpGetWithAuthorization(t *testing.T, url, authorization string) (string, int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request for %s failed: %v", url, err)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	if strings.HasPrefix(url, "https://") {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // Test-only self-signed certificate.
	}
	resp, err := client.Do(req)
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

func TestDebugServerNonLoopbackRequiresToken(t *testing.T) {
	process, logger := startDebugServerTestProcess(t, "0.0.0.0:0", "")
	if process.debugServer != nil {
		t.Fatalf("non-loopback debug server started without a token")
	}
	for deadline := time.Now().Add(2 * time.Second); !logger.hasError("Non-loopback listen requires a Bearer token"); {
		if time.Now().After(deadline) {
			t.Fatalf("missing unsafe-listener error log")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDebugServerNonLoopbackRequiresHTTPS(t *testing.T) {
	process, logger := startDebugServerTestProcess(t, "0.0.0.0:0", "debug-test-token")
	if process.debugServer != nil {
		t.Fatalf("non-loopback debug server started without HTTPS")
	}
	for deadline := time.Now().Add(2 * time.Second); !logger.hasError("Non-loopback listen requires HTTPS"); {
		if time.Now().After(deadline) {
			t.Fatalf("missing HTTPS requirement error log")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDebugServerBearerAuthentication(t *testing.T) {
	const token = "debug-test-token"
	process, _ := startDebugServerTestProcess(t, "0.0.0.0:0", token, true)
	ds := process.debugServer
	if ds == nil {
		t.Fatalf("non-loopback debug server with HTTPS and token did not start")
	}
	if listenerIsLoopback(ds.listener) {
		t.Fatalf("test listener %s is unexpectedly loopback", ds.addr())
	}
	_, port, err := net.SplitHostPort(ds.addr())
	if err != nil {
		t.Fatalf("split debug address failed: %v", err)
	}
	base := "https://127.0.0.1:" + port
	paths := []string{"/", "/elements", "/schema", "/metrics", "/healthz", "/debug/pprof/"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			for _, authorization := range []string{"", "Bearer wrong-token", "Basic " + token} {
				if _, code := httpGetWithAuthorization(t, base+path, authorization); code != http.StatusUnauthorized {
					t.Errorf("GET %s with Authorization %q: status = %d, want 401", path, authorization, code)
				}
			}
			if _, code := httpGetWithAuthorization(t, base+path, "Bearer "+token); code != http.StatusOK {
				t.Errorf("GET %s with valid Bearer token: status = %d, want 200", path, code)
			}
		})
	}
}

func TestDebugServerSlowHeaders(t *testing.T) {
	process, _ := startDebugServerTestProcess(t, "127.0.0.1:0", "")
	ds := process.debugServer
	if ds == nil {
		t.Fatalf("debug server not started")
	}
	if ds.server.ReadHeaderTimeout != debugReadHeaderTimeout || ds.server.IdleTimeout != debugIdleTimeout {
		t.Fatalf("server timeouts = (%v, %v), want (%v, %v)",
			ds.server.ReadHeaderTimeout, ds.server.IdleTimeout, debugReadHeaderTimeout, debugIdleTimeout)
	}
	if ds.server.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %v; long-running pprof requests must not be cut off", ds.server.WriteTimeout)
	}

	conn, err := net.Dial("tcp", ds.addr())
	if err != nil {
		t.Fatalf("dial debug server failed: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("GET /healthz HTTP/1.1\r\nHost: localhost\r\n")); err != nil {
		t.Fatalf("write partial request failed: %v", err)
	}
	started := time.Now()
	if err := conn.SetReadDeadline(started.Add(debugReadHeaderTimeout + time.Second)); err != nil {
		t.Fatalf("set read deadline failed: %v", err)
	}
	if _, err := io.ReadAll(conn); err != nil {
		t.Fatalf("slow request was not closed cleanly: %v", err)
	}
	elapsed := time.Since(started)
	if elapsed < debugReadHeaderTimeout-250*time.Millisecond {
		t.Fatalf("slow request closed too early after %v", elapsed)
	}
	if _, code := httpGet(t, "http://"+ds.addr()+"/healthz"); code != http.StatusOK {
		t.Fatalf("server unhealthy after slow request: status = %d", code)
	}
}
