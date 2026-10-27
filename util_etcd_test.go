package atomos

import (
	"net"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Fail-fast regression tests against an unreachable endpoint.
//
// History: these helpers used to be tested against a real etcd at
// localhost:2379 with no build tag. When no etcd was running, the etcd
// client's WaitForReady retry policy made etcdKeepalive block forever inside
// cli.Grant, hanging the whole test suite at its 600s timeout (and every
// alphabetically later util_* test with it). etcdKeepalive now bounds its
// one-shot RPCs with etcdOpTimeout; these tests pin that behavior without
// needing any etcd at all. Real round-trip semantics live in
// util_etcd_integration_test.go (build tag: integration).

// newDeadEndpointClient returns a client pointed at a loopback port where
// nothing listens: connections are refused immediately and the endpoint never
// becomes reachable, which is exactly the "etcd is down" production scenario.
func newDeadEndpointClient(t *testing.T) EtcdClient {
	t.Helper()
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"127.0.0.1:1"},
		DialTimeout: etcdDialTime * time.Second,
	})
	if err != nil {
		t.Fatalf("client create failed: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return NewEtcdClientV3(cli)
}

func Test_etcdKeepalive_UnreachableFailsFast(t *testing.T) {
	oldTimeout := etcdOpTimeout
	etcdOpTimeout = 500 * time.Millisecond
	t.Cleanup(func() { etcdOpTimeout = oldTimeout })

	start := time.Now()
	_, _, err := etcdKeepalive(newDeadEndpointClient(t), "testKey", "testValue", 5)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Expected error against unreachable etcd, got nil")
	}
	if code := CodeOf(err); code != ErrCodeCosmosEtcdKeepaliveFailed {
		t.Errorf("Expected ErrCodeCosmosEtcdKeepaliveFailed, got code=(%d), err=(%v)", code, err)
	}
	// Generous upper bound: the op timeout is 500ms; anything close to the
	// old behavior (hang until test-timeout) fails here.
	if elapsed > 5*time.Second {
		t.Errorf("etcdKeepalive blocked %v against a dead endpoint; expected it to fail fast", elapsed)
	}
}

func Test_etcdHelpers_UnreachableReturnCodedErrors(t *testing.T) {
	cli := newDeadEndpointClient(t)

	if err := etcdPut(cli, "testKey", []byte("testValue")); CodeOf(err) != ErrCodeCosmosEtcdPutFailed {
		t.Errorf("etcdPut: expected ErrCodeCosmosEtcdPutFailed, got (%v)", err)
	}
	if _, _, err := etcdGet(cli, "testKey"); CodeOf(err) != ErrCodeCosmosEtcdGetFailed {
		t.Errorf("etcdGet: expected ErrCodeCosmosEtcdGetFailed, got (%v)", err)
	}
	if err := etcdDelete(cli, "testKey"); CodeOf(err) != ErrCodeCosmosEtcdDeleteFailed {
		t.Errorf("etcdDelete: expected ErrCodeCosmosEtcdDeleteFailed, got (%v)", err)
	}
}

// newTarpitClient returns a client pointed at a local listener that ACCEPTS
// TCP connections but never answers. Unlike the dead endpoint above (connection
// refused — something actively fails the dial), here the network is "up": the
// connection succeeds, so gRPC keeps waiting for an HTTP/2 response that never
// comes. This is the unresponsive-server scenario: a hung etcd, or a
// middlebox that completes the handshake then drops all packets. Only the
// helper's own RPC deadline can break the wait — WaitForReady never will.
func newTarpitClient(t *testing.T) EtcdClient {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("tarpit listen failed: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	// Hold every accepted connection open (referenced, so finalizers cannot
	// close them) without ever reading or writing.
	var held []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			held = append(held, c)
		}
	}()
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{ln.Addr().String()},
		DialTimeout: etcdDialTime * time.Second,
	})
	if err != nil {
		t.Fatalf("client create failed: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return NewEtcdClientV3(cli)
}

func Test_etcdKeepalive_HungEndpointFailsFast(t *testing.T) {
	oldTimeout := etcdOpTimeout
	etcdOpTimeout = 500 * time.Millisecond
	t.Cleanup(func() { etcdOpTimeout = oldTimeout })

	start := time.Now()
	_, _, err := etcdKeepalive(newTarpitClient(t), "testKey", "testValue", 5)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Expected error against unresponsive etcd, got nil")
	}
	if code := CodeOf(err); code != ErrCodeCosmosEtcdKeepaliveFailed {
		t.Errorf("Expected ErrCodeCosmosEtcdKeepaliveFailed, got code=(%d), err=(%v)", code, err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("etcdKeepalive blocked %v against a hung endpoint; expected it to fail fast", elapsed)
	}
}

func Test_etcdHelpers_HungEndpointReturnCodedErrors(t *testing.T) {
	cli := newTarpitClient(t)

	if err := etcdPut(cli, "testKey", []byte("testValue")); CodeOf(err) != ErrCodeCosmosEtcdPutFailed {
		t.Errorf("etcdPut: expected ErrCodeCosmosEtcdPutFailed, got (%v)", err)
	}
	if _, _, err := etcdGet(cli, "testKey"); CodeOf(err) != ErrCodeCosmosEtcdGetFailed {
		t.Errorf("etcdGet: expected ErrCodeCosmosEtcdGetFailed, got (%v)", err)
	}
	if err := etcdDelete(cli, "testKey"); CodeOf(err) != ErrCodeCosmosEtcdDeleteFailed {
		t.Errorf("etcdDelete: expected ErrCodeCosmosEtcdDeleteFailed, got (%v)", err)
	}
}

func Test_etcdVersionLock_HungEndpointHasBoundedRPCs(t *testing.T) {
	oldTimeout, oldAttempts, oldBackoff := etcdOpTimeout, etcdVersionLockAttempts, etcdVersionLockBackoff
	etcdOpTimeout, etcdVersionLockAttempts, etcdVersionLockBackoff = 100*time.Millisecond, 2, time.Millisecond
	t.Cleanup(func() {
		etcdOpTimeout, etcdVersionLockAttempts, etcdVersionLockBackoff = oldTimeout, oldAttempts, oldBackoff
	})
	p, err := newCosmosProcess("timeout_cosmos", "timeout_node", &reproLogger{})
	if err != nil {
		t.Fatal(err)
	}
	runnable := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: "timeout_cosmos", Node: "timeout_node"})
	if err := p.Start(runnable); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop() })
	p.cluster.etcdClient = newTarpitClient(t)
	start := time.Now()
	if err := p.etcdStartUpTryLockingVersionNodeLock(p.cluster.etcdClient, "timeout_node", 1); err == nil {
		t.Fatal("lock against a hung endpoint unexpectedly succeeded")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("version-lock RPCs were not bounded: %v", elapsed)
	}
}
