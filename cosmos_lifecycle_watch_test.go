package atomos

import (
	"context"
	"errors"
	"testing"

	"github.com/hwangtou/go-atomos/internal/memoryetcd"
	"github.com/hwangtou/go-atomos/internal/testclock"
)

func TestLifecycleOwnerSnapshotRejectsOlderEvents(t *testing.T) {
	manager := &lifecycleManager{owners: map[string]lifecycleOwnerSnapshot{}}
	const key = "owner-key"
	oldOwner := &AtomOwnershipRecord{Ownership: &AtomOwnershipInfo{Fence: 1}}
	newOwner := &AtomOwnershipRecord{Ownership: &AtomOwnershipInfo{Fence: 2}}
	manager.cacheOwnerPut(key, newOwner, 20)
	manager.cacheOwnerPut(key, oldOwner, 19)
	if got := manager.owners[key]; got.record.Ownership.Fence != 2 || got.modRevision != 20 {
		t.Fatalf("older PUT replaced snapshot: %+v", got)
	}
	manager.cacheOwnerDelete(key, 21)
	manager.cacheOwnerPut(key, newOwner, 20)
	if got := manager.owners[key]; got.record != nil || got.modRevision != 21 {
		t.Fatalf("older PUT resurrected deleted owner: %+v", got)
	}
}

func TestLifecycleStorageErrorsDistinguishCorruptionFromUnavailable(t *testing.T) {
	client := memoryetcd.NewMemoryEtcd(testclock.NewVirtualClock())
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.Put(context.Background(), "owner", string([]byte{0xff}), 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lifecycleGet(client, "owner", &AtomOwnershipRecord{}); CodeOf(err) != ErrCodeStorageDataCorrupted {
		t.Fatalf("corrupt record error = %v, want StorageDataCorrupted", err)
	}
	client.InjectError(memoryetcd.MemoryEtcdGet, errors.New("get failed"))
	if _, _, err := lifecycleGet(client, "owner", &AtomOwnershipRecord{}); CodeOf(err) != ErrCodeStorageUnavailable {
		t.Fatalf("Get error = %v, want StorageUnavailable", err)
	}
}

func TestLifecycleOwnerSnapshotRejectsCorruptedRecords(t *testing.T) {
	for _, test := range []struct {
		name  string
		owner func(*lifecycleManager) string
	}{
		{name: "atom", owner: func(m *lifecycleManager) string { return m.ownerKey("element", "atom") }},
		{name: "shard", owner: func(m *lifecycleManager) string { return m.shardOwnerKey(1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := memoryetcd.NewMemoryEtcd(testclock.NewVirtualClock())
			t.Cleanup(func() { _ = client.Close() })
			process := &CosmosProcess{local: &CosmosLocal{runnable: &CosmosRunnable{config: &Config{Cosmos: "watch"}}}}
			manager := &lifecycleManager{
				process: process, cli: client,
				owners: map[string]lifecycleOwnerSnapshot{}, shards: map[string]lifecycleShardSnapshot{},
			}
			if _, err := client.Put(context.Background(), test.owner(manager), string([]byte{0xff}), 0); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.syncOwnerSnapshot(context.Background()); CodeOf(err) != ErrCodeStorageDataCorrupted {
				t.Fatalf("snapshot error = %v, want StorageDataCorrupted", err)
			}
		})
	}
}
