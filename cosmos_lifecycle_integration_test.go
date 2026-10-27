//go:build integration

package atomos

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/proto"
)

func newStrictLifecycleProcess(t *testing.T, ee *embeddedEtcd, cosmos, node string) *CosmosProcess {
	t.Helper()
	p, err := newCosmosProcess(cosmos, node, &reproLogger{})
	if err != nil {
		t.Fatal(err)
	}
	runnable := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: cosmos, Node: node}).SetLifecycleMode(LifecycleModePerAtomStrict)
	runnable.config.EnableCluster = &CosmosClusterConfig{Enable: true, EtcdEndpoints: []string{ee.endpoint}, OptionalPorts: []int32{int32(reserveLocalPort(t))}}
	runnable.config.Customize = map[string][]byte{configKeyClusterAdvertiseIP: []byte("127.0.0.1")}
	if err := p.Start(runnable); err != nil {
		t.Fatalf("strict Start(%s): %v", node, err)
	}
	t.Cleanup(func() { _ = p.Stop() })
	return p
}

func TestLifecycleIntegrationControlledSpawnRecoveryConflictAndHalt(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	p := newStrictLifecycleProcess(t, ee, "lifecycleBasic", "nodeA")
	var constructors atomic.Int32
	oldHook := testAtomConstructorHook
	testAtomConstructorHook = func(name string) *testRunnableAtom {
		constructors.Add(1)
		return &testRunnableAtom{}
	}
	t.Cleanup(func() { testAtomConstructorHook = oldHook })

	const operationID = "spawn-basic-operation"
	atomID, err := SpawnForTestAtomosAtom(p.local, p.local, "basic", &ForTestSpawnArg{Name: "one"}, ArgBaseAtomosOperationID(operationID))
	if err != nil {
		t.Fatalf("controlled Spawn: %v", err)
	}
	defer atomID.Release()
	id := atomID.GetIDInfo()
	if id.Incarnation == 0 || id.StartupId == 0 || id.Fence == 0 || id.InstanceId == 0 {
		t.Fatalf("incomplete controlled ID: %v", id)
	}
	duplicate, err := SpawnForTestAtomosAtom(p.local, p.local, "basic", &ForTestSpawnArg{Name: "one"}, ArgBaseAtomosOperationID(operationID))
	if err != nil {
		t.Fatalf("same-operation recovery: %v", err)
	}
	defer duplicate.Release()
	if duplicate.GetIDInfo().Incarnation != id.Incarnation || constructors.Load() != 1 {
		t.Fatalf("duplicate spawned a new incarnation: id=%v constructors=%d", duplicate.GetIDInfo(), constructors.Load())
	}
	if _, err := SpawnForTestAtomosAtom(p.local, p.local, "basic", &ForTestSpawnArg{Name: "different"}, ArgBaseAtomosOperationID(operationID)); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("fingerprint conflict = %v", err)
	}
	readsBeforeMessages := p.lifecycle.controlReads.Load()
	if _, err := atomID.Greeting(p.local, &ForTestGreetingI{Mode: 1}); err != nil {
		t.Fatalf("owned Sync: %v", err)
	}
	asyncDone := make(chan error, 1)
	if err := atomID.AsyncGreeting(p.local, &ForTestGreetingI{Mode: 1}, func(_ *ForTestGreetingO, err error) { asyncDone <- err }); err != nil {
		t.Fatalf("owned Async: %v", err)
	}
	if err := <-asyncDone; err != nil {
		t.Fatalf("owned Async callback: %v", err)
	}
	if readsAfterMessages := p.lifecycle.controlReads.Load(); readsAfterMessages != readsBeforeMessages {
		t.Fatalf("owned messages accessed etcd: reads %d -> %d", readsBeforeMessages, readsAfterMessages)
	}
	if err := atomID.Kill(p.local, ArgBaseAtomosWaitKilled()); err != nil {
		t.Fatalf("controlled Halt: %v", err)
	}
	lookup, err := p.local.CosmosLookupAtom(ForTestAtomosName, "basic")
	if err != nil || lookup.State != AtomLifecycleState_AtomLifecycleStateAbsent {
		t.Fatalf("lookup after halt = %v, %v", lookup, err)
	}
	op, err := p.local.CosmosGetAtomOperation(operationID)
	if err != nil || op.State != AtomOperationState_AtomOperationStateSucceeded || op.RetainUntilUnixNano <= time.Now().UnixNano() {
		t.Fatalf("retained operation = %v, %v", op, err)
	}
	p.lifecycle.close()
	select {
	case <-p.lifecycle.watchDone:
	default:
		t.Fatal("close returned before owner watch exited")
	}
	select {
	case <-p.lifecycle.stopped:
	default:
		t.Fatal("close returned before janitor exited")
	}
}

func TestLifecycleIntegrationTwoNodesTakeoverLeaseAndDrain(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	const cosmos = "lifecycleCluster"
	pa := newStrictLifecycleProcess(t, ee, cosmos, "nodeA")
	pb := newStrictLifecycleProcess(t, ee, cosmos, "nodeB")
	var constructors atomic.Int32
	oldHook := testAtomConstructorHook
	testAtomConstructorHook = func(name string) *testRunnableAtom {
		constructors.Add(1)
		return &testRunnableAtom{}
	}
	t.Cleanup(func() { testAtomConstructorHook = oldHook })

	processes := []*CosmosProcess{pa, pb}
	const callers = 12
	start := make(chan struct{})
	results := make(chan ID, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		p := processes[i%2]
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			id, _, err := p.local.CosmosSpawnAtom(p.local, ForTestAtomosName, "shared", &ForTestSpawnArg{Name: "shared"})
			if err != nil {
				errs <- err
				return
			}
			results <- id
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	var winner ID
	for id := range results {
		if winner != nil {
			t.Fatal("more than one concurrent Spawn succeeded")
		}
		winner = id
	}
	if winner == nil {
		for err := range errs {
			t.Logf("spawn rejection: %v", err)
		}
		t.Fatal("all concurrent Spawns failed")
	}
	if constructors.Load() != 1 {
		t.Fatalf("constructors = %d, want 1", constructors.Load())
	}
	owner := pa
	other := pb
	if winner.GetIDInfo().Node == pb.local.GetNodeName() {
		owner, other = pb, pa
	}
	waitForRemote(t, other, owner.local.GetNodeName(), 5*time.Second, remoteEnabled, "owner discovery")
	remoteID, tracker, err := other.local.CosmosGetAtomID(ForTestAtomosName, "shared")
	if err != nil {
		t.Fatalf("cross-node strict Get: %v", err)
	}
	if tracker != nil {
		defer tracker.Release()
	}
	t.Logf("winner=%v remote=%v", winner.GetIDInfo(), remoteID.GetIDInfo())
	readsBeforeRemoteMessage := owner.lifecycle.controlReads.Load()
	if _, err := remoteID.SyncMessagingByName(other.local, "Greeting", &ForTestGreetingI{Mode: 1}); err != nil {
		t.Fatalf("cross-node owned Sync: %v", err)
	}
	if readsAfterRemoteMessage := owner.lifecycle.controlReads.Load(); readsAfterRemoteMessage != readsBeforeRemoteMessage {
		t.Fatalf("remote owned Sync accessed etcd: reads %d -> %d", readsBeforeRemoteMessage, readsAfterRemoteMessage)
	}

	oldID := proto.Clone(winner.GetIDInfo()).(*IDInfo)
	newID, newTracker, err := other.local.CosmosTakeoverAtom(other.local, ForTestAtomosName, "shared", oldID, &ForTestSpawnArg{Name: "shared"})
	if err != nil {
		t.Fatalf("Takeover: %v", err)
	}
	if newTracker != nil {
		defer newTracker.Release()
	}
	if newID.GetIDInfo().Fence <= oldID.Fence || newID.GetIDInfo().Incarnation == oldID.Incarnation {
		t.Fatalf("invalid takeover identity old=%v new=%v", oldID, newID.GetIDInfo())
	}
	if oldID.InstanceId != newID.GetIDInfo().InstanceId {
		t.Fatalf("fixture must share instanceID across processes: old=%d new=%d", oldID.InstanceId, newID.GetIDInfo().InstanceId)
	}
	watchDeadline := time.Now().Add(5 * time.Second)
	for {
		cached := owner.lifecycle.cachedOwner(ForTestAtomosName, "shared")
		if cached == nil || !lifecycleSameIdentity(oldID, cached.Ownership) {
			break
		}
		if time.Now().After(watchDeadline) {
			t.Fatal("old owner snapshot did not observe cross-node Takeover")
		}
		time.Sleep(10 * time.Millisecond)
	}
	readsBeforeStaleMessage := owner.lifecycle.controlReads.Load()
	if _, err := winner.SyncMessagingByName(owner.local, "Greeting", &ForTestGreetingI{Mode: 1}); !errors.Is(err, ErrStaleFence) && !errors.Is(err, ErrOwnershipLost) {
		t.Fatalf("stale message after cross-node Takeover = %v", err)
	}
	if readsAfterStaleMessage := owner.lifecycle.controlReads.Load(); readsAfterStaleMessage != readsBeforeStaleMessage {
		t.Fatalf("stale message authorize accessed etcd: reads %d -> %d", readsBeforeStaleMessage, readsAfterStaleMessage)
	}
	if err := winner.Kill(owner.local, ArgBaseAtomosWaitKilled()); !errors.Is(err, ErrStaleFence) && !errors.Is(err, ErrOwnershipLost) {
		t.Fatalf("stale cross-process Kill = %v", err)
	}
	if _, err := newID.SyncMessagingByName(other.local, "Greeting", &ForTestGreetingI{Mode: 1}); err != nil {
		t.Fatalf("new incarnation Sync after stale Kill: %v", err)
	}

	waitForRemote(t, owner, other.local.GetNodeName(), 5*time.Second, remoteEnabled, "new owner discovery")
	remoteNewID, _, err := owner.local.CosmosGetAtomID(ForTestAtomosName, "shared")
	if err != nil {
		t.Fatalf("resolve new owner remotely: %v", err)
	}
	leaseID, _ := other.lifecycle.currentLease()
	other.lifecycle.publishLease(0, false)
	if _, err := remoteNewID.SyncMessagingByName(owner.local, "Greeting", &ForTestGreetingI{Mode: 1}); !errors.Is(err, ErrOwnershipLost) {
		t.Fatalf("lease-loss remote Sync = %v", err)
	}
	other.lifecycle.publishLease(leaseID+1, true)
	if _, err := remoteNewID.SyncMessagingByName(owner.local, "Greeting", &ForTestGreetingI{Mode: 1}); !errors.Is(err, ErrOwnershipLost) {
		t.Fatalf("replacement-lease remote Sync revived old ownership: %v", err)
	}

	legacyService := &atomosRemoteService{process: other}
	legacyID := proto.Clone(newID.GetIDInfo()).(*IDInfo)
	legacyID.Fence = 0
	legacyRsp, legacyErr := legacyService.syncMessagingByName(context.Background(), &CosmosRemoteSyncMessagingByNameReq{To: legacyID, CallerId: other.local.GetIDInfo()})
	if legacyErr != nil || !errors.Is(legacyRsp.Error, ErrFencingRequired) {
		t.Fatalf("strict legacy request = %v, %v", legacyRsp.Error, legacyErr)
	}

	other.lifecycle.publishLease(leaseID, true)
	drainHalt, _, err := other.local.CosmosSpawnAtom(other.local, ForTestAtomosName, "drain_halt", &ForTestSpawnArg{})
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Drain(5 * time.Second); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if _, _, err := other.local.CosmosSpawnAtom(other.local, ForTestAtomosName, "drain_rejected", &ForTestSpawnArg{}); !errors.Is(err, ErrCosmosNodeDraining) {
		t.Fatalf("controlled Spawn during Drain = %v", err)
	}
	if err := drainHalt.Kill(other.local, ArgBaseAtomosWaitKilled()); err != nil {
		t.Fatalf("controlled Halt during Drain = %v", err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for newID.State() != BaseAtomosHalt && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if newID.State() != BaseAtomosHalt {
		t.Fatalf("active atom did not halt during Drain: %s", newID.State())
	}
}

func TestLifecycleIntegrationTakeoverRejectsOldWriteback(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	const cosmos = "lifecycleWriteback"
	pa := newStrictLifecycleProcess(t, ee, cosmos, "nodeA")
	pb := newStrictLifecycleProcess(t, ee, cosmos, "nodeB")
	const operationID = "accepted-old-spawn"
	fingerprint := lifecycleFingerprint(AtomOperationKind_AtomOperationKindSpawn, ForTestAtomosName, "writeback", nil, nil)
	claim, err := pa.lifecycle.claimSpawn(context.Background(), operationID, fingerprint, AtomOperationKind_AtomOperationKindSpawn, ForTestAtomosName, "writeback", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	pa.lifecycle.deleteExpiredOperations(time.Now().Add(100 * time.Hour))
	if _, _, err := pa.lifecycle.loadOperation(operationID); err != nil {
		t.Fatalf("janitor deleted Accepted operation: %v", err)
	}
	oldID := claim.owner.Ownership.Id
	newID, _, takeoverErr := pb.local.CosmosTakeoverAtom(pb.local, ForTestAtomosName, "writeback", oldID, &ForTestSpawnArg{Name: "restored"})
	if takeoverErr != nil {
		t.Fatalf("Takeover accepted Spawn: %v", takeoverErr)
	}
	if _, finishErr := pa.lifecycle.finishSpawn(operationID, claim, nil); !errors.Is(finishErr, ErrOwnershipLost) {
		t.Fatalf("old owner writeback = %v", finishErr)
	}
	lookup, lookupErr := pb.local.CosmosLookupAtom(ForTestAtomosName, "writeback")
	if lookupErr != nil || !lifecycleSameIdentity(newID.GetIDInfo(), lookup.Ownership) {
		t.Fatalf("old writeback replaced new owner: lookup=%v err=%v", lookup, lookupErr)
	}
	oldOp, opErr := pa.local.CosmosGetAtomOperation(operationID)
	if opErr != nil || oldOp.State != AtomOperationState_AtomOperationStateFailed || !errors.Is(oldOp.Error, ErrOwnershipLost) {
		t.Fatalf("old operation was not superseded: %v, %v", oldOp, opErr)
	}
}

func TestLifecycleIntegrationConcurrentHaltJoinsResult(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	p := newStrictLifecycleProcess(t, ee, "lifecycleHaltJoin", "nodeA")
	haltNotify := make(chan struct{}, 1)
	oldHook := testAtomConstructorHook
	testAtomConstructorHook = func(name string) *testRunnableAtom {
		return &testRunnableAtom{haltNotify: haltNotify, haltWait: 400 * time.Millisecond}
	}
	t.Cleanup(func() { testAtomConstructorHook = oldHook })
	atomID, err := SpawnForTestAtomosAtom(p.local, p.local, "join_halt", &ForTestSpawnArg{})
	if err != nil {
		t.Fatal(err)
	}
	defer atomID.Release()
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- atomID.Kill(p.local, ArgBaseAtomosWaitKilled(), ArgBaseAtomosOperationID("halt-first"))
	}()
	select {
	case <-haltNotify:
	case <-time.After(3 * time.Second):
		t.Fatal("first Halt did not enter user handler")
	}
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- atomID.Kill(p.local, ArgBaseAtomosWaitKilled(), ArgBaseAtomosOperationID("halt-second"))
	}()
	for name, done := range map[string]<-chan error{"first": firstDone, "second": secondDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s Halt: %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s Halt did not join completion", name)
		}
	}
	for _, operationID := range []string{"halt-first", "halt-second"} {
		op, err := p.local.CosmosGetAtomOperation(operationID)
		if err != nil || op.State != AtomOperationState_AtomOperationStateSucceeded {
			t.Fatalf("joined operation %s = %v, %v", operationID, op, err)
		}
	}
}

func TestLifecycleIntegrationOwnerWatchReconnects(t *testing.T) {
	clientPort := reserveLocalPort(t)
	dir := t.TempDir()
	first := startEmbeddedEtcdAt(t, dir, clientPort)
	endpoint := fmt.Sprintf("127.0.0.1:%d", clientPort)
	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{endpoint}, DialTimeout: 5 * time.Second})
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	ee := &embeddedEtcd{server: first, client: cli, endpoint: endpoint}
	p := newStrictLifecycleProcess(t, ee, "lifecycleWatchReconnect", "nodeA")
	atomID, err := SpawnForTestAtomosAtom(p.local, p.local, "watch_reconnect", &ForTestSpawnArg{})
	if err != nil {
		t.Fatal(err)
	}
	defer atomID.Release()

	first.Close()
	deadline := time.Now().Add(8 * time.Second)
	for p.lifecycle.watchHealthy.Load() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	watchStayedHealthy := p.lifecycle.watchHealthy.Load()
	_, disconnectedMessageErr := atomID.Greeting(p.local, &ForTestGreetingI{Mode: 1})

	second := startEmbeddedEtcdAt(t, dir, clientPort)
	t.Cleanup(func() {
		_ = p.Stop()
		second.Close()
	})
	if watchStayedHealthy {
		t.Fatal("owner watch stayed healthy after etcd stopped")
	}
	if !errors.Is(disconnectedMessageErr, ErrOwnershipLost) {
		t.Fatalf("message with disconnected owner watch = %v", disconnectedMessageErr)
	}
	deadline = time.Now().Add(15 * time.Second)
	for !p.lifecycle.watchHealthy.Load() && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if !p.lifecycle.watchHealthy.Load() {
		t.Fatal("owner watch did not reconnect and resynchronize")
	}
	if owner := p.lifecycle.cachedOwner(ForTestAtomosName, "watch_reconnect"); owner == nil {
		t.Fatal("owner snapshot was not restored after reconnect")
	}
}
