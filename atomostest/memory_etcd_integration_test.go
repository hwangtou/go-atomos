//go:build integration

package atomostest

import (
	"context"
	"net/url"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

// TestEmbeddedEtcdContract runs the memory contract unchanged against a real
// in-process etcd. No subtest is skipped.
func TestEmbeddedEtcdContract(t *testing.T) {
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.ListenClientUrls = []url.URL{{Scheme: "http", Host: "127.0.0.1:0"}}
	cfg.ListenPeerUrls = []url.URL{{Scheme: "http", Host: "127.0.0.1:0"}}
	cfg.LogLevel = "error"
	server, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start embedded etcd: %v", err)
	}
	t.Cleanup(server.Close)
	select {
	case <-server.Server.ReadyNotify():
	case <-time.After(15 * time.Second):
		t.Fatal("embedded etcd ready timeout")
	}
	raw, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{server.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("create embedded etcd client: %v", err)
	}
	client := &contractClientV3{Client: raw}
	t.Cleanup(func() { _ = client.Close() })
	runEtcdContract(t, etcdContractBackend{
		client:  client,
		advance: time.Sleep,
		compact: func(revision int64) error {
			_, err := raw.Compact(context.Background(), revision)
			return err
		},
	})
}

// contractClientV3 demonstrates the production adaptation without reading any
// private clientv3 fields.
type contractClientV3 struct {
	*clientv3.Client
}

func (c *contractClientV3) Put(ctx context.Context, key, value string, lease clientv3.LeaseID) (*clientv3.PutResponse, error) {
	if lease == 0 {
		return c.Client.Put(ctx, key, value)
	}
	return c.Client.Put(ctx, key, value, clientv3.WithLease(lease))
}

func (c *contractClientV3) Txn(ctx context.Context) EtcdTxn {
	return &contractTxnV3{txn: c.Client.Txn(ctx)}
}

type contractTxnV3 struct {
	txn clientv3.Txn
}

func (t *contractTxnV3) If(cmps ...clientv3.Cmp) EtcdTxn {
	t.txn = t.txn.If(cmps...)
	return t
}

func (t *contractTxnV3) Then(ops ...EtcdOp) EtcdTxn {
	t.txn = t.txn.Then(contractOpsV3(ops)...)
	return t
}

func (t *contractTxnV3) Else(ops ...EtcdOp) EtcdTxn {
	t.txn = t.txn.Else(contractOpsV3(ops)...)
	return t
}

func (t *contractTxnV3) Commit() (*clientv3.TxnResponse, error) {
	return t.txn.Commit()
}

func contractOpsV3(ops []EtcdOp) []clientv3.Op {
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
