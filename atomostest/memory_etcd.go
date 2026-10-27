package atomostest

import (
	"github.com/hwangtou/go-atomos/internal/memoryetcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ErrMemoryEtcdClosed is returned after a MemoryEtcd client is closed.
var ErrMemoryEtcdClosed = memoryetcd.ErrMemoryEtcdClosed

// ErrMemoryEtcdUnsupported is returned for operations outside the memory etcd contract.
var ErrMemoryEtcdUnsupported = memoryetcd.ErrMemoryEtcdUnsupported

// EtcdClient is the narrow etcd seam shared by the memory implementation and
// production adapters.
type EtcdClient = memoryetcd.EtcdClient

// EtcdTxn is the transaction subset implemented by MemoryEtcd.
type EtcdTxn = memoryetcd.EtcdTxn

// EtcdOp wraps a clientv3 operation with explicit lease information.
type EtcdOp = memoryetcd.EtcdOp

// EtcdOpGet constructs a transaction Get operation.
func EtcdOpGet(key string, opts ...clientv3.OpOption) EtcdOp {
	return memoryetcd.EtcdOpGet(key, opts...)
}

// EtcdOpPut constructs a transaction Put operation with an explicit lease.
func EtcdOpPut(key, value string, lease clientv3.LeaseID) EtcdOp {
	return memoryetcd.EtcdOpPut(key, value, lease)
}

// EtcdOpDelete constructs a transaction Delete operation.
func EtcdOpDelete(key string, opts ...clientv3.OpOption) EtcdOp {
	return memoryetcd.EtcdOpDelete(key, opts...)
}

// MemoryEtcdOperation identifies an operation whose result can be fault-injected.
type MemoryEtcdOperation = memoryetcd.MemoryEtcdOperation

const (
	MemoryEtcdGet       = memoryetcd.MemoryEtcdGet
	MemoryEtcdPut       = memoryetcd.MemoryEtcdPut
	MemoryEtcdDelete    = memoryetcd.MemoryEtcdDelete
	MemoryEtcdTxn       = memoryetcd.MemoryEtcdTxn
	MemoryEtcdWatch     = memoryetcd.MemoryEtcdWatch
	MemoryEtcdGrant     = memoryetcd.MemoryEtcdGrant
	MemoryEtcdKeepAlive = memoryetcd.MemoryEtcdKeepAlive
	MemoryEtcdRevoke    = memoryetcd.MemoryEtcdRevoke
)

// MemoryEtcd is a deterministic, revisioned in-memory etcd subset. Lease timers
// use the supplied VirtualClock; no wall-clock goroutine drives expiration.
type MemoryEtcd = memoryetcd.MemoryEtcd

// NewMemoryEtcd creates an empty store at etcd's initial revision of 1.
func NewMemoryEtcd(clock *VirtualClock) *MemoryEtcd {
	return memoryetcd.NewMemoryEtcd(clock)
}
