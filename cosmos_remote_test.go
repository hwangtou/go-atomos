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
	if rsp, er := cli.GetAtomID(ctx, &CosmosRemoteGetAtomIDReq{
		Element: ForTestAtomosName,
		Atom:    testAtom,
	}); er != nil {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call failed. err=(%v)", er)
	} else if rsp == nil || rsp.Id == nil || rsp.Error != nil {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call returned invalid response: rsp=(%v)", rsp)
	} else if rsp.Id.Cosmos != "test_cosmos" || rsp.Id.Node != "test_node_target" || rsp.Id.Element != ForTestAtomosName || rsp.Id.Atom != testAtom {
		t.Fatalf("CosmosRemote: GetAtomID gRPC call returned invalid ID info: rsp.Id=(%v)", rsp.Id)
	} else {
		t.Logf("CosmosRemote: GetAtomID gRPC call succeeded, rsp=(%v)", rsp)
	}

	// Sync Messaging test
	if rsp, er := cli.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId: cluster.sourceProcess.local.GetIDInfo(),
		To: &IDInfo{
			Type:    IDType_Atom,
			Cosmos:  "test_cosmos",
			Node:    "test_node_target",
			Element: ForTestAtomosName,
			Atom:    testAtom,
			Version: 0,
		},
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
		ToId: &IDInfo{
			Type:    IDType_Atom,
			Cosmos:  "test_cosmos",
			Node:    "test_node_target",
			Element: ForTestAtomosName,
			Atom:    testAtom,
			Version: 0,
		},
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
