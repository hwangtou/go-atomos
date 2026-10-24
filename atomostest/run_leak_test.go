package atomostest

// Run integration: booting a real process, auto-stopping via t.Cleanup, and
// AssertNoLeak confirming the mailbox goroutines actually exit — the
// end-to-end pairing of the testkit helpers.

import (
	"testing"
	"time"

	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/protobuf/proto"
)

type leakDev struct{}

func (d *leakDev) ElementConstructor() atomos.Atomos { return &leakAtom{name: "elem"} }
func (d *leakDev) AtomConstructor(name string) atomos.Atomos {
	return &leakAtom{name: name}
}
func (d *leakDev) AtomAutoData() atomos.AtomAutoData       { return nil }
func (d *leakDev) ElementAutoData() atomos.ElementAutoData { return nil }

type leakAtom struct {
	name string
}

func (a *leakAtom) String() string { return "leakAtom:" + a.name }
func (a *leakAtom) Halt(from atomos.ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

type leakScript struct{}

func (s *leakScript) OnBoot(local *atomos.CosmosProcess) error    { return nil }
func (s *leakScript) OnStartUp(local *atomos.CosmosProcess) error { return nil }
func (s *leakScript) OnShutdown() error                           { return nil }

func TestRun_StopsProcessAndLeavesNoGoroutines(t *testing.T) {
	before := TakeGoroutines()

	runnable := (&atomos.CosmosRunnable{}).
		SetConfig(&atomos.Config{
			Cosmos:   "leakCosmos",
			Node:     "leakNode",
			LogLevel: atomos.LogLevel_Debug,
		}).
		SetMainScript(&leakScript{})
	impl := atomos.NewImplementationFromDeveloper(&leakDev{})
	impl.Interface = atomos.NewInterfaceFromDeveloper("leakElement", &leakDev{})
	impl.Interface.ElementSpawner = func(s atomos.ElementSelfID, a atomos.Atomos, data proto.Message, args ...atomos.ArgsForBaseAtomos) error {
		return nil
	}
	impl.Interface.AtomSpawner = func(s atomos.AtomSelfID, a atomos.Atomos, arg, data proto.Message, args ...atomos.ArgsForBaseAtomos) error {
		return nil
	}
	runnable.AddElementImplementationWithSpawn(impl)

	process := Run(t, runnable)
	if process == nil {
		t.Fatal("Run returned nil process")
	}
	// The process reports healthy while running.
	if !process.IsHealthy() {
		t.Fatal("process not healthy after Run")
	}

	// Stopping explicitly (like user tests do) plus the cleanup Stop is safe.
	if err := process.Stop(); err != nil {
		t.Fatalf("explicit Stop: %v", err)
	}

	before.AssertNoLeak(t, 2, 5*time.Second)
}
