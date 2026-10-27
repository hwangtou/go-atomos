//go:build integration

package atomos

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestEtcdInfoPublishRetriesLatestStateAfterOutage(t *testing.T) {
	clientPort := reserveLocalPort(t)
	dir := t.TempDir()
	e1 := startEmbeddedEtcdAt(t, dir, clientPort)
	endpoint := fmt.Sprintf("127.0.0.1:%d", clientPort)
	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{endpoint}, DialTimeout: 5 * time.Second})
	if err != nil {
		e1.Close()
		t.Fatalf("client: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	oldTimeout, oldRegistrationBackoff, oldPublishBackoff, oldAttempts := etcdOpTimeout, etcdRegistrationBackoff, etcdInfoPublishBackoff, etcdRegistrationAttempts
	etcdOpTimeout = 300 * time.Millisecond
	etcdRegistrationBackoff = 50 * time.Millisecond
	etcdInfoPublishBackoff = 50 * time.Millisecond
	etcdRegistrationAttempts = 100
	t.Cleanup(func() {
		etcdOpTimeout = oldTimeout
		etcdRegistrationBackoff = oldRegistrationBackoff
		etcdInfoPublishBackoff = oldPublishBackoff
		etcdRegistrationAttempts = oldAttempts
	})

	const cosmos, node = "infoCosmos", "infoNode"
	const version int64 = 1000
	process, _ := newKeepaliveClusterProcess(t, cli, cosmos, node, version)
	if err := process.etcdStartUpTryLockingVersionNodeLock(cli, node, version); err != nil {
		t.Fatalf("startup lock: %v", err)
	}
	if err := process.trySettingClusterToCurrentAndKeepalive(); err != nil {
		t.Fatalf("registration: %v", err)
	}

	e1.Close()
	if err := process.updateNodeState(ClusterNodeState_Started); err != nil {
		t.Fatalf("queue Started: %v", err)
	}
	if err := process.updateNodeState(ClusterNodeState_Draining); err != nil {
		t.Fatalf("queue Draining: %v", err)
	}
	waitAdmissionCondition(t, func() bool {
		return !process.etcdRuntime().registrationHealthy.Load()
	}, "publisher did not observe the outage")

	e2 := startEmbeddedEtcdAt(t, dir, clientPort)
	t.Cleanup(e2.Close)
	key := etcdCosmosNodeVersionURI(cosmos, node, version)
	deadline := time.Now().Add(20 * time.Second)
	for {
		buf, _, getErr := etcdGet(cli, key)
		if getErr == nil && buf != nil {
			info := &CosmosNodeVersionInfo{}
			if err := json.Unmarshal(buf, info); err != nil {
				t.Fatalf("unmarshal node info: %v", err)
			}
			if info.State == ClusterNodeState_Draining {
				if info.Node != node || info.StartupId != process.startupID || info.Elements[ForTestAtomosName] == nil {
					t.Fatalf("Draining update is not the latest complete state: %+v", info)
				}
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("latest Draining state was not republished: buf=%s err=%v", buf, getErr)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := process.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
