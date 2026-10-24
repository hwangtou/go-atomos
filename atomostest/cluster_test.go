package atomostest

// End-to-end fixture proofs: a real two-node cluster over embedded etcd and
// loopback gRPC, and the Probe lifecycle recorder.

import (
	"testing"
	"time"

	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/protobuf/proto"
)

const clusterElem = "clusterElem"

type clusterDev struct{}

func (d *clusterDev) ElementConstructor() atomos.Atomos { return &clusterAtom{name: "elem"} }
func (d *clusterDev) AtomConstructor(name string) atomos.Atomos {
	return &clusterAtom{name: name}
}
func (d *clusterDev) AtomAutoData() atomos.AtomAutoData       { return nil }
func (d *clusterDev) ElementAutoData() atomos.ElementAutoData { return nil }

type clusterAtom struct {
	name string
}

func (a *clusterAtom) String() string { return "clusterAtom:" + a.name }
func (a *clusterAtom) Halt(from atomos.ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

// clusterRunnable builds the node runnable: every node implements
// clusterElem (a node only routes REMOTE elements it also implements — the
// implementation must be mirrored cluster-wide), with the Ping handler
// answering a marker proto.
func clusterRunnable(node string) *atomos.CosmosRunnable {
	dev := &clusterDev{}
	impl := atomos.NewImplementationFromDeveloper(dev)
	impl.Interface = atomos.NewInterfaceFromDeveloper(clusterElem, dev)
	impl.Interface.ElementSpawner = func(s atomos.ElementSelfID, a atomos.Atomos, data proto.Message, args ...atomos.ArgsForBaseAtomos) error {
		return nil
	}
	impl.Interface.AtomSpawner = func(s atomos.AtomSelfID, a atomos.Atomos, arg, data proto.Message, args ...atomos.ArgsForBaseAtomos) error {
		return nil
	}
	// Hand-built decoders: the Messenger type parameters are phantom for
	// decoding, so FakeID/fakeAtom stand in; *atomos.Config is the wire
	// message both ways.
	dec := (&atomos.Messenger[*FakeID, *FakeID, *clusterAtom, *atomos.Config, *atomos.Config]{Name: "Ping"}).
		Decoder(&atomos.Config{}, &atomos.Config{})
	impl.Interface.ElementDecoders = map[string]*atomos.IOMessageDecoder{"Ping": dec}
	impl.Interface.AtomDecoders = map[string]*atomos.IOMessageDecoder{"Ping": dec}

	ping := func(from atomos.ID, to atomos.Atomos, in proto.Message) (proto.Message, error) {
		return &atomos.Config{Cosmos: "pong"}, nil
	}
	impl.ElementHandlers = map[string]atomos.MessageHandler{"Ping": ping}
	impl.AtomHandlers = map[string]atomos.MessageHandler{"Ping": ping}

	return (&atomos.CosmosRunnable{}).
		SetMainScript(&noopScript{}).
		AddElementImplementationWithSpawn(impl)
}

type noopScript struct{}

func (s *noopScript) OnBoot(local *atomos.CosmosProcess) error    { return nil }
func (s *noopScript) OnStartUp(local *atomos.CosmosProcess) error { return nil }
func (s *noopScript) OnShutdown() error                           { return nil }

func TestRunCluster_CrossNodeRoundTrip(t *testing.T) {
	procs := RunCluster(t, "clusterCosmos", []string{"n1", "n2"}, clusterRunnable)

	// The default router pins each element to a deterministic home node;
	// compute it the same way and pick the OTHER node as the remote caller.
	homeIdx := int(fnv32(clusterElem) % 2)
	home, remote := procs[homeIdx], procs[1-homeIdx]
	homeName := []string{"n1", "n2"}[homeIdx]

	// Wait for discovery: the remote node must resolve the element to the
	// home node over a live gRPC connection before any remote spawn.
	AwaitElementOnNode(t, remote, clusterElem, homeName, 15*time.Second)

	// Spawning from ANY node lands on the home node (the router decides).
	atomID, tracker, err := remote.Self().CosmosSpawnAtom(remote.Self(), clusterElem, "shared", nil)
	if err != nil {
		t.Fatalf("spawn via remote node: %v", err)
	}
	defer tracker.Release()
	if got := atomID.GetIDInfo().Node; got != homeName {
		t.Fatalf("spawn routed to node %q, want home %q", got, homeName)
	}
	_ = home

	// Resolving the atom from the other node proves discovery and remote
	// element registration have propagated — the cross-node readiness gate.
	AwaitAtomOnNode(t, remote, clusterElem, "shared", homeName, 15*time.Second)

	// Cross-node atom round trip: the atom lives on the home node, called
	// from the other one over real gRPC.
	remoteAtom, remoteTracker, err := remote.Self().CosmosGetAtomID(clusterElem, "shared",
		atomos.ArgBaseAtomosFromLocalOnly())
	if err != nil {
		t.Fatalf("CosmosGetAtomID from remote node: %v", err)
	}
	defer remoteTracker.Release()
	out, err := remoteAtom.SyncMessagingByName(remote.Self(), "Ping", &atomos.Config{},
		atomos.ArgBaseAtomosTimeout(10*time.Second))
	if err != nil {
		t.Fatalf("atom Ping cross-node: %v", err)
	}
	if cfg, ok := out.(*atomos.Config); !ok || cfg.Cosmos != "pong" {
		t.Fatalf("atom Ping reply = %#v", out)
	}
}

func TestProbe_LifecycleEvents(t *testing.T) {
	runnable := clusterRunnable("solo").SetConfig(&atomos.Config{
		Cosmos:   "probeCosmos",
		Node:     "solo",
		LogLevel: atomos.LogLevel_Debug,
	})
	probe := NewProbe(runnable)
	process := Run(t, runnable)

	// The element spawned during startup.
	probe.Require(t, ProbeSpawned, clusterElem, 10*time.Second)
	if !probe.Has(ProbeSpawning, clusterElem) {
		t.Fatal("spawning event missing for element")
	}

	// Spawn an atom, then halt it: halted events must carry the atom ID.
	atomID, tracker, err := process.Self().CosmosSpawnAtom(process.Self(), clusterElem, "probed", nil)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer tracker.Release()
	probe.Require(t, ProbeSpawned, "probed", 10*time.Second)
	if err := atomID.Kill(process.Self(), atomos.ArgBaseAtomosWaitKilled()); err != nil {
		t.Fatalf("kill: %v", err)
	}
	probe.Require(t, ProbeHalted, "probed", 10*time.Second)
}
