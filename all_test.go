package go_atomos

import "testing"

func TestAll(t *testing.T) {
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
	TestAppLogging(t)
}
