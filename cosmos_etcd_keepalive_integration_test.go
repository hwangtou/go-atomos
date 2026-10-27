//go:build integration

package atomos

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Keepalive outage-recovery test: the registration path
// (trySettingClusterToCurrentAndKeepalive) must retry through an etcd outage
// and recover once etcd is back, and the graceful Stop path must tear the
// whole registration down (watcher exit handshake, unlock, key delete).

// newKeepaliveClusterProcess returns a started process with the cluster fields
// wired the way prepareClusterLocalNode would leave them, so the production
// registration/keepalive path runs against the embedded etcd.
func newKeepaliveClusterProcess(t *testing.T, cli EtcdClient, cosmos, node string, version int64) (*CosmosProcess, *reproLogger) {
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
	p.cluster.enable = true
	p.cluster.etcdClient = cli
	p.cluster.etcdVersion = version
	p.cluster.etcdInfoCh = make(chan etcdInfoUpdate, 1)
	return p, logger
}

// readLockWithClient reads the version-lock entry with an EtcdClient (the
// readLock helper in the version-lock tests takes an *embeddedEtcd).
func readLockWithClient(t *testing.T, cli EtcdClient, cosmos, node string) *CosmosNodeVersionLock {
	t.Helper()
	buf, _, err := etcdGet(cli, etcdCosmosNodeLockURI(cosmos, node))
	if err != nil {
		t.Fatalf("etcdGet lock: %v", err.Message)
	}
	if buf == nil {
		return nil
	}
	lock := &CosmosNodeVersionLock{}
	if er := json.Unmarshal(buf, lock); er != nil {
		t.Fatalf("unmarshal lock (%s): %v", buf, er)
	}
	return lock
}

func Test_etcdKeepaliveRetry_OutageThenRecover(t *testing.T) {
	clientPort := reserveLocalPort(t)
	dir := t.TempDir()

	e1 := startEmbeddedEtcdAt(t, dir, clientPort)
	endpoint := fmt.Sprintf("127.0.0.1:%d", clientPort)
	raw, err := clientv3.New(clientv3.Config{Endpoints: []string{endpoint}, DialTimeout: 5 * time.Second})
	if err != nil {
		e1.Close()
		t.Fatalf("client: %v", err)
	}
	cli := NewEtcdClientV3(raw)
	t.Cleanup(func() { _ = cli.Close() })

	const cosmos, node = "kaCosmos", "kaNode"
	const version int64 = 1000
	versionKey := etcdCosmosNodeVersionURI(cosmos, node, version)

	p, _ := newKeepaliveClusterProcess(t, cli, cosmos, node, version)

	// Startup takes the version lock before registration begins.
	if err := p.etcdStartUpTryLockingVersionNodeLock(cli, node, version); err != nil {
		t.Fatalf("startup lock: %v", err.Message)
	}

	// Total outage: etcd is killed before the node registers. Shrink the
	// one-shot RPC timeout so each failed attempt is fast — the default 10s
	// would only make the test slow, not more meaningful.
	e1.Close()
	oldTimeout := etcdOpTimeout
	etcdOpTimeout = 300 * time.Millisecond
	t.Cleanup(func() { etcdOpTimeout = oldTimeout })

	done := make(chan *Error, 1)
	go func() { done <- p.trySettingClusterToCurrentAndKeepalive() }()

	// The first attempt fails fast (~300ms) and the loop backs off (3s). At
	// this point the call must still be pending — giving up after one failure
	// would strand the node.
	time.Sleep(600 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("registration gave up after the first failure: %v", err.Message)
	default:
	}
	if buf, _, err := etcdGet(cli, versionKey); err == nil && buf != nil {
		t.Fatalf("version key appeared while etcd is down: %s", buf)
	}

	// etcd comes back. The retry loop must recover on a later attempt and
	// complete registration (lease + keepalive watcher + current-version mark).
	e2 := startEmbeddedEtcdAt(t, dir, clientPort)
	t.Cleanup(func() { e2.Close() })

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("registration after recovery failed: %v", err.Message)
		}
	case <-time.After(45 * time.Second):
		t.Fatal("registration did not recover after etcd restart")
	}

	// The version key is registered under a lease (retry: the client needs a
	// moment to become ready again after the restart).
	deadline := time.Now().Add(15 * time.Second)
	for {
		buf, _, err := etcdGet(cli, versionKey)
		if err == nil && buf != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("version key not readable after recovery: buf=(%s), err=(%v)", buf, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	// The lock marks this version as current (etcdStartedVersionNodeLock ran).
	if lock := readLockWithClient(t, cli, cosmos, node); lock == nil || lock.Current != version {
		t.Fatalf("lock after recovery: %+v, want current=%d", lock, version)
	}

	// Graceful stop tears down registration without closing the injected client.
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if buf, _, err := etcdGet(cli, versionKey); err != nil || buf != nil {
		t.Fatalf("version key after Stop: buf=(%s), err=(%v)", buf, err)
	}
	if lock := readLockWithClient(t, cli, cosmos, node); lock == nil || len(lock.Versions) != 0 {
		t.Fatalf("lock after Stop: %+v, want empty versions", lock)
	}
	if p.cluster.etcdClient != nil || p.cluster.etcdInfoCh != nil {
		t.Fatalf("Stop left cluster state dirty: etcdClient=(%v), etcdInfoCh=(%v)", p.cluster.etcdClient, p.cluster.etcdInfoCh)
	}
}

func Test_etcdKeepaliveLost_ReregistersAndRestoresHealthAndWatch(t *testing.T) {
	clientPort := reserveLocalPort(t)
	dir := t.TempDir()
	e1 := startEmbeddedEtcdAt(t, dir, clientPort)
	endpoint := fmt.Sprintf("127.0.0.1:%d", clientPort)
	raw, er := clientv3.New(clientv3.Config{Endpoints: []string{endpoint}, DialTimeout: 5 * time.Second})
	if er != nil {
		e1.Close()
		t.Fatal(er)
	}
	cli := NewEtcdClientV3(raw)
	t.Cleanup(func() { _ = cli.Close() })

	oldTimeout, oldBackoff, oldAttempts := etcdOpTimeout, etcdRegistrationBackoff, etcdRegistrationAttempts
	etcdOpTimeout, etcdRegistrationBackoff, etcdRegistrationAttempts = 300*time.Millisecond, 200*time.Millisecond, 100
	t.Cleanup(func() {
		etcdOpTimeout, etcdRegistrationBackoff, etcdRegistrationAttempts = oldTimeout, oldBackoff, oldAttempts
	})

	const cosmos, node = "recoveryCosmos", "recoveryNode"
	const version int64 = 1000
	p, _ := newKeepaliveClusterProcess(t, cli, cosmos, node, version)
	startWatch(t, p, cli)
	if err := p.etcdStartUpTryLockingVersionNodeLock(cli, node, version); err != nil {
		t.Fatal(err)
	}
	if err := p.trySettingClusterToCurrentAndKeepalive(); err != nil {
		t.Fatal(err)
	}
	waitFor := func(timeout time.Duration, condition func() bool, what string) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatalf("timeout waiting for %s", what)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	waitFor(5*time.Second, p.EtcdClusterHealthy, "initial cluster health")

	leaseID := func() clientv3.LeaseID {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		resp, err := cli.Get(ctx, etcdCosmosNodeVersionURI(cosmos, node, version))
		if err != nil || len(resp.Kvs) == 0 {
			return 0
		}
		return clientv3.LeaseID(resp.Kvs[0].Lease)
	}
	before := leaseID()
	if before == 0 {
		t.Fatal("initial registration has no lease")
	}

	e1.Close()
	waitFor(20*time.Second, func() bool {
		runtime, ok := etcdRuntimeStates.Load(p)
		return ok && !runtime.(*etcdRuntimeState).registrationHealthy.Load() && !p.EtcdClusterHealthy()
	}, "lost registration to become unhealthy")

	e2 := startEmbeddedEtcdAt(t, dir, clientPort)
	t.Cleanup(e2.Close)
	waitFor(20*time.Second, p.EtcdClusterHealthy, "registration and watch recovery")
	waitFor(5*time.Second, func() bool {
		after := leaseID()
		return after != 0 && after != before
	}, "a replacement lease")

	writeNodeInfo(t, cli, etcdCosmosNodeVersionURI(cosmos, "recoveredPeer", 2000), &CosmosNodeVersionInfo{
		Node: "recoveredPeer", Address: "127.0.0.1:1", StartupId: 99,
		Id:    &IDInfo{Type: IDType_Cosmos, Cosmos: cosmos, Node: "recoveredPeer"},
		State: ClusterNodeState_Started, Elements: map[string]*IDInfo{},
	})
	waitForRemote(t, p, "recoveredPeer", 5*time.Second, func(remote *CosmosRemote) bool {
		return remoteVersionCount(remote) == 1
	}, "watch event after registration recovery")
}
