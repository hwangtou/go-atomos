package go_atomos

import (
	"strings"
	"testing"
	"time"
)

func TestSimulateCosmosNodeLifeCycle(t *testing.T) {
	// 一个节点的模拟

	c1 := simulateCosmosNode(t, "hello", "c1")

	// 两个节点的停止
	defer func() {
		if err := c1.Stop(); err != nil {
			t.Fatal(err)
		}
		<-time.After(5 * time.Second)
	}()

	<-time.After(10 * time.Second)
}

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
		<-time.After(5 * time.Second)
	}()

	<-time.After(10 * time.Second)
}

func TestSimulateUpgradeCosmosNode(t *testing.T) {
	// 两个节点的模拟
	// Simulate two nodes

	c1Old := simulateCosmosNode(t, "hello", "c1")
	_ = c1Old
	<-time.After(5 * time.Second)
	c1New := simulateCosmosNode(t, "hello", "c1")
	<-time.After(5 * time.Second)

	if err := c1New.Stop(); err != nil {
		t.Fatal(err)
	}
	<-time.After(5 * time.Second)
}

func TestSimulateUpgradeCosmosNode100Times(t *testing.T) {
	for i := 0; i < 100; i += 1 {
		TestSimulateUpgradeCosmosNode(t)
	}
}

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
		<-time.After(5 * time.Second)
	}()

	<-time.After(100 * time.Microsecond)
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

	// 发送到c2的Atom。
	id, tracker, err := c1cr2e.GetAtomID("testAtom", NewIDTrackerInfoFromLocalGoroutine(1))
	if id != nil || tracker != nil || err == nil || err.Code != ErrAtomNotExists {
		t.Fatal(err.AddStack(nil))
	}
}

func simulateCosmosNode(t *testing.T, cosmosName, cosmosNode string) *CosmosProcess {
	al, el := getLogger(t)
	process, err := newCosmosProcess(cosmosName, cosmosNode, al, el)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Start(getRunnable(t, cosmosName, cosmosNode)); err != nil {
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

func getRunnable(t *testing.T, cosmosName, cosmosNode string) *CosmosRunnable {
	runnable := &CosmosRunnable{}
	runnable.
		SetConfig(&Config{
			Cosmos:            cosmosName,
			Node:              cosmosNode,
			NodeList:          nil,
			KeepaliveNodeList: nil,
			ReporterUrl:       "",
			ConfigerUrl:       "",
			LogLevel:          0,
			LogPath:           "",
			LogMaxSize:        0,
			BuildPath:         "",
			BinPath:           "",
			RunPath:           "",
			EtcPath:           "",
			EnableCluster: &CosmosClusterConfig{
				Enable:        true,
				EtcdEndpoints: testClusterEtcdEndpoints,
				OptionalPorts: testOptionalPorts,
				EnableCert:    nil,
			},
			Customize: nil,
		}).
		SetMainScript(&testMainScript{t: t}).
		AddElementImplementation(newTestFakeElement(t, false))
	return runnable
}

var (
	testClusterEtcdEndpoints = []string{"127.0.0.1:2379"}
	testOptionalPorts        = []uint32{10001, 10002}
)
