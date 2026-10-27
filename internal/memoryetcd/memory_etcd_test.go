package memoryetcd

import (
	"context"
	"errors"
	"testing"

	"github.com/hwangtou/go-atomos/internal/testclock"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestTxnResponseLossCommitsBeforeReturningError(t *testing.T) {
	client := NewMemoryEtcd(testclock.NewVirtualClock())
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	lost := errors.New("response lost")
	client.InjectTxnResponseLossAfter(0, lost)

	rsp, err := client.Txn(ctx).Then(EtcdOpPut("committed", "value", 0)).Commit()
	if rsp != nil || !errors.Is(err, lost) {
		t.Fatalf("Commit() = (%v, %v), want nil, response loss", rsp, err)
	}
	got, err := client.Get(ctx, "committed")
	if err != nil || len(got.Kvs) != 1 || string(got.Kvs[0].Value) != "value" {
		t.Fatalf("committed key = (%v, %v)", got, err)
	}
}

func TestTxnResponseLossCountsOnlySuccessfulCommits(t *testing.T) {
	client := NewMemoryEtcd(testclock.NewVirtualClock())
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	lost := errors.New("response lost")
	client.InjectTxnResponseLossAfter(1, lost)

	failed, err := client.Txn(ctx).If(clientv3.Compare(clientv3.Version("gate"), "=", 1)).Then(EtcdOpPut("wrong", "value", 0)).Commit()
	if err != nil || failed.Succeeded {
		t.Fatalf("failed CAS = (%v, %v)", failed, err)
	}
	first, err := client.Txn(ctx).Then(EtcdOpPut("first", "value", 0)).Commit()
	if err != nil || !first.Succeeded {
		t.Fatalf("first successful Commit() = (%v, %v)", first, err)
	}
	second, err := client.Txn(ctx).Then(EtcdOpPut("second", "value", 0)).Commit()
	if second != nil || !errors.Is(err, lost) {
		t.Fatalf("second successful Commit() = (%v, %v), want response loss", second, err)
	}
	for _, key := range []string{"first", "second"} {
		got, getErr := client.Get(ctx, key)
		if getErr != nil || len(got.Kvs) != 1 {
			t.Fatalf("Get(%q) = (%v, %v)", key, got, getErr)
		}
	}
	if _, err := client.Txn(ctx).Then(EtcdOpPut("third", "value", 0)).Commit(); err != nil {
		t.Fatalf("one-shot response loss was not consumed: %v", err)
	}
}
