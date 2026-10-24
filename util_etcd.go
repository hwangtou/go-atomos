package atomos

import (
	"context"
	clientv3 "go.etcd.io/etcd/client/v3"
	"time"
)

// etcdOpTimeout bounds each one-shot registration RPC in etcdKeepalive.
// The etcd client's RPCs use WaitForReady, so without a deadline Grant/Put
// block forever against an unreachable endpoint and node startup hangs before
// the caller's retry loop (cosmos_etcd.go) ever sees an error.
// Var instead of const so tests can shrink it.
var etcdOpTimeout = etcdDialTime * time.Second

// etcdKeepalive
// 保持活跃
// 1. 注册服务
// 2. 保持活跃
func etcdKeepalive(cli *clientv3.Client, key, value string, ttl int64) (*clientv3.LeaseGrantResponse, <-chan *clientv3.LeaseKeepAliveResponse, *Error) {
	// grant a lease.
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	lease, er := cli.Grant(ctx, ttl)
	cancel()
	if er != nil {
		return nil, nil, NewErrorf(ErrCodeCosmosEtcdKeepaliveFailed, "etcd: Failed to grant lease. err=(%s)", er).AddStack(nil)
	}

	// register the service with etcd.
	ctx, cancel = context.WithTimeout(context.Background(), etcdOpTimeout)
	_, er = cli.Put(ctx, key, value, clientv3.WithLease(lease.ID))
	cancel()
	if er != nil {
		return nil, nil, NewErrorf(ErrCodeCosmosEtcdKeepaliveFailed, "etcd: Failed to register service. err=(%s)", er).AddStack(nil)
	}

	// keepalive the service so that the lease does not expire.
	// KeepAlive's ctx must stay unbounded: it governs the whole keepalive
	// stream, not one RPC — a deadline here would silently stop lease renewal
	// once it fires. It returns without blocking even on an unreachable
	// endpoint (failures surface later as a closed channel).
	keepAliveCh, er := cli.KeepAlive(context.Background(), lease.ID)
	if er != nil {
		return nil, nil, NewErrorf(ErrCodeCosmosEtcdKeepaliveFailed, "etcd: Failed to keepalive. err=(%s)", er).AddStack(nil)
	}

	return lease, keepAliveCh, nil
}

// etcdPut
// Put a string into etcd.
func etcdPut(cli *clientv3.Client, key string, value []byte) *Error {
	// Put the string into etcd.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, er := cli.Put(ctx, key, string(value))
	cancel()
	if er != nil {
		return NewErrorf(ErrCodeCosmosEtcdPutFailed, "etcd: Put failed. key=(%s),err=(%v)", key, er).AddStack(nil)
	}
	return nil
}

// etcdGet
// Get a string from etcd.
func etcdGet(cli *clientv3.Client, key string) ([]byte, int64, *Error) {
	// Get the string back from etcd.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	resp, er := cli.Get(ctx, key)
	cancel()
	if er != nil {
		return nil, 0, NewErrorf(ErrCodeCosmosEtcdGetFailed, "etcd: Get failed. key=(%s),err=(%v)", key, er).AddStack(nil)
	}
	for _, kv := range resp.Kvs {
		if string(kv.Key) == key {
			return kv.Value, kv.Version, nil
		}
	}
	return nil, 0, nil
}

// etcdDelete
// Delete a string from etcd.
func etcdDelete(cli *clientv3.Client, key string) *Error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, er := cli.Delete(ctx, key)
	cancel()
	if er != nil {
		return NewErrorf(ErrCodeCosmosEtcdDeleteFailed, "etcd: Delete failed. key=(%s),err=(%v)", key, er).AddStack(nil)
	}
	return nil
}
