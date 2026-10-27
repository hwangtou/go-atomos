package atomos

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestRemoteTryKillingRespondsAndStopsDebugServer(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 56100, "remote_kill_review", "remote_kill_review_node")
	defer cluster.close()

	target := cluster.targetProcess
	ds := startDebugServer(target, "127.0.0.1:0")
	if ds == nil {
		t.Fatal("start debug server")
	}
	target.mutex.Lock()
	target.debugServer = ds
	target.mutex.Unlock()
	debugURL := "http://" + ds.addr() + "/healthz"

	nodeName := "remote_kill_review_node_target"
	remote := cluster.sourceProcess.cluster.remoteCosmos[nodeName]
	conn := remote.getCurrentClient()
	if conn == nil {
		t.Fatal("target has no client connection")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rsp, err := NewAtomosRemoteServiceClient(conn).TryKilling(ctx, &CosmosRemoteTryKillingReq{})
	if err != nil {
		t.Fatalf("TryKilling response must arrive before teardown: %v", err)
	}
	if rsp.Error != nil {
		t.Fatalf("TryKilling failed: %v", rsp.Error)
	}

	deadline := time.Now().Add(5 * time.Second)
	for target.IsRunning() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if target.IsRunning() {
		t.Fatal("target did not finish remote shutdown")
	}

	client := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, requestErr := client.Get(debugURL)
		if requestErr != nil {
			return
		}
		_ = resp.Body.Close()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("debug server remained reachable after remote shutdown")
}

func TestRemoteKillAtomRejectsStaleInstance(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 51200, "remote_kill_stale", "remote_kill_stale_node")
	defer cluster.close()

	target := cluster.targetProcess
	elem, ierr := target.local.getLocalElement(ForTestAtomosName)
	if ierr != nil {
		t.Fatal(ierr)
	}
	oldID, _, err := elem.SpawnAtom(target.local, "stale", &Config{Cosmos: "arg"}, nil)
	if err != nil {
		t.Fatalf("spawn old instance: %v", err)
	}
	if err := oldID.Kill(target.local, ArgBaseAtomosWaitKilled()); err != nil {
		t.Fatalf("kill old instance: %v", err)
	}
	newID, _, err := elem.SpawnAtom(target.local, "stale", &Config{Cosmos: "arg"}, nil)
	if err != nil {
		t.Fatalf("spawn replacement: %v", err)
	}

	svc := &atomosRemoteService{process: target}
	rsp, transportErr := svc.killAtom(context.Background(), &CosmosRemoteKillAtomReq{
		CallerId: cluster.sourceProcess.local.GetIDInfo(),
		Id:       oldID.GetIDInfo(),
	})
	if transportErr != nil {
		t.Fatalf("killAtom transport: %v", transportErr)
	}
	if rsp.Error == nil || rsp.Error.Code != ErrCodeAtomInstanceMismatch {
		t.Fatalf("stale KillAtom error = %v, want ErrAtomInstanceMismatch", rsp.Error)
	}
	if newID.State() == BaseAtomosHalt {
		t.Fatal("stale remote handle killed the replacement instance")
	}
}
