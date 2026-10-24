//go:build integration

package atomos

import (
	"encoding/json"
	"testing"
	"time"
)

// Version-lock lifecycle tests against an in-process etcd.
//
// The version lock serializes hot upgrades: at most 2 versions of the same
// node may coexist in a cosmos. Audit finding A2 was "lock list silently
// cleared when full" — now rejected with an error; the third-version test
// below pins that regression.

// newVersionLockProcess returns a started standalone CosmosProcess whose
// cluster etcd client points at the embedded etcd. Logs are captured by
// reproLogger so framework coreError paths (e.g. "version not in lock info")
// can be asserted without failing the test.
func newVersionLockProcess(t *testing.T, ee *embeddedEtcd, cosmos, node string, version int64) (*CosmosProcess, *reproLogger) {
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
	p.cluster.etcdClient = ee.client
	p.cluster.etcdVersion = version
	return p, logger
}

// readLock reads and decodes the version-lock entry for a node.
func readLock(t *testing.T, ee *embeddedEtcd, cosmos, node string) *CosmosNodeVersionLock {
	t.Helper()
	buf, _, err := etcdGet(ee.client, etcdCosmosNodeLockURI(cosmos, node))
	if err != nil {
		t.Fatalf("etcdGet lock: %v", err)
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

func versionsOf(lock *CosmosNodeVersionLock) []int64 {
	if lock == nil {
		return nil
	}
	return lock.Versions
}

func Test_etcdVersionLock_Lifecycle(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	const cosmos, node = "lockCosmos", "lockNode"

	// First version locks into an absent key: creates versions=[100].
	p1, _ := newVersionLockProcess(t, ee, cosmos, node, 100)
	if err := p1.etcdStartUpTryLockingVersionNodeLock(ee.client, node, 100); err != nil {
		t.Fatalf("first lock: %v", err.Message)
	}
	if got := versionsOf(readLock(t, ee, cosmos, node)); len(got) != 1 || got[0] != 100 {
		t.Fatalf("after first lock: versions=%v, want [100]", got)
	}

	// Startup success marks 100 as current.
	if err := p1.etcdStartedVersionNodeLock(); err != nil {
		t.Fatalf("started lock: %v", err.Message)
	}
	if lock := readLock(t, ee, cosmos, node); lock.Current != 100 {
		t.Fatalf("current=%d, want 100", lock.Current)
	}

	// A second version may coexist (hot-upgrade window): versions=[100,200].
	p2, _ := newVersionLockProcess(t, ee, cosmos, node, 200)
	if err := p2.etcdStartUpTryLockingVersionNodeLock(ee.client, node, 200); err != nil {
		t.Fatalf("second lock: %v", err.Message)
	}
	if lock := readLock(t, ee, cosmos, node); len(lock.Versions) != 2 || lock.Current != 100 {
		t.Fatalf("after second lock: %+v, want versions=[100 200] current=100", lock)
	}

	// A third version must be REJECTED (A2 regression: reject, never clear).
	p3, _ := newVersionLockProcess(t, ee, cosmos, node, 300)
	err := p3.etcdStartUpTryLockingVersionNodeLock(ee.client, node, 300)
	if err == nil {
		t.Fatal("third lock: expected error, got nil")
	}
	if code := CodeOf(err); code != ErrCodeCosmosEtcdClusterVersionLockFailed {
		t.Errorf("third lock: code=%d, want ErrCodeCosmosEtcdClusterVersionLockFailed", code)
	}
	if lock := readLock(t, ee, cosmos, node); len(lock.Versions) != 2 {
		t.Fatalf("A2 regression: lock list changed on rejection: %+v", lock)
	}

	// Stopping p2 removes version 200, keeps current=100.
	if err := p2.etcdStoppingNodeVersionUnlock(); err != nil {
		t.Fatalf("stop unlock p2: %v", err.Message)
	}
	if lock := readLock(t, ee, cosmos, node); len(lock.Versions) != 1 || lock.Versions[0] != 100 || lock.Current != 100 {
		t.Fatalf("after p2 stop: %+v, want versions=[100] current=100", lock)
	}

	// Stopping p1 removes the last version and clears current.
	if err := p1.etcdStoppingNodeVersionUnlock(); err != nil {
		t.Fatalf("stop unlock p1: %v", err.Message)
	}
	if lock := readLock(t, ee, cosmos, node); len(lock.Versions) != 0 || lock.Current != 0 {
		t.Fatalf("after p1 stop: %+v, want empty versions and current=0", lock)
	}

	// After both stopped, a new version may lock again.
	if err := p3.etcdStartUpTryLockingVersionNodeLock(ee.client, node, 300); err != nil {
		t.Fatalf("re-lock after full unlock: %v", err.Message)
	}
	if got := versionsOf(readLock(t, ee, cosmos, node)); len(got) != 1 || got[0] != 300 {
		t.Fatalf("after re-lock: versions=%v, want [300]", got)
	}
}

func Test_etcdVersionLock_StartupFailedUnlockRemovesOwnVersion(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	const cosmos, node = "lockCosmos", "lockNode"

	p, _ := newVersionLockProcess(t, ee, cosmos, node, 100)
	if err := p.etcdStartUpTryLockingVersionNodeLock(ee.client, node, 100); err != nil {
		t.Fatalf("lock: %v", err.Message)
	}

	// Startup fails → the failed node must give its version back.
	if err := p.etcdStartUpFailedNodeVersionUnlock(); err != nil {
		t.Fatalf("startup-failed unlock: %v", err.Message)
	}
	if got := versionsOf(readLock(t, ee, cosmos, node)); len(got) != 0 {
		t.Fatalf("after startup-failed unlock: versions=%v, want empty", got)
	}

	// Unlocking when the lock key does not exist at all is an error, not a
	// silent no-op (operator-visible misbehavior).
	p2, _ := newVersionLockProcess(t, ee, cosmos, "otherNode", 200)
	if err := p2.etcdStartUpFailedNodeVersionUnlock(); err == nil {
		t.Fatal("unlock of missing lock key: expected error, got nil")
	}
}

func Test_etcdVersionLock_StartedWithForeignVersion(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	const cosmos, node = "lockCosmos", "lockNode"

	holder, _ := newVersionLockProcess(t, ee, cosmos, node, 100)
	if err := holder.etcdStartUpTryLockingVersionNodeLock(ee.client, node, 100); err != nil {
		t.Fatalf("lock: %v", err.Message)
	}
	if err := holder.etcdStartedVersionNodeLock(); err != nil {
		t.Fatalf("holder started: %v", err.Message)
	}

	// A process whose version is NOT in the lock list must not overwrite
	// Current; the framework logs a core error and returns nil.
	foreign, logger := newVersionLockProcess(t, ee, cosmos, node, 999)
	if err := foreign.etcdStartedVersionNodeLock(); err != nil {
		t.Fatalf("started with foreign version: %v", err.Message)
	}
	if lock := readLock(t, ee, cosmos, node); lock.Current != 100 {
		t.Fatalf("current=%d, want unchanged 100", lock.Current)
	}
	waitDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(waitDeadline) && !logger.hasAny("version is not in the lock info", true, true) {
		time.Sleep(50 * time.Millisecond)
	}
	if !logger.hasAny("version is not in the lock info", true, true) {
		t.Error("Expected core log 'version is not in the lock info' for foreign version")
	}
}
