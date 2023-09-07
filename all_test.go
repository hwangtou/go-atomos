package go_atomos

import (
	"testing"
	"time"
)

func TestAllSingleNode(t *testing.T) {
	clearTest()
	TestCosmosMain(t)

	clearTest()
	TestElementLocalBase(t)
	clearTest()
	TestElementLocalScaleID(t)
	clearTest()

	clearTest()
	TestAtomLocalBase(t)

	clearTest()
	TestAtomLocal_FirstSyncCall(t)
}

func TestAllMultiNodes(t *testing.T) {
	clearTest()
	TestSimulateCosmosNode_LifeCycle(t)
	<-time.After(time.Second * 1)

	clearTest()
	TestSimulateTwoCosmosNodes(t)
	<-time.After(time.Second * 1)

	clearTest()
	TestSimulateUpgradeCosmosNode(t)
	<-time.After(time.Second * 1)

	clearTest()
	TestSimulateTwoCosmosNodeRPC(t)
	<-time.After(time.Second * 1)

	clearTest()
	TestSimulateTwoCosmosNode_FirstSyncCall(t)
	<-time.After(time.Second * 1)
}
