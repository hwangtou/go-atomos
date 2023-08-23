package go_atomos

import (
	"google.golang.org/grpc/connectivity"
	"strings"
	"testing"
	"time"
)

func TestSimulateTwoCosmosNode_FirstSyncCall(t *testing.T) {
	// 两个节点的模拟
	// Simulate two nodes

	c1 := simulateCosmosNode(t, "hello", "c1")
	c2 := simulateCosmosNode(t, "hello", "c2")

	elemName := "testElement"
	atomName1 := "testAtom1"
	atomName2 := "testAtom2"
	//atomOtherName := "testAtomOther"

	// 两个节点的停止
	defer func() {
		if err := c1.Stop(); err != nil {
			t.Fatal(err)
		}
		if err := c2.Stop(); err != nil {
			t.Fatal(err)
		}
		<-time.After(3 * time.Second)
	}()

	<-time.After(500 * time.Millisecond)
	c1.cluster.remoteMutex.RLock()
	c1cr2, has := c1.cluster.remoteCosmos["c2"]
	c1.cluster.remoteMutex.RUnlock()
	if !has {
		t.Fatal("c1 has no c2")
		return
	}
	c2.cluster.remoteMutex.RLock()
	c2cr1, has := c2.cluster.remoteCosmos["c1"]
	c2.cluster.remoteMutex.RUnlock()
	if !has {
		t.Fatal("c2 has no c2")
		return
	}

	if cli := c1cr2.current.client; cli == nil || cli.GetState() != connectivity.Ready {
		t.Fatal("c1cr2 client is not ready")
		return
	}
	if cli := c2cr1.current.client; cli == nil || cli.GetState() != connectivity.Ready {
		t.Fatal("c2cr1 client is not ready")
		return
	}

	c1cr2e, err := c1cr2.getElement(elemName)
	if err != nil {
		t.Fatal(err.AddStack(nil))
		return
	}
	c2cr1e, err := c2cr1.getElement(elemName)
	if err != nil {
		t.Fatal(err.AddStack(nil))
		return
	}
	_ = c2cr1e

	// Spawn local c1 id1
	c1e, err := c1.local.getLocalElement(elemName)
	if err != nil {
		t.Fatal(err.AddStack(nil))
		return
	}
	id1, t1, err := c1e.SpawnAtom(c1.local, atomName1, &String{}, nil, false)
	if err != nil {
		t.Fatal(err.AddStack(nil))
		return
	}
	if id1 == nil || t1 != nil {
		t.Fatal("id1 or t1 is nil")
		return
	}
	if i := id1.GetIDInfo(); i.Atom != atomName1 || i.Element != elemName || i.Node != c1.local.GetNodeName() || i.Cosmos != c1.local.GetIDInfo().Cosmos {
		t.Fatal("id1 atom is not testAtom")
		return
	}
	if c1.local.elements[elemName].atoms[atomName1] == nil {
		t.Fatal("c1 local atom is nil")
		return
	}
	if c2.local.elements[elemName].atoms[atomName1] != nil {
		t.Fatal("c2 local atom is not nil")
		return
	}
	t.Logf("id1=(%v) t1=(%v)", id1, t1)
	t1.Release()

	// Spawn remote c2 id1
	id2, t2, err := c1cr2e.SpawnAtom(c1.local, atomName2, &String{}, nil, false)
	if err != nil {
		t.Fatal(err.AddStack(nil))
		return
	}
	if id2 == nil || t2 != nil {
		t.Fatal("id2 or t2 is nil")
		return
	}
	if i := id2.GetIDInfo(); i.Atom != atomName2 || i.Element != elemName || i.Node != c2.local.GetNodeName() || i.Cosmos != c2.local.GetIDInfo().Cosmos {
		t.Fatal("id2 atom is not testAtom")
		return
	}
	if c1.local.elements[elemName].atoms[atomName2] != nil {
		t.Fatal("c1 local atom is not nil")
		return
	}
	if c2.local.elements[elemName].atoms[atomName2] == nil {
		t.Fatal("c2 local atom is nil")
		return
	}
	t.Logf("id2=(%v) t2=(%v)", id2, t2)
	t2.Release()

	// cr1 get cr2 id
	c1cr2a, c1cr2t, err := c1cr2e.GetAtomID(atomName2, nil, false)
	if err != nil {
		t.Fatal(err.AddStack(nil))
		return
	}
	if c1cr2a == nil || c1cr2t != nil {
		t.Fatal("c1cr2a is nil or c1cr2t is not nil")
		return
	}
	c1cr2t.Release()

	// 让id1开始测试任务
	out, err := id1.SyncMessagingByName(c1.local, "testingRemoteTask", 20*time.Second, &Strings{Ss: []string{elemName, atomName1, atomName2}})
	if err != nil {
		t.Fatal(err.AddStack(nil))
		return
	}
	if out == nil || out.(*String).S != "OK" {
		t.Fatal("out is nil or out is not OK")
		return
	}

	<-time.After(100 * time.Millisecond)
	if remoteSuccessCounter != 7 {
		t.Fatal("remoteSuccessCounter is not 7")
		return
	}
}

// 测试一个节点的生命周期
// Test the life cycle of a nod
func TestSimulateCosmosNode_LifeCycle(t *testing.T) {
	// 一个节点的模拟

	c1 := simulateCosmosNode(t, "hello", "c1")

	// 两个节点的停止
	defer func() {
		if err := c1.Stop(); err != nil {
			t.Fatal(err)
		}
		<-time.After(3 * time.Second)
	}()

	<-time.After(5 * time.Second)
}

// 测试两个节点
// Test two nodes
func TestSimulateTwoCosmosNodes(t *testing.T) {
	// 两个节点的模拟
	// Simulate two nodes

	c1 := simulateCosmosNode(t, "hello", "c1")
	c2 := simulateCosmosNode(t, "hello", "c2")

	// 两个节点的停止
	defer func() {
		if err := c1.Stop(); err != nil {
			t.Fatal(err)
		}
		if err := c2.Stop(); err != nil {
			t.Fatal(err)
		}
		<-time.After(3 * time.Second)
	}()

	<-time.After(5 * time.Second)
}

// 测试一个节点的升级过程
// Test the upgrade process of a node
func TestSimulateUpgradeCosmosNode(t *testing.T) {
	// 两个节点的模拟
	// Simulate two nodes
	c1Old := simulateCosmosNode(t, "hello", "c1")
	watcher := simulateCosmosNode(t, "hello", "watcher")
	if e := c1Old.local.elements["testElement"]; e == nil || len(e.atoms) != 0 {
		t.Fatal("c1Old has no testElement")
	}
	//if len(c1Old.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("c1Old has remoteTrackIDMap")
	//}
	if e := watcher.local.elements["testElement"]; e == nil || len(e.atoms) != 0 {
		t.Fatal("watcher has no testElement")
	}
	//if len(watcher.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("c1Old has remoteTrackIDMap")
	//}
	<-time.After(1 * time.Millisecond)

	watcher.cluster.remoteMutex.RLock()
	watcherC1, has := watcher.cluster.remoteCosmos["c1"]
	watcher.cluster.remoteMutex.RUnlock()
	if !has {
		t.Fatal("watcher has no c1")
	}
	defer func() {
		if err := watcher.Stop(); err != nil {
			t.Fatal(err)
		}
	}()
	c1Old.cluster.remoteMutex.RLock()
	c1OldC1, has := c1Old.cluster.remoteCosmos["watcher"]
	c1Old.cluster.remoteMutex.RUnlock()
	if !has {
		t.Fatal("c1Old has no c1")
	}

	// 构造一个Atom，从watcher到c1Old
	wcr1e, err := watcherC1.getElement("testElement")
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	wcr1a1, wcr1aTracker1, err := wcr1e.SpawnAtom(watcher.local, "testAtom", &String{}, nil, false)
	if err != nil || wcr1a1 == nil || wcr1aTracker1 != nil || wcr1a1.GetIDInfo().Atom != "testAtom" {
		t.Fatal("SpawnAtom failed", err, wcr1a1, wcr1aTracker1)
	}
	t.Logf("old wcr1a1=(%v) wcr1aTracker1=(%v)", wcr1a1, wcr1aTracker1)
	if e := c1Old.local.elements["testElement"]; e == nil || len(e.atoms) != 1 {
		t.Fatal("c1Old has no testElement")
	}
	//if len(c1Old.cluster.remoteTrackIDMap) != 1 {
	//	t.Fatal("c1Old has remoteTrackIDMap")
	//}
	//if c1Old.cluster.remoteTrackIDMap[1] == nil {
	//	t.Fatal("c1Old has no remoteTrackIDMap[1]")
	//}
	//if len(watcher.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("watcher has remoteTrackIDMap")
	//}
	wcr1aTracker1.Release()
	//if len(c1Old.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("c1Old has remoteTrackIDMap")
	//}
	//if len(watcher.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("watcher has remoteTrackIDMap")
	//}

	// 构造一个Atom
	c1OldWe, err := c1OldC1.getElement("testElement")
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	c1OldWa, c1OldWaTracker, err := c1OldWe.SpawnAtom(c1Old.local, "testAtom", &String{}, nil, false)
	if err != nil || c1OldWa == nil || c1OldWaTracker != nil || c1OldWa.GetIDInfo().Atom != "testAtom" {
		t.Fatal("SpawnAtom failed", err, c1OldWa, c1OldWaTracker)
	}
	if e := watcher.local.elements["testElement"]; e == nil || len(e.atoms) != 1 {
		t.Fatal("c1Old has no testElement")
	}
	//if len(c1Old.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("c1Old has remoteTrackIDMap")
	//}
	//if len(watcher.cluster.remoteTrackIDMap) != 1 {
	//	t.Fatal("watcher has remoteTrackIDMap")
	//}
	//if watcher.cluster.remoteTrackIDMap[1] == nil {
	//	t.Fatal("watcher has no remoteTrackIDMap[1]")
	//}
	c1OldWaTracker.Release()
	//if len(c1Old.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("c1Old has remoteTrackIDMap")
	//}
	//if len(watcher.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("watcher has remoteTrackIDMap")
	//}

	// watcher获取c1Old的Atom
	wcr1a2, wcr1aTracker2, err := wcr1e.GetAtomID("testAtom", nil, false)
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	defer wcr1aTracker2.Release()
	//if len(c1Old.cluster.remoteTrackIDMap) != 1 {
	//	t.Fatal("c1Old has remoteTrackIDMap")
	//}
	//if c1Old.cluster.remoteTrackIDMap[2] == nil {
	//	t.Fatal("c1Old has no remoteTrackIDMap[1]")
	//}
	//if len(watcher.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("watcher has remoteTrackIDMap")
	//}

	// c1Old获取watcher的Atom
	c1OldWa2, c1OldWa2Tracker, err := c1OldWe.GetAtomID("testAtom", nil, false)
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	defer c1OldWa2Tracker.Release()
	//if len(watcher.cluster.remoteTrackIDMap) != 1 {
	//	t.Fatal("watcher has remoteTrackIDMap")
	//}
	//if watcher.cluster.remoteTrackIDMap[2] == nil {
	//	t.Fatal("watcher has no remoteTrackIDMap[1]")
	//}
	//if len(c1Old.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("c1Old has remoteTrackIDMap")
	//}
	_ = c1OldWa2

	<-time.After(3 * time.Second)
	c1New := simulateCosmosNode(t, "hello", "c1")
	defer func() {
		if err := c1New.Stop(); err != nil {
			t.Fatal(err)
		}
		<-time.After(1 * time.Second)
	}()
	<-time.After(5 * time.Millisecond)
	if c1Old.state != CosmosProcessStateOff {
		t.Fatal("c1Old.state != CosmosProcessStateOff")
	}
	if c1New.state != CosmosProcessStateRunning {
		t.Fatal("c1New.state != CosmosProcessStateOn")
	}

	//if len(c1Old.cluster.remoteTrackIDMap) != 1 {
	//	t.Fatal("c1Old has remoteTrackIDMap")
	//}
	//if len(c1New.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("c1New has remoteTrackIDMap")
	//}
	//if len(watcher.cluster.remoteTrackIDMap) != 0 {
	//	t.Fatal("watcher has remoteTrackIDMap")
	//}

	<-time.After(1 * time.Millisecond)

	out, err := wcr1a2.SyncMessagingByName(watcher.local, "testMessage", 0, &Nil{})
	if err.Code != ErrAtomNotExists {
		t.Fatal(err)
	}

	wcr3a, wcr3aTracker, err := wcr1e.SpawnAtom(watcher.local, "testAtom", &String{}, nil, false)
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	_ = wcr3a
	defer wcr3aTracker.Release()

	out, err = wcr1a2.SyncMessagingByName(watcher.local, "testMessage", 0, &Nil{})
	t.Log(out, err)

	<-time.After(5 * time.Second)

}

// 简单模拟两个节点的各种互相调用情况，把atomos_remote_service的代码都过一遍
// Simply simulate the various mutual calls between two nodes, and go through the code of atomos_remote_service
func TestSimulateTwoCosmosNodeRPC(t *testing.T) {
	// 两个节点的模拟
	// Simulate two nodes

	c1 := simulateCosmosNode(t, "hello", "c1")
	c2 := simulateCosmosNode(t, "hello", "c2")

	// 两个节点的停止
	defer func() {
		if err := c1.Stop(); err != nil {
			t.Fatal(err)
		}
		if err := c2.Stop(); err != nil {
			t.Fatal(err)
		}
		<-time.After(3 * time.Second)
	}()

	<-time.After(1 * time.Microsecond)
	c1.cluster.remoteMutex.RLock()
	c1cr2, has := c1.cluster.remoteCosmos["c2"]
	c1.cluster.remoteMutex.RUnlock()
	if !has {
		t.Fatal("c1 has no c2")
	}
	c2.cluster.remoteMutex.RLock()
	c2cr1, has := c2.cluster.remoteCosmos["c1"]
	c2.cluster.remoteMutex.RUnlock()
	if !has {
		t.Fatal("c2 has no c2")
	}

	// 发送到c2的Cosmos，因为不支持，所以应该返回错误。
	// The Cosmos sent to c2 should return an error because it is not supported.
	_, err := c1cr2.SyncMessagingByName(c1.local, "testElement", 0, &Nil{})
	if err == nil {
		t.Fatal(err.AddStack(nil))
	} else {
		t.Logf("ok to get err=(%v)", err.Message)
	}
	// 发送到c1的Element，因为不支持，所以应该返回错误。
	// The Element sent to c1 should return an error because it is not supported.
	_, err = c2cr1.SyncMessagingByName(c2.local, "testElement", 0, &Nil{})
	if err == nil {
		t.Fatal(err.AddStack(nil))
	} else {
		t.Logf("ok to get err=(%v)", err.Message)
	}

	// 发送到c2的Element。
	c1cr2e, err := c1cr2.getElement("testElement")
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	out, err := c1cr2e.SyncMessagingByName(c1.local, "testMessage", 0, &Nil{})
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	t.Logf("c1cr2e out=(%v)", out)
	// 发送到c1的Element。
	c2cr1e, err := c2cr1.getElement("testElement")
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	out, err = c2cr1e.SyncMessagingByName(c1.local, "testMessage", 0, &Nil{})
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	t.Logf("c1cr2e out=(%v)", out)

	// 测试AtomID的获取，因为没有，所以应该返回错误。
	id1, t1, err := c1cr2e.GetAtomID("testAtom", nil, false)
	if id1 != nil || t1 != nil || err == nil || err.Code != ErrAtomNotExists {
		t.Fatal(err.AddStack(nil))
	}

	// Spawn
	id2, t2, err := c1cr2e.SpawnAtom(c1.local, "testAtom", &String{}, nil, false)
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	if id2 == nil || t2 != nil {
		t.Fatal("id2 or t2 is nil")
	}
	if id2.GetIDInfo().Atom != "testAtom" {
		t.Fatal("id2 atom is not testAtom")
	}
	t.Logf("id2=(%v) t2=(%v)", id2, t2)
	// Messaging
	out, err = id2.SyncMessagingByName(c1.local, "testMessage", 0, &Nil{})
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	t.Logf("id2 out=(%v)", out)

	// Check Element
	num := c1cr2e.GetActiveAtomsNum()
	if num != 1 {
		t.Fatal("num is not 1")
	}

	// Get Atom ID
	id3, t3, err := c1cr2e.GetAtomID("testAtom", nil, false)
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	if id3 == nil || t3 != nil {
		t.Fatal("id3 or t3 is nil")
	}
	if id3.GetIDInfo().Atom != "testAtom" {
		t.Fatal("id3 atom is not testAtom")
	}
	t.Logf("id3=(%v) t3=(%v)", id3, t3)
	// Check remote reference directly
	if a := c2.local.elements["testElement"].atoms["testAtom"]; a == nil {
		t.Fatal("testAtom is nil")
		//} else if a.idTrackerManager.refCount() != 2 {
		//	t.Fatal("refCount is not 2")
	}

	// Release Atom ID
	t3.Release()
	if a := c2.local.elements["testElement"].atoms["testAtom"]; a == nil {
		t.Fatal("testAtom is nil")
		//} else if a.idTrackerManager.refCount() != 1 {
		//	t.Fatal("refCount is not 1")
	}

	// Get Element ID State
	if c1cr2e.State() != AtomosWaiting {
		t.Fatal("id state is not AtomosWaiting")
	}
	// Get Atom ID State
	if id3.State() != AtomosWaiting {
		t.Fatal("id state is not AtomosWaiting")
	}

	// Get Element ID Idle Time
	<-time.After(1 * time.Second)
	if c1cr2e.IdleTime() < 1*time.Second {
		t.Fatal("element id idle time is less than 1 second")
	} else {
		t.Logf("element id idle time=(%v)", c1cr2e.IdleTime())
	}
	// Get Atom ID Idle Time
	<-time.After(1 * time.Second)
	if id3.IdleTime() < 2*time.Second {
		t.Fatal("atom id idle time is less than 2 second")
	} else {
		t.Logf("atom id idle time=(%v)", id3.IdleTime())
	}

	// Get Scale ID Not Exists
	id4, t4, err := c1cr2.CosmosGetScaleAtomID(c1.local, "testElement", "ScaleTestMessageError", 0, &Nil{})
	if err == nil {
		t.Fatal(err.AddStack(nil))
	}
	if id4 != nil || t4 != nil {
		t.Fatal("id4 or t4 is not nil")
	}

	// Get Scale ID
	id5, t5, err := c1cr2.CosmosGetScaleAtomID(c1.local, "testElement", "ScaleTestMessage", 0, &Nil{})
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	if id5 == nil || t5 != nil {
		t.Fatal("id5 or t5 is nil")
	}
	t.Logf("id5=(%v) t5=(%v)", id5, t5)
	// Messaging
	out, err = id5.SyncMessagingByName(c1.local, "testMessage", 0, &Nil{})
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	t.Logf("id5 out=(%v)", out)
	if a := c2.local.elements["testElement"].atoms["testAtom"]; a == nil {
		t.Fatal("testAtom is nil")
		//} else if a.idTrackerManager.refCount() != 2 {
		//	t.Fatal("refCount is not 2")
	}
	t5.Release()
	if a := c2.local.elements["testElement"].atoms["testAtom"]; a == nil {
		t.Fatal("testAtom is nil")
		//} else if a.idTrackerManager.refCount() != 1 {
		//	t.Fatal("refCount is not 1")
	}

	// Element Broadcast
	err = c2cr1.ElementBroadcast(c1.local, "testKey", "type", []byte("buffer"))
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}

	// Kill
	if a := c2.local.elements["testElement"].atoms["testAtom"]; a == nil {
		t.Fatal("testAtom is nil")
		//} else if a.idTrackerManager.refCount() != 1 {
		//	t.Fatal("refCount is not 1")
	} else if a.State() != AtomosWaiting {
		t.Fatal("atom state is not AtomosHalt")
	} else if len(a.atomos.it.idMap) != 0 {
		t.Fatal("atom idMap length is not 1")
	}
	err = id2.Kill(c1.local, 0)
	if err != nil {
		t.Fatal(err.AddStack(nil))
	}
	if a := c2.local.elements["testElement"].atoms["testAtom"]; a != nil {
		t.Fatal("testAtom is not nil")
	}
}

func simulateCosmosNode(t *testing.T, cosmosName, cosmosNode string) *CosmosProcess {
	al, el := getLogger(t)
	process, err := newCosmosProcess(cosmosName, cosmosNode, al, el)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Start(getRunnable(t, process, cosmosName, cosmosNode)); err != nil {
		t.Fatal(err)
	}
	return process
}

func getLogger(t *testing.T) (loggingFn, loggingFn) {
	return func(s string) {
			t.Log(strings.TrimSuffix(s, "\n"))
		}, func(s string) {
			t.Error(strings.TrimSuffix(s, "\n"))
		}
}

func getRunnable(t *testing.T, process *CosmosProcess, cosmosName, cosmosNode string) *CosmosRunnable {
	runnable := &CosmosRunnable{}
	runnable.
		SetConfig(&Config{
			Cosmos:     cosmosName,
			Node:       cosmosNode,
			LogLevel:   0,
			LogPath:    "",
			LogMaxSize: 0,
			BuildPath:  "",
			BinPath:    "",
			RunPath:    "",
			EtcPath:    "",
			EnableCluster: &CosmosClusterConfig{
				Enable:        true,
				EtcdEndpoints: testClusterEtcdEndpoints,
				OptionalPorts: testOptionalPorts,
				EnableCert:    nil,
			},
			Customize: nil,
		}).
		SetMainScript(&testMainScript{t: t}).
		AddElementImplementation(newTestFakeElement(t, process, false))
	return runnable
}

var (
	testClusterEtcdEndpoints = []string{"127.0.0.1:2379"}
	testOptionalPorts        = []int32{10001, 10002, 10003}
)
