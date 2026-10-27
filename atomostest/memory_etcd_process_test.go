package atomostest

import (
	"context"
	"testing"

	atomos "github.com/hwangtou/go-atomos"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestMemoryEtcdStrictProcessSharedClient(t *testing.T) {
	clock := NewVirtualClock()
	client := NewMemoryEtcd(clock)
	t.Cleanup(func() { _ = client.Close() })

	newRunnable := func(node string) *atomos.CosmosRunnable {
		return clusterRunnable(node).
			SetConfig(&atomos.Config{
				Cosmos:   "memoryEtcdStrict",
				Node:     node,
				LogLevel: atomos.LogLevel_Debug,
				EnableCluster: &atomos.CosmosClusterConfig{
					Enable:        true,
					EtcdEndpoints: []string{"unused:0"},
					OptionalPorts: []int32{int32(reservePort())},
				},
				Customize: map[string][]byte{"_cluster_advertise_ip": []byte("127.0.0.1")},
			}).
			SetClock(clock).
			SetEtcdClient(client).
			SetLifecycleMode(atomos.LifecycleModePerAtomStrict)
	}

	process := Run(t, newRunnable("nodeA"))
	atomID, tracker, err := process.Self().CosmosSpawnAtom(process.Self(), clusterElem, "shared", nil)
	if err != nil {
		t.Fatalf("strict Spawn with MemoryEtcd: %v", err)
	}
	if atomID.GetIDInfo().Fence == 0 {
		t.Fatal("strict Spawn returned an unfenced ID")
	}
	tracker.Release()
	ownerKey := "/cosmos/memoryEtcdStrict/lifecycle/atom/clusterElem/shared/owner"
	if rsp, err := client.Get(context.Background(), ownerKey); err != nil || len(rsp.Kvs) != 1 {
		t.Fatalf("strict owner record: kvs=(%d), err=(%v)", len(rsp.Kvs), err)
	}
	if err := process.Stop(); err != nil {
		t.Fatalf("Stop with injected MemoryEtcd: %v", err)
	}

	rsp, err := client.Get(context.Background(), "/cosmos/memoryEtcdStrict/node/nodeA/", clientv3.WithPrefix())
	if err != nil {
		t.Fatalf("injected MemoryEtcd was closed by Stop: %v", err)
	}
	if len(rsp.Kvs) != 1 || string(rsp.Kvs[0].Key) != "/cosmos/memoryEtcdStrict/node/nodeA/version/" {
		t.Fatalf("Stop left unexpected node keys: %v", rsp.Kvs)
	}

	second := Run(t, newRunnable("nodeB"))
	if !second.EtcdClusterHealthy() {
		t.Fatal("second process could not reuse the injected MemoryEtcd")
	}
	if err := second.Stop(); err != nil {
		t.Fatalf("second Stop with shared MemoryEtcd: %v", err)
	}
	if _, err := client.Get(context.Background(), ownerKey); err != nil {
		t.Fatalf("shared MemoryEtcd was closed by second Stop: %v", err)
	}
}
