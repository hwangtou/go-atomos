//go:build integration

package atomos

import (
	"net/url"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

// embeddedEtcd starts an in-process etcd server for integration tests.
// It listens on ephemeral ports and returns a ready client + cleanup func.
//
// This avoids the previous dependency on an external etcd at localhost:2379
// (which made util_etcd_test.go unrunnable in CI). Tests that need real lease /
// watch / txn semantics use this instead of mocking.
type embeddedEtcd struct {
	server   *embed.Etcd
	client   *clientv3.Client
	endpoint string
}

func newEmbeddedEtcd(t *testing.T) *embeddedEtcd {
	t.Helper()
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	// :0 = ephemeral port, avoids clashes with parallel tests / real etcd.
	cfg.ListenClientUrls = []url.URL{{Scheme: "http", Host: "127.0.0.1:0"}}
	cfg.ListenPeerUrls = []url.URL{{Scheme: "http", Host: "127.0.0.1:0"}}
	cfg.LogLevel = "error" // silence etcd's chatty logs

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("embedded_etcd: start failed: %v", err)
	}

	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(15 * time.Second):
		e.Close()
		t.Fatal("embedded_etcd: ready timeout")
	}

	endpoint := e.Clients[0].Addr().String()
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		t.Fatalf("embedded_etcd: client failed: %v", err)
	}

	ee := &embeddedEtcd{server: e, client: cli, endpoint: endpoint}
	t.Cleanup(func() {
		cli.Close()
		e.Close()
	})
	return ee
}
