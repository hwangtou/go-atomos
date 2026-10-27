package atomos

import (
	"encoding/json"
	"time"

	etcdClient "go.etcd.io/etcd/client/v3"
)

const etcdMaxNodeVersions = 2

var (
	etcdVersionLockAttempts = 8
	etcdVersionLockBackoff  = 20 * time.Millisecond
)

// etcdUpdateVersionLock performs an optimistic, bounded read/CAS loop. Etcd's
// ModRevision changes on every write; key Version does not and therefore cannot
// detect all competing updates.
func etcdUpdateVersionLock(cli EtcdClient, lockURI string, update func(*CosmosNodeVersionLock) (bool, *Error)) *Error {
	var lastErr *Error
	for attempt := 0; attempt < etcdVersionLockAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(etcdVersionLockBackoff)
		}
		buf, _, modRevision, err := etcdGetMeta(cli, lockURI)
		if err != nil {
			lastErr = err
			continue
		}

		lockInfo := &CosmosNodeVersionLock{}
		exists := buf != nil
		if exists {
			if er := json.Unmarshal(buf, lockInfo); er != nil {
				return NewErrorf(ErrCodeStorageDataCorrupted, "etcd: failed to unmarshal lock info. err=(%s)", er).AddStack(nil)
			}
		}
		changed, err := update(lockInfo)
		if err != nil {
			return err.AddStack(nil)
		}
		if !changed {
			return nil
		}
		newBuf, er := json.Marshal(lockInfo)
		if er != nil {
			return NewErrorf(ErrCodeStorageSerializationFailed, "etcd: failed to marshal lock info. err=(%s)", er).AddStack(nil)
		}

		ctx, cancel := etcdContext()
		txn := cli.Txn(ctx)
		if exists {
			txn = txn.If(etcdClient.Compare(etcdClient.ModRevision(lockURI), "=", modRevision))
		} else {
			txn = txn.If(etcdClient.Compare(etcdClient.Version(lockURI), "=", 0))
		}
		resp, er := txn.Then(EtcdOpPut(lockURI, string(newBuf), 0)).Commit()
		cancel()
		if er != nil {
			lastErr = NewErrorf(ErrCodeStorageConflict, "etcd: failed to commit transaction. err=(%s)", er).AddStack(nil)
			continue
		}
		if resp.Succeeded {
			return nil
		}
		lastErr = NewErrorf(ErrCodeStorageConflict, "etcd: lock changed during transaction. uri=(%s)", lockURI).AddStack(nil)
	}
	if lastErr == nil {
		lastErr = NewErrorf(ErrCodeStorageConflict, "etcd: lock transaction retry exhausted. uri=(%s)", lockURI).AddStack(nil)
	}
	return lastErr
}

func hasVersion(lockInfo *CosmosNodeVersionLock, version int64) bool {
	for _, v := range lockInfo.Versions {
		if v == version {
			return true
		}
	}
	return false
}

func removeVersion(lockInfo *CosmosNodeVersionLock, version int64) bool {
	versions := lockInfo.Versions[:0]
	removed := false
	for _, v := range lockInfo.Versions {
		if v == version {
			removed = true
			continue
		}
		versions = append(versions, v)
	}
	lockInfo.Versions = versions
	if lockInfo.Current == version {
		lockInfo.Current = 0
	}
	return removed
}

// reclaimCrashedVersions removes versions whose leased registration key has
// disappeared. It is only needed when the lock is full, avoiding interference
// with the usual startup window of a second version.
func (p *CosmosProcess) reclaimCrashedVersions(cli EtcdClient, nodeName string, lockInfo *CosmosNodeVersionLock) *Error {
	versions := append([]int64(nil), lockInfo.Versions...)
	for _, version := range versions {
		key := etcdCosmosNodeVersionURI(p.local.runnable.config.Cosmos, nodeName, version)
		buf, _, err := etcdGet(cli, key)
		if err != nil {
			return err.AddStack(nil)
		}
		if buf == nil {
			removeVersion(lockInfo, version)
		}
	}
	if lockInfo.Current == 0 && len(lockInfo.Versions) == 1 {
		lockInfo.Current = lockInfo.Versions[0]
	}
	return nil
}

// 启动时，检查并锁定版本。重复锁定同一版本是幂等操作。
func (p *CosmosProcess) etcdStartUpTryLockingVersionNodeLock(cli EtcdClient, nodeName string, version int64) *Error {
	lockURI := etcdCosmosNodeLockURI(p.local.runnable.config.Cosmos, nodeName)
	return etcdUpdateVersionLock(cli, lockURI, func(lockInfo *CosmosNodeVersionLock) (bool, *Error) {
		if hasVersion(lockInfo, version) {
			return false, nil
		}
		if len(lockInfo.Versions) >= etcdMaxNodeVersions {
			if err := p.reclaimCrashedVersions(cli, nodeName, lockInfo); err != nil {
				return false, err
			}
		}
		if len(lockInfo.Versions) >= etcdMaxNodeVersions {
			return false, NewErrorf(ErrCodeStorageConflict, "etcd: the number of versions is equal or more than the maximum allowed. versions=(%v),maxNodes=(%d)", lockInfo.Versions, etcdMaxNodeVersions).AddStack(nil)
		}
		lockInfo.Versions = append(lockInfo.Versions, version)
		return true, nil
	})
}

// 启动失败时，解锁当前进程的版本。
func (p *CosmosProcess) etcdStartUpFailedNodeVersionUnlock() *Error {
	return p.etcdUnlockVersion()
}

// 启动成功时，将当前进程版本标记为 current。
func (p *CosmosProcess) etcdStartedVersionNodeLock() *Error {
	lockURI := etcdCosmosNodeLockURI(p.local.runnable.config.Cosmos, p.local.runnable.config.Node)
	version := p.cluster.etcdVersion
	return etcdUpdateVersionLock(p.cluster.etcdClient, lockURI, func(lockInfo *CosmosNodeVersionLock) (bool, *Error) {
		if !hasVersion(lockInfo, version) {
			return false, NewErrorf(ErrCodeStorageConflict, "etcd: version is not in the lock info. version=(%d),versions=(%v)", version, lockInfo.Versions).AddStack(nil)
		}
		if lockInfo.Current == version {
			return false, nil
		}
		lockInfo.Current = version
		return true, nil
	})
}

// 停止时，解锁当前进程的版本。
func (p *CosmosProcess) etcdStoppingNodeVersionUnlock() *Error {
	return p.etcdUnlockVersion()
}

func (p *CosmosProcess) etcdUnlockVersion() *Error {
	lockURI := etcdCosmosNodeLockURI(p.local.runnable.config.Cosmos, p.local.runnable.config.Node)
	version := p.cluster.etcdVersion
	return etcdUpdateVersionLock(p.cluster.etcdClient, lockURI, func(lockInfo *CosmosNodeVersionLock) (bool, *Error) {
		if len(lockInfo.Versions) == 0 && lockInfo.Current != version {
			return false, NewError(ErrCodeStorageConflict, "etcd: failed to get lock info.").AddStack(nil)
		}
		wasCurrent := lockInfo.Current == version
		changed := removeVersion(lockInfo, version)
		return changed || wasCurrent, nil
	})
}
