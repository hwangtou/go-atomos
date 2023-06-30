package go_atomos

import (
	"testing"
	"time"
)

func TestAll(t *testing.T) {
	TestAllSingleNode(t)
	TestAllMultiNodes(t)
	TestAllApp(t)
}

func TestAllSingleNode(t *testing.T) {
	clearTest()
	//TestBaseAtomos(t)

	clearTest()
	TestCosmosMain(t)

	clearTest()
	TestElementLocalBase(t)
	clearTest()
	TestElementLocalScaleID(t)
	clearTest()
	//TestElementLocalLifeCycle(t)

	clearTest()
	TestAtomLocalBase(t)

	clearTest()
	TestAtomLocal_FirstSyncCall(t)
}

func TestAllMultiNodes(t *testing.T) {
	clearTest()
	TestSimulateCosmosNode_LifeCycle(t)
	<-time.After(time.Second * 5)

	clearTest()
	TestSimulateTwoCosmosNodes(t)
	<-time.After(time.Second * 5)

	clearTest()
	TestSimulateUpgradeCosmosNode(t)
	<-time.After(time.Second * 5)

	clearTest()
	TestSimulateTwoCosmosNodeRPC(t)
	<-time.After(time.Second * 5)

	clearTest()
	TestSimulateTwoCosmosNode_FirstSyncCall(t)
	<-time.After(time.Second * 5)
}

func TestAllApp(t *testing.T) {
	//clearTest()
	//TestAppLogging(t)
}
