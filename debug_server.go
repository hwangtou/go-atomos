package atomos

// Debug server: an opt-in HTTP introspection endpoint for a running Cosmos
// process. Enabled by setting Config.Customize[configKeyDebugServer] to a TCP
// listen address (YAML `customize: {_debug_server: "127.0.0.1:9099"}` or the
// WithDebugServer Run option). It serves until CosmosProcess.Stop. Non-loopback
// listeners require HTTPS (configKeyDebugServerCert/Key) and a Bearer token
// (configKeyDebugServerToken). Whenever a token is configured, every endpoint
// requires Bearer authentication.
//
// Endpoints:
//   - /elements      human-readable inventory: elements, live atoms, mailbox
//     depths, idle times, and the current sync-call wait graph
//   - /metrics       Prometheus text-format gauges (no client library needed)
//   - /healthz       liveness mirroring CosmosProcess.IsHealthy
//   - /debug/pprof/* stock net/http/pprof handlers
//
// The server is auxiliary: an unsafe configuration or listen failure is logged
// and the process keeps running without introspection.

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	debugReadHeaderTimeout = 2 * time.Second
	debugIdleTimeout       = 2 * time.Minute
)

type debugServer struct {
	process      *CosmosProcess
	listener     net.Listener
	server       *http.Server
	started      time.Time
	tokenHash    [sha256.Size]byte
	authRequired bool
}

// startDebugServer starts the introspection HTTP server on addr. It returns
// nil (after logging) if the configuration is unsafe or the address cannot be
// listened on — observability must not take the process down.
func startDebugServer(p *CosmosProcess, addr string) *debugServer {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		p.local.coreLog().coreError("DebugServer: Invalid listen address, debug server disabled. addr=(%s),err=(%v)", addr, err)
		return nil
	}
	customize := p.local.runnable.config.Customize
	token := debugServerToken(customize)
	certPath := strings.TrimSpace(string(customize[configKeyDebugServerCert]))
	keyPath := strings.TrimSpace(string(customize[configKeyDebugServerKey]))
	if (certPath == "") != (keyPath == "") {
		p.local.coreLog().coreError("DebugServer: HTTPS cert and key must be configured together, debug server disabled. addr=(%s)", addr)
		return nil
	}
	if !tcpAddr.IP.IsLoopback() && token == "" {
		p.local.coreLog().coreError("DebugServer: Non-loopback listen requires a Bearer token, debug server disabled. addr=(%s)", addr)
		return nil
	}
	if !tcpAddr.IP.IsLoopback() && certPath == "" {
		p.local.coreLog().coreError("DebugServer: Non-loopback listen requires HTTPS, debug server disabled. addr=(%s)", addr)
		return nil
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		p.local.coreLog().coreError("DebugServer: Listen failed, debug server disabled. addr=(%s),err=(%v)", addr, err)
		return nil
	}
	if !listenerIsLoopback(ln) && (token == "" || certPath == "") {
		_ = ln.Close()
		p.local.coreLog().coreError("DebugServer: Non-loopback listener without HTTPS and a token, debug server disabled. addr=(%s)", ln.Addr())
		return nil
	}
	ds := &debugServer{
		process:      p,
		listener:     ln,
		started:      time.Now(),
		tokenHash:    sha256.Sum256([]byte(token)),
		authRequired: token != "",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", ds.handleIndex)
	mux.HandleFunc("/elements", ds.handleElements)
	mux.HandleFunc("/schema", ds.handleSchema)
	mux.HandleFunc("/metrics", ds.handleMetrics)
	mux.HandleFunc("/healthz", ds.handleHealthz)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	ds.server = &http.Server{
		Handler:           ds.authenticate(mux),
		ReadHeaderTimeout: debugReadHeaderTimeout,
		IdleTimeout:       debugIdleTimeout,
		MaxHeaderBytes:    16 * 1024,
	}
	go func() {
		var serveErr error
		if certPath != "" {
			serveErr = ds.server.ServeTLS(ln, certPath, keyPath)
		} else {
			serveErr = ds.server.Serve(ln)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			p.local.coreLog().coreError("DebugServer: Serve failed, debug server disabled. addr=(%s),err=(%v)", ds.addr(), serveErr)
		}
	}()
	p.local.coreLog().coreInfo("DebugServer: Serving. addr=(%s)", ds.addr())
	return ds
}

func listenerIsLoopback(ln net.Listener) bool {
	addr, ok := ln.Addr().(*net.TCPAddr)
	return ok && addr.IP.IsLoopback()
}

func (ds *debugServer) authenticate(next http.Handler) http.Handler {
	if !ds.authRequired {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Values("Authorization")
		if len(authorization) == 1 && strings.HasPrefix(authorization[0], "Bearer ") {
			providedHash := sha256.Sum256([]byte(strings.TrimPrefix(authorization[0], "Bearer ")))
			if subtle.ConstantTimeCompare(providedHash[:], ds.tokenHash[:]) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="atomos-debug"`)
		http.Error(w, "unauthorized\n", http.StatusUnauthorized)
	})
}

// stop closes the listener and any in-flight request connections.
func (ds *debugServer) stop() {
	if ds == nil {
		return
	}
	_ = ds.server.Close()
}

// addr returns the bound listen address (resolved when listening on :0).
func (ds *debugServer) addr() string {
	if ds == nil || ds.listener == nil {
		return ""
	}
	return ds.listener.Addr().String()
}

// ---- snapshots ----

type atomDebugInfo struct {
	Name     string
	State    string
	MailNum  uint32
	IdleTime time.Duration
}

type elementDebugInfo struct {
	Name        string
	State       string
	AtomsTotal  int
	AtomsActive int
	Messages    int
	Atoms       []atomDebugInfo
}

// collect walks the local cosmos and snapshots element/atom inventory. Every
// read goes through the owning lock (cosmos elements lock, element atoms
// lock, mailbox mutex), so it is safe against concurrent spawn/kill/mail.
func (ds *debugServer) collect() []elementDebugInfo {
	local := ds.process.local
	elems, err := local.getLocalAllElements()
	if err != nil {
		return nil
	}
	out := make([]elementDebugInfo, 0, len(elems))
	for _, elem := range elems {
		if elem == nil {
			continue
		}
		info := elementDebugInfo{
			Name:        elem.GetIDInfo().Element,
			State:       elem.State().String(),
			AtomsTotal:  elem.GetAtomsNum(),
			AtomsActive: elem.GetActiveAtomsNum(),
		}
		if iface := elem.elemImpl.Interface; iface != nil && iface.Config != nil {
			info.Messages = len(iface.Config.Messages)
		}
		for _, atom := range elem.GetAtoms() {
			info.Atoms = append(info.Atoms, atomDebugInfo{
				Name:     atom.GetIDInfo().Atom,
				State:    atom.State().String(),
				MailNum:  atom.atomos.mailbox.mailNum(),
				IdleTime: atom.atomos.idleTime(),
			})
		}
		sort.Slice(info.Atoms, func(i, j int) bool { return info.Atoms[i].Name < info.Atoms[j].Name })
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// waitEdgesSnapshot copies the sync-call wait graph for display.
func (ds *debugServer) waitEdgesSnapshot() []waitEdge {
	p := ds.process
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	edges := make([]waitEdge, 0, len(p.waitGraph))
	for _, edge := range p.waitGraph {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].targetID.Info() < edges[j].targetID.Info() })
	return edges
}

// syncChainsSnapshot copies the ambient cross-node wait chains keyed by the
// mailbox goroutine executing a chain-carrying sync mail.
func (ds *debugServer) syncChainsSnapshot() map[uint64][]string {
	p := ds.process
	p.syncChainMu.Lock()
	defer p.syncChainMu.Unlock()
	chains := make(map[uint64][]string, len(p.syncChains))
	for goID, chain := range p.syncChains {
		chains[goID] = append([]string(nil), chain...)
	}
	return chains
}

// ---- handlers ----

func (ds *debugServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>atomos debug</title></head>
<body><h1>atomos debug server</h1><ul>
<li><a href="/elements">/elements</a> — element &amp; atom inventory, wait graph</li>
<li><a href="/schema">/schema</a> — element message schema (JSON)</li>
<li><a href="/metrics">/metrics</a> — Prometheus text format</li>
<li><a href="/healthz">/healthz</a> — liveness</li>
<li><a href="/debug/pprof/">/debug/pprof/</a> — CPU / heap / goroutine profiles</li>
</ul></body></html>`))
}

func (ds *debugServer) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if ds.process.IsHealthy() {
		_, _ = w.Write([]byte("ok\n"))
		return
	}
	http.Error(w, "unhealthy\n", http.StatusServiceUnavailable)
}

func (ds *debugServer) processStateSnapshot() CosmosProcessState {
	ds.process.mutex.RLock()
	defer ds.process.mutex.RUnlock()
	return ds.process.state
}

func (ds *debugServer) handleElements(w http.ResponseWriter, _ *http.Request) {
	elements := ds.collect()
	p := ds.process
	id := p.local.GetIDInfo()
	processState := ds.processStateSnapshot()

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>atomos elements</title>
<style>body{font-family:monospace;margin:1em}table{border-collapse:collapse;margin-bottom:1.5em}
th,td{border:1px solid #999;padding:2px 8px;text-align:left}th{background:#eee}</style></head><body>`)
	fmt.Fprintf(&b, "<h1>atomos %s / node %s</h1>", html.EscapeString(id.Cosmos), html.EscapeString(id.Node))
	fmt.Fprintf(&b, "<p>pid=%d state=%s uptime=%s <a href=\"/metrics\">metrics</a> <a href=\"/debug/pprof/\">pprof</a></p>",
		os.Getpid(), html.EscapeString(processState.String()), time.Since(ds.started).Round(time.Second))

	b.WriteString("<table><tr><th>element</th><th>state</th><th>atoms</th><th>active</th><th>messages</th></tr>")
	for _, e := range elements {
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%d</td></tr>",
			html.EscapeString(e.Name), html.EscapeString(e.State), e.AtomsTotal, e.AtomsActive, e.Messages)
	}
	b.WriteString("</table>")

	for _, e := range elements {
		fmt.Fprintf(&b, "<h2>element %s</h2>", html.EscapeString(e.Name))
		if len(e.Atoms) == 0 {
			b.WriteString("<p>no live atoms</p>")
			continue
		}
		b.WriteString("<table><tr><th>atom</th><th>state</th><th>mailbox mails</th><th>idle</th></tr>")
		for _, a := range e.Atoms {
			fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>",
				html.EscapeString(a.Name), html.EscapeString(a.State), a.MailNum, a.IdleTime.Round(time.Millisecond))
		}
		b.WriteString("</table>")
	}

	edges := ds.waitEdgesSnapshot()
	fmt.Fprintf(&b, "<h2>sync wait graph (%d edges)</h2>", len(edges))
	if len(edges) > 0 {
		b.WriteString("<table><tr><th>blocked goroutine</th><th>waiting for</th></tr>")
		for _, edge := range edges {
			fmt.Fprintf(&b, "<tr><td>%d</td><td>%s</td></tr>", edge.targetGoID, html.EscapeString(edge.targetID.Info()))
		}
		b.WriteString("</table>")
	}

	chains := ds.syncChainsSnapshot()
	fmt.Fprintf(&b, "<h2>cross-node sync chains (%d)</h2>", len(chains))
	if len(chains) > 0 {
		b.WriteString("<table><tr><th>mailbox goroutine</th><th>chain</th></tr>")
		goIDs := make([]uint64, 0, len(chains))
		for goID := range chains {
			goIDs = append(goIDs, goID)
		}
		sort.Slice(goIDs, func(i, j int) bool { return goIDs[i] < goIDs[j] })
		for _, goID := range goIDs {
			fmt.Fprintf(&b, "<tr><td>%d</td><td>%s</td></tr>", goID, html.EscapeString(strings.Join(chains[goID], " &rarr; ")))
		}
		b.WriteString("</table>")
	}

	b.WriteString("</body></html>")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

// promLabel escapes a label value per the Prometheus text format.
func promLabel(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return strings.ReplaceAll(s, "\n", `\n`)
}

func (ds *debugServer) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(ds.metricsText()))
}

// metricsText renders the full exposition; split from the handler for tests.
func (ds *debugServer) metricsText() string {
	elements := ds.collect()
	p := ds.process
	id := p.local.GetIDInfo()

	var b strings.Builder
	metric := func(name, help string, v any) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n%s %v\n", name, help, name, name, v)
	}
	lmetric := func(name, help string, labels string, v any) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n%s{%s} %v\n", name, help, name, name, labels, v)
	}

	b.WriteString("# HELP atomos_process_info Process identity labels: value is always 1.\n# TYPE atomos_process_info gauge\n")
	fmt.Fprintf(&b, "atomos_process_info{cosmos=\"%s\",node=\"%s\"} 1\n", promLabel(id.Cosmos), promLabel(id.Node))

	elementsCount := 0
	for _, e := range elements {
		elementsCount++
		l := fmt.Sprintf("element=\"%s\"", promLabel(e.Name))
		lmetric("atomos_element_atoms", "Total atoms registered on the element.", l, e.AtomsTotal)
		lmetric("atomos_element_atoms_active", "Atoms in Spawning/Waiting/Busy state.", l, e.AtomsActive)
		lmetric("atomos_element_messages", "Message names in the element schema (Config.Messages).", l, e.Messages)
		for _, a := range e.Atoms {
			la := fmt.Sprintf("element=\"%s\",atom=\"%s\"", promLabel(e.Name), promLabel(a.Name))
			lmetric("atomos_atom_mails", "Mails currently queued in the atom mailbox.", la, a.MailNum)
			lmetric("atomos_atom_idle_seconds", "Seconds since the atom last finished a mail.", la, a.IdleTime.Seconds())
		}
	}
	metric("atomos_elements", "Number of local elements.", elementsCount)

	p.waitMu.Lock()
	waitEdges := len(p.waitGraph)
	p.waitMu.Unlock()
	metric("atomos_sync_wait_edges", "Edges in the sync-call wait graph; sustained non-zero values suggest blocked sync calls.", waitEdges)

	p.syncChainMu.Lock()
	syncChains := len(p.syncChains)
	p.syncChainMu.Unlock()
	metric("atomos_sync_chains", "Mailbox goroutines currently executing a chain-carrying cross-node sync call.", syncChains)

	counts, sum, total, perCall := p.snapshotRPCMetrics()
	if total > 0 {
		b.WriteString("# HELP atomos_rpc_latency_seconds Client-observed latency of call-class RPCs.\n# TYPE atomos_rpc_latency_seconds histogram\n")
		for i, bound := range rpcLatencyBuckets {
			fmt.Fprintf(&b, "atomos_rpc_latency_seconds_bucket{le=\"%v\"} %d\n", bound, counts[i])
		}
		fmt.Fprintf(&b, "atomos_rpc_latency_seconds_bucket{le=\"+Inf\"} %d\n", counts[len(rpcLatencyBuckets)])
		fmt.Fprintf(&b, "atomos_rpc_latency_seconds_sum %v\natomos_rpc_latency_seconds_count %d\n", sum, total)
		b.WriteString("# HELP atomos_rpc_calls_total Call-class RPCs by entry point.\n# TYPE atomos_rpc_calls_total counter\n")
		methods := make([]string, 0, len(perCall))
		for m := range perCall {
			methods = append(methods, m)
		}
		sort.Strings(methods)
		for _, m := range methods {
			fmt.Fprintf(&b, "atomos_rpc_calls_total{method=\"%s\"} %d\n", promLabel(m), perCall[m])
		}
	}
	metric("atomos_remote_connections", "Remote nodes currently tracked by this process.", p.remoteConnectionsCount())

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	metric("go_goroutines", "Number of goroutines.", runtime.NumGoroutine())
	metric("go_memstats_alloc_bytes", "Bytes allocated and still in use.", ms.Alloc)
	metric("go_memstats_sys_bytes", "Bytes obtained from system.", ms.Sys)
	metric("go_memstats_heap_inuse_bytes", "Heap bytes in use.", ms.HeapInuse)

	return b.String()
}

// handleSchema serves the element message schema (Config.Messages) as JSON:
// for every element, each message name with its in/out proto type URLs.
// This is the machine-readable face of the framework — the foundation for
// management CLIs and MCP-style tooling.
func (ds *debugServer) handleSchema(w http.ResponseWriter, _ *http.Request) {
	type messageSchema struct {
		In  string `json:"in"`
		Out string `json:"out"`
	}
	type elementSchema struct {
		Name     string                   `json:"name"`
		Version  uint64                   `json:"version"`
		Atoms    int                      `json:"atoms"`
		Active   int                      `json:"active"`
		Messages map[string]messageSchema `json:"messages"`
	}
	out := struct {
		Cosmos   string          `json:"cosmos"`
		Node     string          `json:"node"`
		Elements []elementSchema `json:"elements"`
	}{Cosmos: ds.process.local.GetIDInfo().Cosmos, Node: ds.process.local.GetIDInfo().Node}
	elems, err := ds.process.local.getLocalAllElements()
	if err == nil {
		out.Elements = make([]elementSchema, 0, len(elems))
		for _, elem := range elems {
			if elem == nil || elem.elemImpl == nil || elem.elemImpl.Interface == nil {
				continue
			}
			es := elementSchema{
				Name:     elem.GetIDInfo().Element,
				Version:  elem.elemImpl.Interface.Config.GetVersion(),
				Atoms:    elem.GetAtomsNum(),
				Active:   elem.GetActiveAtomsNum(),
				Messages: map[string]messageSchema{},
			}
			for name, mc := range elem.elemImpl.Interface.Config.Messages {
				es.Messages[name] = messageSchema{
					In:  mc.GetIn().GetTypeUrl(),
					Out: mc.GetOut().GetTypeUrl(),
				}
			}
			out.Elements = append(out.Elements, es)
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(mustJSON(out)))
}

// mustJSON marshals v with stable indentation; introspection payloads are
// plain maps and slices, which MarshalIndent orders deterministically.
func mustJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("{\n  \"error\": %q\n}", err.Error())
	}
	return string(data)
}

// debugServerAddr extracts the configured debug listen address, if any.
// A bare port like "9099" is accepted as ":9099".
func debugServerAddr(customize map[string][]byte) string {
	v, ok := customize[configKeyDebugServer]
	if !ok {
		return ""
	}
	addr := strings.TrimSpace(string(v))
	if isAllDigits(addr) {
		return ":" + addr
	}
	return addr
}

func debugServerToken(customize map[string][]byte) string {
	return strings.TrimSpace(string(customize[configKeyDebugServerToken]))
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
