package atomos

import (
	"context"
	clientv3 "go.etcd.io/etcd/client/v3"
	"time"
)

// etcdOpTimeout bounds every one-shot etcd RPC. Streaming RPCs such as
// KeepAlive and Watch must not use this as their stream lifetime.
// Var instead of const so tests can shrink it.
var etcdOpTimeout = etcdDialTime * time.Second

func etcdContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), etcdOpTimeout)
}

// etcdGetPrefix performs a bounded prefix read and returns the response header
// revision needed to start a gap-free watch.
func etcdGetPrefix(cli EtcdClient, key string) (*clientv3.GetResponse, *Error) {
	ctx, cancel := etcdContext()
	defer cancel()
	resp, er := cli.Get(ctx, key, clientv3.WithPrefix())
	if er != nil {
		return nil, NewErrorf(ErrCodeCosmosEtcdGetFailed, "etcd: Get prefix failed. key=(%s),err=(%v)", key, er).AddStack(nil)
	}
	return resp, nil
}

// etcdPutLease is shared by registration and its update path so both have the
// same deadline behavior.
func etcdPutLease(cli EtcdClient, key, value string, lease clientv3.LeaseID) *Error {
	ctx, cancel := etcdContext()
	defer cancel()
	_, er := cli.Put(ctx, key, value, lease)
	if er != nil {
		return NewErrorf(ErrCodeCosmosEtcdPutFailed, "etcd: Put lease failed. key=(%s),err=(%v)", key, er).AddStack(nil)
	}
	return nil
}

// etcdKeepalive
// 保持活跃
// 1. 注册服务
// 2. 保持活跃
func etcdKeepalive(cli EtcdClient, key, value string, ttl int64) (*clientv3.LeaseGrantResponse, <-chan *clientv3.LeaseKeepAliveResponse, *Error) {
	ctx, cancel := etcdContext()
	lease, er := cli.Grant(ctx, ttl)
	cancel()
	if er != nil {
		return nil, nil, NewErrorf(ErrCodeCosmosEtcdKeepaliveFailed, "etcd: Failed to grant lease. err=(%s)", er).AddStack(nil)
	}

	ctx, cancel = etcdContext()
	_, er = cli.Put(ctx, key, value, lease.ID)
	cancel()
	if er != nil {
		return nil, nil, NewErrorf(ErrCodeCosmosEtcdKeepaliveFailed, "etcd: Failed to register service. err=(%s)", er).AddStack(nil)
	}

	// KeepAlive's context governs the whole stream, not an individual RPC.
	keepAliveCh, er := cli.KeepAlive(context.Background(), lease.ID)
	if er != nil {
		return nil, nil, NewErrorf(ErrCodeCosmosEtcdKeepaliveFailed, "etcd: Failed to keepalive. err=(%s)", er).AddStack(nil)
	}

	return lease, keepAliveCh, nil
}

// etcdPut puts a string into etcd with a bounded RPC.
func etcdPut(cli EtcdClient, key string, value []byte) *Error {
	ctx, cancel := etcdContext()
	defer cancel()
	_, er := cli.Put(ctx, key, string(value), 0)
	if er != nil {
		return NewErrorf(ErrCodeCosmosEtcdPutFailed, "etcd: Put failed. key=(%s),err=(%v)", key, er).AddStack(nil)
	}
	return nil
}

// etcdGet gets a string and its key version from etcd.
func etcdGet(cli EtcdClient, key string) ([]byte, int64, *Error) {
	buf, version, _, err := etcdGetMeta(cli, key)
	return buf, version, err
}

// etcdGetMeta also returns ModRevision, which is the revision required for an
// etcd compare-and-swap transaction. Key Version is only a per-key write count.
func etcdGetMeta(cli EtcdClient, key string) ([]byte, int64, int64, *Error) {
	ctx, cancel := etcdContext()
	defer cancel()
	resp, er := cli.Get(ctx, key)
	if er != nil {
		return nil, 0, 0, NewErrorf(ErrCodeCosmosEtcdGetFailed, "etcd: Get failed. key=(%s),err=(%v)", key, er).AddStack(nil)
	}
	for _, kv := range resp.Kvs {
		if string(kv.Key) == key {
			return kv.Value, kv.Version, kv.ModRevision, nil
		}
	}
	return nil, 0, 0, nil
}

// etcdDelete deletes a string from etcd with a bounded RPC.
func etcdDelete(cli EtcdClient, key string) *Error {
	ctx, cancel := etcdContext()
	defer cancel()
	_, er := cli.Delete(ctx, key)
	if er != nil {
		return NewErrorf(ErrCodeCosmosEtcdDeleteFailed, "etcd: Delete failed. key=(%s),err=(%v)", key, er).AddStack(nil)
	}
	return nil
}
