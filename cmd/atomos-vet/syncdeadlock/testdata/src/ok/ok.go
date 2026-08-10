// Package ok contains GOOD cases the sync-deadlock analyzer must NOT flag.
package ok

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

// GOOD: handler sync-calls a DIFFERENT atom (not self).
type atom struct {
	self SelfID
}

func (t *atom) Greeting(from ID, in any) (any, *Error) {
	id, _ := GetFooAtomID(t.self.Cosmos(), "other_atom") // literal name, not self
	_, e := id.Greeting(t.self, in)
	return nil, e
}

// GOOD: handler makes no sync call at all.
type atom2 struct{}

func (a *atom2) Greeting(from ID, in any) (any, *Error) {
	return nil, nil
}

// GOOD: function is not a sync handler (no "from" param) — not analyzed.
func notAHandler(in any) (any, *Error) {
	return nil, nil
}
