// Package deadlock contains BAD cases the sync-deadlock analyzer must flag.
package deadlock

// stub types.
type IDTracker struct{}
type ID struct{}
type Error struct{}
type IDInfo struct{ Atom string }
type CosmosNode interface{}
type SelfID interface {
	Cosmos() CosmosNode
	GetIDInfo() *IDInfo
}

func GetFooAtomID(cosmos CosmosNode, name string) (*FooAtomID, *Error) { return nil, nil }

type FooAtomID struct{}

func (a *FooAtomID) Greeting(caller SelfID, in any) (any, *Error) { return nil, nil }
func (a *FooAtomID) SayHello(caller SelfID, in any) (any, *Error)  { return nil, nil }
func (a *FooAtomID) Ping(caller SelfID, in any) (any, *Error)      { return nil, nil }
func (a *FooAtomID) Pong(caller SelfID, in any) (any, *Error)      { return nil, nil }

// --- BAD: self-call (pattern 1: A→A) ---

type atom struct {
	self SelfID
}

func (t *atom) Greeting(from ID, in any) (any, *Error) {
	id, _ := GetFooAtomID(t.self.Cosmos(), t.self.GetIDInfo().Atom) // want `sync handler Greeting resolves an ID to its own atom`
	_, e := id.Greeting(t.self, in)
	return nil, e
}

// --- BAD: cross-handler cycle (pattern 2: A→B→A, pattern 3: mutual A↔B) ---
// Greeting calls .SayHello (edge Greeting→SayHello), SayHello calls .Greeting
// (edge SayHello→Greeting) → cycle.

func (t *atom) SayHello(from ID, in any) (any, *Error) {
	var otherID *FooAtomID
	// This call closes the cycle Greeting↔SayHello (Greeting already has an edge
	// to SayHello via the self-call above referencing .Greeting... but actually
	// Greeting calls .Greeting, not .SayHello). For a clean cycle we need a
	// separate pair. Use Ping/Pong below.
	_ = otherID
	return nil, nil
}

// Clean two-node cycle: Ping→Pong→Ping.
type pingAtom struct {
	self SelfID
}

func (p *pingAtom) Ping(from ID, in any) (any, *Error) {
	var otherID *FooAtomID
	_, _ = otherID.Pong(p.self, in) // edge Ping→Pong
	return nil, nil
}

func (p *pingAtom) Pong(from ID, in any) (any, *Error) {
	var otherID *FooAtomID
	_, _ = otherID.Ping(p.self, in) // want `potential sync deadlock cycle: handler Ping`
	return nil, nil
}
