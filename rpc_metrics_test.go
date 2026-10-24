package atomos

import (
	"testing"
	"time"
)

func TestRPCMetrics_Bucketing(t *testing.T) {
	m := newRPCMetrics()
	p := &CosmosProcess{rpcMetrics: m}

	// One fast call (<=1ms bucket and every larger bucket), one ~40ms call,
	// one 20s call (overflow: only +Inf).
	p.observeRPC("A", time.Now().Add(-100*time.Microsecond))
	p.observeRPC("B", time.Now().Add(-40*time.Millisecond))
	p.observeRPC("C", time.Now().Add(-20*time.Second))

	counts, sum, total, perCall := p.snapshotRPCMetrics()
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if perCall["A"] != 1 || perCall["B"] != 1 || perCall["C"] != 1 {
		t.Fatalf("perCall = %v", perCall)
	}
	// le=0.001 catches only the fast call; le=0.05 catches fast+B; +Inf all.
	le := func(bound float64) uint64 {
		for i, b := range rpcLatencyBuckets {
			if b == bound {
				return counts[i]
			}
		}
		t.Fatalf("bound %v not in buckets", bound)
		return 0
	}
	if got := le(0.001); got != 1 {
		t.Errorf("le=0.001 = %d, want 1", got)
	}
	if got := le(0.05); got != 2 {
		t.Errorf("le=0.05 = %d, want 2", got)
	}
	if got := counts[len(rpcLatencyBuckets)]; got != 3 {
		t.Errorf("+Inf = %d, want 3", got)
	}
	if sum <= 20 {
		t.Errorf("sum = %v, want > 20s (dominated by the 20s call)", sum)
	}

	// Snapshot must be a copy: recording more must not mutate it.
	p.observeRPC("A", time.Now())
	_, _, total2, _ := p.snapshotRPCMetrics()
	if total2 != 4 {
		t.Fatalf("total after extra observe = %d, want 4", total2)
	}
	if total != 3 {
		t.Fatalf("earlier snapshot mutated: %d", total)
	}
}

func TestRPCMetrics_NilSafe(t *testing.T) {
	var p *CosmosProcess
	p.observeRPC("A", time.Now()) // must not panic
	p.snapshotRPCMetrics()
	if got := p.remoteConnectionsCount(); got != 0 {
		t.Fatalf("nil process connections = %d, want 0", got)
	}
	p2 := &CosmosProcess{}
	p2.observeRPC("A", time.Now()) // nil metrics field must not panic
}
