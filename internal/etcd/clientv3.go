package etcd

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type clientV3 struct {
	*clientv3.Client
}

// NewClientV3 adapts a clientv3 client to Client.
func NewClientV3(client *clientv3.Client) Client {
	return &clientV3{Client: client}
}

func (c *clientV3) Put(ctx context.Context, key, value string, lease clientv3.LeaseID) (*clientv3.PutResponse, error) {
	if lease == 0 {
		return c.Client.Put(ctx, key, value)
	}
	return c.Client.Put(ctx, key, value, clientv3.WithLease(lease))
}

func (c *clientV3) Txn(ctx context.Context) Txn {
	return &txnV3{txn: c.Client.Txn(ctx)}
}

type txnV3 struct {
	txn clientv3.Txn
}

func (t *txnV3) If(cmps ...clientv3.Cmp) Txn {
	t.txn = t.txn.If(cmps...)
	return t
}

func (t *txnV3) Then(ops ...Op) Txn {
	t.txn = t.txn.Then(opsV3(ops)...)
	return t
}

func (t *txnV3) Else(ops ...Op) Txn {
	t.txn = t.txn.Else(opsV3(ops)...)
	return t
}

func (t *txnV3) Commit() (*clientv3.TxnResponse, error) {
	return t.txn.Commit()
}

func opsV3(ops []Op) []clientv3.Op {
	converted := make([]clientv3.Op, 0, len(ops))
	for _, operation := range ops {
		if operation.Op.IsPut() && operation.Lease != 0 {
			converted = append(converted, clientv3.OpPut(
				string(operation.Op.KeyBytes()),
				string(operation.Op.ValueBytes()),
				clientv3.WithLease(operation.Lease),
			))
			continue
		}
		converted = append(converted, operation.Op)
	}
	return converted
}
