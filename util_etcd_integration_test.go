//go:build integration

package atomos

import (
	"context"
	"testing"
	"time"
)

// Round-trip tests of the raw etcd helpers against an in-process etcd server
// (embedded_etcd_test.go). Lease/keepalive semantics and version behavior
// cannot be asserted meaningfully against mocks or dead endpoints.

func Test_etcdPutGetDeleteRoundTrip(t *testing.T) {
	cli := newEmbeddedEtcd(t).client
	key := "/atomos/test/roundtrip"

	// First put creates the key at version 1.
	if err := etcdPut(cli, key, []byte("v1")); err != nil {
		t.Fatalf("etcdPut v1 failed: %v", err)
	}
	val, ver, err := etcdGet(cli, key)
	if err != nil {
		t.Fatalf("etcdGet after first put failed: %v", err)
	}
	if string(val) != "v1" {
		t.Errorf("Expected v1, got (%s)", val)
	}
	if ver != 1 {
		t.Errorf("Expected version 1 after first put, got %d", ver)
	}

	// Overwriting bumps the etcd version; the helper's contract is
	// last-write-wins (no compare-and-swap).
	if err := etcdPut(cli, key, []byte("v2")); err != nil {
		t.Fatalf("etcdPut v2 failed: %v", err)
	}
	val, ver, err = etcdGet(cli, key)
	if err != nil {
		t.Fatalf("etcdGet after second put failed: %v", err)
	}
	if string(val) != "v2" || ver != 2 {
		t.Errorf("Expected v2/version 2, got (%s)/%d", val, ver)
	}

	// Missing key: nil value, nil error (helper distinguishes miss from failure).
	if val, _, err = etcdGet(cli, "/atomos/test/no-such-key"); err != nil {
		t.Errorf("etcdGet on missing key: expected nil error, got %v", err)
	} else if val != nil {
		t.Errorf("etcdGet on missing key: expected nil value, got (%s)", val)
	}

	// Delete is idempotent from the caller's perspective: no error, and the
	// key reads back as missing.
	if err := etcdDelete(cli, key); err != nil {
		t.Fatalf("etcdDelete failed: %v", err)
	}
	if val, _, err = etcdGet(cli, key); err != nil {
		t.Fatalf("etcdGet after delete failed: %v", err)
	} else if val != nil {
		t.Errorf("etcdGet after delete: expected nil value, got (%s)", val)
	}
}

func Test_etcdKeepalive_Success(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	key := "/atomos/test/keepalive"

	lease, keepAliveCh, err := etcdKeepalive(ee.client, key, "testValue", 5)
	if err != nil {
		t.Fatalf("etcdKeepalive failed: %v", err)
	}
	if lease == nil || lease.ID == 0 {
		t.Fatalf("Expected non-zero lease, got %+v", lease)
	}
	if keepAliveCh == nil {
		t.Fatal("Expected keepAliveCh, got nil")
	}

	// The registered value is immediately visible under the lease.
	if val, _, err := etcdGet(ee.client, key); err != nil || string(val) != "testValue" {
		t.Errorf("etcdGet under lease: val=(%s), err=(%v); expected testValue, nil", val, err)
	}

	// The client renews at ~TTL/3, so the first response must arrive well
	// before the lease itself would expire.
	select {
	case resp := <-keepAliveCh:
		if resp == nil {
			t.Fatal("keepalive channel delivered nil response")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("No keepalive response within 10s; lease renewal not working")
	}

	// Revoking the lease must remove the registered key — the property the
	// cluster relies on to expire dead nodes.
	if _, er := ee.client.Revoke(context.Background(), lease.ID); er != nil {
		t.Fatalf("Revoke failed: %v", er)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		val, _, err := etcdGet(ee.client, key)
		if err != nil {
			t.Fatalf("etcdGet after revoke failed: %v", err)
		}
		if val == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Key still present 5s after lease revoke")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
