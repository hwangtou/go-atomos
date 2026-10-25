package atomostest

// RunCluster: a real multi-node cluster fixture — an in-process embedded
// etcd plus one Cosmos process per node name, all joined to one cosmos.
// Cross-node messaging runs over real gRPC between the processes and real
// etcd watch/discovery, without any external services.

import (
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	"go.etcd.io/etcd/server/v3/embed"

	atomos "github.com/hwangtou/go-atomos"
)

// reservePort picks an unused loopback TCP port (best effort: the port could
// be taken between close and use; callers pass several candidates to the
// framework, which tries them in order).
func reservePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// elementRouter routes every lookup of an element (and all its atoms) to
// one deterministic node: hash(element) % len(nodes). Spawn and GetAtomID
// then agree on the home node, which is exactly what a fixture needs.
type elementRouter struct {
	nodes []string
}

func (r elementRouter) GetCosmosNodeName(selfNode, element, atom string) (string, bool) {
	if len(r.nodes) == 0 {
		return "", false
	}
	h := fnv32(element) % uint32(len(r.nodes))
	return r.nodes[h], true
}

func fnv32(s string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime32
	}
	return h
}

// RunCluster boots an embedded etcd and one Cosmos process per entry of
// nodes, all in the same cosmos. newRunnable builds each node's runnable
// (register element implementations as usual; no Config needed — the fixture
// sets it). The processes are returned in nodes order and stopped via
// t.Cleanup before the embedded etcd closes.
//
// Cross-node traffic needs a global router (without one, cosmos lookups
// stay local). If newRunnable's runnable carries no router, a default is
// installed that pins each element — and therefore all its atoms — to one
// deterministic node: nodes[hash(element) % len(nodes)].
//
// Readiness is asynchronous: nodes discover each other through etcd watch.
// Use AwaitAtomOnNode before issuing cross-node traffic:
//
//	procs := atomostest.RunCluster(t, "myCosmos", []string{"n1", "n2"}, func(node string) *atomos.CosmosRunnable { ... })
//	atomostest.AwaitAtomOnNode(t, procs[0], "MyElement", "myAtom", home, 10*time.Second)
//
// Cost: an embedded etcd plus N processes, a few seconds wall clock. Each
// node's gRPC server listens on reserved loopback ports.
func RunCluster(t *testing.T, cosmos string, nodes []string, newRunnable func(node string) *atomos.CosmosRunnable) []*atomos.CosmosProcess {
	t.Helper()
	if len(nodes) == 0 {
		t.Fatal("RunCluster: at least one node is required")
	}

	// Embedded etcd on a reserved loopback port; peer port derives from it.
	etcdPort := reservePort()
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.ListenClientUrls = []url.URL{{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", etcdPort)}}
	cfg.ListenPeerUrls = []url.URL{{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", etcdPort+1)}}
	cfg.AdvertiseClientUrls = cfg.ListenClientUrls
	cfg.LogLevel = "error"
	ee, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("RunCluster: embedded etcd start failed: %v", err)
	}
	select {
	case <-ee.Server.ReadyNotify():
	case <-time.After(15 * time.Second):
		ee.Close()
		t.Fatal("RunCluster: embedded etcd ready timeout")
	}
	// Cleanups run LIFO: this is registered first, so the processes below
	// stop before their etcd goes away.
	t.Cleanup(func() { ee.Close() })
	endpoint := fmt.Sprintf("127.0.0.1:%d", etcdPort)

	processes := make([]*atomos.CosmosProcess, 0, len(nodes))
	for _, node := range nodes {
		runnable := newRunnable(node)
		if runnable == nil {
			t.Fatalf("RunCluster: newRunnable(%q) returned nil", node)
		}
		if runnable.GetRouter() == nil {
			runnable.SetRouter(elementRouter{nodes: nodes})
		}
		runnable.SetConfig(&atomos.Config{
			Cosmos:   cosmos,
			Node:     node,
			LogLevel: atomos.LogLevel_Debug,
			EnableCluster: &atomos.CosmosClusterConfig{
				Enable:        true,
				EtcdEndpoints: []string{endpoint},
				OptionalPorts: []int32{int32(reservePort()), int32(reservePort()), int32(reservePort())},
			},
			// Advertise loopback: all fixture processes run on this host,
			// and the auto-detected LAN address is not always reachable from
			// the same machine (VPNs, multiple interfaces).
			Customize: map[string][]byte{"_cluster_advertise_ip": []byte("127.0.0.1")},
		})
		processes = append(processes, Run(t, runnable))
	}
	return processes
}

// AwaitElementOnNode waits until the element resolves (via cosmos routing,
// local or remote) to the given node — the readiness signal that cluster
// discovery has propagated far enough for cross-node calls.
func AwaitElementOnNode(t TestReporter, p *atomos.CosmosProcess, elem, node string, timeout time.Duration) {
	t.Helper()
	Eventually(t, timeout, 50*time.Millisecond, func() bool {
		id, err := p.Self().CosmosGetElementID(elem)
		if err != nil {
			return false
		}
		return id.GetIDInfo().Node == node
	}, "element %q did not resolve to node %q within %v", elem, node, timeout)
}

// AwaitAtomOnNode waits until the named atom resolves to the given node.
// Atoms live on exactly one node, so this is the true cross-node readiness
// probe: it requires discovery AND remote element registration (a node only
// routes remote elements it also implements itself — mirror the element
// implementation on every fixture node).
func AwaitAtomOnNode(t TestReporter, p *atomos.CosmosProcess, elem, atomName, node string, timeout time.Duration) {
	t.Helper()
	Eventually(t, timeout, 50*time.Millisecond, func() bool {
		id, tracker, err := p.Self().CosmosGetAtomID(elem, atomName, atomos.ArgBaseAtomosFromLocalOnly())
		if err != nil {
			return false
		}
		defer tracker.Release()
		return id.GetIDInfo().Node == node
	}, "atom %q/%q did not resolve to node %q within %v", elem, atomName, node, timeout)
}
