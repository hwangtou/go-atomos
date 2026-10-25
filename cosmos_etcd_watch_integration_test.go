//go:build integration

package atomos

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

// Watch-layer fault-injection tests against a real (embedded) etcd:
//
//   - node death by lease expiry → DELETE event → remote version removed and
//     the remote disabled (the cluster's self-healing path after a crash)
//   - dirty keys / values under the watch prefix → reported, watch survives
//   - total etcd outage (server killed) → the watch reconnects after the
//     server restarts and delivers new events

// newWatchProcess returns a started process whose logs are captured by a
// reproLogger. Watch fault tests deliberately write garbage into etcd; the
// framework reports those via etcdErrorHandler → Fatal logs, which must be
// asserted, not fail the test (newTestLogging would turn them into t.Error).
func newWatchProcess(t *testing.T, cosmos, node string) (*CosmosProcess, *reproLogger) {
	t.Helper()
	logger := &reproLogger{}
	p, err := newCosmosProcess(cosmos, node, logger)
	if err != nil {
		t.Fatalf("newCosmosProcess: %v", err.Message)
	}
	r := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: cosmos, Node: node})
	if err := p.Start(r); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop() })
	return p, logger
}

// startWatch starts watchCluster and cancels it when the test ends.
func startWatch(t *testing.T, p *CosmosProcess, cli *clientv3.Client) {
	t.Helper()
	if err := p.watchCluster(cli); err != nil {
		t.Fatalf("watchCluster failed: %v", err)
	}
	t.Cleanup(func() {
		if p.cluster.etcdCancelWatch != nil {
			p.cluster.etcdCancelWatch()
		}
	})
}

// getRemote returns the CosmosRemote for nodeName, or nil.
func getRemote(p *CosmosProcess, nodeName string) *CosmosRemote {
	p.cluster.remoteMutex.RLock()
	defer p.cluster.remoteMutex.RUnlock()
	return p.cluster.remoteCosmos[nodeName]
}

// waitForRemote polls until the remote for nodeName exists and cond passes.
func waitForRemote(t *testing.T, p *CosmosProcess, nodeName string, timeout time.Duration, cond func(*CosmosRemote) bool, what string) *CosmosRemote {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remote := getRemote(p, nodeName)
		if remote != nil && cond(remote) {
			return remote
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for remote node=(%s): %s", nodeName, what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func remoteEnabled(remote *CosmosRemote) bool {
	remote.mutex.RLock()
	defer remote.mutex.RUnlock()
	return remote.enable
}

func remoteVersionCount(remote *CosmosRemote) int {
	remote.mutex.RLock()
	defer remote.mutex.RUnlock()
	return len(remote.version)
}

// waitLogged polls until the logger has captured a message containing substr.
func waitLogged(t *testing.T, logger *reproLogger, substr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !logger.hasAny(substr, true, true) {
		if time.Now().After(deadline) {
			t.Fatalf("expected log containing %q within %v", substr, timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// reserveLocalPort picks an unused loopback TCP port. Used to give an embedded
// etcd a STABLE address across a kill/restart cycle (ephemeral :0 ports would
// change on restart, and production clients keep pointing at one address).
func reserveLocalPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("reserve port: close: %v", err)
	}
	return port
}

// startEmbeddedEtcdAt starts an embedded etcd bound to a FIXED client port and
// data dir, so it can be killed and restarted on the same address — what an
// etcd restart looks like to clients in production. The peer port is derived
// from the client port; both must stay identical across restarts for the
// member ID (and therefore the data dir) to match.
func startEmbeddedEtcdAt(t *testing.T, dir string, clientPort int) *embed.Etcd {
	t.Helper()
	cfg := embed.NewConfig()
	cfg.Dir = dir
	cfg.ListenClientUrls = []url.URL{{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", clientPort)}}
	cfg.ListenPeerUrls = []url.URL{{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", clientPort+1)}}
	cfg.AdvertiseClientUrls = cfg.ListenClientUrls
	cfg.LogLevel = "error"
	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("embedded etcd start failed: %v", err)
	}
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(15 * time.Second):
		e.Close()
		t.Fatal("embedded etcd ready timeout")
	}
	return e
}

func Test_etcdWatch_NodeDeathByLeaseExpiryRemovesRemoteVersion(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	const cosmos, node = "watchCosmos", "deadNode"
	const version int64 = 1000

	versionKey := etcdCosmosNodeVersionURI(cosmos, node, version)
	lockKey := etcdCosmosNodeLockURI(cosmos, node)

	// Node A registers exactly like a live process: version key under a lease
	// (etcdKeepalive, as used by trySettingClusterToCurrentAndKeepalive) plus
	// a lock key marking itself current.
	info := &CosmosNodeVersionInfo{
		Node:      node,
		Address:   "1.1.1.1:1234",
		Id:        &IDInfo{Type: IDType_Cosmos, Cosmos: cosmos, Node: node},
		State:     ClusterNodeState_Started,
		Elements:  map[string]*IDInfo{},
		StartupId: 111,
	}
	buf, er := json.Marshal(info)
	if er != nil {
		t.Fatalf("marshal info: %v", er)
	}
	lease, _, err := etcdKeepalive(ee.client, versionKey, string(buf), etcdKeepaliveTime)
	if err != nil {
		t.Fatalf("etcdKeepalive: %v", err.Message)
	}
	writeLockInfo(t, ee.client, lockKey, &CosmosNodeVersionLock{Current: version, Versions: []int64{version}})

	// Node B watches the cluster and discovers A (enabled via the lock key).
	p, _ := newWatchProcess(t, cosmos, "watcherNode")
	startWatch(t, p, ee.client)
	remote := waitForRemote(t, p, node, 3*time.Second, remoteEnabled, "discovery of leased node")
	if got := remoteVersionCount(remote); got != 1 {
		t.Fatalf("remote version count=%d, want 1", got)
	}

	// Node A dies: nothing renews its lease anymore. Revoking the lease is
	// exactly what etcd does on expiry — the version key disappears without
	// any cooperation from the dead process.
	if _, er := ee.client.Revoke(context.Background(), lease.ID); er != nil {
		t.Fatalf("revoke lease: %v", er)
	}

	// The watcher must observe the DELETE: the version is removed from the
	// remote and, with no Started version left, the remote is disabled. This
	// is how the cluster stops routing to a crashed node.
	remote = waitForRemote(t, p, node, 5*time.Second, func(r *CosmosRemote) bool {
		return remoteVersionCount(r) == 0 && !remoteEnabled(r)
	}, "remote removal after lease expiry")

	// And the watcher itself must have survived the event: a subsequent valid
	// registration is still discovered.
	const survivor, survivorVersion = "survivorNode", int64(2000)
	writeNodeInfo(t, ee.client, etcdCosmosNodeVersionURI(cosmos, survivor, survivorVersion), &CosmosNodeVersionInfo{
		Node:      survivor,
		Address:   "1.1.1.1:1235",
		Id:        &IDInfo{Type: IDType_Cosmos, Cosmos: cosmos, Node: survivor},
		State:     ClusterNodeState_Started,
		Elements:  map[string]*IDInfo{},
		StartupId: 222,
	})
	waitForRemote(t, p, survivor, 3*time.Second, func(r *CosmosRemote) bool {
		return remoteVersionCount(r) == 1
	}, "watcher still alive after node death")
}

func Test_etcdWatch_DirtyKeysAndValuesAreLoggedAndSurvived(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	const cosmos = "dirtyCosmos"
	prefixKey := etcdCosmosNodeWatchAllURI(cosmos) // /cosmos/{cosmos}/node/

	p, logger := newWatchProcess(t, cosmos, "watcherNode")
	startWatch(t, p, ee.client)

	// Garbage that an operator, a buggy client, or a half-written migration
	// could leave under the watch prefix. The watcher must report each one
	// through the error handler and keep watching.
	dirty := []struct {
		key, value, wantLog string
	}{
		// Trimmed key "garbage" → no version segment at all.
		{prefixKey + "garbage", "x", "got invalid key format"},
		// Trimmed key "n1/version" → missing the version number.
		{prefixKey + "n1/version", "x", "got invalid key format"},
		// Trimmed key "n1/x/100" → middle segment is not "version".
		{prefixKey + "n1/x/100", "x", "got invalid key version"},
		// Valid version-key shape, value is not JSON.
		{prefixKey + "n1/version/100", "{not json", "got invalid cluster info value"},
		// Valid lock-key shape, value is not JSON.
		{prefixKey + "n1/version/", "{not json", "got invalid value"},
	}
	for _, d := range dirty {
		if err := etcdPut(ee.client, d.key, []byte(d.value)); err != nil {
			t.Fatalf("write dirty key (%s): %v", d.key, err.Message)
		}
		waitLogged(t, logger, d.wantLog, 5*time.Second)
	}

	// Every dirty entry goes through etcdErrorHandler, which marks it CRITICAL.
	waitLogged(t, logger, "etcd: CRITICAL ERROR", 5*time.Second)

	// No remote may have been created from garbage data.
	if r := getRemote(p, "n1"); r != nil {
		t.Fatalf("remote created from dirty data: %+v", r)
	}

	// The watcher must still be alive and correct: a valid registration that
	// arrives after all the garbage is discovered normally.
	const survivor, survivorVersion = "survivorNode", int64(3000)
	writeNodeInfo(t, ee.client, etcdCosmosNodeVersionURI(cosmos, survivor, survivorVersion), &CosmosNodeVersionInfo{
		Node:      survivor,
		Address:   "2.2.2.2:1234",
		Id:        &IDInfo{Type: IDType_Cosmos, Cosmos: cosmos, Node: survivor},
		State:     ClusterNodeState_Started,
		Elements:  map[string]*IDInfo{},
		StartupId: 333,
	})
	waitForRemote(t, p, survivor, 3*time.Second, func(r *CosmosRemote) bool {
		return remoteVersionCount(r) == 1
	}, "watcher still alive after dirty data")
}

func Test_etcdWatch_ServerRestartReconnectsAndResumes(t *testing.T) {
	clientPort := reserveLocalPort(t)
	dir := t.TempDir()

	e1 := startEmbeddedEtcdAt(t, dir, clientPort)
	endpoint := fmt.Sprintf("127.0.0.1:%d", clientPort)
	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{endpoint}, DialTimeout: 5 * time.Second})
	if err != nil {
		e1.Close()
		t.Fatalf("client: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	const cosmos, nodeA = "restartCosmos", "nodeA"
	const versionA = int64(1000)
	versionKeyA := etcdCosmosNodeVersionURI(cosmos, nodeA, versionA)
	writeNodeInfo(t, cli, versionKeyA, &CosmosNodeVersionInfo{
		Node:      nodeA,
		Address:   "3.3.3.3:1234",
		Id:        &IDInfo{Type: IDType_Cosmos, Cosmos: cosmos, Node: nodeA},
		State:     ClusterNodeState_Started,
		Elements:  map[string]*IDInfo{},
		StartupId: 444,
	})

	// The watcher discovers A before the outage.
	p, _ := newWatchProcess(t, cosmos, "watcherNode")
	startWatch(t, p, cli)
	waitForRemote(t, p, nodeA, 3*time.Second, func(r *CosmosRemote) bool {
		return remoteVersionCount(r) == 1
	}, "discovery before outage")

	// Total outage: kill etcd entirely. To a client this is the same as a
	// network partition that drops every packet — the endpoint stops answering.
	e1.Close()

	// etcd is observed unavailable: writes fail (bounded by the helpers' own
	// RPC deadlines, not by a hang).
	if err := etcdPut(cli, "/atomos/outage/probe", []byte("x")); err == nil {
		t.Fatal("write during total outage unexpectedly succeeded")
	}

	// etcd comes back on the same address, with its data (same dir).
	e2 := startEmbeddedEtcdAt(t, dir, clientPort)
	t.Cleanup(func() { e2.Close() })

	// Data written before the outage survives the restart (retry: the client
	// needs a moment to re-establish its connections).
	deadline := time.Now().Add(15 * time.Second)
	for {
		val, _, err := etcdGet(cli, versionKeyA)
		if err == nil && val != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("node A key not readable after restart: val=(%s), err=(%v)", val, err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// New events written after recovery must reach the watcher: the etcd
	// client resumes the watch stream, and watchCluster keeps consuming it.
	// Reconnect backoff makes this take a few seconds, so the budget is loose.
	const nodeB, versionB = "nodeB", int64(2000)
	writeNodeInfo(t, cli, etcdCosmosNodeVersionURI(cosmos, nodeB, versionB), &CosmosNodeVersionInfo{
		Node:      nodeB,
		Address:   "3.3.3.3:1235",
		Id:        &IDInfo{Type: IDType_Cosmos, Cosmos: cosmos, Node: nodeB},
		State:     ClusterNodeState_Started,
		Elements:  map[string]*IDInfo{},
		StartupId: 555,
	})
	waitForRemote(t, p, nodeB, 30*time.Second, func(r *CosmosRemote) bool {
		return remoteVersionCount(r) == 1
	}, "watch reconnected after server restart")

	// The pre-outage remote must still be there (it was discovered before the
	// outage; nothing removed it).
	if r := getRemote(p, nodeA); r == nil || remoteVersionCount(r) != 1 {
		t.Fatalf("node A remote lost across restart: %+v", r)
	}
}

func Test_etcdWatch_RebuildReconcilesMissedDeleteAndCancelExits(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	const cosmos, node = "reconcileCosmos", "departedNode"
	const version int64 = 1000
	p, _ := newWatchProcess(t, cosmos, "watcherNode")
	startWatch(t, p, ee.client)
	writeNodeInfo(t, ee.client, etcdCosmosNodeVersionURI(cosmos, node, version), &CosmosNodeVersionInfo{
		Node: node, Address: "127.0.0.1:1", StartupId: 1,
		Id:    &IDInfo{Type: IDType_Cosmos, Cosmos: cosmos, Node: node},
		State: ClusterNodeState_Started, Elements: map[string]*IDInfo{},
	})
	waitForRemote(t, p, node, 3*time.Second, func(remote *CosmosRemote) bool {
		return remoteVersionCount(remote) == 1
	}, "initial discovery")

	p.cluster.etcdCancelWatch()
	deadline := time.Now().Add(3 * time.Second)
	for p.etcdRuntime().watchHealthy.Load() {
		if time.Now().After(deadline) {
			t.Fatal("watch cancellation did not stop the watch loop")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := etcdDelete(ee.client, etcdCosmosNodeVersionURI(cosmos, node, version)); err != nil {
		t.Fatal(err)
	}

	// The DELETE happened while no watch existed. The next snapshot must remove
	// the stale cache entry and subscribe after that snapshot's revision.
	if err := p.watchCluster(ee.client); err != nil {
		t.Fatal(err)
	}
	waitForRemote(t, p, node, 3*time.Second, func(remote *CosmosRemote) bool {
		return remoteVersionCount(remote) == 0
	}, "snapshot reconciliation of missed delete")
}
