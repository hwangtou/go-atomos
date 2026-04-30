package atomos

import (
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var (
	testEtcdEndpoints = []string{"localhost:2379"}
)

func newClient() *clientv3.Client {
	cli, er := clientv3.New(clientv3.Config{
		Endpoints:   testEtcdEndpoints,
		DialTimeout: etcdDialTime * time.Second,
	})
	if er != nil {
		panic(er)
	}
	return cli
}

func Test_etcdKeepalive_Success(t *testing.T) {
	cli := newClient()
	defer func() {
		if er := cli.Close(); er != nil {
			t.Errorf("Expected no error, got %v", er)
		}
	}()

	key := "testKey"
	value := "testValue"
	ttl := int64(5)
	lease, keepAliveCh, err := etcdKeepalive(cli, key, value, ttl)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if lease == nil {
		t.Errorf("Expected lease, got nil")
	}
	if keepAliveCh == nil {
		t.Errorf("Expected keepAliveCh, got nil")
	}
}

func Test_etcdPut_Success(t *testing.T) {
	cli := newClient()
	defer func() {
		if er := cli.Close(); er != nil {
			t.Errorf("Expected no error, got %v", er)
		}
	}()

	key := "testKey"
	value := []byte("testValue")
	err := etcdPut(cli, key, value)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}
