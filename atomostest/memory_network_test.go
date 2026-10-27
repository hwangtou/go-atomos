package atomostest

import (
	"testing"
	"time"

	atomos "github.com/hwangtou/go-atomos"
)

type memoryClusterFixture struct {
	clock      *VirtualClock
	client     *MemoryEtcd
	network    *MemoryNetwork
	remote     *atomos.CosmosProcess
	home       *atomos.CosmosProcess
	homeName   string
	remoteName string
}

func newMemoryClusterFixture(t *testing.T, cosmos string) *memoryClusterFixture {
	t.Helper()
	clock := NewVirtualClock()
	client := NewMemoryEtcd(clock)
	network := NewMemoryNetwork(clock)
	t.Cleanup(func() { _ = network.Close() })
	t.Cleanup(func() { _ = client.Close() })
	nodes := []string{"n1", "n2"}
	processes := make([]*atomos.CosmosProcess, 0, len(nodes))
	for _, node := range nodes {
		runnable := clusterRunnable(node).
			SetRouter(elementRouter{nodes: nodes}).
			SetClock(clock).
			SetEtcdClient(client).
			SetGRPCTransport(network.Transport(node)).
			SetLifecycleMode(atomos.LifecycleModePerAtomStrict).
			SetConfig(&atomos.Config{
				Cosmos:   cosmos,
				Node:     node,
				LogLevel: atomos.LogLevel_Debug,
				EnableCluster: &atomos.CosmosClusterConfig{
					Enable:        true,
					EtcdEndpoints: []string{"unused:0"},
					OptionalPorts: []int32{0},
					AllowInsecure: true,
				},
				Customize: map[string][]byte{"_cluster_advertise_ip": []byte("127.0.0.1")},
			})
		processes = append(processes, Run(t, runnable))
	}
	homeIndex := int(fnv32(clusterElem) % 2)
	fixture := &memoryClusterFixture{
		clock: clock, client: client, network: network,
		home: processes[homeIndex], remote: processes[1-homeIndex],
		homeName: nodes[homeIndex], remoteName: nodes[1-homeIndex],
	}
	AwaitElementOnNode(t, fixture.remote, clusterElem, fixture.homeName, 5*time.Second)
	return fixture
}

func (f *memoryClusterFixture) spawn(t *testing.T, atom, operationID string) (atomos.ID, *atomos.IDTracker, error) {
	t.Helper()
	return f.remote.Self().CosmosSpawnAtom(f.remote.Self(), clusterElem, atom, nil, atomos.ArgBaseAtomosOperationID(operationID))
}

func TestDST_MemoryNetworkSpawnResponseLossReplay(t *testing.T) {
	fixture := newMemoryClusterFixture(t, "memorySpawnResponseLoss")
	const operationID = "memory-spawn-response-loss"
	controller := fixture.network.Fault(fixture.remoteName, fixture.homeName)
	controller.AddRule(FaultRule{
		Method:      atomos.AtomosRemoteService_SpawnAtomControlled_FullMethodName,
		OperationID: operationID, Action: FaultDropResponse, Once: true,
	})
	id, tracker, err := fixture.spawn(t, "shared", operationID)
	if err == nil {
		if tracker != nil {
			tracker.Release()
		}
		t.Fatalf("Spawn with dropped response unexpectedly succeeded: %v", id)
	}
	replayed, replayTracker, err := fixture.spawn(t, "shared", operationID)
	if err != nil {
		t.Fatalf("same-operation Spawn replay: %v", err)
	}
	defer replayTracker.Release()
	if replayed.GetIDInfo().Node != fixture.homeName {
		t.Fatalf("replayed owner = %q, want %q", replayed.GetIDInfo().Node, fixture.homeName)
	}
	out, err := replayed.SyncMessagingByName(fixture.remote.Self(), "Ping", &atomos.Config{}, atomos.ArgBaseAtomosTimeout(5*time.Second))
	if err != nil || out.(*atomos.Config).Cosmos != "pong" {
		t.Fatalf("Ping after response-loss recovery = (%v, %v)", out, err)
	}
}

func TestDST_MemoryNetworkDroppedRequestDoesNotExecute(t *testing.T) {
	fixture := newMemoryClusterFixture(t, "memorySpawnRequestLoss")
	const operationID = "memory-spawn-request-loss"
	controller := fixture.network.Fault(fixture.remoteName, fixture.homeName)
	controller.AddRule(FaultRule{
		Method:      atomos.AtomosRemoteService_SpawnAtomControlled_FullMethodName,
		OperationID: operationID, Action: FaultDropRequest, Once: true,
	})
	_, tracker, err := fixture.spawn(t, "absent", operationID)
	if err == nil {
		if tracker != nil {
			tracker.Release()
		}
		t.Fatal("Spawn with dropped request unexpectedly succeeded")
	}
	lookup, err := fixture.remote.Self().CosmosLookupAtom(clusterElem, "absent")
	if err != nil || lookup.State != atomos.AtomLifecycleState_AtomLifecycleStateAbsent {
		t.Fatalf("Lookup after dropped request = (%v, %v)", lookup, err)
	}
	id, replayTracker, err := fixture.spawn(t, "absent", operationID)
	if err != nil {
		t.Fatalf("Spawn after request loss: %v", err)
	}
	replayTracker.Release()
	if id.GetIDInfo().Node != fixture.homeName {
		t.Fatalf("replacement owner = %q", id.GetIDInfo().Node)
	}
}

func TestDST_MemoryNetworkDuplicateHaltExecutesOnce(t *testing.T) {
	fixture := newMemoryClusterFixture(t, "memoryHaltDuplicate")
	id, tracker, err := fixture.spawn(t, "halted", "memory-halt-spawn")
	if err != nil {
		t.Fatal(err)
	}
	defer tracker.Release()
	const operationID = "memory-halt-duplicate"
	controller := fixture.network.Fault(fixture.remoteName, fixture.homeName)
	controller.AddRule(FaultRule{
		Method:      atomos.AtomosRemoteService_HaltAtomControlled_FullMethodName,
		OperationID: operationID, Action: FaultDuplicateRequest, Once: true,
	})
	if err := id.Kill(fixture.remote.Self(), atomos.ArgBaseAtomosWaitKilled(), atomos.ArgBaseAtomosOperationID(operationID)); err != nil {
		t.Fatalf("duplicated controlled Halt: %v", err)
	}
	if err := id.Kill(fixture.remote.Self(), atomos.ArgBaseAtomosWaitKilled(), atomos.ArgBaseAtomosOperationID(operationID)); err != nil {
		t.Fatalf("persisted Halt replay: %v", err)
	}
}

func TestDST_MemoryNetworkVirtualDelay(t *testing.T) {
	fixture := newMemoryClusterFixture(t, "memoryVirtualDelay")
	id, tracker, err := fixture.spawn(t, "delayed", "memory-delay-spawn")
	if err != nil {
		t.Fatal(err)
	}
	defer tracker.Release()
	controller := fixture.network.Fault(fixture.remoteName, fixture.homeName)
	controller.AddRule(FaultRule{
		Method: atomos.AtomosRemoteService_SyncMessagingByName_FullMethodName,
		Action: FaultDelay, Duration: time.Second, Once: true,
	})
	type result struct {
		out any
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		out, err := id.SyncMessagingByName(fixture.remote.Self(), "Ping", &atomos.Config{}, atomos.ArgBaseAtomosTimeout(5*time.Second))
		resultCh <- result{out: out, err: err}
	}()
	waitFaultEvents(t, controller, 1)
	select {
	case result := <-resultCh:
		t.Fatalf("delayed RPC completed before Advance: %+v", result)
	default:
	}
	fixture.clock.Advance(time.Second - time.Millisecond)
	select {
	case result := <-resultCh:
		t.Fatalf("delayed RPC completed before exact deadline: %+v", result)
	default:
	}
	fixture.clock.Advance(time.Millisecond)
	select {
	case result := <-resultCh:
		if result.err != nil || result.out.(*atomos.Config).Cosmos != "pong" {
			t.Fatalf("delayed RPC = (%v, %v)", result.out, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("delayed RPC did not complete at virtual deadline")
	}
}

func TestDST_MemoryNetworkBlackholeHeal(t *testing.T) {
	fixture := newMemoryClusterFixture(t, "memoryBlackhole")
	controller := fixture.network.Fault(fixture.remoteName, fixture.homeName)
	controller.AddRule(FaultRule{
		Method: atomos.AtomosRemoteService_SpawnAtomControlled_FullMethodName,
		Action: FaultBlackhole, Once: true,
	})
	_, tracker, err := fixture.spawn(t, "partitioned", "memory-partition-spawn")
	if err == nil {
		if tracker != nil {
			tracker.Release()
		}
		t.Fatal("blackholed Spawn unexpectedly succeeded")
	}
	controller.Heal()
	id, replayTracker, err := fixture.spawn(t, "partitioned", "memory-partition-spawn")
	if err != nil {
		t.Fatalf("Spawn after Heal: %v", err)
	}
	replayTracker.Release()
	if id.GetIDInfo().Node != fixture.homeName {
		t.Fatalf("owner after Heal = %q", id.GetIDInfo().Node)
	}
}
