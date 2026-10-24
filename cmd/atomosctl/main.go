// Command atomosctl is a thin management CLI for the atomos debug server:
// it fetches the introspection endpoints of a running node and prints them.
//
//	atomosctl schema  http://127.0.0.1:9099   # element message schema (JSON)
//	atomosctl metrics http://127.0.0.1:9099   # Prometheus exposition text
//	atomosctl elements http://127.0.0.1:9099  # inventory page (HTML source)
//	atomosctl health  http://127.0.0.1:9099   # exit 0 when healthy, 1 otherwise
package main

import (
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
(enable with atomos.WithDebugServer or the _debug_server customize key)`

func main() {
	if len(os.Args) != 3 {
		failUsage("expected exactly two arguments")
	}
	command, addr := os.Args[1], os.Args[2]
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
		failUsage(fmt.Sprintf("unknown command %q", command))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(addr + path)
	if err != nil {
		fatal(fmt.Sprintf("request failed: %v", err))
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fatal(fmt.Sprintf("read body failed: %v", err))
	}
	if command == "health" {
		if resp.StatusCode == http.StatusOK {
			fmt.Print("healthy\n")
			return
		}
		fatal(fmt.Sprintf("unhealthy: status %d, body %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}
	if resp.StatusCode != http.StatusOK {
		fatal(fmt.Sprintf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}
	os.Stdout.Write(body)
	if len(body) > 0 && body[len(body)-1] != '\n' {
		fmt.Println()
	}
}

func failUsage(msg string) {
	fmt.Fprintf(os.Stderr, "atomosctl: %s\n\n%s\n", msg, usage)
	os.Exit(2)
}

func fatal(msg string) {
	fmt.Fprintf(os.Stderr, "atomosctl: %s\n", msg)
	os.Exit(1)
}
