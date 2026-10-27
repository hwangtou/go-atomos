package atomos

import (
	internaletcd "github.com/hwangtou/go-atomos/internal/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdClient is the narrow etcd contract accepted by CosmosRunnable.SetEtcdClient.
type EtcdClient = internaletcd.Client

// EtcdTxn is the transaction contract used by EtcdClient.
type EtcdTxn = internaletcd.Txn

// EtcdOp is a transaction operation with explicit lease information.
type EtcdOp = internaletcd.Op

// NewEtcdClientV3 adapts a clientv3 client to EtcdClient. The caller retains
// ownership of clients passed to CosmosRunnable.SetEtcdClient.
func NewEtcdClientV3(client *clientv3.Client) EtcdClient {
	return internaletcd.NewClientV3(client)
}

// EtcdOpGet constructs a transaction Get operation.
func EtcdOpGet(key string, opts ...clientv3.OpOption) EtcdOp {
	return internaletcd.OpGet(key, opts...)
}

// EtcdOpPut constructs a transaction Put operation with an explicit lease.
func EtcdOpPut(key, value string, lease clientv3.LeaseID) EtcdOp {
	return internaletcd.OpPut(key, value, lease)
}

// EtcdOpDelete constructs a transaction Delete operation.
func EtcdOpDelete(key string, opts ...clientv3.OpOption) EtcdOp {
	return internaletcd.OpDelete(key, opts...)
}
