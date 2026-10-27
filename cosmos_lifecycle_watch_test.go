package atomos

import "testing"

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
