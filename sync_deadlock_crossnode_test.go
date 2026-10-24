package atomos

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestDeadlock_CrossNodeServiceCheck verifies the receiving-side check of the
// propagated wait chain: a SyncMessagingByName request whose CosmosArgs wait
// chain already contains the target ID is rejected immediately with
// ErrIDFirstSyncCallDeadlock, while a chain not containing the target
// dispatches normally.
func TestDeadlock_CrossNodeServiceCheck(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 50400, "cross_cosmos", "cross_node")
	defer cluster.close()

	client := cluster.sourceProcess.cluster.remoteCosmos["cross_node_target"].current.client
	cli := NewAtomosRemoteServiceClient(client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Spawn an atom on the target node and resolve its IDInfo.
	const atomName = "cross_check_atom"
	rsp, er := cli.SpawnAtom(ctx, &CosmosRemoteSpawnAtomReq{
		CallerId: cluster.sourceProcess.local.GetIDInfo(),
		Element:  ForTestAtomosName,
		Atom:     atomName,
	})
	if er != nil {
		t.Fatalf("SpawnAtom gRPC call failed: %v", er)
	}
	if rsp.Id == nil || rsp.Error != nil {
		t.Fatalf("SpawnAtom failed: rsp=(%v)", rsp)
	}
	target := rsp.Id

	// Control: a chain NOT containing the target dispatches normally.
	if rsp, er := cli.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId: cluster.sourceProcess.local.GetIDInfo(),
		To:       target,
		CosmosArgs: &CosmosArgs{
			WaitChain: []string{"cross_node_source::ForTestAtomos::someone_else"},
		},
		Message: "Greeting",
		Args:    toAnyPb(t, &ForTestGreetingI{Mode: 1}),
	}); er != nil {
		t.Fatalf("SyncMessagingByName with unrelated chain should succeed, got: %v", er)
	} else if rsp.Error != nil {
		t.Fatalf("SyncMessagingByName with unrelated chain should succeed, got rsp.Error: %v", rsp.Error)
	}

	// Deadlock: the chain contains the target → immediate rejection.
	// v3 contract (§8 double-write): business failure arrives as a non-OK
	// gRPC status carrying the *Error in its details.
	if rsp, er := cli.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId: cluster.sourceProcess.local.GetIDInfo(),
		To:       target,
		CosmosArgs: &CosmosArgs{
			WaitChain: []string{
				"cross_node_source::ForTestAtomos::origin",
				target.Info(),
			},
		},
		Message: "Greeting",
		Args:    toAnyPb(t, &ForTestGreetingI{Mode: 1}),
	}); er == nil {
		t.Fatalf("SyncMessagingByName with cycling chain must fail, got rsp=(%v)", rsp)
	} else if st, _ := status.FromError(er); st.Code() == codes.OK {
		t.Fatalf("expected non-OK gRPC status, got: %v", st)
	} else if restored := grpcErrorToError(er); restored.Code != ErrCodeIDFirstSyncCallDeadlock {
		t.Fatalf("expected ErrCodeIDFirstSyncCallDeadlock, got code=%d: %v", restored.Code, restored)
	} else if !strings.Contains(restored.Message, "cross-node cycle") {
		t.Fatalf("error should mention cross-node cycle: %v", restored)
	}
}

// TestDeadlock_CrossNodeCycle is the end-to-end cross-node deadlock test:
//
//	A@source --remote sync--> B@target --remote sync--> A@source
//
// Without wait-chain propagation, both mailboxes block until timeout. With it,
// the source node's service entry rejects the closing call immediately with
// ErrIDFirstSyncCallDeadlock, which travels back and unblocks the whole chain.
func TestDeadlock_CrossNodeCycle(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 50500, "xnode_cosmos", "xnode_node")
	defer cluster.close()

	const (
		nodeSource = "xnode_node_source"
		nodeTarget = "xnode_node_target"
		atomA      = "xnode_a"
		atomB      = "xnode_b"
	)

	srcElem, gerr := cluster.sourceProcess.local.getLocalElement(ForTestAtomosName)
	if gerr != nil {
		t.Fatalf("getLocalElement(source): %v", gerr)
	}
	tgtElem, gerr := cluster.targetProcess.local.getLocalElement(ForTestAtomosName)
	if gerr != nil {
		t.Fatalf("getLocalElement(target): %v", gerr)
	}
	implA := spawnTestAtom(t, srcElem, atomA)
	implB := spawnTestAtom(t, tgtElem, atomB)

	srcToTarget := cluster.sourceProcess.cluster.remoteCosmos[nodeTarget].elements[ForTestAtomosName]
	tgtToSource := cluster.targetProcess.cluster.remoteCosmos[nodeSource].elements[ForTestAtomosName]

	// A's handler remote-calls B (mode 5 → B's hook fires); B's handler
	// remote-calls back A (mode 1).
	implA.remoteHook = func(self AtomSelfID) *Error {
		id, tracker, e := srcToTarget.GetAtomID(atomB, nil)
		if e != nil {
			return asError(e).AddStack(self)
		}
		defer tracker.Release()
		_, e = id.SyncMessagingByName(self, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(5*time.Second))
		if e != nil {
			return asError(e).AddStack(self)
		}
		return nil
	}
	implB.remoteHook = func(self AtomSelfID) *Error {
		id, tracker, e := tgtToSource.GetAtomID(atomA, nil)
		if e != nil {
			return asError(e).AddStack(self)
		}
		defer tracker.Release()
		_, e = id.SyncMessagingByName(self, "Greeting", &ForTestGreetingI{Mode: 1}, ArgBaseAtomosTimeout(5*time.Second))
		if e != nil {
			return asError(e).AddStack(self)
		}
		return nil
	}

	// Trigger the cycle locally on A. The error must come back as
	// ErrIDFirstSyncCallDeadlock (not a 5s timeout), proving the chain was
	// propagated and checked across both nodes.
	atomAID, _, err := srcElem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
	if err != nil {
		t.Fatalf("GetAtomID(A): %v", err)
	}
	start := time.Now()
	_, err = atomAID.SyncMessagingByName(cluster.sourceProcess.local, "Greeting",
		&ForTestGreetingI{Mode: 5},
		ArgBaseAtomosTimeout(10*time.Second))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected cross-node deadlock error, got nil")
	}
	if CodeOf(err) != ErrCodeIDFirstSyncCallDeadlock {
		t.Fatalf("expected ErrCodeIDFirstSyncCallDeadlock, got code=%d: %v", CodeOf(err), err)
	}
	if !strings.Contains(err.Error(), "cross-node cycle") {
		t.Fatalf("error should mention cross-node cycle: %v", err)
	}
	if elapsed > 9*time.Second {
		t.Fatalf("cycle should be detected immediately, took %v: %v", elapsed, err)
	}
	t.Logf("cross-node cycle detected in %v: %v", elapsed, err)
}
