package main

import (
	"testing"
	"time"

	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/atomostest"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
)

// captureScript captures the running process at startup so the test can
// drive actor calls against it.
type captureScript struct {
	started chan<- *atomos.CosmosProcess
}

func (s *captureScript) OnBoot(local *atomos.CosmosProcess) error { return nil }
func (s *captureScript) OnStartUp(local *atomos.CosmosProcess) error {
	s.started <- local
	return nil
}
func (s *captureScript) OnShutdown() error { return nil }

func TestGreeter(t *testing.T) {
	started := make(chan *atomos.CosmosProcess, 1)
	runnable := newRunnable(&captureScript{started: started})
	runnable.SetConfig(&atomos.Config{
		Cosmos:   "hello",
		Node:     "test_node",
		LogLevel: atomos.LogLevel_Debug,
	})

	atomostest.Run(t, runnable)

	var process *atomos.CosmosProcess
	select {
	case process = <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for process startup")
	}
	t.Cleanup(func() { _ = process.Stop() })

	self := process.Self()
	id, err := api.SpawnGreeterAtom(self, self, "world", &api.HelloSpawnArg{Name: "world"})
	if err != nil {
		t.Fatalf("SpawnGreeterAtom err=(%v)", err)
	}
	defer id.Release()

	out, err := id.SayHello(self, &api.HelloRequest{Name: "Atomos"})
	if err != nil {
		t.Fatalf("SayHello err=(%v)", err)
	}
	if want := "Hello, Atomos!"; out.GetMessage() != want {
		t.Fatalf("SayHello message=(%q), want (%q)", out.GetMessage(), want)
	}

	// Getting the same atom again returns the running instance.
	again, err := api.GetGreeterAtomID(self, "world")
	if err != nil {
		t.Fatalf("GetGreeterAtomID err=(%v)", err)
	}
	defer again.Release()
	if again.String() != id.String() {
		t.Fatalf("GetGreeterAtomID String()=(%q), want (%q)", again.String(), id.String())
	}

	// Kill the atom; a subsequent get must fail with ErrAtomNotExists.
	if err := id.Kill(self, atomos.ArgBaseAtomosWaitKilled()); err != nil {
		t.Fatalf("Kill err=(%v)", err)
	}
	if _, err := api.GetGreeterAtomID(self, "world"); err == nil {
		t.Fatal("GetGreeterAtomID after Kill should fail")
	}
}
