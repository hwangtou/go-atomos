package atomos

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestCosmosRemote_LifeCycle(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 50100, "test_cosmos", "test_node")
	defer cluster.close()

	client := cluster.sourceProcess.cluster.remoteCosmos["test_node_target"].current.client
	cli := NewAtomosRemoteServiceClient(client)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// GetAtomID test not existing Element
	if rsp, er := cli.GetAtomID(ctx, &CosmosRemoteGetAtomIDReq{
		Element: "",
		Atom:    "",
	}); er != nil {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call failed. err=(%v)", er)
	} else if rsp == nil || rsp.Id != nil || rsp.Error == nil {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call returned invalid response: rsp=(%v)", rsp)
	} else if rsp.Error.Code != ErrMainElementNotFound {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call returned invalid error code: rsp.Error.Code=(%v)", rsp.Error.Code)
	} else {
		t.Logf("CosmosRemote: GetAtomID gRPC call succeeded, rsp=(%v)", rsp)
	}

	// GetAtomID test get existing Element ForTest
	const testAtom = "test_atom_1"
	if rsp, er := cli.GetAtomID(ctx, &CosmosRemoteGetAtomIDReq{
		Element: ForTestAtomosName,
		Atom:    testAtom,
	}); er != nil {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call failed. err=(%v)", er)
	} else if rsp == nil || rsp.Id != nil || rsp.Error == nil {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call returned invalid response: rsp=(%v)", rsp)
	} else if rsp.Error.Code != ErrAtomNotExists {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call returned invalid error code: rsp.Error.Code=(%v)", rsp.Error.Code)
	} else {
		t.Logf("CosmosRemote: GetAtomID gRPC call succeeded, rsp=(%v)", rsp)
	}

	// SpawnAtomID test existing Element ForTest
	if rsp, er := cli.SpawnAtom(ctx, &CosmosRemoteSpawnAtomReq{
		CallerId:   cluster.sourceProcess.local.GetIDInfo(),
		Element:    ForTestAtomosName,
		Atom:       testAtom,
		Args:       nil,
		CosmosArgs: nil,
	}); er != nil {
		t.Fatalf("CosmosRemote: SpawnAtomID gRPC call failed. err=(%v)", er)
	} else if rsp == nil || rsp.Id == nil || rsp.Error != nil {
		t.Fatalf("CosmosRemote: SpawnAtomID gRPC call returned invalid response: rsp=(%v)", rsp)
	} else if rsp.Id.Cosmos != "test_cosmos" || rsp.Id.Node != "test_node_target" || rsp.Id.Element != ForTestAtomosName || rsp.Id.Atom != testAtom {
		t.Fatalf("CosmosRemote: SpawnAtomID gRPC call returned invalid ID info: rsp.Id=(%v)", rsp.Id)
	} else {
		t.Logf("CosmosRemote: SpawnAtomID gRPC call succeeded, rsp=(%v)", rsp)
	}

	// Check GetAtomID again to see if the Atom now exists
	var atomIDInfo *IDInfo
	if rsp, er := cli.GetAtomID(ctx, &CosmosRemoteGetAtomIDReq{
		Element: ForTestAtomosName,
		Atom:    testAtom,
	}); er != nil {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call failed. err=(%v)", er)
	} else if rsp == nil || rsp.Id == nil || rsp.Error != nil {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call returned invalid response: rsp=(%v)", rsp)
	} else if rsp.Id.Cosmos != "test_cosmos" || rsp.Id.Node != "test_node_target" || rsp.Id.Element != ForTestAtomosName || rsp.Id.Atom != testAtom {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call returned invalid ID info: rsp.Id=(%v)", rsp.Id)
	} else if rsp.Id.InstanceId == 0 {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call returned zero instance_id: rsp.Id=(%v)", rsp.Id)
	} else {
		atomIDInfo = rsp.Id
		t.Logf("CosmosRemote: GetAtomID gRPC call succeeded, rsp=(%v)", rsp)
	}

	// Sync Messaging test (use the resolved IDInfo so it carries instance_id)
	if rsp, er := cli.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId: cluster.sourceProcess.local.GetIDInfo(),
		To:       atomIDInfo,
		CosmosArgs: nil,
		Message:    "Greeting",
		Args:       toAnyPb(t, &ForTestGreetingI{Mode: 1}),
	}); er != nil {
		t.Fatalf("CosmosRemote: SyncMessagingByName gRPC call failed. err=(%v)", er)
	} else if rsp == nil || rsp.Reply == nil || rsp.Error != nil {
		t.Fatalf("CosmosRemote: SyncMessagingByName gRPC call returned invalid response: rsp=(%v)", rsp)
	} else {
		var greetingO ForTestGreetingO
		if er = rsp.Reply.UnmarshalTo(&greetingO); er != nil {
			t.Fatalf("CosmosRemote: SyncMessagingByName gRPC call reply unmarshal failed. err=(%v)", er)
		}
		t.Logf("CosmosRemote: SyncMessagingByName gRPC call succeeded, reply=(%v)", &greetingO)
	}

	// Async Messaging test
	callbackCh := make(chan struct{})
	startupID, asyncID := cluster.sourceProcess.local.asyncSet(func(out proto.Message, err *Error) {
		t.Logf("CosmosRemote: AsyncMessagingByName callback executed. out=(%v) err=(%v)", out, err)
		callbackCh <- struct{}{}
	})
	if startupID == 0 || asyncID == 0 {
		t.Fatalf("CosmosRemote: AsyncMessagingByName asyncSet failed.")
	}
	if rsp, er := cli.AsyncMessagingByName(ctx, &CosmosRemoteAsyncMessagingByNameReq{
		CallerId: cluster.sourceProcess.local.GetIDInfo(),
		ToId:     atomIDInfo,
		CosmosArgs: nil,
		StartupId:  startupID,
		AsyncId:    asyncID,
		Message:    "Greeting",
		Args:       toAnyPb(t, &ForTestGreetingI{Mode: 1}),
	}); er != nil {
		t.Fatalf("CosmosRemote: AsyncMessagingByName gRPC call failed. err=(%v)", er)
	} else if rsp == nil || rsp.Error != nil {
		t.Fatalf("CosmosRemote: AsyncMessagingByName gRPC call returned invalid response: rsp=(%v)", rsp)
	} else {
		t.Logf("CosmosRemote: AsyncMessagingByName gRPC call succeeded, rsp=(%v)", rsp)
	}

	<-callbackCh
	t.Logf("CosmosRemote: AsyncMessagingByName callback received.")

	// Wait a moment to let logs flush.
	//<-time.After(time.Minute)
	<-time.After(time.Millisecond)
}

func toAnyPb(t *testing.T, msg proto.Message) *anypb.Any {
	arg, er := anypb.New(msg)
	if er != nil {
		t.Fatalf("CosmosRemote: toAnyPb. err=(%v)", er)
	}
	return arg
}

// TestCosmosRemote_InstanceMismatchOnRespawn covers M5-0: a remote caller that
// holds an IDInfo resolved before a halt+respawn must get ErrAtomInstanceMismatch
// when it next calls, because the live instance's instance_id changed. Rebinding
// (re-GetAtomID) yields the new instance_id and succeeds.
func TestCosmosRemote_InstanceMismatchOnRespawn(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 50200, "test_cosmos", "mm_node")
	defer cluster.close()

	client := cluster.sourceProcess.cluster.remoteCosmos["mm_node_target"].current.client
	cli := NewAtomosRemoteServiceClient(client)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	const atomName = "test_mismatch_atom"

	// Spawn an atom on the target node and capture its instance_id.
	var staleID *IDInfo
	if rsp, er := cli.SpawnAtom(ctx, &CosmosRemoteSpawnAtomReq{
		CallerId:   cluster.sourceProcess.local.GetIDInfo(),
		Element:    ForTestAtomosName,
		Atom:       atomName,
		Args:       nil,
		CosmosArgs: nil,
	}); er != nil {
		t.Fatalf("SpawnAtom failed: %v", er)
	} else if rsp == nil || rsp.Id == nil || rsp.Error != nil {
		t.Fatalf("SpawnAtom invalid rsp: %v", rsp)
	} else {
		staleID = rsp.Id
		if staleID.InstanceId == 0 {
			t.Fatalf("SpawnAtom returned zero instance_id: %v", staleID)
		}
	}

	// Kill the atom on the target node directly (simulate halt), then respawn it
	// under the same name — the new instance gets a NEW instance_id.
	elem, err := cluster.targetProcess.local.getLocalElement(ForTestAtomosName)
	if err != nil {
		t.Fatalf("getLocalElement on target: %v", err)
	}
	targetAtom, _, err := elem.GetAtomID(atomName, nil, false)
	if err != nil {
		t.Fatalf("target GetAtomID: %v", err)
	}
	oldImpl := targetAtom.(*AtomLocal).atomos.instance.(*testRunnableAtom)
	oldImpl.haltNotify = make(chan struct{}, 1)
	targetAtom.(*AtomLocal).KillSelf()
	<-oldImpl.haltNotify
	// Respawn on the target (new instance_id).
	if _, _, err := elem.elementAtomSpawn(nil, atomName, nil, elem.elemImpl, nil, NewIDTrackerInfoFromLocalGoroutine(3), false, false); err != nil {
		t.Fatalf("target respawn failed: %v", err)
	}

	// A sync call with the STALE instance_id must be rejected.
	if rsp, er := cli.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId:   cluster.sourceProcess.local.GetIDInfo(),
		To:         staleID, // carries the old instance_id
		CosmosArgs: nil,
		Message:    "Greeting",
		Args:       toAnyPb(t, &ForTestGreetingI{Mode: 1}),
	}); er != nil {
		t.Fatalf("Sync call transport error: %v", er)
	} else if rsp == nil || rsp.Error == nil || rsp.Error.Code != ErrAtomInstanceMismatch {
		t.Fatalf("Expected ErrAtomInstanceMismatch with stale instance_id, got rsp=%v", rsp)
	}

	// Rebind: re-GetAtomID yields the fresh instance_id.
	var freshID *IDInfo
	if rsp, er := cli.GetAtomID(ctx, &CosmosRemoteGetAtomIDReq{
		Element: ForTestAtomosName, Atom: atomName,
	}); er != nil {
		t.Fatalf("rebind GetAtomID transport error: %v", er)
	} else if rsp == nil || rsp.Id == nil || rsp.Error != nil {
		t.Fatalf("rebind GetAtomID invalid rsp: %v", rsp)
	} else {
		freshID = rsp.Id
		if freshID.InstanceId == staleID.InstanceId {
			t.Fatalf("rebind did not yield a new instance_id: stale=%d fresh=%d", staleID.InstanceId, freshID.InstanceId)
		}
	}

	// A sync call with the FRESH instance_id must succeed.
	if rsp, er := cli.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId:   cluster.sourceProcess.local.GetIDInfo(),
		To:         freshID,
		CosmosArgs: nil,
		Message:    "Greeting",
		Args:       toAnyPb(t, &ForTestGreetingI{Mode: 1}),
	}); er != nil {
		t.Fatalf("fresh sync transport error: %v", er)
	} else if rsp == nil || rsp.Error != nil {
		t.Fatalf("fresh sync should succeed, got rsp=%v", rsp)
	}
}

// TestCosmosRemote_InstanceIDZeroRejected covers the strict policy: a remote
// call whose To.InstanceId is 0 must be rejected (legacy/unset client).
func TestCosmosRemote_InstanceIDZeroRejected(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 50300, "test_cosmos", "zr_node")
	defer cluster.close()

	client := cluster.sourceProcess.cluster.remoteCosmos["zr_node_target"].current.client
	cli := NewAtomosRemoteServiceClient(client)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// Spawn so a live instance exists, then call with instance_id explicitly 0.
	if _, er := cli.SpawnAtom(ctx, &CosmosRemoteSpawnAtomReq{
		CallerId:   cluster.sourceProcess.local.GetIDInfo(),
		Element:    ForTestAtomosName,
		Atom:       "test_zero_atom",
		Args:       nil,
		CosmosArgs: nil,
	}); er != nil {
		t.Fatalf("SpawnAtom failed: %v", er)
	}

	if rsp, er := cli.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId: cluster.sourceProcess.local.GetIDInfo(),
		To: &IDInfo{
			Type: IDType_Atom, Cosmos: "test_cosmos", Node: "zr_node_target",
			Element: ForTestAtomosName, Atom: "test_zero_atom",
			// InstanceId intentionally left 0.
		},
		CosmosArgs: nil,
		Message:    "Greeting",
		Args:       toAnyPb(t, &ForTestGreetingI{Mode: 1}),
	}); er != nil {
		t.Fatalf("Sync transport error: %v", er)
	} else if rsp == nil || rsp.Error == nil || rsp.Error.Code != ErrAtomInstanceMismatch {
		t.Fatalf("Expected ErrAtomInstanceMismatch for instance_id=0, got rsp=%v", rsp)
	}
}

// TestWithRebind_RetriesOnInstanceMismatch covers M5-2: WithRebind automatically
// re-resolves the ID and retries once when a call fails with a rebind-triggering
// error (here ErrAtomInstanceMismatch). The cluster integration of the mismatch
// signal itself is covered by TestCosmosRemote_InstanceMismatchOnRespawn (M5-0);
// this test exercises the WithRebind retry logic directly with a stub ID/call.
func TestWithRebind_RetriesOnInstanceMismatch(t *testing.T) {
	calls := 0
	out, err := WithRebind(func() (ID, *Error) {
		return stubID{}, nil
	}, func(id ID) (string, *Error) {
		calls++
		if calls == 1 {
			return "", NewError(ErrAtomInstanceMismatch, "stale").AddStack(nil)
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("WithRebind failed: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected out=ok, got %q", out)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (initial + 1 rebind retry), got %d", calls)
	}
}

// TestWithRebind_NoRetryOnSuccess verifies WithRebind calls once and returns on
// success without retrying.
func TestWithRebind_NoRetryOnSuccess(t *testing.T) {
	calls := 0
	out, err := WithRebind(func() (ID, *Error) {
		return stubID{}, nil
	}, func(id ID) (string, *Error) {
		calls++
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("WithRebind failed: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected out=ok, got %q", out)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call (no retry on success), got %d", calls)
	}
}

// TestWithRebind_NoRetryOnUnrelatedError verifies WithRebind does NOT retry on a
// non-rebind error (it returns the error directly after one call).
func TestWithRebind_NoRetryOnUnrelatedError(t *testing.T) {
	sentinel := NewError(ErrFrameworkInternalError, "sentinel business error").AddStack(nil)
	calls := 0
	_, err := WithRebind(func() (ID, *Error) {
		return stubID{}, nil
	}, func(id ID) (string, *Error) {
		calls++
		return "", sentinel
	})
	if err == nil {
		t.Fatal("expected the sentinel error back")
	}
	if err.Code != sentinel.Code {
		t.Fatalf("expected sentinel error code %d, got %d", sentinel.Code, err.Code)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call (no retry on unrelated error), got %d", calls)
	}
}

// TestWithRebind_ResolveErrorNoCall verifies WithRebind returns the resolve
// error without invoking call.
func TestWithRebind_ResolveErrorNoCall(t *testing.T) {
	resolveErr := NewError(ErrCosmosRemoteConnectFailed, "no node").AddStack(nil)
	calls := 0
	_, err := WithRebind(func() (ID, *Error) {
		return nil, resolveErr
	}, func(id ID) (string, *Error) {
		calls++
		return "", nil
	})
	if err == nil || err.Code != resolveErr.Code {
		t.Fatalf("expected resolve error back, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected 0 calls when resolve fails, got %d", calls)
	}
}

// stubID is a minimal ID used to exercise WithRebind's control flow without a
// full cosmos/cluster setup. It is NOT ReleasableID (no Release method), so
// releaseID is a no-op on it — matching the remote-ID behavior.
type stubID struct{}

func (stubID) GetIDInfo() *IDInfo       { return nil }
func (stubID) String() string           { return "stub" }
func (stubID) Cosmos() CosmosNode       { return nil }
func (stubID) State() BaseAtomosState   { return 0 }
func (stubID) IdleTime() time.Duration  { return 0 }
func (stubID) SyncMessagingByName(ID, string, proto.Message, []ArgsForBaseAtomos) (proto.Message, *Error) {
	return nil, nil
}
func (stubID) AsyncMessagingByName(ID, string, proto.Message, func(proto.Message, *Error), []ArgsForBaseAtomos) *Error {
	return nil
}
func (stubID) asyncSet(func(proto.Message, *Error)) (uint64, uint64) { return 0, 0 }
func (stubID) asyncCallback(ID, string, uint64, uint64, proto.Message, *Error) {
}
func (stubID) DecoderByName(string) (MessageDecoder, MessageDecoder) { return nil, nil }
func (stubID) Kill(ID, []ArgsForBaseAtomos) *Error                    { return nil }
func (stubID) SendWormhole(ID, BaseAtomosWormhole, []ArgsForBaseAtomos) *Error {
	return nil
}
func (stubID) getGoID() uint64 { return 0 }


// TestCosmosRemote_AddressReuseDetectsNewGeneration covers the address-reuse
// scenario: a node restarts and re-registers the SAME address. The startup_id
// in CosmosNodeVersionInfo is what lets peers tell the new process generation
// apart from the dead one — the version object (and its connection/state)
// must be replaced, not silently reused.
func TestCosmosRemote_AddressReuseDetectsNewGeneration(t *testing.T) {
	p := newTestCosmosProcessWithoutCluster(t, "reuse_cosmos", "reuse_node")

	newInfo := func(state ClusterNodeState, startupID uint64) *CosmosNodeVersionInfo {
		return &CosmosNodeVersionInfo{
			Node:    "peer_node",
			Address: "127.0.0.1:59999",
			Id: &IDInfo{
				Type:   IDType_Cosmos,
				Cosmos: "reuse_cosmos",
				Node:   "peer_node",
			},
			State:     state,
			StartupId: startupID,
		}
	}

	remote := newCosmosRemoteFromNodeInfo(p, newInfo(ClusterNodeState_Started, 111))
	remote.etcdCreateVersion(newInfo(ClusterNodeState_Started, 111), "1")
	v1 := remote.version["1"]
	if v1 == nil {
		t.Fatal("version 1 should exist after create")
	}

	// Same generation, state transition Started→Draining: the version object
	// must be kept, and the new state must reach the stored info (previously
	// the info was never refreshed on same-address updates, so routing kept
	// using the state captured at creation).
	remote.etcdUpdateVersion(newInfo(ClusterNodeState_Draining, 111), "1")
	if remote.version["1"] != v1 {
		t.Fatal("same generation must keep the version object")
	}
	if got := v1.getInfo().GetState(); got != ClusterNodeState_Draining {
		t.Fatalf("same-generation state refresh lost: got=(%v),want=(%v)", got, ClusterNodeState_Draining)
	}

	// Identical re-publish (keepalive update): proto.Equal → complete no-op.
	remote.etcdUpdateVersion(newInfo(ClusterNodeState_Draining, 111), "1")
	if remote.version["1"] != v1 {
		t.Fatal("identical re-publish must be a no-op")
	}

	// Address reuse by a NEW generation: same address, different startup_id.
	// The old version object must be disabled and replaced — otherwise gRPC
	// would reconnect to the new process while we keep old-generation state.
	remote.etcdUpdateVersion(newInfo(ClusterNodeState_Started, 222), "1")
	v2 := remote.version["1"]
	if v2 == v1 {
		t.Fatal("new generation on the same address must replace the version object")
	}
	if got := v2.getInfo().GetStartupId(); got != 222 {
		t.Fatalf("replaced version carries wrong startup_id: got=(%d),want=(%d)", got, 222)
	}
	if got := v2.getInfo().GetState(); got != ClusterNodeState_Started {
		t.Fatalf("replaced version carries wrong state: got=(%v)", got)
	}

	// And a generation rolling BACK (stale etcd replay) is also a replacement,
	// since any startup_id mismatch means a different process instance.
	remote.etcdUpdateVersion(newInfo(ClusterNodeState_Started, 111), "1")
	if remote.version["1"] == v2 {
		t.Fatal("any startup_id change must replace the version object")
	}
}
