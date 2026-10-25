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

type FooElement interface {
	Atomos
	Greeting(from ID, in any) (any, error)
	ElementOnly(from ID, in any) (any, error)
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

type FooElementID struct{ id ID }

func (c *FooElementID) Greeting(caller SelfID, in any, ext ...ArgsForBaseAtomos) (any, error) {
	return nil, nil
}

type BarAtomID struct{ id ID }

func (c *BarAtomID) Greeting(caller SelfID, in any, ext ...ArgsForBaseAtomos) (any, error) {
	return nil, nil
}

type GreeterAtomID struct{ id ID }

// -- Generated factories. --

func GetFooAtomID(cosmos CosmosNode, name string) (*FooAtomID, error) { return nil, nil }

func GetFooElementID(cosmos CosmosNode) (*FooElementID, error) { return nil, nil }

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
	foo, _ := GetFooAtomID(a.self.Cosmos(), "other_atom")
	_, e := foo.Ping(a.self, in)
	return nil, e
}

func (a *atomFoo) Ping(from ID, in any) (any, error) {
	// Merely resolving self, then using async or go, is not a self-deadlock.
	foo, _ := GetFooAtomID(a.self.Cosmos(), a.self.GetIDInfo().Atom)
	_ = foo.AsyncGreeting(a.self, in, func(any, error) {})
	go func() {
		_, _ = foo.Ping(a.self, in)
	}()
	// Atom.Ping→Element.Greeting and Atom.Greeting→Atom.Ping are not a cycle.
	// Treating Element and Atom as one node would create a false Ping/Greeting ring.
	element, _ := GetFooElementID(a.self.Cosmos())
	_, _ = element.Greeting(a.self, in)
	// Spawn of a third service: one-way edge, no cycle in this package.
	if _, e := SpawnGreeterAtom(a.self, a.self.Cosmos(), "g", nil); e != nil {
		return nil, e
	}
	// SyncMessagingByName with a RUNTIME-COMPUTED name: no literal, no edge
	// (documented limitation — the runtime wait-graph covers it).
	name := "Gre" + "eting"
	_, _ = foo.SyncMessagingByName(a.self, name, in)
	return nil, nil
}

// GOOD: obtaining one's own ID without making a call is safe.
type atomBar struct{ self SelfID }

func (a *atomBar) String() string { return "atomBar" }

func (a *atomBar) Greeting(from ID, in any) (any, error) {
	own, _ := GetFooAtomID(a.self.Cosmos(), a.self.GetIDInfo().Atom)
	_ = own
	return nil, nil
}

type elementFoo struct{}

func (e *elementFoo) String() string { return "elementFoo" }

func (e *elementFoo) Greeting(from ID, in any) (any, error) {
	return nil, nil
}

func (e *elementFoo) ElementOnly(from ID, in any) (any, error) {
	return nil, nil
}

// GOOD: function is not a sync handler (does not implement a generated
// handler interface) — its calls are not analyzed.
func helperCall(c CosmosNode) {
	id, _ := GetFooAtomID(c, "x")
	_, _ = id.Greeting(nil, nil)
}
