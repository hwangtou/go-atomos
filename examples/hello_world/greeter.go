package main

import (
	"google.golang.org/protobuf/proto"

	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
)

// greeterDeveloper is the ElementDeveloper factory for the Greeter element:
// ElementConstructor is called once per element, AtomConstructor once per atom.
type greeterDeveloper struct{}

var _ atomos.ElementDeveloper = (*greeterDeveloper)(nil)

func (d *greeterDeveloper) ElementConstructor() atomos.Atomos         { return &greeterElement{} }
func (d *greeterDeveloper) AtomConstructor(name string) atomos.Atomos { return &greeterAtom{} }

// greeterElement implements the generated api.GreeterElement interface.
type greeterElement struct {
	greeting string
}

var _ api.GreeterElement = (*greeterElement)(nil)

func (e *greeterElement) String() string { return "GreeterElement" }

// Spawn is the element spawner declared as ElementSpawn in hello.proto.
// data is nil when no AutoData persistence is configured.
func (e *greeterElement) Spawn(self atomos.ElementSelfID, data *api.HelloData, args ...atomos.ArgsForBaseAtomos) error {
	e.greeting = "Hello"
	if data != nil && data.GetGreeting() != "" {
		e.greeting = data.GetGreeting()
	}
	self.Log().Info("Greeter element spawned. greeting=(%s)", e.greeting)
	return nil
}

func (e *greeterElement) Halt(from atomos.ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

// greeterAtom implements the generated api.GreeterAtom interface.
type greeterAtom struct {
	self atomos.AtomSelfID
	name string
}

var _ api.GreeterAtom = (*greeterAtom)(nil)

func (a *greeterAtom) String() string { return "GreeterAtom:" + a.name }

// Spawn is the atom spawner declared as Spawn in hello.proto. arg carries the
// caller-supplied spawn argument; data carries persisted state (nil here).
func (a *greeterAtom) Spawn(self atomos.AtomSelfID, arg *api.HelloSpawnArg, data *api.HelloData, args ...atomos.ArgsForBaseAtomos) error {
	a.self = self
	a.name = arg.GetName()
	self.Log().Info("Greeter atom spawned. name=(%s)", a.name)
	return nil
}

// SayHello is the atom message handler declared in hello.proto.
func (a *greeterAtom) SayHello(from atomos.ID, in *api.HelloRequest) (*api.HelloReply, error) {
	a.self.Log().Info("Atom %s received SayHello from %s", a.name, from)
	return &api.HelloReply{Message: "Hello, " + in.GetName() + "!"}, nil
}

func (a *greeterAtom) Halt(from atomos.ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

// mainScript demonstrates the process lifecycle hooks. OnStartUp runs after
// all elements are spawned, so it is the place to spawn initial atoms.
type mainScript struct{}

var _ atomos.CosmosMainScript = (*mainScript)(nil)

func (s *mainScript) OnBoot(local *atomos.CosmosProcess) error { return nil }

func (s *mainScript) OnStartUp(local *atomos.CosmosProcess) error {
	self := local.Self()
	id, err := api.SpawnGreeterAtom(self, self, "world", &api.HelloSpawnArg{Name: "world"})
	if err != nil {
		return err
	}
	defer id.Release()

	out, err := id.SayHello(self, &api.HelloRequest{Name: "Atomos"})
	if err != nil {
		return err
	}
	self.Log().Info("SayHello reply: %s", out.GetMessage())
	return nil
}

func (s *mainScript) OnShutdown() error { return nil }
