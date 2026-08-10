// Package leak contains BAD cases the analyzer must flag. The functions here
// stub the go-atomos factory names; the analyzer matches by call name, not by
// resolved type, so no real atomos import is needed.
package leak

// stub types so the testdata compiles standalone.
type IDTracker struct{}
type ID struct{}
type Error struct{}
type SelfID struct{}
type CosmosNode interface {
	CosmosGetAtomID(elem, name string) (ID, *IDTracker, *Error)
	CosmosSpawnAtom(caller SelfID, elem, name string, arg any) (ID, *IDTracker, *Error)
}

// BAD: tracker assigned, no defer Release / WithID / WithRebind.
func leakAtomID(node CosmosNode) {
	_, tr, err := node.CosmosGetAtomID("elem", "atom") // want `tr from CosmosGetAtomID requires defer .Release\(\), WithID, or WithRebind in this function \(possible IDTracker leak\)`
	if err != nil {
		return
	}
	_ = tr
}

// BAD: tracker assigned from Spawn, no release.
func leakSpawn(node CosmosNode, self SelfID) {
	_, tr, err := node.CosmosSpawnAtom(self, "elem", "atom", nil) // want `tr from CosmosSpawnAtom requires defer .Release\(\), WithID, or WithRebind in this function \(possible IDTracker leak\)`
	if err != nil {
		return
	}
	_ = tr
}

// BAD: generated-style free function Get<Svc>AtomID, no release. For generated
// factories the releasable is index 0 (the ID embeds *IDTracker).
func GetFooAtomID(node any, name string) (*FooAtomID, *Error) { return nil, nil }
func SpawnFooAtom(self SelfID, node any, name string, arg any) (*FooAtomID, *Error) {
	return nil, nil
}

type FooAtomID struct{}

func leakGenerated(node any) {
	id, _ := GetFooAtomID(node, "atom") // want `id from GetFooAtomID requires defer .Release\(\), WithID, or WithRebind in this function \(possible IDTracker leak\)`
	_ = id
}

func leakGeneratedSpawn(self SelfID, node any) {
	id, _ := SpawnFooAtom(self, node, "atom", nil) // want `id from SpawnFooAtom requires defer .Release\(\), WithID, or WithRebind in this function \(possible IDTracker leak\)`
	_ = id
}
