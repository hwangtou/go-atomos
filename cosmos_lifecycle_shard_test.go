package atomos

import "testing"

func TestLifecycleShardConfigAndStableMapping(t *testing.T) {
	id := &IDInfo{Type: IDType_Cosmos, Cosmos: "test_cosmos", Node: "test_node"}
	runnable := newTestCosmosRunnable(id).SetLifecycleShardCount(0)
	if err := runnable.Check(); CodeOf(err) != ErrCodeCosmosConfigInvalid {
		t.Fatalf("zero shard count error = %v", err)
	}
	p, err := newCosmosProcess(id.Cosmos, id.Node, &reproLogger{})
	if err != nil {
		t.Fatal(err)
	}
	manager := &lifecycleManager{process: p}
	first := manager.shardID(ForTestAtomosName, "atom-a")
	second := manager.shardID(ForTestAtomosName, "atom-a")
	if first != second || first >= uint32(defaultLifecycleShardCount) {
		t.Fatalf("unstable shard mapping: %d, %d", first, second)
	}
	p.lifecycleShardCount = 1
	if got := manager.shardID(ForTestAtomosName, "atom-a"); got != 0 {
		t.Fatalf("single shard mapping = %d", got)
	}
	runnable = newTestCosmosRunnable(id).SetLifecycleMode(LifecycleModePerShardStrict)
	if err := p.Start(runnable); CodeOf(err) != ErrCodeFencingRequired {
		t.Fatalf("shard strict without cluster = %v", err)
	}
	if p.lifecycleShardCount != defaultLifecycleShardCount {
		t.Fatalf("default shard count = %d", p.lifecycleShardCount)
	}
}
