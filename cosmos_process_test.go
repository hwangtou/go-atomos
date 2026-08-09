package atomos

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newTestCosmosProcessWithoutCluster(t *testing.T, cosmosName, cosmosNode string) *CosmosProcess {
	id := &IDInfo{Type: IDType_Cosmos, Cosmos: cosmosName, Node: cosmosNode}
	p := &CosmosProcess{
		mutex:     sync.RWMutex{},
		state:     CosmosProcessStateRunning,
		startupID: uint64(time.Now().UnixNano()),
		logging:   &loggingAtomos{},
		local: &CosmosLocal{
			process:  nil,
			runnable: nil,
			atomos:   nil,
			mutex:    sync.RWMutex{},
			elements: map[string]*ElementLocal{},
		},
	}
	if err := p.logging.init(newTestLogging(t)); err != nil {
		t.Fatalf("Failed to init logging: %v", err)
	}
	p.local.process = p
	p.local.atomos = NewBaseAtomos(p.local, id, LogLevel_Debug, p.local, p.local, p)
	if err := p.local.atomos.start(func() *Error {
		return nil
	}); err != nil {
		t.Fatalf("Failed to start local BaseAtomos: %v", err)
	}

	// Stop the local atomos mailbox and the process logging mailbox when the
	// test finishes. Leaked goroutines keep allocating debug mails and calling
	// t.Log after the test completed, which races with the testing framework
	// and breaks other tests' allocation-debug assertions.
	t.Cleanup(func() {
		if p.local.atomos.mailbox.isRunning() {
			if err := p.local.atomos.PushKillMail(p.local, nil); err != nil {
				t.Logf("Cleanup: PushKillMail local atomos failed: %v", err)
			}
			for deadline := time.Now().Add(5 * time.Second); p.local.atomos.mailbox.isRunning() && time.Now().Before(deadline); {
				time.Sleep(time.Millisecond)
			}
		}
		waitMailboxGoroutineExit(p.local.atomos.mailbox, 5*time.Second)
		if p.logging.logBox != nil && p.logging.logBox.isRunning() {
			p.logging.stop()
		}
		waitMailboxGoroutineExit(p.logging.logBox, 5*time.Second)
	})

	return p
}

func newTestCosmosProcessAsClusterNode(t *testing.T, cosmosName, cosmosNode string) *CosmosProcess {
	p, err := newCosmosProcess(cosmosName, cosmosNode, newTestLogging(t))
	if err != nil {
		t.Fatalf("Failed to create CosmosProcess: %v", err)
	}
	r := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: cosmosName, Node: cosmosNode})
	if err := p.Start(r); err != nil {
		t.Fatalf("Failed to start CosmosProcess: %v", err)
	}

	return p
}

type testCluster struct {
	t *testing.T

	sourcePort    int
	sourceProcess *CosmosProcess
	sourceServer  *grpc.Server

	targetPort    int
	targetProcess *CosmosProcess
	targetServer  *grpc.Server
}

func newTestCosmosProcessSimulateCluster(t *testing.T, basePort int, cosmosName, cosmosNodePrefix string) *testCluster {
	tc := &testCluster{
		t: t,
	}
	tc.sourcePort = basePort
	tc.sourceProcess = newTestCosmosProcessAsClusterNode(t, cosmosName, cosmosNodePrefix+"_source")
	tc.sourceServer = tc.newTestGRPCServer(tc.sourceProcess, tc.sourcePort)
	tc.sourceProcess.cluster.remoteCosmos[cosmosNodePrefix+"_target"] = newCosmosRemoteFromNodeInfo(tc.sourceProcess, &CosmosNodeVersionInfo{
		Node:    "",
		Address: "",
		Id: &IDInfo{
			Type:   IDType_Cosmos,
			Cosmos: cosmosName,
			Node:   cosmosNodePrefix + "_target",
		},
		State: 0,
		Elements: map[string]*IDInfo{
			ForTestAtomosName: {
				Type:    IDType_Element,
				Cosmos:  cosmosName,
				Node:    cosmosNodePrefix + "_target",
				Element: ForTestAtomosName,
			},
		},
	})

	tc.targetPort = basePort + 1
	tc.targetProcess = newTestCosmosProcessAsClusterNode(t, cosmosName, cosmosNodePrefix+"_target")
	tc.targetServer = tc.newTestGRPCServer(tc.targetProcess, tc.targetPort)
	tc.targetProcess.cluster.remoteCosmos[cosmosNodePrefix+"_source"] = newCosmosRemoteFromNodeInfo(tc.targetProcess, &CosmosNodeVersionInfo{
		Id: &IDInfo{
			Type:   IDType_Cosmos,
			Cosmos: cosmosName,
			Node:   cosmosNodePrefix + "_source",
		},
	})

	dialOption := grpc.WithTransportCredentials(insecure.NewCredentials())
	// Tests need a deterministic connection: grpc.NewClient (used in
	// production since 885846a) is lazy/non-blocking, which makes the first
	// RPC bear the connect cost and intermittently trips its own timeout on
	// Windows. DialContext+WithBlock blocks here until the connection is truly
	// up (or fails fast), so every subsequent RPC in the test starts from a
	// ready connection. (NewClient remains correct for production.)
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	sourceConn, er := grpc.DialContext(dialCtx, fmt.Sprintf(":%d", tc.targetPort), dialOption, grpc.WithBlock())
	dialCancel()
	if er != nil {
		t.Fatalf("Failed to dial source→target :%d: %v", tc.targetPort, er)
	}
	sourceToTarget := tc.sourceProcess.cluster.remoteCosmos[cosmosNodePrefix+"_target"]
	sourceToTarget.enable = true
	sourceToTarget.current = &cosmosRemoteVersion{
		process: tc.sourceProcess,
		info:    nil,
		avail:   true,
		client:  sourceConn,
		version: "",
	}
	sourceToTarget.elements = map[string]*ElementRemoteFromSource{
		ForTestAtomosName: newElementRemoteFromSource(
			sourceToTarget,
			// Element-level IDInfo (NOT the node-level remote.info): the
			// ElementRemote carries this info and ElementRemote.SpawnAtom/
			// GetAtomID read e.remote.info.Element to fill the wire request.
			// Passing the node-level info (empty Element) made every cosmos-layer
			// remote spawn/get fail with "Local element not found" on the target.
			&IDInfo{
				Type:    IDType_Element,
				Cosmos:  cosmosName,
				Node:    cosmosNodePrefix + "_target",
				Element: ForTestAtomosName,
			},
			tc.sourceProcess.local.runnable.implements[ForTestAtomosName].Interface,
			""),
	}

	dialOption = grpc.WithTransportCredentials(insecure.NewCredentials())
	dialCtx2, dialCancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	targetConn, er := grpc.DialContext(dialCtx2, fmt.Sprintf(":%d", tc.sourcePort), dialOption, grpc.WithBlock())
	dialCancel2()
	if er != nil {
		t.Fatalf("Failed to dial target→source :%d: %v", tc.sourcePort, er)
	}
	targetToSource := tc.targetProcess.cluster.remoteCosmos[cosmosNodePrefix+"_source"]
	targetToSource.enable = true
	targetToSource.current = &cosmosRemoteVersion{
		process: tc.targetProcess,
		info:    nil,
		avail:   true,
		client:  targetConn,
		version: "",
	}
	targetToSource.elements = map[string]*ElementRemoteFromSource{
		ForTestAtomosName: newElementRemoteFromSource(
			targetToSource,
			// Element-level IDInfo, see source→target block above for rationale.
			&IDInfo{
				Type:    IDType_Element,
				Cosmos:  cosmosName,
				Node:    cosmosNodePrefix + "_source",
				Element: ForTestAtomosName,
			},
			tc.targetProcess.local.runnable.implements[ForTestAtomosName].Interface,
			""),
	}

	return tc
}

func (tc *testCluster) close() {
	if tc.sourceServer != nil {
		tc.sourceServer.Stop()
		tc.sourceServer = nil
	}
	if tc.sourceProcess != nil {
		tc.sourceProcess.Stop()
		// Stop() returns before the mailbox loop goroutines have fully exited
		// (their deferred final log runs after the stop acknowledgment). Wait
		// for actual goroutine exit so no late t.Log races with test teardown.
		waitMailboxGoroutineExit(tc.sourceProcess.local.atomos.mailbox, 5*time.Second)
		waitMailboxGoroutineExit(tc.sourceProcess.logging.logBox, 5*time.Second)
		tc.sourceProcess = nil
	}
	if tc.targetServer != nil {
		tc.targetServer.Stop()
		tc.targetServer = nil
	}
	if tc.targetProcess != nil {
		tc.targetProcess.Stop()
		waitMailboxGoroutineExit(tc.targetProcess.local.atomos.mailbox, 5*time.Second)
		waitMailboxGoroutineExit(tc.targetProcess.logging.logBox, 5*time.Second)
		tc.targetProcess = nil
	}
}

func (tc *testCluster) newTestGRPCServer(process *CosmosProcess, port int) *grpc.Server {
	listener, er := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if er != nil {
		tc.t.Fatalf("CosmosProcess: gRPC server listen failed. err=(%v)", er)
	}
	// Try to start grpc server.
	var svr *grpc.Server
	svr = grpc.NewServer()
	// Register AtomosRemoteService.
	grpcImpl := &atomosRemoteService{
		process: process,
	}
	RegisterAtomosRemoteServiceServer(svr, grpcImpl)
	go func() {
		if err := svr.Serve(listener); err != nil {
			tc.t.Fatalf("CosmosProcess: gRPC server serve failed. err=(%v)", err)
		}
	}()
	return svr
}
