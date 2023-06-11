package go_atomos

import (
	"context"
	"encoding/json"
	etcdClient "go.etcd.io/etcd/client/v3"
)

// 启动时，检查etcd中同一个cosmos的node有多少个version。因为version是用于热更的设计，而不是为了支持多个版本，所以集群同时只应该有不超过两个version。
// 当检查到当前cosmos的node已经有两个version，且两个version都不是自己时，应该主动退出。
// Check how many versions of the node of the same cosmos are in etcd. Because the version is designed for hot updates, not to support multiple versions, there should be no more than two versions in the cluster at the same time.
// When it is found that the current cosmos node already has two versions, and neither of the two versions is itself, it should actively exit.
// Key and prefix to be used in conditions
func (p *CosmosProcess) etcdStartUpTryLockingVersionNodeLock(cli *etcdClient.Client, nodeName string, version int64) *Error {
	maxNodes := 2
	lockURI := etcdCosmosNodeLockURI(p.local.runnable.config.Cosmos, nodeName)

	buf, etcdVer, err := etcdGet(cli, lockURI)
	if err != nil {
		return err.AddStack(nil)
	}

	lockInfo := &CosmosNodeVersionLock{}
	// Transaction
	txn := cli.Txn(context.Background())
	if buf == nil {
		lockInfo.Versions = append(lockInfo.Versions, version)
		buf, er := json.Marshal(lockInfo)
		if er != nil {
			return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to marshal lock info. err=(%s)", er).AddStack(nil)
		}
		// Try to insert the key only if it does not exist
		resp, er := txn.If(
			// Condition - key does not exist
			etcdClient.Compare(etcdClient.Version(lockURI), "=", 0),
		).Then(
			// Then - put the key with the value
			etcdClient.OpPut(lockURI, string(buf)),
		).Commit()

		if er != nil {
			return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to commit transaction. err=(%s)", er).AddStack(nil)
		}
		if !resp.Succeeded {
			return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to commit transaction. err=(%s)", er).AddStack(nil)
		}
		return nil
	} else {
		// Unmarshal the lock info
		er := json.Unmarshal(buf, lockInfo)
		if er != nil {
			return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to unmarshal lock info. err=(%s)", er).AddStack(nil)
		}
		// Check if the number of keys is equal or more than the maximum allowed
		if len(lockInfo.Versions) >= maxNodes {
			return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: the number of versions is equal or more than the maximum allowed. err=(%s)", er).AddStack(nil)
		}
		lockInfo.Versions = append(lockInfo.Versions, version)

		// Marshal the lock info
		buf, er := json.Marshal(lockInfo)
		if er != nil {
			return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to marshal lock info. err=(%s)", er).AddStack(nil)
		}
		// Try to update the key only if the version is the same
		resp, er := txn.If(
			// Condition - key exists
			etcdClient.Compare(etcdClient.Version(lockURI), ">", 0),
			// Condition - version is the same
			etcdClient.Compare(etcdClient.Version(lockURI), "=", etcdVer),
		).Then(
			// Then - put the key with the value
			etcdClient.OpPut(lockURI, string(buf)),
		).Commit()

		if er != nil {
			return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to commit transaction. err=(%s)", er).AddStack(nil)
		}
		if !resp.Succeeded {
			return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to commit transaction. err=(%s)", er).AddStack(nil)
		}
		return nil
	}
}

// 启动失败时，解锁etcd中同一个cosmos的node的version。
// Unlock the version of the node of the same cosmos in etcd when startup fails.
func (p *CosmosProcess) etcdStartUpFailedNodeVersionUnlock() *Error {
	lockURI := etcdCosmosNodeLockURI(p.local.runnable.config.Cosmos, p.local.runnable.config.Node)
	buf, etcdVer, err := etcdGet(p.cluster.etcdClient, lockURI)
	if err != nil {
		return err.AddStack(nil)
	}

	lockInfo := &CosmosNodeVersionLock{}
	// Transaction
	txn := p.cluster.etcdClient.Txn(context.Background())
	if buf == nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to get lock info.").AddStack(nil)
	}
	// Unmarshal the lock info
	if er := json.Unmarshal(buf, lockInfo); er != nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to unmarshal lock info. err=(%s)", er).AddStack(nil)
	}
	// Remove the version from the lock info
	for i, v := range lockInfo.Versions {
		if v == p.cluster.etcdVersion {
			lockInfo.Versions = append(lockInfo.Versions[:i], lockInfo.Versions[i+1:]...)
			break
		}
	}
	if lockInfo.Current == p.cluster.etcdVersion {
		lockInfo.Current = 0
	}

	// Marshal the lock info
	buf, er := json.Marshal(lockInfo)
	if er != nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to marshal lock info. err=(%s)", er).AddStack(nil)
	}
	// Try to update the key only if the version is the same
	resp, er := txn.If(
		// Condition - key exists
		etcdClient.Compare(etcdClient.Version(lockURI), ">", 0),
		// Condition - version is the same
		etcdClient.Compare(etcdClient.Version(lockURI), "=", etcdVer),
	).Then(
		// Then - put the key with the value
		etcdClient.OpPut(lockURI, string(buf)),
	).Commit()

	if er != nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to commit transaction. err=(%s)", er).AddStack(nil)
	}
	if !resp.Succeeded {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to commit transaction. err=(%s)", er).AddStack(nil)
	}
	return nil
}

// 启动成功时，更新etcd中同一个cosmos的node的version。
// Update the version of the node of the same cosmos in etcd when startup succeeds.
func (p *CosmosProcess) etcdStartedVersionNodeLock() *Error {
	lockURI := etcdCosmosNodeLockURI(p.local.runnable.config.Cosmos, p.local.runnable.config.Node)
	buf, etcdVer, err := etcdGet(p.cluster.etcdClient, lockURI)
	if err != nil {
		return err.AddStack(nil)
	}

	lockInfo := &CosmosNodeVersionLock{}
	// Transaction
	txn := p.cluster.etcdClient.Txn(context.Background())
	if buf == nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to get lock info.").AddStack(nil)
	}
	// Unmarshal the lock info
	if er := json.Unmarshal(buf, lockInfo); er != nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to unmarshal lock info. err=(%s)", er).AddStack(nil)
	}
	// Check version is in the lock info
	found := false
	for _, v := range lockInfo.Versions {
		if v == p.cluster.etcdVersion {
			found = true
			break
		}
	}
	if !found {
		p.logging.PushLogging(p.local.info, LogLevel_Err, "etcd: version is not in the lock info.")
		return nil
	}
	lockInfo.Current = p.cluster.etcdVersion

	// Marshal the lock info
	buf, er := json.Marshal(lockInfo)
	if er != nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to marshal lock info. err=(%s)", er).AddStack(nil)
	}
	// Try to update the key only if the version is the same
	resp, er := txn.If(
		// Condition - key exists
		etcdClient.Compare(etcdClient.Version(lockURI), ">", 0),
		// Condition - version is the same
		etcdClient.Compare(etcdClient.Version(lockURI), "=", etcdVer),
	).Then(
		// Then - put the key with the value
		etcdClient.OpPut(lockURI, string(buf)),
	).Commit()

	if er != nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to commit transaction. err=(%s)", er).AddStack(nil)
	}
	if !resp.Succeeded {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to commit transaction. err=(%s)", er).AddStack(nil)
	}
	return nil
}

// 停止时，解锁etcd中同一个cosmos的node的version。
// Unlock the version of the node of the same cosmos in etcd when stopping.
func (p *CosmosProcess) etcdStoppingNodeVersionUnlock() *Error {
	lockURI := etcdCosmosNodeLockURI(p.local.runnable.config.Cosmos, p.local.runnable.config.Node)
	buf, etcdVer, err := etcdGet(p.cluster.etcdClient, lockURI)
	if err != nil {
		return err.AddStack(nil)
	}

	lockInfo := &CosmosNodeVersionLock{}
	// Transaction
	txn := p.cluster.etcdClient.Txn(context.Background())
	if buf == nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to get lock info.").AddStack(nil)
	}
	// Unmarshal the lock info
	if er := json.Unmarshal(buf, lockInfo); er != nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to unmarshal lock info. err=(%s)", er).AddStack(nil)
	}
	// Remove the version from the lock info
	for i, v := range lockInfo.Versions {
		if v == p.cluster.etcdVersion {
			lockInfo.Versions = append(lockInfo.Versions[:i], lockInfo.Versions[i+1:]...)
			break
		}
	}
	if lockInfo.Current == p.cluster.etcdVersion {
		lockInfo.Current = 0
	}

	// Marshal the lock info
	buf, er := json.Marshal(lockInfo)
	if er != nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to marshal lock info. err=(%s)", er).AddStack(nil)
	}
	// Try to update the key only if the version is the same
	resp, er := txn.If(
		// Condition - key exists
		etcdClient.Compare(etcdClient.Version(lockURI), ">", 0),
		// Condition - version is the same
		etcdClient.Compare(etcdClient.Version(lockURI), "=", etcdVer),
	).Then(
		// Then - put the key with the value
		etcdClient.OpPut(lockURI, string(buf)),
	).Commit()

	if er != nil {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to commit transaction. err=(%s)", er).AddStack(nil)
	}
	if !resp.Succeeded {
		return NewErrorf(ErrCosmosEtcdClusterVersionLockFailed, "etcd: failed to commit transaction. err=(%s)", er).AddStack(nil)
	}
	return nil
}
