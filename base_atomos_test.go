package atomos

//import (
//	"google.golang.org/protobuf/proto"
//	"testing"
//)
//
//// testAtomosHolder is a test implementation of AtomosHolder
//
//type testAtomosHolder struct {
//	t *testing.T
//}
//
//func (t *testAtomosHolder) OnMessaging(fromID ID, name string, in proto.Message) (out proto.Message, err *Error) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (t *testAtomosHolder) OnAsyncMessaging(fromID ID, name string, in proto.Message, callback func(reply proto.Message, err *Error)) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (t *testAtomosHolder) OnAsyncMessagingCallback(in proto.Message, err *Error, callback func(reply proto.Message, err *Error)) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (t *testAtomosHolder) OnScaling(from ID, name string, args proto.Message) (id ID, err *Error) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (t *testAtomosHolder) OnWormhole(from ID, wormhole AtomosWormhole) *Error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (t *testAtomosHolder) OnStopping(from ID, cancelled []uint64) *Error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (t *testAtomosHolder) OnIDsReleased() {
//	//TODO implement me
//	panic("implement me")
//}
//
//// testAtomosInstance tests the NewBaseAtomos function
//
//type testAtomosInstance struct {
//	t *testing.T
//}
//
//func (t *testAtomosInstance) String() string {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (t *testAtomosInstance) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func TestNewBaseAtomos(t *testing.T) {
//	id := &IDInfo{
//		Type:    IDType_Atom,
//		Cosmos:  "test_cosmos",
//		Node:    "test_node",
//		Element: "test_element",
//		Atom:    "test_atom",
//		Version: 10,
//	}
//	holder := &testAtomosHolder{t: t}
//	inst := &testAtomosInstance{}
//	NewBaseAtomos(id, LogLevel_Info, holder, inst, process)
//}
//
//func test() {
//	testRunnable := CosmosRunnable{}
//	testRunnable.
//		AddElementImplementation(GetTestImplement(&tDev{}), true).SetElementSpawn(TestName).
//		SetConfig(&Config{Cosmos: testCosmos, Node: testNode1, LogLevel: LogLevel_Debug}).
//		SetMainScript(&s)
//	MainForTest(testRunnable, t)
//}
