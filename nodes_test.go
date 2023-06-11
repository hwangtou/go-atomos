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
		<-time.After(1 * time.Second)
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
		<-time.After(1 * time.Second)
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
