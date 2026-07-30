package atomos

import (
	"testing"
)

// TestIDRouting_CurrentSwitchBreaksExistingCalls 证明现状的核心缺陷：
// 当 CosmosRemote.current 被切换（模拟 drain/版本切换）后，
// 已持有旧版本 atom ID 的调用方，其调用会被错误地路由到新版本，
// 导致 ErrAtomNotExists —— 存量对局断裂。
//
// 改造目标（见 docs/DESIGN_DRAIN_GRADUAL.md 阶段 1-2）：
// AtomRemote 应记住创建时的逻辑定位，current 切换后仍能路由到正确的旧版本。
// 改造后本测试应通过（调用成功而非 ErrAtomNotExists）。
func TestIDRouting_CurrentSwitchBreaksExistingCalls(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 50200, "route_cosmos", "route_node")
	defer cluster.close()

	targetNodeName := "route_node_target"

	// 1. source 通过 ElementRemote 在 target 上 spawn 一个 atom，拿到 AtomRemote ID
	//    注意：helper 构造的 ElementRemote 用的是节点级 info（Element 为空），
	//    远端 SpawnAtom 需要 element 名，所以这里重新构造一个带 element info 的 ElementRemote。
	targetRemote := cluster.sourceProcess.cluster.remoteCosmos[targetNodeName]
	elemInfo := &IDInfo{
		Type:    IDType_Element,
		Cosmos:  "route_cosmos",
		Node:    targetNodeName,
		Element: ForTestAtomosName,
	}
	sourceElemRemote := newElementRemoteFromSource(
		targetRemote,
		elemInfo,
		cluster.sourceProcess.local.runnable.implements[ForTestAtomosName].Interface,
		"v1",
	)
	atomID, _, err := sourceElemRemote.SpawnAtom(
		cluster.sourceProcess.local,
		"route_test_atom",
		nil,
		nil,
		true,
	)
	if err != nil {
		t.Fatalf("SpawnAtom failed: %v", err)
	}
	if atomID == nil {
		t.Fatal("SpawnAtom returned nil ID")
	}
	t.Logf("Spawned atom on target, id=%s", atomID.GetIDInfo().Info())

	// 2. 验证：当前 current 指向 target，调用成功
	_, err = atomID.SyncMessagingByName(cluster.sourceProcess.local, "Greeting", &ForTestGreetingI{Mode: 1}, nil)
	if err != nil {
		t.Fatalf("Call BEFORE current switch should succeed, got err=%v", err)
	}
	t.Logf("Call before current switch succeeded.")

	// 3. 模拟 current 切换：把 source→target 的 CosmosRemote.current 指向一个"新版本"
	//    新版本连的是 source 自己（模拟新版本起来，current 切走），target 上的 atom 不在 source 上
	targetRemote.mutex.Lock()
	// 保存旧 current（指向 target 的连接）
	oldCurrent := targetRemote.current
	// 构造一个"新版本"current：连到 source 自己的端口（target 上没有 route_test_atom）
	sourceConn := cluster.targetProcess.cluster.remoteCosmos["route_node_source"].current.client
	targetRemote.current = &cosmosRemoteVersion{
		process: cluster.sourceProcess,
		info:    nil,
		avail:   true,
		client:  sourceConn, // 连到 source，不是 target
		version: "new_version",
	}
	targetRemote.mutex.Unlock()
	t.Logf("Switched current to a 'new version' (points to source, not target).")

	// 恢复用，避免泄漏
	defer func() {
		targetRemote.mutex.Lock()
		targetRemote.current = oldCurrent
		targetRemote.mutex.Unlock()
	}()

	// 4. 用同一个 atomID 再调用 —— 这就是核心断言
	_, err = atomID.SyncMessagingByName(cluster.sourceProcess.local, "Greeting", &ForTestGreetingI{Mode: 1}, nil)

	// === 核心断言（阶段1-2改造后：正向） ===
	// 改造前（已删除）：err != nil，current 切换导致请求到错误节点（存量断裂）
	// 改造后：err == nil，AtomRemote pin 了创建时的连接，current 切换不影响存量调用
	if err != nil {
		t.Fatalf("[回归] Call AFTER current switch should succeed (pinned conn fix), got err=%v", err)
	}
	t.Logf("[修复验证] Call after current switch SUCCEEDED — AtomRemote correctly routed to the old version despite current change.")
	t.Logf("This proves: pinned connection survives current switch (存量不断裂).")
}
