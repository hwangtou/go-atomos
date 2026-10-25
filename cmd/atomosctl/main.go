// Command atomosctl is a thin management CLI for the atomos debug server:
// it fetches the introspection endpoints of a running node and prints them.
//
//	atomosctl schema  http://127.0.0.1:9099   # element message schema (JSON)
//	atomosctl metrics http://127.0.0.1:9099   # Prometheus exposition text
//	atomosctl elements http://127.0.0.1:9099  # inventory page (HTML source)
//	atomosctl health  http://127.0.0.1:9099   # exit 0 when healthy, 1 otherwise
package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const usage = `usage: atomosctl <command> <debug-server-address>

commands:
  schema   <addr>   element message schema (JSON)
  metrics  <addr>   Prometheus exposition text
  elements <addr>   element & atom inventory (HTML source)
  health   <addr>   liveness probe; exit code 0 = healthy

<addr> is the debug server listen address, e.g. http://127.0.0.1:9099
(enable with atomos.WithDebugServer or the _debug_server customize key).
Set ATOMOS_DEBUG_TOKEN when the server requires Bearer authentication.`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		return failUsage(stderr, "expected exactly two arguments")
	}
	command, addr := args[0], args[1]
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	var path string
	switch command {
	case "schema":
		path = "/schema"
	case "metrics":
		path = "/metrics"
	case "elements":
		path = "/elements"
	case "health":
		path = "/healthz"
	default:
		return failUsage(stderr, fmt.Sprintf("unknown command %q", command))
	}

	req, err := http.NewRequest(http.MethodGet, addr+path, nil)
	if err != nil {
		return fatal(stderr, fmt.Sprintf("create request failed: %v", err))
	}
	if token := strings.TrimSpace(os.Getenv("ATOMOS_DEBUG_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fatal(stderr, fmt.Sprintf("request failed: %v", err))
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fatal(stderr, fmt.Sprintf("read body failed: %v", err))
	}
	if command == "health" {
		if resp.StatusCode != http.StatusOK {
			return fatal(stderr, fmt.Sprintf("unhealthy: status %d, body %s", resp.StatusCode, strings.TrimSpace(string(body))))
		}
		body = []byte("healthy\n")
	} else if resp.StatusCode != http.StatusOK {
		return fatal(stderr, fmt.Sprintf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}
	if err := writeOutput(stdout, body); err != nil {
		return fatal(stderr, fmt.Sprintf("write output failed: %v", err))
	}
	return 0
}

func writeOutput(w io.Writer, body []byte) error {
	if len(body) > 0 && body[len(body)-1] != '\n' {
		body = append(body, '\n')
	}
	_, err := io.Copy(w, bytes.NewReader(body))
	return err
}

func failUsage(w io.Writer, msg string) int {
	if _, err := fmt.Fprintf(w, "atomosctl: %s\n\n%s\n", msg, usage); err != nil {
		return 1
	}
	return 2
}

func fatal(w io.Writer, msg string) int {
	_, _ = fmt.Fprintf(w, "atomosctl: %s\n", msg)
	return 1
}
