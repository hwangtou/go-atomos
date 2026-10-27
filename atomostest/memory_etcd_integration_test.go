//go:build integration

package atomostest

import (
	"context"
	"net/url"
	"testing"
	"time"

	internaletcd "github.com/hwangtou/go-atomos/internal/etcd"
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
	client := internaletcd.NewClientV3(raw)
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
