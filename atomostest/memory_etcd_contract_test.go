package atomostest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type etcdContractBackend struct {
	client  EtcdClient
	advance func(time.Duration)
	compact func(revision int64) error
}

func TestMemoryEtcdContract(t *testing.T) {
	clock := NewVirtualClock()
	client := NewMemoryEtcd(clock)
	t.Cleanup(func() { _ = client.Close() })
	runEtcdContract(t, etcdContractBackend{
		client:  client,
		advance: clock.Advance,
		compact: func(revision int64) error {
			_, err := client.Compact(context.Background(), revision)
			return err
		},
	})
}

func runEtcdContract(t *testing.T, backend etcdContractBackend) {
	t.Helper()
	ctx := context.Background()
	client := backend.client

	t.Run("KVRangeAndMetadata", func(t *testing.T) {
		for _, item := range []struct{ key, value string }{
			{"contract/kv/b", "2"},
			{"contract/kv/a", "1"},
			{"contract/kv/c", "3"},
			{"contract/other", "x"},
		} {
			if _, err := client.Put(ctx, item.key, item.value, 0); err != nil {
				t.Fatalf("Put(%q): %v", item.key, err)
			}
		}
		response, err := client.Get(ctx, "contract/kv/", clientv3.WithPrefix(), clientv3.WithLimit(2))
		if err != nil {
			t.Fatal(err)
		}
		if response.Count != 3 || !response.More || len(response.Kvs) != 2 {
			t.Fatalf("limited Get = count %d, more %v, kvs %d", response.Count, response.More, len(response.Kvs))
		}
		gotKeys := []string{string(response.Kvs[0].Key), string(response.Kvs[1].Key)}
		if want := []string{"contract/kv/a", "contract/kv/b"}; !reflect.DeepEqual(gotKeys, want) {
			t.Fatalf("sorted keys = %v, want %v", gotKeys, want)
		}
		created := response.Kvs[0].CreateRevision
		if created == 0 || response.Kvs[0].ModRevision != created || response.Kvs[0].Version != 1 {
			t.Fatalf("new KV metadata = %+v", response.Kvs[0])
		}
		if _, err := client.Put(ctx, "contract/kv/a", "updated", 0); err != nil {
			t.Fatal(err)
		}
		updated, err := client.Get(ctx, "contract/kv/a")
		if err != nil {
			t.Fatal(err)
		}
		kv := updated.Kvs[0]
		if kv.CreateRevision != created || kv.ModRevision <= created || kv.Version != 2 {
			t.Fatalf("updated KV metadata = %+v, create %d", kv, created)
		}
		counted, err := client.Get(ctx, "contract/kv/", clientv3.WithPrefix(), clientv3.WithCountOnly())
		if err != nil {
			t.Fatal(err)
		}
		if counted.Count != 3 || len(counted.Kvs) != 0 {
			t.Fatalf("count-only Get = %+v", counted)
		}
		ranged, err := client.Get(ctx, "contract/kv/a", clientv3.WithRange("contract/kv/c"))
		if err != nil {
			t.Fatal(err)
		}
		if ranged.Count != 2 || string(ranged.Kvs[1].Key) != "contract/kv/b" {
			t.Fatalf("range Get = %+v", ranged)
		}

		lease, err := client.Grant(ctx, 60)
		if err != nil {
			t.Fatal(err)
		}
		keysOnlyKey := "contract/keys-only"
		if _, err := client.Put(ctx, keysOnlyKey, "value", lease.ID); err != nil {
			t.Fatal(err)
		}
		full, err := client.Get(ctx, keysOnlyKey)
		if err != nil {
			t.Fatal(err)
		}
		keysOnly, err := client.Get(ctx, keysOnlyKey, clientv3.WithKeysOnly())
		if err != nil {
			t.Fatal(err)
		}
		want := full.Kvs[0]
		want.Value = nil
		if want.Lease == 0 || !reflect.DeepEqual(keysOnly.Kvs[0], want) {
			t.Fatalf("keys-only Get = %+v, want metadata from %+v", keysOnly.Kvs[0], want)
		}
	})

	t.Run("TxnCAS", func(t *testing.T) {
		key := "contract/cas/key"
		created, err := client.Txn(ctx).If(
			clientv3.Compare(clientv3.Version(key), "=", 0),
		).Then(
			EtcdOpPut(key, "v1", 0),
		).Commit()
		if err != nil {
			t.Fatal(err)
		}
		if !created.Succeeded || len(created.Responses) != 1 {
			t.Fatalf("create CAS = %+v", created)
		}
		current, err := client.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		kv := current.Kvs[0]
		updated, err := client.Txn(ctx).If(
			clientv3.Compare(clientv3.ModRevision(key), "=", kv.ModRevision),
			clientv3.Compare(clientv3.CreateRevision(key), "=", kv.CreateRevision),
			clientv3.Compare(clientv3.Version(key), "=", 1),
			clientv3.Compare(clientv3.Value(key), "=", "v1"),
		).Then(
			EtcdOpPut(key, "v2", 0),
			EtcdOpPut("contract/cas/other", "same-revision", 0),
		).Else(
			EtcdOpPut("contract/cas/unexpected", "bad", 0),
		).Commit()
		if err != nil {
			t.Fatal(err)
		}
		if !updated.Succeeded || len(updated.Responses) != 2 || updated.Responses[0].GetResponsePut().Header.Revision != updated.Header.Revision || updated.Responses[1].GetResponsePut().Header.Revision != updated.Header.Revision {
			t.Fatalf("update CAS = %+v", updated)
		}
		stale, err := client.Txn(ctx).If(
			clientv3.Compare(clientv3.ModRevision(key), "=", kv.ModRevision),
		).Then(
			EtcdOpPut(key, "stale", 0),
		).Else(
			EtcdOpGet(key),
		).Commit()
		if err != nil {
			t.Fatal(err)
		}
		if stale.Succeeded || len(stale.Responses) != 1 || string(stale.Responses[0].GetResponseRange().Kvs[0].Value) != "v2" {
			t.Fatalf("stale CAS = %+v", stale)
		}
		missing, err := client.Get(ctx, "contract/cas/unexpected")
		if err != nil {
			t.Fatal(err)
		}
		if missing.Count != 0 {
			t.Fatal("failed CAS executed the Then branch")
		}
	})

	t.Run("TxnMissingAndEmptyRangeCompare", func(t *testing.T) {
		missing := "contract/compare/missing"
		emptyPrefix := "contract/compare/empty/"
		tests := []struct {
			name string
			cmp  clientv3.Cmp
			want bool
		}{
			{"missing-value-equal-empty", clientv3.Compare(clientv3.Value(missing), "=", ""), false},
			{"missing-value-not-equal-empty", clientv3.Compare(clientv3.Value(missing), "!=", ""), false},
			{"missing-value-greater-empty", clientv3.Compare(clientv3.Value(missing), ">", ""), false},
			{"missing-value-less-empty", clientv3.Compare(clientv3.Value(missing), "<", ""), false},
			{"missing-value-equal", clientv3.Compare(clientv3.Value(missing), "=", "value"), false},
			{"missing-value-not-equal", clientv3.Compare(clientv3.Value(missing), "!=", "value"), false},
			{"missing-version-zero", clientv3.Compare(clientv3.Version(missing), "=", 0), true},
			{"missing-version-not-zero", clientv3.Compare(clientv3.Version(missing), "!=", 0), false},
			{"missing-create-zero", clientv3.Compare(clientv3.CreateRevision(missing), "=", 0), true},
			{"missing-mod-zero", clientv3.Compare(clientv3.ModRevision(missing), "=", 0), true},
			{"empty-value-equal", clientv3.Compare(clientv3.Value(emptyPrefix), "=", "").WithPrefix(), false},
			{"empty-value-not-equal", clientv3.Compare(clientv3.Value(emptyPrefix), "!=", "").WithPrefix(), false},
			{"empty-value-greater", clientv3.Compare(clientv3.Value(emptyPrefix), ">", "").WithPrefix(), false},
			{"empty-value-less", clientv3.Compare(clientv3.Value(emptyPrefix), "<", "").WithPrefix(), false},
			{"empty-version-zero", clientv3.Compare(clientv3.Version(emptyPrefix), "=", 0).WithPrefix(), true},
			{"empty-version-not-zero", clientv3.Compare(clientv3.Version(emptyPrefix), "!=", 0).WithPrefix(), false},
			{"empty-create-zero", clientv3.Compare(clientv3.CreateRevision(emptyPrefix), "=", 0).WithPrefix(), true},
			{"empty-mod-zero", clientv3.Compare(clientv3.ModRevision(emptyPrefix), "=", 0).WithPrefix(), true},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				response, err := client.Txn(ctx).If(test.cmp).Commit()
				if err != nil {
					t.Fatal(err)
				}
				if response.Succeeded != test.want {
					t.Fatalf("Succeeded = %v, want %v", response.Succeeded, test.want)
				}
			})
		}
	})

	t.Run("WatchReplayAndCompact", func(t *testing.T) {
		key := "contract/watch/key"
		put, err := client.Put(ctx, key, "v1", 0)
		if err != nil {
			t.Fatal(err)
		}
		deleted, err := client.Delete(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		watchCtx, cancel := context.WithCancel(ctx)
		watch := client.Watch(watchCtx, "contract/watch/", clientv3.WithPrefix(), clientv3.WithRev(put.Header.Revision))
		events := awaitEtcdEvents(t, watch, 2)
		if events[0].Type != clientv3.EventTypePut || events[0].Kv.ModRevision != put.Header.Revision || string(events[0].Kv.Value) != "v1" {
			t.Fatalf("replayed PUT = %+v", events[0])
		}
		if events[1].Type != clientv3.EventTypeDelete || events[1].Kv.ModRevision != deleted.Header.Revision {
			t.Fatalf("replayed DELETE = %+v", events[1])
		}
		if _, err := client.Put(ctx, key, "live", 0); err != nil {
			t.Fatal(err)
		}
		live := awaitEtcdEvents(t, watch, 1)
		if live[0].Type != clientv3.EventTypePut || string(live[0].Kv.Value) != "live" {
			t.Fatalf("live event = %+v", live[0])
		}
		cancel()
		awaitEtcdWatchClosed(t, watch)

		revision := currentEtcdRevision(t, client)
		if err := backend.compact(revision); err != nil {
			t.Fatalf("Compact(%d): %v", revision, err)
		}
		compacted := client.Watch(ctx, "contract/watch/", clientv3.WithPrefix(), clientv3.WithRev(1))
		response := awaitEtcdWatchResponse(t, compacted)
		if !response.Canceled || response.CompactRevision != revision || !errors.Is(response.Err(), rpctypes.ErrCompacted) {
			t.Fatalf("compacted watch = %+v, err %v", response, response.Err())
		}
		awaitEtcdWatchClosed(t, compacted)
	})

	t.Run("WatchFutureRevision", func(t *testing.T) {
		key := "contract/watch/future"
		start := currentEtcdRevision(t, client) + 3
		watchCtx, cancel := context.WithCancel(ctx)
		watch := client.Watch(watchCtx, key, clientv3.WithRev(start))
		defer func() {
			cancel()
			awaitEtcdWatchClosed(t, watch)
		}()
		awaitEtcdWatchSilent(t, watch)
		if _, err := client.Put(ctx, "contract/watch/future-filler-a", "1", 0); err != nil {
			t.Fatal(err)
		}
		before, err := client.Put(ctx, "contract/watch/future-filler-b", "2", 0)
		if err != nil {
			t.Fatal(err)
		}
		if before.Header.Revision != start-1 {
			t.Fatalf("revision before target = %d, want %d", before.Header.Revision, start-1)
		}
		awaitEtcdWatchSilent(t, watch)
		target, err := client.Put(ctx, key, "arrived", 0)
		if err != nil {
			t.Fatal(err)
		}
		if target.Header.Revision != start {
			t.Fatalf("target revision = %d, want %d", target.Header.Revision, start)
		}
		events := awaitEtcdEvents(t, watch, 1)
		if events[0].Type != clientv3.EventTypePut || events[0].Kv.ModRevision != start || string(events[0].Kv.Value) != "arrived" {
			t.Fatalf("future watch event = %+v", events[0])
		}
	})

	t.Run("RevisionValidation", func(t *testing.T) {
		current := currentEtcdRevision(t, client)
		for _, revision := range []int64{-1, 0} {
			response, err := client.Get(ctx, "contract/revision/key", clientv3.WithRev(revision))
			if err != nil || response.Header.Revision != current {
				t.Fatalf("Get(WithRev(%d)) = %+v, error %v; want current revision %d", revision, response, err, current)
			}
		}
		if err := backend.compact(current + 1); !errors.Is(err, rpctypes.ErrFutureRev) {
			t.Fatalf("future Compact error = %v, want ErrFutureRev", err)
		}
		for _, revision := range []int64{0, -1} {
			if err := backend.compact(revision); !errors.Is(err, rpctypes.ErrCompacted) {
				t.Fatalf("Compact(%d) error = %v, want ErrCompacted", revision, err)
			}
		}
	})

	t.Run("LeaseKeepAliveExpiryAndRevoke", func(t *testing.T) {
		grant, err := client.Grant(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if grant.ID == 0 || grant.TTL <= 0 {
			t.Fatalf("Grant ID = %d, TTL = %d", grant.ID, grant.TTL)
		}
		key := "contract/lease/key"
		put, err := client.Put(ctx, key, "leased", grant.ID)
		if err != nil {
			t.Fatal(err)
		}
		watch := client.Watch(ctx, key, clientv3.WithRev(put.Header.Revision+1))
		keepAliveCtx, stopKeepAlive := context.WithCancel(ctx)
		keepAlive, err := client.KeepAlive(keepAliveCtx, grant.ID)
		if err != nil {
			t.Fatal(err)
		}
		awaitEtcdKeepAlive(t, keepAlive, grant.ID)
		backend.advance(1500 * time.Millisecond)
		current, err := client.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if current.Count != 1 {
			t.Fatal("lease expired while KeepAlive was active")
		}
		stopKeepAlive()
		awaitEtcdKeepAliveClosed(t, keepAlive)
		backend.advance(1500 * time.Millisecond)
		awaitEtcdKeyAbsent(t, client, key)
		events := awaitEtcdEvents(t, watch, 1)
		if events[0].Type != clientv3.EventTypeDelete || string(events[0].Kv.Key) != key {
			t.Fatalf("lease expiration event = %+v", events[0])
		}
		expiredKeepAlive, err := client.KeepAlive(ctx, grant.ID)
		if err != nil && !errors.Is(err, rpctypes.ErrLeaseNotFound) {
			t.Fatalf("KeepAlive(expired) error = %v", err)
		}
		if err == nil {
			awaitEtcdKeepAliveClosed(t, expiredKeepAlive)
		}

		revoked, err := client.Grant(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		attached, err := client.Txn(ctx).Then(EtcdOpPut(key, "revoke", revoked.ID)).Commit()
		if err != nil {
			t.Fatal(err)
		}
		if !attached.Succeeded {
			t.Fatal("lease-attaching transaction took the Else branch")
		}
		if _, err := client.Revoke(ctx, revoked.ID); err != nil {
			t.Fatal(err)
		}
		awaitEtcdKeyAbsent(t, client, key)
	})
}

func currentEtcdRevision(t *testing.T, client EtcdClient) int64 {
	t.Helper()
	response, err := client.Get(context.Background(), "contract/revision/probe")
	if err != nil {
		t.Fatal(err)
	}
	return response.Header.Revision
}

func awaitEtcdEvents(t *testing.T, watch clientv3.WatchChan, count int) []*clientv3.Event {
	t.Helper()
	events := make([]*clientv3.Event, 0, count)
	deadline := time.After(10 * time.Second)
	for len(events) < count {
		select {
		case response, ok := <-watch:
			if !ok {
				t.Fatalf("watch closed after %d/%d events", len(events), count)
			}
			if err := response.Err(); err != nil {
				t.Fatalf("watch error after %d/%d events: %v", len(events), count, err)
			}
			events = append(events, response.Events...)
		case <-deadline:
			t.Fatalf("timed out after %d/%d watch events", len(events), count)
		}
	}
	return events
}

func awaitEtcdWatchResponse(t *testing.T, watch clientv3.WatchChan) clientv3.WatchResponse {
	t.Helper()
	select {
	case response, ok := <-watch:
		if !ok {
			t.Fatal("watch closed without a response")
		}
		return response
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for watch response")
		return clientv3.WatchResponse{}
	}
}

func awaitEtcdWatchSilent(t *testing.T, watch clientv3.WatchChan) {
	t.Helper()
	select {
	case response, ok := <-watch:
		if !ok {
			t.Fatal("future watch closed before its revision")
		}
		t.Fatalf("future watch sent an early response: %+v", response)
	case <-time.After(50 * time.Millisecond):
	}
}

func awaitEtcdWatchClosed(t *testing.T, watch clientv3.WatchChan) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case response, ok := <-watch:
			if !ok {
				return
			}
			if err := response.Err(); err != nil {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for watch to close")
		}
	}
}

func awaitEtcdKeepAlive(t *testing.T, ch <-chan *clientv3.LeaseKeepAliveResponse, id clientv3.LeaseID) {
	t.Helper()
	select {
	case response, ok := <-ch:
		if !ok || response == nil || response.ID != id || response.TTL <= 0 {
			t.Fatalf("KeepAlive response = %+v, open %v", response, ok)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for KeepAlive")
	}
}

func awaitEtcdKeepAliveClosed(t *testing.T, ch <-chan *clientv3.LeaseKeepAliveResponse) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for KeepAlive to close")
		}
	}
}

func awaitEtcdKeyAbsent(t *testing.T, client EtcdClient, key string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, err := client.Get(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if response.Count == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("key %q still exists", key)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestMemoryEtcdComparisonOperators(t *testing.T) {
	clock := NewVirtualClock()
	client := NewMemoryEtcd(clock)
	defer client.Close()
	ctx := context.Background()
	if _, err := client.Put(ctx, "compare", "b", 0); err != nil {
		t.Fatal(err)
	}
	kv, err := client.Get(ctx, "compare")
	if err != nil {
		t.Fatal(err)
	}
	metadata := kv.Kvs[0]
	tests := []struct {
		name string
		cmp  clientv3.Cmp
		want bool
	}{
		{"mod-equal", clientv3.Compare(clientv3.ModRevision("compare"), "=", metadata.ModRevision), true},
		{"mod-not-equal", clientv3.Compare(clientv3.ModRevision("compare"), "!=", metadata.ModRevision+1), true},
		{"mod-greater", clientv3.Compare(clientv3.ModRevision("compare"), ">", metadata.ModRevision-1), true},
		{"mod-less", clientv3.Compare(clientv3.ModRevision("compare"), "<", metadata.ModRevision+1), true},
		{"create", clientv3.Compare(clientv3.CreateRevision("compare"), "=", metadata.CreateRevision), true},
		{"version", clientv3.Compare(clientv3.Version("compare"), ">", 0), true},
		{"value-less", clientv3.Compare(clientv3.Value("compare"), "<", "c"), true},
		{"value-greater", clientv3.Compare(clientv3.Value("compare"), ">", "a"), true},
		{"value-not-equal", clientv3.Compare(clientv3.Value("compare"), "!=", "a"), true},
		{"missing-version", clientv3.Compare(clientv3.Version("missing"), "=", 0), true},
		{"missing-value", clientv3.Compare(clientv3.Value("missing"), "=", ""), false},
		{"false", clientv3.Compare(clientv3.Version("compare"), "=", 2), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := client.Txn(ctx).If(test.cmp).Commit()
			if err != nil {
				t.Fatal(err)
			}
			if response.Succeeded != test.want {
				t.Fatalf("Succeeded = %v, want %v", response.Succeeded, test.want)
			}
		})
	}
}

func TestMemoryEtcdHistoricalGetAndAtomicRevision(t *testing.T) {
	client := NewMemoryEtcd(NewVirtualClock())
	defer client.Close()
	ctx := context.Background()
	first, err := client.Put(ctx, "history", "v1", 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Txn(ctx).Then(
		EtcdOpPut("history", "v2", 0),
		EtcdOpPut("history-other", "v2", 0),
	).Commit()
	if err != nil {
		t.Fatal(err)
	}
	if second.Header.Revision != first.Header.Revision+1 {
		t.Fatalf("transaction revisions = %d then %d", first.Header.Revision, second.Header.Revision)
	}
	old, err := client.Get(ctx, "history", clientv3.WithRev(first.Header.Revision))
	if err != nil {
		t.Fatal(err)
	}
	if string(old.Kvs[0].Value) != "v1" || old.Kvs[0].ModRevision != first.Header.Revision {
		t.Fatalf("historical Get = %+v", old.Kvs[0])
	}
	one, err := client.Get(ctx, "history")
	if err != nil {
		t.Fatal(err)
	}
	other, err := client.Get(ctx, "history-other")
	if err != nil {
		t.Fatal(err)
	}
	if one.Kvs[0].ModRevision != second.Header.Revision || other.Kvs[0].CreateRevision != second.Header.Revision {
		t.Fatalf("transaction KVs have different revisions: %+v %+v", one.Kvs[0], other.Kvs[0])
	}
}

func TestMemoryEtcdRejectedTxnDoesNotWrite(t *testing.T) {
	client := NewMemoryEtcd(NewVirtualClock())
	defer client.Close()
	ctx := context.Background()
	before := client.Revision()
	_, err := client.Txn(ctx).Then(
		EtcdOpPut("atomic/key", "must-not-commit", 0),
		EtcdOpGet("atomic/key", clientv3.WithRev(before+1)),
	).Commit()
	if !errors.Is(err, rpctypes.ErrFutureRev) {
		t.Fatalf("Commit error = %v", err)
	}
	response, err := client.Get(ctx, "atomic/key")
	if err != nil {
		t.Fatal(err)
	}
	if response.Count != 0 || client.Revision() != before {
		t.Fatalf("rejected transaction wrote state: count %d, revision %d", response.Count, client.Revision())
	}
}

func TestMemoryEtcdWatchDisconnectResumeAndErrors(t *testing.T) {
	client := NewMemoryEtcd(NewVirtualClock())
	defer client.Close()
	ctx := context.Background()
	watch := client.Watch(ctx, "watch/", clientv3.WithPrefix())
	put, err := client.Put(ctx, "watch/key", "v1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if event := awaitEtcdEvents(t, watch, 1)[0]; event.Kv.ModRevision != put.Header.Revision {
		t.Fatalf("live event = %+v", event)
	}
	client.DisconnectWatches()
	awaitEtcdWatchClosed(t, watch)

	second, err := client.Put(ctx, "watch/key", "v2", 0)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := client.Delete(ctx, "watch/key")
	if err != nil {
		t.Fatal(err)
	}
	resumed := client.Watch(ctx, "watch/", clientv3.WithPrefix(), clientv3.WithRev(second.Header.Revision))
	events := awaitEtcdEvents(t, resumed, 2)
	if events[0].Kv.ModRevision != second.Header.Revision || events[1].Kv.ModRevision != deleted.Header.Revision {
		t.Fatalf("resumed events = %+v", events)
	}
	client.FailWatches()
	response := awaitEtcdWatchResponse(t, resumed)
	if !response.Canceled || !errors.Is(response.Err(), rpctypes.ErrFutureRev) {
		t.Fatalf("failed watch = %+v, err %v", response, response.Err())
	}
	awaitEtcdWatchClosed(t, resumed)
}

func TestMemoryEtcdWatchCloseInterruptsBlockedSend(t *testing.T) {
	tests := []struct {
		name string
		stop func(*MemoryEtcd, context.CancelFunc)
	}{
		{"cancel", func(_ *MemoryEtcd, cancel context.CancelFunc) { cancel() }},
		{"disconnect", func(client *MemoryEtcd, _ context.CancelFunc) { client.DisconnectWatches() }},
		{"close", func(client *MemoryEtcd, _ context.CancelFunc) { _ = client.Close() }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewMemoryEtcd(NewVirtualClock())
			defer client.Close()
			watchCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			watch := client.Watch(watchCtx, "watch/blocked")
			if _, err := client.Put(context.Background(), "watch/blocked", "pending", 0); err != nil {
				t.Fatal(err)
			}
			time.Sleep(50 * time.Millisecond)
			test.stop(client, cancel)
			timeout := time.After(10 * time.Second)
			for {
				select {
				case _, ok := <-watch:
					if !ok {
						return
					}
				case <-timeout:
					t.Fatal("blocked watch send did not wake up")
				}
			}
		})
	}
}

func TestMemoryEtcdFaultInjection(t *testing.T) {
	client := NewMemoryEtcd(NewVirtualClock())
	defer client.Close()
	ctx := context.Background()
	injected := errors.New("injected")
	grant, err := client.Grant(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		op   MemoryEtcdOperation
		call func() error
	}{
		{MemoryEtcdGet, func() error { _, err := client.Get(ctx, "fault"); return err }},
		{MemoryEtcdPut, func() error { _, err := client.Put(ctx, "fault", "v", 0); return err }},
		{MemoryEtcdDelete, func() error { _, err := client.Delete(ctx, "fault"); return err }},
		{MemoryEtcdTxn, func() error { _, err := client.Txn(ctx).Commit(); return err }},
		{MemoryEtcdGrant, func() error { _, err := client.Grant(ctx, 1); return err }},
		{MemoryEtcdKeepAlive, func() error { _, err := client.KeepAlive(ctx, grant.ID); return err }},
		{MemoryEtcdRevoke, func() error { _, err := client.Revoke(ctx, grant.ID); return err }},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("persistent/%s", test.op), func(t *testing.T) {
			client.InjectError(test.op, injected)
			if err := test.call(); !errors.Is(err, injected) {
				t.Fatalf("error = %v", err)
			}
			client.InjectError(test.op, nil)
			client.InjectNextError(test.op, injected)
			if err := test.call(); !errors.Is(err, injected) {
				t.Fatalf("one-shot error = %v", err)
			}
			if err := test.call(); err != nil {
				t.Fatalf("one-shot fault did not clear: %v", err)
			}
		})
	}
	client.InjectError(MemoryEtcdWatch, injected)
	watch := client.Watch(ctx, "fault")
	response := awaitEtcdWatchResponse(t, watch)
	if !response.Canceled || response.Err() == nil {
		t.Fatalf("injected watch failure = %+v", response)
	}
}

func TestMemoryEtcdLeaseRevokeDeletesAllKeysInOneRevision(t *testing.T) {
	clock := NewVirtualClock()
	client := NewMemoryEtcd(clock)
	defer client.Close()
	ctx := context.Background()
	lease, err := client.Grant(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Put(ctx, "lease/a", "a", lease.ID); err != nil {
		t.Fatal(err)
	}
	before, err := client.Put(ctx, "lease/b", "b", lease.ID)
	if err != nil {
		t.Fatal(err)
	}
	watch := client.Watch(ctx, "lease/", clientv3.WithPrefix(), clientv3.WithRev(before.Header.Revision+1))
	revoked, err := client.Revoke(ctx, lease.ID)
	if err != nil {
		t.Fatal(err)
	}
	events := awaitEtcdEvents(t, watch, 2)
	for _, event := range events {
		if event.Type != clientv3.EventTypeDelete || event.Kv.ModRevision != revoked.Header.Revision {
			t.Fatalf("revoke event = %+v, revision %d", event, revoked.Header.Revision)
		}
	}
	clock.Advance(2 * time.Second)
	if client.Revision() != revoked.Header.Revision {
		t.Fatal("revoked lease expired a second time")
	}
}
