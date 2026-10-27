//go:build integration

package atomos

import (
	"sort"
	"sync"
	"testing"
)

// Concurrent version-lock tests against a real etcd. The lock is an optimistic
// ModRevision CAS with bounded retries, so correctness under concurrency is
// exactly what the serialized lifecycle test cannot prove: the create/append
// races preserve the capacity cap, and conflicting unlocks retry rather than
// silently dropping either writer's update.

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
		// Every contender is live; otherwise full-lock crash reclamation would
		// deliberately remove its registration-free version.
		if err := etcdPut(ee.client, etcdCosmosNodeVersionURI(cosmos, node, versions[i]), []byte("registered")); err != nil {
			t.Fatal(err)
		}
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
		if code := CodeOf(err); code != ErrCodeStorageConflict {
			t.Errorf("racer v%d: expected ErrCodeStorageConflict, got code=(%d), err=(%v)", versions[i], code, err)
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

	// A ModRevision conflict retries from a fresh snapshot, so both removals
	// eventually commit without either writer losing the other's update.
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
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent unlock %d failed after bounded retry: %v", i, err)
		}
	}
	lock := readLock(t, ee, cosmos, node)
	if lock == nil || len(lock.Versions) != 0 || lock.Current != 0 {
		t.Fatalf("concurrent unlocks did not remove both versions: %+v", lock)
	}
}
