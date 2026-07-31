//go:build integration

package atomos

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// TestEtcdDrain_StateBroadcastAndWatch 验证 drain 的 etcd 广播链路：
//
//  1. 节点 A 写入自己的版本信息到 etcd（Started）
//  2. 节点 B 的 watch 感知到 A，建 remote，refresh 后 enable=true
//  3. A 的状态变为 Draining，写入 etcd
//  4. B 的 watch 感知到状态变化，refresh 后路由绕开 Draining（若有 Started 版本则选它）
//
// 这验证了真实 etcd watch 的端到端行为（非 mock）。
func TestEtcdDrain_StateBroadcastAndWatch(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	cli := ee.client

	cosmosName := "etcd_drain_cosmos"
	nodeName := "etcd_drain_node"

	// 模拟节点 A 写入版本信息：key = /cosmos/{c}/node/{n}/version/{v}, value = Started
	keyPrefix := etcdCosmosNodeWatchAllURI(cosmosName) // /cosmos/{c}/node/
	versionKey := etcdCosmosNodeVersionURI(cosmosName, nodeName, 1000)

	// Step 1: 节点 A 注册为 Started
	infoStarted := &CosmosNodeVersionInfo{
		Node:    nodeName,
		Address: "1.1.1.1:1234",
		Id: &IDInfo{
			Type:   IDType_Cosmos,
			Cosmos: cosmosName,
			Node:   nodeName,
		},
		State:    ClusterNodeState_Started,
		Elements: map[string]*IDInfo{},
		StartupId: 111,
	}
	writeNodeInfo(t, cli, versionKey, infoStarted)

	// 同时写 lock key，使 refresh() 能通过 lock.Current 找到这个版本并 enable。
	// lock key = /cosmos/{c}/node/{n}/version/ （无 version 后缀），value = CosmosNodeVersionLock
	lockKey := etcdCosmosNodeLockURI(cosmosName, nodeName)
	writeLockInfo(t, cli, lockKey, &CosmosNodeVersionLock{
		Current:  1000,
		Versions: []int64{1000},
	})
	t.Logf("Step 1: node A registered as Started (version + lock)")

	// Step 2: 节点 B 初始化 watch，感知 A
	// 用 newTestCosmosProcessAsClusterNode（带 runnable，watchCluster 依赖 runnable.config.Cosmos）
	p := newTestCosmosProcessAsClusterNode(t, cosmosName, "etcd_drain_self")
	if err := p.watchCluster(cli); err != nil {
		t.Fatalf("watchCluster failed: %v", err)
	}
	t.Cleanup(func() {
		if p.cluster.etcdCancelWatch != nil {
			p.cluster.etcdCancelWatch()
		}
	})

	// 等待 watch 事件传播
	remote := waitRemoteReady(t, p, nodeName, 3*time.Second)
	if remote == nil {
		t.Fatal("Step 2: node B did not discover node A via watch")
	}
	remote.mutex.RLock()
	enabled := remote.enable
	remote.mutex.RUnlock()
	if !enabled {
		t.Fatal("Step 2: remote should be enabled after discovering Started node")
	}
	t.Logf("Step 2: node B discovered A via etcd watch, remote enabled")

	// Step 3: 节点 A 变为 Draining，写入 etcd
	infoDraining := &CosmosNodeVersionInfo{
		Node:    nodeName,
		Address: "1.1.1.1:1234",
		Id: &IDInfo{
			Type:   IDType_Cosmos,
			Cosmos: cosmosName,
			Node:   nodeName,
		},
		State:    ClusterNodeState_Draining,
		Elements: map[string]*IDInfo{},
		StartupId: 111, // 同 startupId = 同代状态刷新
	}
	writeNodeInfo(t, cli, versionKey, infoDraining)
	t.Logf("Step 3: node A state changed to Draining")

	// Step 4: 等待 watch 传播 Draining 状态，验证 refresh 结果
	// Draining 后且没有其他 Started 版本 → enable 应保留（优雅降级不硬失败）
	time.Sleep(500 * time.Millisecond) // 等 watch 传播
	remote.mutex.RLock()
	drainingState := remote.current.info.GetState()
	remote.mutex.RUnlock()
	if drainingState != ClusterNodeState_Draining {
		t.Fatalf("Step 4: remote should reflect Draining state, got=%v", drainingState)
	}
	t.Logf("Step 4: node B observed Draining state via watch. state=%v", drainingState)
	t.Logf("[验证] etcd drain 状态广播 + watch 感知链路正常工作")

	_ = keyPrefix // keep for reference
}

// writeNodeInfo marshals and puts a CosmosNodeVersionInfo to etcd.
// writeNodeInfo JSON-marshals and puts a CosmosNodeVersionInfo to etcd.
// Uses JSON (not proto) to match cosmos_etcd.go's etcdNodeVersion serialization.
func writeNodeInfo(t *testing.T, cli *clientv3.Client, key string, info *CosmosNodeVersionInfo) {
	t.Helper()
	buf, er := json.Marshal(info)
	if er != nil {
		t.Fatalf("writeNodeInfo marshal: %v", er)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, er = cli.Put(ctx, key, string(buf))
	if er != nil {
		t.Fatalf("writeNodeInfo put: %v", er)
	}
}

// writeLockInfo JSON-marshals and puts a CosmosNodeVersionLock to etcd.
func writeLockInfo(t *testing.T, cli *clientv3.Client, key string, lock *CosmosNodeVersionLock) {
	t.Helper()
	buf, er := json.Marshal(lock)
	if er != nil {
		t.Fatalf("writeLockInfo marshal: %v", er)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, er = cli.Put(ctx, key, string(buf))
	if er != nil {
		t.Fatalf("writeLockInfo put: %v", er)
	}
}

// waitRemoteReady polls until the remote Cosmos for nodeName exists in the
// process's remoteCosmos map (i.e. watch has processed the PUT event).
func waitRemoteReady(t *testing.T, p *CosmosProcess, nodeName string, timeout time.Duration) *CosmosRemote {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.cluster.remoteMutex.RLock()
		remote, has := p.cluster.remoteCosmos[nodeName]
		p.cluster.remoteMutex.RUnlock()
		if has && remote != nil {
			return remote
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}
