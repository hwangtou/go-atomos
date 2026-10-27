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
		if controlled.Error == nil || controlled.Error.Code != ErrCodeAtomNotImplemented || controlled.Error.OperationId != "controlled-op" || controlled.Result != nil {
			t.Fatalf("controlled response = %v", controlled)
		}
		if got := elem.GetAtomsNum(); got != 0 {
			t.Fatalf("unimplemented controlled spawn had side effects; atoms = %d", got)
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
