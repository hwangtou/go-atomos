// Package ok contains GOOD cases the sync-deadlock analyzer must NOT flag.
//
// The stubs mirror the shapes protoc-gen-go-atomos emits: handler interfaces
// named <Svc>Atom/<Svc>Element embedding an Atomos interface, and typed ID
// clients named <Svc>AtomID/<Svc>ElementID.
package ok

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

// -- Generated handler interfaces (embed Atomos; explicit RPC methods). --

type FooAtom interface {
	Atomos
	Greeting(from ID, in any) (any, error)
	Ping(from ID, in any) (any, error)
}

type BarAtom interface {
	Atomos
	Greeting(from ID, in any) (any, error)
}

// -- Generated ID clients. --

type FooAtomID struct{ id ID }

func (c *FooAtomID) Greeting(caller SelfID, in any, ext ...ArgsForBaseAtomos) (any, error) {
	return nil, nil
}

func (c *FooAtomID) AsyncGreeting(caller SelfID, in any, cb func(any, error)) error { return nil }

func (c *FooAtomID) Ping(caller SelfID, in any, ext ...ArgsForBaseAtomos) (any, error) {
	return nil, nil
}

func (c *FooAtomID) SyncMessagingByName(caller SelfID, name string, in any, ext ...ArgsForBaseAtomos) (any, error) {
	return nil, nil
}

type BarAtomID struct{ id ID }

func (c *BarAtomID) Greeting(caller SelfID, in any, ext ...ArgsForBaseAtomos) (any, error) {
	return nil, nil
}

type GreeterAtomID struct{ id ID }

// -- Generated factories. --

func GetFooAtomID(cosmos CosmosNode, name string) (*FooAtomID, error) { return nil, nil }

func GetBarAtomID(cosmos CosmosNode, name string) (*BarAtomID, error) { return nil, nil }

func SpawnGreeterAtom(caller SelfID, c CosmosNode, name string, arg any) (*GreeterAtomID, error) {
	return nil, nil
}

// -- Implementations. --

// GOOD: handler sync-calls another atom by a literal name (not self).
type atomFoo struct {
	self SelfID
}

func (a *atomFoo) String() string { return "atomFoo" }

func (a *atomFoo) Greeting(from ID, in any) (any, error) {
	id, _ := GetFooAtomID(a.self.Cosmos(), "other_atom") // literal name, not self
	// Same handler on another atom is not a static self-cycle.
	_, e := id.Greeting(a.self, in)
	return nil, e
}

func (a *atomFoo) Ping(from ID, in any) (any, error) {
	// Same-named method on a DIFFERENT service: must not be conflated with
	// Foo.Greeting or Foo.Ping (old name-only analyzer could false-positive).
	bar, _ := GetBarAtomID(a.self.Cosmos(), "bar")
	if _, e := bar.Greeting(a.self, in); e != nil {
		return nil, e
	}
	// Async client call: never blocks the mailbox, never an edge.
	foo, _ := GetFooAtomID(a.self.Cosmos(), "other")
	_ = foo.AsyncGreeting(a.self, in, func(any, error) {})
	// Spawn of a third service: one-way edge, no cycle in this package.
	if _, e := SpawnGreeterAtom(a.self, a.self.Cosmos(), "g", nil); e != nil {
		return nil, e
	}
	// A sync call inside a `go` statement runs on another goroutine.
	go func() {
		_, _ = foo.Ping(a.self, in)
	}()
	return nil, nil
}

// GOOD: handler makes no sync call at all.
type atomBar struct{}

func (a *atomBar) String() string { return "atomBar" }

func (a *atomBar) Greeting(from ID, in any) (any, error) {
	return nil, nil
}

// GOOD: function is not a sync handler (does not implement a generated
// handler interface) — its calls are not analyzed.
func helperCall(c CosmosNode) {
	id, _ := GetFooAtomID(c, "x")
	_, _ = id.Greeting(nil, nil)
}
