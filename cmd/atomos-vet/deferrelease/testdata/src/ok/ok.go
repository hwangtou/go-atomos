// Package ok contains GOOD cases the analyzer must NOT flag.
package ok

type IDTracker struct{}
type ID struct{}
type Error struct{}
type SelfID struct{}
type CosmosNode interface {
	CosmosGetAtomID(elem, name string) (ID, *IDTracker, *Error)
	CosmosSpawnAtom(caller SelfID, elem, name string, arg any) (ID, *IDTracker, *Error)
}

func (t *IDTracker) Release() {}

// WithID/WithRebind stubs matching the framework's release-path helpers.
func WithID[R any](tr *IDTracker, fn func() (R, *Error)) (R, *Error) {
	defer tr.Release()
	return fn()
}
func WithRebind[R any](resolve func() (ID, *Error), call func(ID) (R, *Error)) (R, *Error) {
	var zero R
	id, e := resolve()
	if e != nil {
		return zero, e
	}
	return call(id)
}

func GetFooAtomID(node any, name string) (*FooAtomID, *Error) { return nil, nil }
func GetFooElementID(node any) (*FooElementID, *Error)        { return nil, nil }
func SpawnFooAtom(self SelfID, node any, name string, arg any) (*FooAtomID, *Error) {
	return nil, nil
}

type FooAtomID struct{}

func (a *FooAtomID) Release() {}

type FooElementID struct{}

// GOOD: defer Release on the tracker (runtime multi-return: releasable is the
// tracker at index 1).
func okDefer(node CosmosNode) {
	_, tr, err := node.CosmosGetAtomID("elem", "atom")
	if err != nil {
		return
	}
	defer tr.Release()
	_ = tr
}

// GOOD: WithID wraps the tracker.
func okWithID(node CosmosNode) {
	_, tr, err := node.CosmosGetAtomID("elem", "atom")
	if err != nil {
		return
	}
	_, _ = WithID(tr, func() (struct{}, *Error) { return struct{}{}, nil })
}

// GOOD: WithRebind wraps the resolve/call.
func okWithRebind(node CosmosNode) {
	_, _ = WithRebind(func() (ID, *Error) {
		id, _, e := node.CosmosGetAtomID("elem", "atom")
		return id, e
	}, func(id ID) (struct{}, *Error) {
		return struct{}{}, nil
	})
}

// GOOD: result discarded (tracker to _) — not assigned to a named var, not flagged.
func okDiscard(node CosmosNode) {
	_, _, _ = node.CosmosGetAtomID("elem", "atom")
}

// GOOD: Element ID factory (GetFooElementID) — not flagged (nil tracker).
func okElement(node any) {
	_, _ = GetFooElementID(node)
}

// GOOD: generated Atom ID factory (single-return, releasable=0) with defer.
func okGeneratedDefer(node any) {
	id, _ := GetFooAtomID(node, "atom")
	defer id.Release()
	_ = id
}
