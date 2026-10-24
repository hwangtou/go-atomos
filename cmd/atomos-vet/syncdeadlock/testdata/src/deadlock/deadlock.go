// Package deadlock contains BAD cases the sync-deadlock analyzer must flag.
//
// The stubs mirror the shapes protoc-gen-go-atomos emits: handler interfaces
// named <Svc>Atom/<Svc>Element embedding an Atomos interface, and typed ID
// clients named <Svc>AtomID/<Svc>ElementID.
package deadlock

// -- Framework stubs. --

type Atomos interface {
	String() string
}

type IDInfo struct{ Atom string }

type ID interface{ GetIDInfo() *IDInfo }

type CosmosNode interface{}

type SelfID interface {
	ID
	Cosmos() CosmosNode
}

type ArgsForBaseAtomos interface{}

// -- Generated handler interfaces. --

type FooAtom interface {
	Atomos
	Greeting(from ID, in any) (any, error)
	Ping(from ID, in any) (any, error)
}

type BarAtom interface {
	Atomos
	Pong(from ID, in any) (any, error)
}

type GreeterAtom interface {
	Atomos
	Spawn(self SelfID, arg any) error
}

// -- Generated ID clients. --

type FooAtomID struct{ id ID }

func (c *FooAtomID) Greeting(caller SelfID, in any, ext ...ArgsForBaseAtomos) (any, error) {
	return nil, nil
}

func (c *FooAtomID) Ping(caller SelfID, in any, ext ...ArgsForBaseAtomos) (any, error) {
	return nil, nil
}

type BarAtomID struct{ id ID }

func (c *BarAtomID) Pong(caller SelfID, in any, ext ...ArgsForBaseAtomos) (any, error) {
	return nil, nil
}

type GreeterAtomID struct{ id ID }

// -- Generated factories. --

func GetFooAtomID(cosmos CosmosNode, name string) (*FooAtomID, error) { return nil, nil }

func GetBarAtomID(cosmos CosmosNode, name string) (*BarAtomID, error) { return nil, nil }

func SpawnGreeterAtom(caller SelfID, c CosmosNode, name string, arg any) (*GreeterAtomID, error) {
	return nil, nil
}

// -- BAD: self-call (pattern 1: A→A). --

type atomFoo struct {
	self SelfID
}

func (a *atomFoo) String() string { return "atomFoo" }

func (a *atomFoo) Greeting(from ID, in any) (any, error) {
	id, _ := GetFooAtomID(a.self.Cosmos(), a.self.GetIDInfo().Atom) // want `sync handler Foo.Greeting resolves an ID to its own atom`
	_, e := id.Greeting(a.self, in)
	return nil, e
}

// BAD: two cross-service cycles seeded from one handler:
//
//	Foo.Ping → Bar.Pong → Foo.Ping        (typed client cycle)
//	Foo.Ping → Greeter.Spawn → Foo.Ping   (generated Spawn factory cycle)
func (a *atomFoo) Ping(from ID, in any) (any, error) {
	bar, _ := GetBarAtomID(a.self.Cosmos(), "bar")
	if _, e := bar.Pong(a.self, in); e != nil { // want `potential sync deadlock cycle: handler Bar.Pong`
		return nil, e
	}
	_, e := SpawnGreeterAtom(a.self, a.self.Cosmos(), "g", nil)
	return nil, e
}

type atomBar struct {
	self SelfID
}

func (a *atomBar) String() string { return "atomBar" }

func (a *atomBar) Pong(from ID, in any) (any, error) {
	foo, _ := GetFooAtomID(a.self.Cosmos(), "foo")
	_, e := foo.Ping(a.self, in)
	return nil, e
}

type atomGreeter struct {
	self SelfID
}

func (a *atomGreeter) String() string { return "atomGreeter" }

func (a *atomGreeter) Spawn(self SelfID, arg any) error {
	foo, _ := GetFooAtomID(a.self.Cosmos(), "foo")
	_, e := foo.Ping(a.self, nil) // want `potential sync deadlock cycle: handler Foo.Ping`
	return e
}
