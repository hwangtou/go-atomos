package atomos

import (
	"context"
	"encoding/hex"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestLifecycleProtoCompatibility(t *testing.T) {
	fields := []struct {
		message protoreflect.Message
		name    protoreflect.Name
		number  protoreflect.FieldNumber
	}{
		{(&IDInfo{}).ProtoReflect(), "incarnation", 8},
		{(&IDInfo{}).ProtoReflect(), "startup_id", 9},
		{(&IDInfo{}).ProtoReflect(), "fence", 10},
		{(&IDInfo{}).ProtoReflect(), "shard_id", 11},
		{(&Error{}).ProtoReflect(), "operation_id", 6},
		{(&CosmosNodeVersionInfo{}).ProtoReflect(), "protocol_version", 7},
		{(&CosmosArgs{}).ProtoReflect(), "protocol_version", 6},
	}
	for _, field := range fields {
		d := field.message.Descriptor().Fields().ByNumber(field.number)
		if d == nil || d.Name() != field.name {
			t.Errorf("%s field %d = %v, want %s", field.message.Descriptor().FullName(), field.number, d, field.name)
		}
	}

	in := &IDInfo{Incarnation: 11, StartupId: 12, Fence: 13}
	buf, err := proto.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out IDInfo
	if err := proto.Unmarshal(buf, &out); err != nil {
		t.Fatal(err)
	}
	if out.Incarnation != 11 || out.StartupId != 12 || out.Fence != 13 {
		t.Fatalf("lifecycle fields did not survive wire round trip: %v", &out)
	}

	service := (&CosmosRemoteSpawnAtomControlledReq{}).ProtoReflect().Descriptor().ParentFile().Services().ByName("AtomosRemoteService")
	for _, name := range []string{"SpawnAtomControlled", "HaltAtomControlled", "LookupAtom", "GetAtomOperation", "TakeoverAtom"} {
		if service.Methods().ByName(protoreflect.Name(name)) == nil {
			t.Errorf("RPC %s is missing", name)
		}
	}
}

func TestOperationIDArgument(t *testing.T) {
	first, err := NewOperationID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewOperationID()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || first == second {
		t.Fatalf("operation IDs are not distinct 128-bit hex values: %q, %q", first, second)
	}
	if decoded, err := hex.DecodeString(first); err != nil || len(decoded) != 16 {
		t.Fatalf("operation ID %q is not 128-bit hex: %v", first, err)
	}

	helper := createBaseAtomosHelper(BaseAtomosMailSync, []ArgsForBaseAtomos{ArgBaseAtomosOperationID(first)})
	if helper.hasErrors() {
		t.Fatal(helper.getError())
	}
	if helper.operationID != first {
		t.Fatalf("operationID = %q, want %q", helper.operationID, first)
	}
	remote := helper.getRemoteArg()
	decodedHelper := createBaseAtomosHelper(BaseAtomosMailSync, createExtForCosmosArgs(remote))
	if decodedHelper.operationID != "" {
		t.Fatalf("operation ID leaked through legacy CosmosArgs: %q", decodedHelper.operationID)
	}
}

func TestGeneratedSpawnForwardsExt(t *testing.T) {
	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		arg := &cosmosLocalRecordingArg{}
		atomID, err := SpawnForTestAtomosAtom(p.local, p.local, "generated_ext", &ForTestSpawnArg{}, arg)
		if err != nil {
			t.Fatal(err)
		}
		defer atomID.Release()
		if arg.calls == 0 {
			t.Fatal("generated Spawn discarded ext")
		}
	})
}

func TestLifecycleKeysEscapeNames(t *testing.T) {
	p := &CosmosProcess{local: &CosmosLocal{runnable: &CosmosRunnable{config: &Config{Cosmos: "cosmos/with%slash"}}}}
	manager := &lifecycleManager{process: p}
	first := manager.recordKey("element/part", "atom")
	second := manager.recordKey("element", "part/atom")
	if first == second {
		t.Fatalf("ambiguous lifecycle keys: %s", first)
	}
	if manager.operationKey("operation/part") == manager.operationKey("operation%2Fpart") {
		t.Fatal("operation key escaping is reversible")
	}
}

func TestLifecycleModeSavedOnProcess(t *testing.T) {
	id := &IDInfo{Type: IDType_Cosmos, Cosmos: "test_cosmos", Node: "test_node"}
	p, err := newCosmosProcess(id.Cosmos, id.Node, &reproLogger{})
	if err != nil {
		t.Fatal(err)
	}
	if p.lifecycleMode != LifecycleModeLegacyUnsafe {
		t.Fatalf("default lifecycle mode = %v", p.lifecycleMode)
	}
	runnable := newTestCosmosRunnable(id).SetLifecycleMode(LifecycleModePerAtomStrict)
	if err := p.Start(runnable); CodeOf(err) != ErrCodeFencingRequired {
		t.Fatalf("strict start without etcd error = %v, want FencingRequired", err)
	}
	if p.lifecycleMode != LifecycleModePerAtomStrict {
		t.Fatalf("process lifecycle mode = %v, want PerAtomStrict", p.lifecycleMode)
	}
}

func TestLookupAtomHasNoSpawnSideEffects(t *testing.T) {
	testSpawnLifecycleHelper(t, func(p *CosmosProcess) {
		service := &atomosRemoteService{process: p}
		req := &CosmosRemoteLookupAtomReq{Id: &IDInfo{Type: IDType_Atom, Element: ForTestAtomosName, Atom: "lookup_only"}}
		rsp, err := service.LookupAtom(context.Background(), req)
		if err != nil || rsp.Error != nil {
			t.Fatalf("LookupAtom transport=%v response=%v", err, rsp.Error)
		}
		if rsp.State != AtomLifecycleState_AtomLifecycleStateAbsent || rsp.Ownership != nil || rsp.Operation != nil {
			t.Fatalf("absent lookup = %v", rsp)
		}
		elem, localErr := p.local.getLocalElement(ForTestAtomosName)
		if localErr != nil {
			t.Fatal(localErr)
		}
		if got := elem.GetAtomsNum(); got != 0 {
			t.Fatalf("LookupAtom spawned an atom; atoms = %d", got)
		}

		controlled, err := service.SpawnAtomControlled(context.Background(), &CosmosRemoteSpawnAtomControlledReq{
			Element: ForTestAtomosName, Atom: "lookup_only", OperationId: "controlled-op",
		})
		if err != nil {
			t.Fatalf("controlled operation returned transport error: %v", err)
		}
		if controlled.Error == nil || controlled.Error.Code != ErrCodeOperationUnsupported || controlled.Error.OperationId != "controlled-op" || controlled.Result != nil {
			t.Fatalf("controlled response = %v", controlled)
		}
		if got := elem.GetAtomsNum(); got != 0 {
			t.Fatalf("unsupported controlled spawn had side effects; atoms = %d", got)
		}

		atomID, err := SpawnForTestAtomosAtom(p.local, p.local, "lookup_only", &ForTestSpawnArg{})
		if err != nil {
			t.Fatal(err)
		}
		defer atomID.Release()
		rsp, err = service.LookupAtom(context.Background(), req)
		if err != nil || rsp.Error != nil {
			t.Fatalf("running LookupAtom transport=%v response=%v", err, rsp.Error)
		}
		if rsp.State != AtomLifecycleState_AtomLifecycleStateRunning || rsp.Id == nil || rsp.Id.InstanceId == 0 || rsp.Ownership != nil {
			t.Fatalf("running lookup = %v", rsp)
		}
	})
}

func TestLifecycleAtomStateErrorClassification(t *testing.T) {
	for _, test := range []struct {
		state AtomLifecycleState
		code  int64
	}{
		{state: AtomLifecycleState_AtomLifecycleStateSpawning, code: ErrCodeAtomSpawning},
		{state: AtomLifecycleState_AtomLifecycleStateStopping, code: ErrCodeAtomIsStopping},
		{state: AtomLifecycleState_AtomLifecycleStateHalted, code: ErrCodeAtomNotRunning},
		{state: AtomLifecycleState_AtomLifecycleStateAbsent, code: ErrCodeAtomNotRunning},
		{state: AtomLifecycleState_AtomLifecycleStateUnknown, code: ErrCodeAtomNotRunning},
	} {
		if got := lifecycleAtomStateErrorCode(test.state); got != test.code {
			t.Errorf("state %s error code = %d, want %d", test.state, got, test.code)
		}
	}
}

func TestLifecycleAuthorizeReadinessStateAndOwnership(t *testing.T) {
	process, err := newCosmosProcess("authorize", "node", &reproLogger{})
	if err != nil {
		t.Fatal(err)
	}
	process.lifecycleMode = LifecycleModePerAtomStrict
	process.local.runnable = &CosmosRunnable{config: &Config{Cosmos: "authorize"}}
	manager := &lifecycleManager{
		process: process,
		owners:  map[string]lifecycleOwnerSnapshot{},
		shards:  map[string]lifecycleShardSnapshot{},
	}
	id := &IDInfo{
		Type: IDType_Atom, Cosmos: "authorize", Node: process.local.GetNodeName(),
		Element: "element", Atom: "atom", Incarnation: 1, StartupId: 1, Fence: 1,
	}
	if err := manager.authorize(id); CodeOf(err) != ErrCodeOwnershipLost {
		t.Fatalf("unhealthy lease error = %v, want OwnershipLost", err)
	}
	manager.publishLease(7, true)
	if err := manager.authorize(id); CodeOf(err) != ErrCodeOwnershipNotReady {
		t.Fatalf("unsynchronized watch error = %v, want OwnershipNotReady", err)
	}
	manager.watchHealthy.Store(true)
	if err := manager.authorize(id); CodeOf(err) != ErrCodeStaleFence {
		t.Fatalf("missing owner error = %v, want StaleFence", err)
	}
	owner := &AtomOwnershipRecord{
		Ownership: &AtomOwnershipInfo{
			Id: proto.Clone(id).(*IDInfo), Incarnation: id.Incarnation,
			StartupId: id.StartupId, Fence: id.Fence, State: AtomLifecycleState_AtomLifecycleStateRunning,
		},
		LeaseId: 8,
	}
	manager.cacheOwnerPut(manager.ownerKey(id.Element, id.Atom), owner, 1)
	if err := manager.authorize(id); CodeOf(err) != ErrCodeOwnershipLost {
		t.Fatalf("changed lease error = %v, want OwnershipLost", err)
	}
	owner.LeaseId = 7
	for i, test := range []struct {
		state AtomLifecycleState
		code  int64
	}{
		{state: AtomLifecycleState_AtomLifecycleStateSpawning, code: ErrCodeAtomSpawning},
		{state: AtomLifecycleState_AtomLifecycleStateStopping, code: ErrCodeAtomIsStopping},
		{state: AtomLifecycleState_AtomLifecycleStateHalted, code: ErrCodeAtomNotRunning},
		{state: AtomLifecycleState_AtomLifecycleStateRunning},
	} {
		owner.Ownership.State = test.state
		manager.cacheOwnerPut(manager.ownerKey(id.Element, id.Atom), owner, int64(i+2))
		err := manager.authorize(id)
		if test.state == AtomLifecycleState_AtomLifecycleStateRunning {
			if err != nil {
				t.Errorf("Running authorize error = %v", err)
			}
		} else if CodeOf(err) != test.code {
			t.Errorf("state %s error = %v, want code %d", test.state, err, test.code)
		}
	}
}

func TestLifecycleClaimReadinessAndSelfHaltConflict(t *testing.T) {
	manager := &lifecycleManager{}
	if _, err := manager.claimSpawn(
		context.Background(), "", "fingerprint", AtomOperationKind_AtomOperationKindSpawn, "element", "atom", nil, false,
	); CodeOf(err) != ErrCodeOwnershipNotReady {
		t.Fatalf("claim without lease error = %v, want OwnershipNotReady", err)
	}
	if _, err := manager.joinStoppingHalt(
		context.Background(), "halt-operation", "fingerprint", false, &AtomOwnershipRecord{OperationId: "halt-operation"},
	); CodeOf(err) != ErrCodeOperationConflict {
		t.Fatalf("self halt join error = %v, want OperationConflict", err)
	}
}
