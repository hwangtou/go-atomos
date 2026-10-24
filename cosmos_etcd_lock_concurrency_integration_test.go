//go:build integration

package atomos

import (
	"sort"
	"sync"
	"testing"
)

// Concurrent version-lock tests against a real etcd. The lock is an optimistic
// transaction (read state, then CAS on the etcd key version), so correctness
// under concurrency is exactly what the serialized lifecycle test in
// cosmos_etcd_version_lock_integration_test.go cannot prove:
//
//   - concurrent TryLock: the create-race (read "absent", lose the create),
//     the append-race (two writers CAS on the same key version), and the
//     capacity cap must never let a third version slip in or corrupt the list
//   - concurrent unlock: the loser of the CAS race must fail loudly, never
//     silently drop the other writer's update (lost update)

func Test_etcdVersionLock_ConcurrentTryLock(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	const cosmos, node = "lockRacyCosmos", "lockRacyNode"

	// One process, many versions: TryLock is per-(node,version) and only reads
	// the cosmos/node names from the process, so this is equivalent to many
	// processes racing to hot-upgrade the same node.
	p, _ := newVersionLockProcess(t, ee, cosmos, node, 100)

	const racers = 8
	versions := make([]int64, racers)
	for i := range versions {
		versions[i] = int64(100 * (i + 1))
	}

	results := make([]*Error, racers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = p.etcdStartUpTryLockingVersionNodeLock(ee.client, node, versions[i])
		}(i)
	}
	close(start)
	wg.Wait()

	var winners []int64
	for i, err := range results {
		if err == nil {
			winners = append(winners, versions[i])
			continue
		}
		if code := CodeOf(err); code != ErrCodeCosmosEtcdClusterVersionLockFailed {
			t.Errorf("racer v%d: expected ErrCodeCosmosEtcdClusterVersionLockFailed, got code=(%d), err=(%v)", versions[i], code, err)
		}
	}

	// At least the create-race winner succeeds; at most maxNodes (2) win.
	// Which racers win depends on the interleaving — the invariants do not.
	if len(winners) < 1 || len(winners) > 2 {
		t.Fatalf("expected 1-2 winners, got %d: %v", len(winners), winners)
	}

	// The lock must contain EXACTLY the winners — no lost update (a winner
	// missing), no ghost version (a loser admitted), no duplicates.
	lock := readLock(t, ee, cosmos, node)
	if lock == nil {
		t.Fatal("lock key missing after race")
	}
	got := append([]int64(nil), lock.Versions...)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	sort.Slice(winners, func(i, j int) bool { return winners[i] < winners[j] })
	if len(got) != len(winners) {
		t.Fatalf("lock contents do not match winners. lock=(%v), winners=(%v)", got, winners)
	}
	for i := range got {
		if got[i] != winners[i] {
			t.Fatalf("lock contents do not match winners. lock=(%v), winners=(%v)", got, winners)
		}
	}
}

func Test_etcdVersionLock_ConcurrentUnlockCASConflict(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	const cosmos, node = "lockRacyCosmos", "lockRacyNode"

	// Two coexisting versions of one node (the hot-upgrade window).
	p100, _ := newVersionLockProcess(t, ee, cosmos, node, 100)
	p200, _ := newVersionLockProcess(t, ee, cosmos, node, 200)
	if err := p100.etcdStartUpTryLockingVersionNodeLock(ee.client, node, 100); err != nil {
		t.Fatalf("lock v100: %v", err.Message)
	}
	if err := p200.etcdStartUpTryLockingVersionNodeLock(ee.client, node, 200); err != nil {
		t.Fatalf("lock v200: %v", err.Message)
	}

	// Both stop concurrently. Each unlock reads the lock, removes its own
	// version, and CAS-commits; when they read the same key version, the
	// etcd transaction serializes them and the loser's stale write must be
	// REJECTED — the failure mode being pinned is the silent lost update.
	errs := make([]*Error, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, p := range []*CosmosProcess{p100, p200} {
		wg.Add(1)
		go func(i int, p *CosmosProcess) {
			defer wg.Done()
			<-start
			errs[i] = p.etcdStoppingNodeVersionUnlock()
		}(i, p)
	}
	close(start)
	wg.Wait()

	// Either the calls serialize cleanly (both succeed) or the CAS conflict
	// rejects one — but at least the first commit always succeeds.
	succeeded := 0
	var remaining []int64
	for i, err := range errs {
		version := int64(100 * (i + 1))
		if err == nil {
			succeeded++
		} else {
			remaining = append(remaining, version)
			if code := CodeOf(err); code != ErrCodeCosmosEtcdClusterVersionLockFailed {
				t.Errorf("unlock v%d: expected ErrCodeCosmosEtcdClusterVersionLockFailed, got code=(%d), err=(%v)", version, code, err)
			}
		}
	}
	if succeeded < 1 || succeeded > 2 {
		t.Fatalf("expected 1-2 successful unlocks, got %d: %v", succeeded, errs)
	}

	// The lock must end up holding exactly the versions whose unlock did NOT
	// succeed — one commit lost is a bug, both versions surviving a double
	// unlock is a bug.
	lock := readLock(t, ee, cosmos, node)
	if lock == nil {
		t.Fatal("lock key missing after unlock race")
	}
	got := append([]int64(nil), lock.Versions...)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if len(got) != len(remaining) {
		t.Fatalf("lock=(%v), want exactly the not-unlocked versions=(%v)", got, remaining)
	}
	for i := range got {
		if got[i] != remaining[i] {
			t.Fatalf("lock=(%v), want exactly the not-unlocked versions=(%v)", got, remaining)
		}
	}
}
