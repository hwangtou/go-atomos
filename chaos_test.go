package atomos

import (
	"testing"
	"time"
)

// TestRemoteCall_ServerStopReturnsBoundedError verifies that stopping the
// target gRPC server makes the next call fail within its deadline rather than
// panic or block indefinitely.
func TestRemoteCall_ServerStopReturnsBoundedError(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 50600, "chaos_cosmos", "chaos_node")
	defer cluster.close()

	targetNodeName := "chaos_node_target"
	targetRemote := cluster.sourceProcess.cluster.remoteCosmos[targetNodeName]

	// 1. spawn atom on target
	elemInfo := &IDInfo{
		Type:    IDType_Element,
		Cosmos:  "chaos_cosmos",
		Node:    targetNodeName,
		Element: ForTestAtomosName,
	}
	sourceElemRemote := newElementRemoteFromSource(
		targetRemote,
		elemInfo,
		cluster.sourceProcess.local.runnable.implements[ForTestAtomosName].Interface,
		"v1",
	)
	atomID, _, err := sourceElemRemote.SpawnAtom(cluster.sourceProcess.local, "chaos_atom", nil, nil, ArgBaseAtomosFromLocalOnly())
	if err != nil {
		t.Fatalf("SpawnAtom failed: %v", err)
	}

	// 确认调用成功
	_, err = atomID.SyncMessagingByName(cluster.sourceProcess.local, "Greeting", &ForTestGreetingI{Mode: 1})
	if err != nil {
		t.Fatalf("Call before kill should succeed: %v", err)
	}
	t.Logf("Step 1: spawned atom, call succeeded.")

	// 2. 强杀 target 的 gRPC server（模拟进程崩溃）
	cluster.targetServer.Stop()
	t.Logf("Step 2: killed target gRPC server (simulated crash).")

	// 3. 存量调用 —— 连接已断，应返回错误而非 panic/死锁
	//    用短超时避免测试卡住
	done := make(chan error, 1)
	go func() {
		// 给一点时间让 gRPC 感知连接断开
		time.Sleep(100 * time.Millisecond)
		_, e := atomID.SyncMessagingByName(cluster.sourceProcess.local, "Greeting", &ForTestGreetingI{Mode: 1})
		done <- e
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("call after server stop succeeded, want a bounded connection error")
		}
		t.Logf("call after server stop returned a bounded error: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("Step 3: call after kill hung (deadlock!) — should have returned error.")
	}

	// 4. 验证 pinned 连接已被 validPinnedConn 检测并清除（降级触发）
	atomRemote := atomID.(*atomRemoteInSourceProcess).AtomRemote
	pinned := atomRemote.remote.getPinnedConn()
	if pinned != nil {
		// pinned 可能还没被清除（取决于 gRPC 是否已标记 Shutdown），这不是硬性断言
		t.Logf("Step 4: pinned conn still set (gRPC may not have marked Shutdown yet) — not a failure.")
	} else {
		t.Logf("Step 4: pinned conn cleared by validPinnedConn (failover triggered).")
	}
}

// TestAtomSpawnAdmission_InternalDrainTransition exercises the internal
// admission flags directly. It is not a public CancelDrain API test.
func TestAtomSpawnAdmission_InternalDrainTransition(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 50700, "recover_cosmos", "recover_node")
	defer cluster.close()

	targetNodeName := "recover_node_target"
	targetRemote := cluster.sourceProcess.cluster.remoteCosmos[targetNodeName]

	elemInfo := &IDInfo{
		Type:    IDType_Element,
		Cosmos:  "recover_cosmos",
		Node:    targetNodeName,
		Element: ForTestAtomosName,
	}
	sourceElemRemote := newElementRemoteFromSource(
		targetRemote,
		elemInfo,
		cluster.sourceProcess.local.runnable.implements[ForTestAtomosName].Interface,
		"v1",
	)

	// 1. drain 前 spawn 成功
	_, _, err := sourceElemRemote.SpawnAtom(cluster.sourceProcess.local, "recover_atom_1", nil, nil, ArgBaseAtomosFromLocalOnly())
	if err != nil {
		t.Fatalf("Spawn before drain should succeed: %v", err)
	}
	t.Logf("Step 1: spawn succeeded before drain.")

	cluster.targetProcess.mutex.Lock()
	cluster.targetProcess.draining = true
	cluster.targetProcess.atomSpawnAccepting = false
	cluster.targetProcess.mutex.Unlock()

	_, _, err = sourceElemRemote.SpawnAtom(cluster.sourceProcess.local, "recover_atom_2", nil, nil, ArgBaseAtomosFromLocalOnly())
	if CodeOf(err) != ErrCodeCosmosNodeDraining {
		t.Fatalf("spawn during internal drain state error = %v, want ErrCosmosNodeDraining", err)
	}

	cluster.targetProcess.mutex.Lock()
	cluster.targetProcess.draining = false
	cluster.targetProcess.atomSpawnAccepting = true
	cluster.targetProcess.mutex.Unlock()

	_, _, err = sourceElemRemote.SpawnAtom(cluster.sourceProcess.local, "recover_atom_3", nil, nil, ArgBaseAtomosFromLocalOnly())
	if err != nil {
		t.Fatalf("Spawn after drain cancel should succeed: %v", err)
	}
	t.Logf("[验证] spawn recovered after drain cancelled.")
}
