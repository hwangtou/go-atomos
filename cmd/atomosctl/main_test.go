package main

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

// runCLI executes the built binary against ts's URL. Building once per test
// run keeps this an end-to-end check of arg handling, URL completion, and
// exit codes.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	bin := buildCLI(t)
	cmd := exec.Command(bin, args...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("run binary: %v", err)
		} else {
			exitCode = ee.ExitCode()
		}
	}
	return out.String(), errb.String(), exitCode
}

func buildCLI(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/atomosctl"
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build atomosctl: %v\n%s", err, out)
	}
	return bin
}

func TestCLIEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/schema", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"cosmos":"c","elements":[]}`))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("atomos_elements 1\n"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	addr := strings.TrimPrefix(ts.URL, "http://")

	t.Run("Schema", func(t *testing.T) {
		out, _, code := runCLI(t, "schema", addr)
		if code != 0 || !strings.Contains(out, `"cosmos":"c"`) {
			t.Fatalf("schema: code=%d out=%q", code, out)
		}
	})
	t.Run("Metrics", func(t *testing.T) {
		out, _, code := runCLI(t, "metrics", ts.URL)
		if code != 0 || !strings.Contains(out, "atomos_elements 1") {
			t.Fatalf("metrics: code=%d out=%q", code, out)
		}
	})
	t.Run("HealthOK", func(t *testing.T) {
		out, _, code := runCLI(t, "health", addr)
		if code != 0 || !strings.Contains(out, "healthy") {
			t.Fatalf("health: code=%d out=%q", code, out)
		}
	})
	t.Run("Unreachable", func(t *testing.T) {
		_, errOut, code := runCLI(t, "health", "127.0.0.1:1")
		if code != 1 || !strings.Contains(errOut, "request failed") {
			t.Fatalf("unreachable: code=%d err=%q", code, errOut)
		}
	})
	t.Run("BadCommand", func(t *testing.T) {
		_, errOut, code := runCLI(t, "nope", addr)
		if code != 2 || !strings.Contains(errOut, "unknown command") {
			t.Fatalf("bad command: code=%d err=%q", code, errOut)
		}
	})
	t.Run("NoArgs", func(t *testing.T) {
		_, errOut, code := runCLI(t)
		if code != 2 || !strings.Contains(errOut, "usage") {
			t.Fatalf("no args: code=%d err=%q", code, errOut)
		}
	})
}
