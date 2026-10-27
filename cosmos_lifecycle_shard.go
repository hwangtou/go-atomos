package atomos

import (
	"context"
	"hash/fnv"
	"strconv"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/proto"
)

type lifecycleShardSnapshot struct {
	record      *ShardOwnershipRecord
	modRevision int64
}

func (m *lifecycleManager) perShard() bool {
	return m != nil && m.process.lifecycleMode == LifecycleModePerShardStrict
}

func (m *lifecycleManager) shardID(element, atom string) uint32 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(element))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(atom))
	return uint32(h.Sum64() % uint64(m.process.lifecycleShardCount))
}

func (m *lifecycleManager) shardPrefix(shardID uint32) string {
	return "/cosmos/" + lifecycleEscape(m.process.local.runnable.config.Cosmos) + "/lifecycle/shard/" + strconv.FormatUint(uint64(shardID), 10)
}
func (m *lifecycleManager) shardRecordKey(shardID uint32) string {
	return m.shardPrefix(shardID) + "/record"
}
func (m *lifecycleManager) shardOwnerKey(shardID uint32) string {
	return m.shardPrefix(shardID) + "/owner"
}

func (m *lifecycleManager) loadShardRecord(shardID uint32) (*ShardLifecycleRecord, int64, *Error) {
	record := &ShardLifecycleRecord{}
	mod, ok, err := lifecycleGet(m.process.cluster.etcdClient, m.shardRecordKey(shardID), record)
	if err != nil || !ok {
		return nil, mod, err
	}
	return record, mod, nil
}

func (m *lifecycleManager) loadShardOwner(shardID uint32) (*ShardOwnershipRecord, int64, *Error) {
	m.controlReads.Add(1)
	owner := &ShardOwnershipRecord{}
	mod, ok, err := lifecycleGet(m.process.cluster.etcdClient, m.shardOwnerKey(shardID), owner)
	if err != nil || !ok {
		return nil, mod, err
	}
	return owner, mod, nil
}

func (m *lifecycleManager) cacheShardPut(key string, record *ShardOwnershipRecord, modRevision int64) {
	if m == nil || record == nil || modRevision <= 0 {
		return
	}
	m.shardMu.Lock()
	defer m.shardMu.Unlock()
	if current, ok := m.shards[key]; ok && current.modRevision >= modRevision {
		return
	}
	m.shards[key] = lifecycleShardSnapshot{record: proto.Clone(record).(*ShardOwnershipRecord), modRevision: modRevision}
}

func (m *lifecycleManager) cacheShardDelete(key string, modRevision int64) {
	if m == nil || modRevision <= 0 {
		return
	}
	m.shardMu.Lock()
	defer m.shardMu.Unlock()
	if current, ok := m.shards[key]; ok && current.modRevision >= modRevision {
		return
	}
	m.shards[key] = lifecycleShardSnapshot{modRevision: modRevision}
}

func (m *lifecycleManager) cachedShard(shardID uint32) *ShardOwnershipRecord {
	m.shardMu.RLock()
	snapshot := m.shards[m.shardOwnerKey(shardID)]
	m.shardMu.RUnlock()
	return snapshot.record
}

func (m *lifecycleManager) ensureShardOwnership(ctx context.Context, element, atom string, takeover bool) (*ShardOwnershipRecord, *Error) {
	if !m.perShard() {
		return nil, nil
	}
	leaseID, healthy := m.currentLease()
	if !healthy {
		return nil, NewError(ErrCodeOwnershipLost, "lifecycle: cannot claim shard without a healthy process lease")
	}
	shardID := m.shardID(element, atom)
	for attempt := 0; attempt < 32; attempt++ {
		record, recordMod, err := m.loadShardRecord(shardID)
		if err != nil {
			return nil, err
		}
		owner, ownerMod, err := m.loadShardOwner(shardID)
		if err != nil {
			return nil, err
		}
		if owner != nil && owner.Node == m.process.local.GetNodeName() && owner.StartupId == m.process.startupID && owner.LeaseId == int64(leaseID) {
			return owner, nil
		}
		if owner != nil && !takeover {
			return nil, NewErrorf(ErrCodeTakeoverRequired, "lifecycle: shard %d is owned by node %s", shardID, owner.Node)
		}
		fence := uint64(1)
		if record != nil && record.NextFence > 0 {
			fence = record.NextFence
		}
		if owner != nil && fence <= owner.Fence {
			fence = owner.Fence + 1
		}
		if record == nil {
			record = &ShardLifecycleRecord{ShardId: shardID}
		}
		record.NextFence = fence + 1
		newOwner := &ShardOwnershipRecord{ShardId: shardID, Fence: fence, StartupId: m.process.startupID, Node: m.process.local.GetNodeName(), LeaseId: int64(leaseID)}
		recordBuf, err := lifecycleBytes(record)
		if err != nil {
			return nil, err
		}
		ownerBuf, err := lifecycleBytes(newOwner)
		if err != nil {
			return nil, err
		}
		recordKey, ownerKey := m.shardRecordKey(shardID), m.shardOwnerKey(shardID)
		rsp, er := m.process.cluster.etcdClient.Txn(ctx).If(
			clientv3.Compare(clientv3.ModRevision(recordKey), "=", recordMod),
			clientv3.Compare(clientv3.ModRevision(ownerKey), "=", ownerMod),
		).Then(
			clientv3.OpPut(recordKey, recordBuf),
			clientv3.OpPut(ownerKey, ownerBuf, clientv3.WithLease(leaseID)),
		).Commit()
		if er != nil {
			return nil, asError(er)
		}
		if rsp.Succeeded {
			m.cacheShardPut(ownerKey, newOwner, rsp.Header.Revision)
			return newOwner, nil
		}
	}
	return nil, NewErrorf(ErrCodeOperationConflict, "lifecycle: shard %d claim contention", shardID)
}

func (m *lifecycleManager) shardWriteGuard(id *IDInfo) (*ShardOwnershipRecord, int64, *Error) {
	if !m.perShard() {
		return nil, 0, nil
	}
	owner, mod, err := m.loadShardOwner(id.ShardId)
	if err != nil {
		return nil, 0, err
	}
	leaseID, healthy := m.currentLease()
	if owner == nil || !healthy || owner.Fence != id.Fence || owner.LeaseId != int64(leaseID) || owner.Node != m.process.local.GetNodeName() || owner.StartupId != id.StartupId {
		return nil, 0, NewErrorf(ErrCodeOwnershipLost, "lifecycle: shard %d ownership changed", id.ShardId)
	}
	return owner, mod, nil
}
