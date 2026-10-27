// Package etcd defines the narrow etcd contract used by the runtime.
package etcd

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Client is the etcd subset required by the runtime. Put and transaction
// operations carry lease IDs explicitly because clientv3.Op does not expose its
// lease option.
type Client interface {
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Put(ctx context.Context, key, value string, lease clientv3.LeaseID) (*clientv3.PutResponse, error)
	Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error)
	Txn(ctx context.Context) Txn
	Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan
	Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error)
	KeepAlive(ctx context.Context, id clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error)
	Revoke(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error)
	Close() error
}

// Txn is the transaction subset required by the runtime.
type Txn interface {
	If(cmps ...clientv3.Cmp) Txn
	Then(ops ...Op) Txn
	Else(ops ...Op) Txn
	Commit() (*clientv3.TxnResponse, error)
}

// Op wraps a clientv3 operation with its explicit lease information. Lease must
// be zero for non-put operations.
type Op struct {
	Op    clientv3.Op
	Lease clientv3.LeaseID
}

// OpGet constructs a transaction Get operation.
func OpGet(key string, opts ...clientv3.OpOption) Op {
	return Op{Op: clientv3.OpGet(key, opts...)}
}

// OpPut constructs a transaction Put operation with an explicit lease.
func OpPut(key, value string, lease clientv3.LeaseID) Op {
	return Op{Op: clientv3.OpPut(key, value), Lease: lease}
}

// OpDelete constructs a transaction Delete operation.
func OpDelete(key string, opts ...clientv3.OpOption) Op {
	return Op{Op: clientv3.OpDelete(key, opts...)}
}
