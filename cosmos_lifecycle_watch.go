package atomos

import (
	"context"
	"strings"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/proto"
)

var (
	lifecycleWatchReconnect = 200 * time.Millisecond
	lifecycleWatchProbe     = time.Second
)

type lifecycleOwnerSnapshot struct {
	record      *AtomOwnershipRecord
	modRevision int64
}

func (m *lifecycleManager) ownerPrefix() string {
	return "/cosmos/" + lifecycleEscape(m.process.local.runnable.config.Cosmos) + "/lifecycle/atom/"
}

func (m *lifecycleManager) cacheOwnerPut(key string, record *AtomOwnershipRecord, modRevision int64) {
	if m == nil || record == nil || modRevision <= 0 {
		return
	}
	m.ownerMu.Lock()
	defer m.ownerMu.Unlock()
	if current, ok := m.owners[key]; ok && current.modRevision >= modRevision {
		return
	}
	m.owners[key] = lifecycleOwnerSnapshot{record: proto.Clone(record).(*AtomOwnershipRecord), modRevision: modRevision}
}

func (m *lifecycleManager) cacheOwnerDelete(key string, modRevision int64) {
	if m == nil || modRevision <= 0 {
		return
	}
	m.ownerMu.Lock()
	defer m.ownerMu.Unlock()
	if current, ok := m.owners[key]; ok && current.modRevision >= modRevision {
		return
	}
	m.owners[key] = lifecycleOwnerSnapshot{modRevision: modRevision}
}

func (m *lifecycleManager) cachedOwner(element, atom string) *AtomOwnershipRecord {
	m.ownerMu.RLock()
	snapshot := m.owners[m.ownerKey(element, atom)]
	m.ownerMu.RUnlock()
	return snapshot.record
}

func (m *lifecycleManager) syncOwnerSnapshot(ctx context.Context) (int64, *Error) {
	m.watchHealthy.Store(false)
	ctx, cancel := context.WithTimeout(ctx, etcdOpTimeout)
	defer cancel()
	rsp, err := m.cli.Get(ctx, m.ownerPrefix(), clientv3.WithPrefix())
	if err != nil {
		return 0, NewErrorf(ErrCodeCosmosEtcdGetFailed, "lifecycle: sync owner snapshot: %v", err)
	}
	revision := rsp.Header.Revision
	owners := make(map[string]lifecycleOwnerSnapshot, len(rsp.Kvs))
	for _, kv := range rsp.Kvs {
		key := string(kv.Key)
		if !strings.HasSuffix(key, "/owner") {
			continue
		}
		record := &AtomOwnershipRecord{}
		if err := proto.Unmarshal(kv.Value, record); err != nil {
			return 0, NewErrorf(ErrCodeCosmosEtcdGetFailed, "lifecycle: decode owner %s: %v", key, err)
		}
		owners[key] = lifecycleOwnerSnapshot{record: record, modRevision: kv.ModRevision}
	}
	m.ownerMu.Lock()
	for key, current := range m.owners {
		if current.modRevision > revision {
			owners[key] = current
		}
	}
	m.owners = owners
	m.ownerMu.Unlock()
	m.watchHealthy.Store(true)
	return revision, nil
}

func (m *lifecycleManager) startWatch() *Error {
	if m == nil || m.watchDone != nil {
		return nil
	}
	m.cli = m.process.cluster.etcdClient
	revision, err := m.syncOwnerSnapshot(context.Background())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.watchCancel = cancel
	m.watchDone = make(chan struct{})
	go m.watchOwners(ctx, revision+1)
	return nil
}

func (m *lifecycleManager) probeOwnerWatch(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, etcdOpTimeout)
	defer cancel()
	_, err := m.cli.Get(ctx, m.ownerPrefix(), clientv3.WithPrefix(), clientv3.WithLimit(1))
	return err == nil
}

func (m *lifecycleManager) watchOwners(ctx context.Context, revision int64) {
	defer close(m.watchDone)
	defer m.watchHealthy.Store(false)
	for {
		if revision == 0 {
			updated, err := m.syncOwnerSnapshot(ctx)
			if err != nil {
				timer := time.NewTimer(lifecycleWatchReconnect)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				continue
			}
			revision = updated + 1
		}
		watchCh := m.cli.Watch(clientv3.WithRequireLeader(ctx), m.ownerPrefix(), clientv3.WithPrefix(), clientv3.WithRev(revision))
		probe := time.NewTicker(lifecycleWatchProbe)
		reconnect := false
		for !reconnect {
			select {
			case <-ctx.Done():
				probe.Stop()
				return
			case <-probe.C:
				if !m.probeOwnerWatch(ctx) {
					m.watchHealthy.Store(false)
					reconnect = true
				}
			case rsp, ok := <-watchCh:
				if !ok || rsp.Err() != nil || rsp.CompactRevision > 0 {
					m.watchHealthy.Store(false)
					reconnect = true
					continue
				}
				for _, event := range rsp.Events {
					key := string(event.Kv.Key)
					if !strings.HasSuffix(key, "/owner") {
						continue
					}
					switch event.Type {
					case mvccpb.PUT:
						record := &AtomOwnershipRecord{}
						if err := proto.Unmarshal(event.Kv.Value, record); err != nil {
							m.watchHealthy.Store(false)
							reconnect = true
							continue
						}
						m.cacheOwnerPut(key, record, event.Kv.ModRevision)
					case mvccpb.DELETE:
						m.cacheOwnerDelete(key, event.Kv.ModRevision)
					}
				}
				revision = rsp.Header.Revision
			}
		}
		probe.Stop()
		timer := time.NewTimer(lifecycleWatchReconnect)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		updated, err := m.syncOwnerSnapshot(ctx)
		if err != nil {
			revision = 0
			continue
		}
		revision = updated + 1
	}
}
