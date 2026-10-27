//go:build integration

package atomos

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestShardLifecycleIntegrationOwnershipRebalanceAndDrain(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	const cosmos = "shardLifecycle"
	pa := newShardLifecycleProcess(t, ee, cosmos, "nodeA")
	pb := newShardLifecycleProcess(t, ee, cosmos, "nodeB")
	var constructors atomic.Int32
	oldHook := testAtomConstructorHook
	testAtomConstructorHook = func(name string) *testRunnableAtom {
		constructors.Add(1)
		return &testRunnableAtom{}
	}
	t.Cleanup(func() { testAtomConstructorHook = oldHook })

	processes := []*CosmosProcess{pa, pb}
	start := make(chan struct{})
	type spawnResult struct {
		id  ID
		p   *CosmosProcess
		err error
	}
	results := make(chan spawnResult, 2)
	var wg sync.WaitGroup
	for i, p := range processes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			name := []string{"first", "competitor"}[i]
			id, _, err := p.local.CosmosSpawnAtom(p.local, ForTestAtomosName, name, &ForTestSpawnArg{Name: name})
			results <- spawnResult{id: id, p: p, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var winner ID
	var owner, other *CosmosProcess
	for result := range results {
		if result.err == nil {
			if winner != nil {
				t.Fatal("both shard claims spawned")
			}
			winner, owner = result.id, result.p
		} else if !errors.Is(result.err, ErrTakeoverRequired) {
			t.Fatalf("competing shard Spawn = %v", result.err)
		}
	}
	if winner == nil || constructors.Load() != 1 {
		t.Fatalf("winner=%v constructors=%d", winner, constructors.Load())
	}
	if owner == pa {
		other = pb
	} else {
		other = pa
	}
	firstID := winner.GetIDInfo()
	secondID, _, err := owner.local.CosmosSpawnAtom(owner.local, ForTestAtomosName, "second", &ForTestSpawnArg{})
	if err != nil {
		t.Fatalf("second atom on owned shard: %v", err)
	}
	if secondID.GetIDInfo().Fence != firstID.Fence || secondID.GetIDInfo().ShardId != firstID.ShardId {
		t.Fatalf("same-lease atom changed shard fence: first=%v second=%v", firstID, secondID.GetIDInfo())
	}
	record, _, recordErr := owner.lifecycle.loadShardRecord(firstID.ShardId)
	if recordErr != nil || record.NextFence != firstID.Fence+1 {
		t.Fatalf("shard fence advanced during atom activation: record=%v err=%v", record, recordErr)
	}

	reads := owner.lifecycle.controlReads.Load()
	for _, id := range []ID{winner, secondID} {
		if _, err := id.SyncMessagingByName(owner.local, "Greeting", &ForTestGreetingI{Mode: 1}); err != nil {
			t.Fatalf("owned shard message: %v", err)
		}
	}
	if got := owner.lifecycle.controlReads.Load(); got != reads {
		t.Fatalf("shard messages accessed etcd: %d -> %d", reads, got)
	}
	leaseID, _ := owner.lifecycle.currentLease()
	owner.lifecycle.publishLease(0, false)
	for _, id := range []ID{winner, secondID} {
		if _, err := id.SyncMessagingByName(owner.local, "Greeting", &ForTestGreetingI{Mode: 1}); !errors.Is(err, ErrOwnershipLost) {
			t.Fatalf("message after shard lease loss = %v", err)
		}
	}
	owner.lifecycle.publishLease(leaseID, true)

	fingerprint := lifecycleFingerprint(AtomOperationKind_AtomOperationKindSpawn, ForTestAtomosName, "accepted_old", nil, nil)
	claim, claimErr := owner.lifecycle.claimSpawn(context.Background(), "shard-old-writeback", fingerprint, AtomOperationKind_AtomOperationKindSpawn, ForTestAtomosName, "accepted_old", nil, false)
	if claimErr != nil {
		t.Fatal(claimErr)
	}
	newID, _, err := other.local.CosmosTakeoverAtom(other.local, ForTestAtomosName, firstID.Atom, firstID, &ForTestSpawnArg{})
	if err != nil {
		t.Fatalf("cross-node shard Takeover: %v", err)
	}
	if newID.GetIDInfo().Fence <= firstID.Fence || newID.GetIDInfo().Incarnation == firstID.Incarnation {
		t.Fatalf("invalid shard Takeover identity: old=%v new=%v", firstID, newID.GetIDInfo())
	}
	if _, finishErr := owner.lifecycle.finishSpawn("shard-old-writeback", claim, nil); !errors.Is(finishErr, ErrOwnershipLost) {
		t.Fatalf("old shard writeback = %v", finishErr)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		shard := owner.lifecycle.cachedShard(firstID.ShardId)
		if shard == nil || shard.Fence != firstID.Fence {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("old shard snapshot did not observe takeover")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reads = owner.lifecycle.controlReads.Load()
	if _, err := secondID.SyncMessagingByName(owner.local, "Greeting", &ForTestGreetingI{Mode: 1}); !errors.Is(err, ErrOwnershipLost) && !errors.Is(err, ErrStaleFence) {
		t.Fatalf("other atom survived shard takeover: %v", err)
	}
	if got := owner.lifecycle.controlReads.Load(); got != reads {
		t.Fatalf("stale shard message accessed etcd: %d -> %d", reads, got)
	}

	if err := other.Drain(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if _, _, err := other.local.CosmosSpawnAtom(other.local, ForTestAtomosName, "drain_claim", &ForTestSpawnArg{}); !errors.Is(err, ErrCosmosNodeDraining) {
		t.Fatalf("new shard claim during Drain = %v", err)
	}
	if err := newID.Kill(other.local, ArgBaseAtomosWaitKilled()); err != nil {
		t.Fatalf("Halt during shard Drain = %v", err)
	}
}
