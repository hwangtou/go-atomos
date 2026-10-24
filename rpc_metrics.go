package atomos

// Client-observed metrics for call-class RPCs: a process-level latency
// histogram (atomos_rpc_latency_seconds) and per-method counters
// (atomos_rpc_calls_total), exported through the debug server. Recording
// happens on the calling side (BaseRemote / ElementRemote entry points), so
// the numbers include connection setup and retries as the caller experienced
// them.

import (
	"sync"
	"time"
)

// rpcLatencyBuckets are the histogram upper bounds, in seconds.
var rpcLatencyBuckets = []float64{
	0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

// rpcMetrics holds cumulative counts per bucket (index len(rpcLatencyBuckets)
// is +Inf), the sum, and per-method call counts.
type rpcMetrics struct {
	mu      sync.Mutex
	counts  []uint64
	sum     float64
	total   uint64
	perCall map[string]uint64
}

func newRPCMetrics() *rpcMetrics {
	return &rpcMetrics{
		counts:  make([]uint64, len(rpcLatencyBuckets)+1),
		perCall: map[string]uint64{},
	}
}

// observeRPC records one call-class RPC round trip started at start and
// bumps the per-method counter. Cheap enough for every call: two locked
// increments.
func (p *CosmosProcess) observeRPC(method string, start time.Time) {
	if p == nil || p.rpcMetrics == nil {
		return
	}
	d := time.Since(start).Seconds()
	m := p.rpcMetrics
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, b := range rpcLatencyBuckets {
		if d <= b {
			m.counts[i]++
		}
	}
	m.counts[len(rpcLatencyBuckets)]++
	m.sum += d
	m.total++
	m.perCall[method]++
}

// snapshotRPCMetrics copies the current counters for exposition.
func (p *CosmosProcess) snapshotRPCMetrics() (counts []uint64, sum float64, total uint64, perCall map[string]uint64) {
	if p == nil || p.rpcMetrics == nil {
		return nil, 0, 0, nil
	}
	m := p.rpcMetrics
	m.mu.Lock()
	defer m.mu.Unlock()
	counts = append([]uint64(nil), m.counts...)
	perCall = make(map[string]uint64, len(m.perCall))
	for k, v := range m.perCall {
		perCall[k] = v
	}
	return counts, m.sum, m.total, perCall
}

// remoteConnectionsCount returns how many remote nodes this process tracks.
func (p *CosmosProcess) remoteConnectionsCount() int {
	if p == nil {
		return 0
	}
	p.cluster.remoteMutex.RLock()
	defer p.cluster.remoteMutex.RUnlock()
	return len(p.cluster.remoteCosmos)
}
