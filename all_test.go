package go_atomos

import "testing"

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
	TestSimulateCosmosNodeLifeCycle(t)

	clearTest()
	TestSimulateTwoCosmosNodes(t)

	clearTest()
	TestSimulateUpgradeCosmosNode(t)

	clearTest()
	TestSimulateTwoCosmosNodeRPC(t)

	clearTest()
	TestSimulateTwoCosmosNode_FirstSyncCall(t)
}

func TestAllApp(t *testing.T) {
	//clearTest()
	//TestAppLogging(t)
}
