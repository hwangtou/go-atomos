package atomos

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hwangtou/go-atomos/internal/memoryetcd"
	"github.com/hwangtou/go-atomos/internal/testclock"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/proto"
)

type dstLifecycleFixture struct {
	clock        *testclock.VirtualClock
	client       *memoryetcd.MemoryEtcd
	process      *CosmosProcess
	constructors atomic.Int32
	haltNotify   chan struct{}
}

func newDSTLifecycleFixture(t *testing.T, name string) *dstLifecycleFixture {
	t.Helper()
	clock := testclock.NewVirtualClock()
	client := memoryetcd.NewMemoryEtcd(clock)
	process, err := newCosmosProcess(name, "nodeA", &reproLogger{})
	if err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	runnable := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: name, Node: "nodeA"}).
		SetLifecycleMode(LifecycleModePerAtomStrict).
		SetClock(clock).
		SetEtcdClient(client)
	runnable.config.EnableCluster = &CosmosClusterConfig{Enable: true, EtcdEndpoints: []string{"unused:0"}, OptionalPorts: []int32{0}}
	runnable.config.Customize = map[string][]byte{configKeyClusterAdvertiseIP: []byte("127.0.0.1")}
	if err := process.Start(runnable); err != nil {
		_ = client.Close()
		t.Fatalf("DST Start: %v", err)
	}
	fixture := &dstLifecycleFixture{clock: clock, client: client, process: process, haltNotify: make(chan struct{}, 8)}
	oldHook := testAtomConstructorHook
	testAtomConstructorHook = func(string) *testRunnableAtom {
		fixture.constructors.Add(1)
		return &testRunnableAtom{haltNotify: fixture.haltNotify}
	}
	t.Cleanup(func() {
		testAtomConstructorHook = oldHook
		if err := process.Stop(); err != nil {
			t.Errorf("DST Stop: %v", err)
		}
		if err := client.Close(); err != nil {
			t.Errorf("MemoryEtcd Close: %v", err)
		}
	})
	return fixture
}

func (f *dstLifecycleFixture) spawn(t *testing.T, atom, operationID string) *ForTestAtomosAtomID {
	t.Helper()
	id, err := SpawnForTestAtomosAtom(f.process.local, f.process.local, atom, &ForTestSpawnArg{Name: atom}, ArgBaseAtomosOperationID(operationID))
	if err != nil {
		t.Fatalf("Spawn(%s): %v", operationID, err)
	}
	t.Cleanup(id.Release)
	return id
}

func (f *dstLifecycleFixture) advanceUntil(t *testing.T, step time.Duration, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		f.clock.Advance(step)
		time.Sleep(time.Millisecond)
	}
}

func waitDSTLifecycle(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(time.Millisecond)
	}
}

func assertDSTSucceeded(t *testing.T, p *CosmosProcess, operationID string) {
	t.Helper()
	op, err := p.local.CosmosGetAtomOperation(operationID)
	if err != nil || op.State != AtomOperationState_AtomOperationStateSucceeded {
		t.Fatalf("operation %s = (%v, %v), want Succeeded", operationID, op, err)
	}
}

func TestDST_SpawnClaimResponseLossReplay(t *testing.T) {
	fixture := newDSTLifecycleFixture(t, "dstSpawnClaim")
	const operationID = "spawn-claim-lost"
	fixture.client.InjectTxnResponseLossAfter(0, errors.New("claim response lost"))
	id := fixture.spawn(t, "claimed", operationID)
	if got := fixture.constructors.Load(); got != 1 {
		t.Fatalf("constructors after recovered claim = %d, want 1", got)
	}
	replay := fixture.spawn(t, "claimed", operationID)
	if !proto.Equal(id.GetIDInfo(), replay.GetIDInfo()) {
		t.Fatalf("replay identity = %v, want %v", replay.GetIDInfo(), id.GetIDInfo())
	}
	if got := fixture.constructors.Load(); got != 1 {
		t.Fatalf("constructors after replay = %d, want 1", got)
	}
	assertDSTSucceeded(t, fixture.process, operationID)
}

func TestDST_ConcurrentOperationReplayExecutesOnce(t *testing.T) {
	fixture := newDSTLifecycleFixture(t, "dstConcurrentReplay")
	started := make(chan struct{})
	release := make(chan struct{})
	oldHook := testAtomConstructorHook
	testAtomConstructorHook = func(string) *testRunnableAtom {
		fixture.constructors.Add(1)
		return &testRunnableAtom{haltNotify: fixture.haltNotify, spawnHook: func(AtomSelfID) error {
			close(started)
			<-release
			return nil
		}}
	}
	defer func() { testAtomConstructorHook = oldHook }()

	const operationID = "concurrent-spawn-operation"
	type result struct {
		id  *ForTestAtomosAtomID
		err error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := SpawnForTestAtomosAtom(fixture.process.local, fixture.process.local, "concurrent", &ForTestSpawnArg{Name: "concurrent"}, ArgBaseAtomosOperationID(operationID))
			results <- result{id: id, err: err}
		}()
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first Spawn did not enter user code")
	}
	close(release)
	wg.Wait()
	close(results)
	var first *IDInfo
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent Spawn: %v", result.err)
		}
		result.id.Release()
		if first == nil {
			first = result.id.GetIDInfo()
		} else if !proto.Equal(first, result.id.GetIDInfo()) {
			t.Fatalf("concurrent replay identities differ: %v != %v", first, result.id.GetIDInfo())
		}
	}
	if got := fixture.constructors.Load(); got != 1 {
		t.Fatalf("concurrent replay constructors = %d, want 1", got)
	}
	fixture.process.lifecycle.operationMu.Lock()
	remaining := len(fixture.process.lifecycle.operations)
	fixture.process.lifecycle.operationMu.Unlock()
	if remaining != 0 {
		t.Fatalf("completed operation locks retained = %d", remaining)
	}
}

func TestDST_SpawnCompletionResponseLoss(t *testing.T) {
	fixture := newDSTLifecycleFixture(t, "dstSpawnCompletion")
	const operationID = "spawn-completion-lost"
	fixture.client.InjectTxnResponseLossAfter(1, errors.New("completion response lost"))
	id := fixture.spawn(t, "completed", operationID)
	if got := fixture.constructors.Load(); got != 1 {
		t.Fatalf("constructors = %d, want 1", got)
	}
	if id.State() != BaseAtomosWaiting {
		t.Fatalf("atom state after lost completion = %s, want Running", id.State())
	}
	if _, err := id.Greeting(fixture.process.local, &ForTestGreetingI{Mode: 1}); err != nil {
		t.Fatalf("Sync after lost completion: %v", err)
	}
	replay := fixture.spawn(t, "completed", operationID)
	if !proto.Equal(id.GetIDInfo(), replay.GetIDInfo()) || fixture.constructors.Load() != 1 {
		t.Fatalf("replay constructed a replacement: id=%v replay=%v constructors=%d", id.GetIDInfo(), replay.GetIDInfo(), fixture.constructors.Load())
	}
	assertDSTSucceeded(t, fixture.process, operationID)
}

func TestDST_HaltClaimAndCompletionResponseLoss(t *testing.T) {
	for _, test := range []struct {
		name              string
		successfulCommits int
	}{
		{name: "Claim", successfulCommits: 0},
		{name: "Completion", successfulCommits: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newDSTLifecycleFixture(t, "dstHalt"+test.name)
			id := fixture.spawn(t, "halted", "spawn-"+test.name)
			const operationID = "halt-response-lost"
			fixture.client.InjectTxnResponseLossAfter(test.successfulCommits, errors.New("halt response lost"))
			if err := id.Kill(fixture.process.local, ArgBaseAtomosWaitKilled(), ArgBaseAtomosOperationID(operationID)); err != nil {
				t.Fatalf("Halt with lost response: %v", err)
			}
			select {
			case <-fixture.haltNotify:
			case <-time.After(5 * time.Second):
				t.Fatal("local Halt did not run")
			}
			if id.State() != BaseAtomosHalt {
				t.Fatalf("atom state = %s, want Halt", id.State())
			}
			assertDSTSucceeded(t, fixture.process, operationID)
			if err := id.Kill(fixture.process.local, ArgBaseAtomosWaitKilled(), ArgBaseAtomosOperationID(operationID)); err != nil {
				t.Fatalf("same-operation Halt replay: %v", err)
			}
			select {
			case <-fixture.haltNotify:
				t.Fatal("same-operation replay killed the atom twice")
			default:
			}
			lookup, err := fixture.process.local.CosmosLookupAtom(ForTestAtomosName, "halted")
			if err != nil || lookup.State != AtomLifecycleState_AtomLifecycleStateAbsent {
				t.Fatalf("lookup after Halt = (%v, %v)", lookup, err)
			}
		})
	}
}

func TestDST_LeaseExpiryFencesOldOwner(t *testing.T) {
	fixture := newDSTLifecycleFixture(t, "dstLeaseExpiry")
	oldID := fixture.spawn(t, "leased", "spawn-before-expiry")
	oldInfo := proto.Clone(oldID.GetIDInfo()).(*IDInfo)
	fixture.client.InjectError(memoryetcd.MemoryEtcdGrant, errors.New("grant unavailable"))
	fixture.client.DisconnectKeepAlives()
	fixture.clock.Advance(etcdKeepaliveTime * time.Second)
	ownerKey := fixture.process.lifecycle.ownerKey(ForTestAtomosName, "leased")
	rsp, err := fixture.client.Get(context.Background(), ownerKey)
	if err != nil || len(rsp.Kvs) != 0 {
		t.Fatalf("owner after lease expiry = (%v, %v), want deleted", rsp, err)
	}
	waitDSTLifecycle(t, func() bool {
		_, healthy := fixture.process.lifecycle.currentLease()
		return !healthy
	}, "process did not observe keepalive loss")
	if _, err := oldID.Greeting(fixture.process.local, &ForTestGreetingI{Mode: 1}); !errors.Is(err, ErrOwnershipLost) && !errors.Is(err, ErrStaleFence) {
		t.Fatalf("old ID Sync after expiry = %v", err)
	}
	if err := oldID.Kill(fixture.process.local, ArgBaseAtomosWaitKilled()); !errors.Is(err, ErrOwnershipLost) && !errors.Is(err, ErrStaleFence) {
		t.Fatalf("old ID Kill after expiry = %v", err)
	}

	fixture.client.InjectError(memoryetcd.MemoryEtcdGrant, nil)
	fixture.advanceUntil(t, etcdRegistrationBackoff, func() bool {
		_, healthy := fixture.process.lifecycle.currentLease()
		return healthy
	}, "process did not re-register after healing Grant")
	replacement, processErr := newCosmosProcess("dstLeaseExpiry", "nodeB", &reproLogger{})
	if processErr != nil {
		t.Fatalf("new replacement process: %v", processErr)
	}
	runnable := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: "dstLeaseExpiry", Node: "nodeB"}).
		SetLifecycleMode(LifecycleModePerAtomStrict).SetClock(fixture.clock).SetEtcdClient(fixture.client)
	runnable.config.EnableCluster = &CosmosClusterConfig{Enable: true, EtcdEndpoints: []string{"unused:0"}, OptionalPorts: []int32{0}}
	runnable.config.Customize = map[string][]byte{configKeyClusterAdvertiseIP: []byte("127.0.0.1")}
	if err := replacement.Start(runnable); err != nil {
		t.Fatalf("replacement Start: %v", err)
	}
	t.Cleanup(func() {
		if err := replacement.Stop(); err != nil {
			t.Errorf("replacement Stop: %v", err)
		}
	})
	newID, err := SpawnForTestAtomosAtom(replacement.local, replacement.local, "leased", &ForTestSpawnArg{Name: "leased"}, ArgBaseAtomosOperationID("spawn-after-expiry"))
	if err != nil {
		t.Fatalf("replacement Spawn: %v", err)
	}
	t.Cleanup(newID.Release)
	if newID.GetIDInfo().Fence <= oldInfo.Fence {
		t.Fatalf("new fence = %d, old fence = %d", newID.GetIDInfo().Fence, oldInfo.Fence)
	}
	if _, err := oldID.Greeting(fixture.process.local, &ForTestGreetingI{Mode: 1}); !errors.Is(err, ErrOwnershipLost) && !errors.Is(err, ErrStaleFence) {
		t.Fatalf("old ID Sync after fenced replacement = %v", err)
	}
	if err := oldID.Kill(fixture.process.local, ArgBaseAtomosWaitKilled()); !errors.Is(err, ErrOwnershipLost) && !errors.Is(err, ErrStaleFence) {
		t.Fatalf("old ID Kill after fenced replacement = %v", err)
	}
	if _, err := newID.Greeting(replacement.local, &ForTestGreetingI{Mode: 1}); err != nil {
		t.Fatalf("new ID Sync: %v", err)
	}
}

func TestDST_OwnerWatchDisconnectResync(t *testing.T) {
	fixture := newDSTLifecycleFixture(t, "dstOwnerWatch")
	id := fixture.spawn(t, "watched", "spawn-watched")
	oldFence := id.GetIDInfo().Fence
	ownerPrefix := fixture.process.lifecycle.ownerPrefix()
	waitDSTLifecycle(t, func() bool { return fixture.client.WatcherCount(ownerPrefix) == 1 }, "owner watch was not registered")
	fixture.client.DisconnectWatches()
	waitDSTLifecycle(t, func() bool { return !fixture.process.lifecycle.watchHealthy.Load() }, "owner watch stayed healthy after disconnect")
	ownerKey := fixture.process.lifecycle.ownerKey(ForTestAtomosName, "watched")
	if _, err := fixture.client.Delete(context.Background(), ownerKey); err != nil {
		t.Fatalf("backend owner Delete: %v", err)
	}
	fixture.advanceUntil(t, lifecycleWatchReconnect, func() bool {
		return fixture.process.lifecycle.watchHealthy.Load() && fixture.process.lifecycle.cachedOwner(ForTestAtomosName, "watched") == nil
	}, "owner snapshot did not resynchronize after reconnect")
	if _, err := id.Greeting(fixture.process.local, &ForTestGreetingI{Mode: 1}); !errors.Is(err, ErrStaleFence) && !errors.Is(err, ErrOwnershipLost) {
		t.Fatalf("stale fence %d Sync after resync = %v", oldFence, err)
	}
	if err := id.Kill(fixture.process.local, ArgBaseAtomosWaitKilled()); !errors.Is(err, ErrStaleFence) && !errors.Is(err, ErrOwnershipLost) {
		t.Fatalf("stale fence %d Kill after resync = %v", oldFence, err)
	}
}

func TestDST_MemoryEtcdLeaseDisconnectDoesNotRevoke(t *testing.T) {
	clock := testclock.NewVirtualClock()
	client := memoryetcd.NewMemoryEtcd(clock)
	t.Cleanup(func() { _ = client.Close() })
	lease, err := client.Grant(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	keepAlive, err := client.KeepAlive(context.Background(), clientv3.LeaseID(lease.ID))
	if err != nil {
		t.Fatal(err)
	}
	client.DisconnectKeepAlives()
	for {
		select {
		case _, ok := <-keepAlive:
			if !ok {
				goto disconnected
			}
		case <-time.After(5 * time.Second):
			t.Fatal("keepalive stream did not close")
		}
	}

disconnected:
	key := fmt.Sprintf("lease/%d", lease.ID)
	if _, err := client.Put(context.Background(), key, "value", clientv3.LeaseID(lease.ID)); err != nil {
		t.Fatal(err)
	}
	clock.Advance(19 * time.Second)
	if rsp, err := client.Get(context.Background(), key); err != nil || len(rsp.Kvs) != 1 {
		t.Fatalf("DisconnectKeepAlives revoked lease early: (%v, %v)", rsp, err)
	}
	clock.Advance(time.Second)
	if rsp, err := client.Get(context.Background(), key); err != nil || len(rsp.Kvs) != 0 {
		t.Fatalf("lease did not expire at TTL: (%v, %v)", rsp, err)
	}
}
