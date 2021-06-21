package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"sync"
)

type CosmosClusterHelper struct {
	remotes map[string]*CosmosRemote
	//Scheduler   ElementLordScheduler
}

func (h CosmosClusterHelper) close() {
	// todo
}

func newCosmosClusterHelper() *CosmosClusterHelper {
	return &CosmosClusterHelper{
		remotes: map[string]*CosmosRemote{},
	}
}

type CosmosRemote struct {
	mutex    sync.RWMutex
	elements map[string]*ElementRemote
}

func (c *CosmosLocal) CosmosRemote() bool {
	return false
}

func (c *CosmosRemote) GetAtomId(elem, name string) (Id, error) {
	panic("")
}

func (c *CosmosRemote) SpawnAtom(elem, name string, arg proto.Message) (Id, error) {
	panic("")
}

func (c *CosmosRemote) CallAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error) {
	panic("")
}

func (c *CosmosRemote) KillAtom(fromId, toId Id) error {
	panic("")
}
