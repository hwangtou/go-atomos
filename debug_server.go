package atomos

// Debug server: an opt-in HTTP introspection endpoint for a running Cosmos
// process. Enabled by setting Config.Customize[configKeyDebugServer] to a TCP
// listen address (YAML `customize: {_debug_server: "127.0.0.1:9099"}` or the
// WithDebugServer Run option). It serves until CosmosProcess.Stop.
//
// Endpoints:
//   - /elements      human-readable inventory: elements, live atoms, mailbox
//     depths, idle times, and the current sync-call wait graph
//   - /metrics       Prometheus text-format gauges (no client library needed)
//   - /healthz       liveness mirroring CosmosProcess.IsHealthy
//   - /debug/pprof/* stock net/http/pprof handlers
//
// The server is auxiliary: a listen failure is logged and the process keeps
// running without introspection.

import (
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

type debugServer struct {
	process  *CosmosProcess
	listener net.Listener
	server   *http.Server
	started  time.Time
}

// startDebugServer starts the introspection HTTP server on addr. It returns
// nil (after logging) if the address cannot be listened on — observability
// must not take the process down.
func startDebugServer(p *CosmosProcess, addr string) *debugServer {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		p.local.coreLog().coreError("DebugServer: Listen failed, debug server disabled. addr=(%s),err=(%v)", addr, err)
		return nil
	}
	ds := &debugServer{
		process:  p,
		listener: ln,
		started:  time.Now(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", ds.handleIndex)
	mux.HandleFunc("/elements", ds.handleElements)
	mux.HandleFunc("/metrics", ds.handleMetrics)
	mux.HandleFunc("/healthz", ds.handleHealthz)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	ds.server = &http.Server{Handler: mux}
	go func() {
		_ = ds.server.Serve(ln)
	}()
	p.local.coreLog().coreInfo("DebugServer: Serving. addr=(%s)", ds.addr())
	return ds
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

func (ds *debugServer) handleElements(w http.ResponseWriter, _ *http.Request) {
	elements := ds.collect()
	p := ds.process
	id := p.local.GetIDInfo()

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>atomos elements</title>
<style>body{font-family:monospace;margin:1em}table{border-collapse:collapse;margin-bottom:1.5em}
th,td{border:1px solid #999;padding:2px 8px;text-align:left}th{background:#eee}</style></head><body>`)
	fmt.Fprintf(&b, "<h1>atomos %s / node %s</h1>", html.EscapeString(id.Cosmos), html.EscapeString(id.Node))
	fmt.Fprintf(&b, "<p>pid=%d state=%s uptime=%s <a href=\"/metrics\">metrics</a> <a href=\"/debug/pprof/\">pprof</a></p>",
		os.Getpid(), html.EscapeString(p.state.String()), time.Since(ds.started).Round(time.Second))

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

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	metric("go_goroutines", "Number of goroutines.", runtime.NumGoroutine())
	metric("go_memstats_alloc_bytes", "Bytes allocated and still in use.", ms.Alloc)
	metric("go_memstats_sys_bytes", "Bytes obtained from system.", ms.Sys)
	metric("go_memstats_heap_inuse_bytes", "Heap bytes in use.", ms.HeapInuse)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
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
