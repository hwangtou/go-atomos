package atomos

import (
	"context"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

func TestRuntimeReviewSyncChainLinkAndWaitLifecycle(t *testing.T) {
	id := &IDInfo{
		Type:       IDType_Atom,
		Cosmos:     "runtime_review",
		Node:       "node",
		Element:    "element",
		Atom:       "atom",
		InstanceId: 41,
	}
	if got, want := syncChainLink(id), id.Info()+"#41"; got != want {
		t.Fatalf("syncChainLink() = %q, want %q", got, want)
	}
	otherInstance := proto.Clone(id).(*IDInfo)
	otherInstance.InstanceId = 42
	if syncChainLink(id) == syncChainLink(otherInstance) {
		t.Fatal("different atom instances must have different sync-chain links")
	}

	p := &CosmosProcess{}
	key1 := p.beginSyncWait(id)
	key2 := p.beginSyncWait(id)
	otherKey := p.beginSyncWait(otherInstance)
	if key1 == "" || key1 != key2 {
		t.Fatalf("wait keys for one instance differ: %q != %q", key1, key2)
	}
	if got := p.syncWaits[key1]; got != 2 {
		t.Fatalf("active wait count = %d, want 2", got)
	}
	p.endSyncWait(key1)
	if !p.isSyncWaiting(id) {
		t.Fatal("one completed wait must not clear another active wait")
	}
	p.endSyncWait(key2)
	if p.isSyncWaiting(id) {
		t.Fatal("last completed wait must clear the instance entry")
	}
	if _, ok := p.syncWaits[key1]; ok {
		t.Fatal("last completed wait left an empty map entry")
	}
	if !p.isSyncWaiting(otherInstance) {
		t.Fatal("clearing one instance must not clear another")
	}
	p.endSyncWait(otherKey)

	func() {
		key := p.beginSyncWait(id)
		defer p.endSyncWait(key)
		if !p.isSyncWaiting(id) {
			t.Fatal("instance must be active while its remote call can time out")
		}
	}()
	if p.isSyncWaiting(id) {
		t.Fatal("deferred cleanup after a call return must clear timed-out waits")
	}
}

func TestRuntimeReviewSyncChainIncludesLocalIntermediate(t *testing.T) {
	cluster := newTestCosmosProcessSimulateCluster(t, 51000, "runtime_review_chain", "runtime_review_node")
	defer cluster.close()

	const (
		nodeTarget = "runtime_review_node_target"
		atomA      = "runtime_review_a"
		atomB      = "runtime_review_b"
		atomC      = "runtime_review_c"
	)
	srcElem, gerr := cluster.sourceProcess.local.getLocalElement(ForTestAtomosName)
	if gerr != nil {
		t.Fatalf("getLocalElement(source): %v", gerr)
	}
	tgtElem, gerr := cluster.targetProcess.local.getLocalElement(ForTestAtomosName)
	if gerr != nil {
		t.Fatalf("getLocalElement(target): %v", gerr)
	}
	implA := spawnTestAtom(t, srcElem, atomA)
	implB := spawnTestAtom(t, tgtElem, atomB)
	implC := spawnTestAtom(t, tgtElem, atomC)

	srcToTarget := cluster.sourceProcess.cluster.remoteCosmos[nodeTarget].elements[ForTestAtomosName]
	implA.remoteHook = func(self AtomSelfID) *Error {
		id, tracker, callErr := srcToTarget.GetAtomID(atomB, nil)
		if callErr != nil {
			return asError(callErr).AddStack(self)
		}
		defer tracker.Release()
		_, callErr = id.SyncMessagingByName(self, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(5*time.Second))
		return asError(callErr).AddStack(self)
	}
	implB.remoteHook = matrixCallAtom(t, atomC, 5, 5*time.Second)
	chainAtC := make(chan []string, 1)
	implC.remoteHook = func(self AtomSelfID) *Error {
		chainAtC <- cluster.targetProcess.currentSyncChain(getGoID())
		return nil
	}

	atomAID, tracker, err := srcElem.GetAtomID(atomA, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
	if err != nil {
		t.Fatalf("GetAtomID(A): %v", err)
	}
	defer tracker.Release()
	if _, err = atomAID.SyncMessagingByName(cluster.sourceProcess.local, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(10*time.Second)); err != nil {
		t.Fatalf("A -> remote B -> local C failed: %v", err)
	}

	want := []string{syncChainLink(implA.self.GetIDInfo()), syncChainLink(implB.self.GetIDInfo())}
	select {
	case got := <-chainAtC:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ambient chain at C = %v, want %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("C did not record its ambient sync chain")
	}

	// Close the ring through a remote loopback hop to the intermediate B. The
	// receiver must see B in the propagated chain and still actively waiting.
	targetRemote := newCosmosRemoteFromNodeInfo(cluster.targetProcess, &CosmosNodeVersionInfo{Id: srcToTarget.cosmos.remote.info})
	targetRemote.current = &cosmosRemoteVersion{
		process: cluster.targetProcess,
		avail:   true,
		client:  srcToTarget.cosmos.current.getClient(),
	}
	targetRemote.enable = true
	targetElem := newElementRemoteFromSource(targetRemote, srcToTarget.remote.info, srcToTarget.current, srcToTarget.version)
	implC.remoteHook = func(self AtomSelfID) *Error {
		id, _, callErr := targetElem.GetAtomID(atomB, nil)
		if callErr != nil {
			return asError(callErr).AddStack(self)
		}
		_, callErr = id.SyncMessagingByName(self, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(5*time.Second))
		return asError(callErr).AddStack(self)
	}
	if _, err = atomAID.SyncMessagingByName(cluster.sourceProcess.local, "Greeting", &ForTestGreetingI{Mode: 5}, ArgBaseAtomosTimeout(10*time.Second)); CodeOf(err) != ErrCodeIDFirstSyncCallDeadlock {
		t.Fatalf("A -> remote B -> local C -> remote B error = %v, want cross-node deadlock", err)
	}
}

func TestRuntimeReviewAsyncPushToStoppedTargetClearsPending(t *testing.T) {
	p := newTestCosmosProcessWithoutCluster(t, "runtime_review_async", "node")
	caller := newBaseAtomosForTest(t, &IDInfo{
		Type: IDType_Atom, Cosmos: "runtime_review_async", Node: "node", Element: "element", Atom: "caller",
	}, p)
	target := newBaseAtomosForTest(t, &IDInfo{
		Type: IDType_Atom, Cosmos: "runtime_review_async", Node: "node", Element: "element", Atom: "target",
	}, p)
	t.Cleanup(func() {
		if caller.atomos.mailbox.isRunning() {
			_ = caller.Kill(p.local, ArgBaseAtomosWaitKilled())
		}
		waitMailboxGoroutineExit(caller.atomos.mailbox, 5*time.Second)
	})
	if err := target.Kill(p.local, ArgBaseAtomosWaitKilled()); err != nil {
		t.Fatalf("stop target: %v", err)
	}
	waitMailboxGoroutineExit(target.atomos.mailbox, 5*time.Second)

	callbackCalled := false
	err := target.AsyncMessagingByName(caller, "unused", nil, func(proto.Message, error) {
		callbackCalled = true
	})
	if CodeOf(err) != ErrCodeAtomosIsNotRunning {
		t.Fatalf("PushAsyncMessage error = %v, want ErrCodeAtomosIsNotRunning", err)
	}
	if got := caller.atomos.AsyncCallbackPending(); got != 0 {
		t.Fatalf("pending callbacks after rejected push = %d, want 0", got)
	}
	if callbackCalled {
		t.Fatal("a rejected push must pop, not execute, the registered callback")
	}
}

func TestRuntimeReviewRawRPCMissingID(t *testing.T) {
	p := newTestCosmosProcessWithoutCluster(t, "runtime_review_rpc", "node")
	svc := &atomosRemoteService{process: p}
	tests := []struct {
		name string
		call func(nilRequest bool) (*Error, error)
	}{
		{
			name: "GetIDState",
			call: func(nilRequest bool) (*Error, error) {
				var req *CosmosRemoteGetIDStateReq
				if !nilRequest {
					req = &CosmosRemoteGetIDStateReq{}
				}
				rsp, err := svc.GetIDState(context.Background(), req)
				if rsp == nil {
					return nil, err
				}
				return rsp.Error, err
			},
		},
		{
			name: "GetIDIdleTime",
			call: func(nilRequest bool) (*Error, error) {
				var req *CosmosRemoteGetIDIdleTimeReq
				if !nilRequest {
					req = &CosmosRemoteGetIDIdleTimeReq{}
				}
				rsp, err := svc.GetIDIdleTime(context.Background(), req)
				if rsp == nil {
					return nil, err
				}
				return rsp.Error, err
			},
		},
		{
			name: "SyncMessagingByName",
			call: func(nilRequest bool) (*Error, error) {
				var req *CosmosRemoteSyncMessagingByNameReq
				if !nilRequest {
					req = &CosmosRemoteSyncMessagingByNameReq{}
				}
				rsp, err := svc.SyncMessagingByName(context.Background(), req)
				if rsp == nil {
					return nil, err
				}
				return rsp.Error, err
			},
		},
		{
			name: "AsyncMessagingByName",
			call: func(nilRequest bool) (*Error, error) {
				var req *CosmosRemoteAsyncMessagingByNameReq
				if !nilRequest {
					req = &CosmosRemoteAsyncMessagingByNameReq{}
				}
				rsp, err := svc.AsyncMessagingByName(context.Background(), req)
				if rsp == nil {
					return nil, err
				}
				return rsp.Error, err
			},
		},
		{
			name: "KillAtom",
			call: func(nilRequest bool) (*Error, error) {
				var req *CosmosRemoteKillAtomReq
				if !nilRequest {
					req = &CosmosRemoteKillAtomReq{}
				}
				rsp, err := svc.KillAtom(context.Background(), req)
				if rsp == nil {
					return nil, err
				}
				return rsp.Error, err
			},
		},
	}

	for _, tc := range tests {
		for _, nilRequest := range []bool{true, false} {
			name := tc.name + "/empty_request"
			if nilRequest {
				name = tc.name + "/nil_request"
			}
			t.Run(name, func(t *testing.T) {
				bodyErr, transportErr := tc.call(nilRequest)
				if bodyErr == nil || bodyErr.Code != ErrCodeCosmosRemoteRequestInvalid {
					t.Fatalf("body error = %v, want ErrCodeCosmosRemoteRequestInvalid", bodyErr)
				}
				if transportErr == nil {
					t.Fatal("call-class wrapper must also return a non-OK status")
				}
				if got := CodeOf(grpcErrorToError(transportErr)); got != ErrCodeCosmosRemoteRequestInvalid {
					t.Fatalf("status error code = %d, want ErrCodeCosmosRemoteRequestInvalid", got)
				}
			})
		}
	}
}

func TestRuntimeReviewLifecycleHooksAllowStateAndIdleTime(t *testing.T) {
	p := newTestCosmosProcessWithoutCluster(t, "runtime_review_hooks", "node")
	target := newBaseAtomosForTest(t, &IDInfo{
		Type: IDType_Atom, Cosmos: "runtime_review_hooks", Node: "node", Element: "element", Atom: "atom",
	}, p)
	runnable := &CosmosRunnable{}
	p.local.runnable = runnable
	t.Cleanup(func() {
		p.local.runnable = nil
		if target.atomos.mailbox.isRunning() {
			target.atomos.mailbox.mutex.Lock()
			target.atomos.state = BaseAtomosWaiting
			target.atomos.mailbox.mutex.Unlock()
			_ = target.Kill(p.local, ArgBaseAtomosWaitKilled())
		}
		waitMailboxGoroutineExit(target.atomos.mailbox, 5*time.Second)
	})

	tests := []struct {
		name       string
		want       BaseAtomosState
		setHook    func(func(*IDInfo))
		transition func()
	}{
		{
			name: "spawning",
			want: BaseAtomosSpawning,
			setHook: func(hook func(*IDInfo)) {
				runnable.SetSpawningHook(hook)
			},
			transition: target.atomos.setSpawning,
		},
		{
			name: "spawn",
			want: BaseAtomosWaiting,
			setHook: func(hook func(*IDInfo)) {
				runnable.SetSpawnHook(hook)
			},
			transition: target.atomos.setSpawn,
		},
		{
			name: "stopping",
			want: BaseAtomosStopping,
			setHook: func(hook func(*IDInfo)) {
				runnable.SetStoppingHook(hook)
			},
			transition: target.atomos.setStopping,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got BaseAtomosState
			tc.setHook(func(*IDInfo) {
				got = target.State()
				_ = target.IdleTime()
			})
			done := make(chan struct{})
			go func() {
				tc.transition()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("lifecycle hook deadlocked while reading State or IdleTime")
			}
			if got != tc.want {
				t.Fatalf("state observed in hook = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRuntimeReviewSyncPartialReplyKeepsTransportOK(t *testing.T) {
	p, cerr := newCosmosProcess("runtime_review_partial", "node", newTestLogging(t))
	if cerr != nil {
		t.Fatalf("newCosmosProcess: %v", cerr)
	}
	runnable := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: "runtime_review_partial", Node: "node"})
	const message = "RuntimeReviewPartialReply"
	runnable.implements[ForTestAtomosName].ElementHandlers[message] = func(ID, Atomos, proto.Message) (proto.Message, error) {
		return &String{S: "partial"}, NewError(ErrCodeAtomIsStopping, "partial failure").AddStack(nil)
	}
	if err := p.Start(runnable); err != nil {
		t.Fatalf("start process: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop() })

	callerInfo := &IDInfo{Type: IDType_Cosmos, Cosmos: "runtime_review_partial", Node: "caller"}
	p.cluster.remoteCosmos[callerInfo.Node] = newCosmosRemoteFromNodeInfo(p, &CosmosNodeVersionInfo{Id: callerInfo})
	elem, gerr := p.local.getLocalElement(ForTestAtomosName)
	if gerr != nil {
		t.Fatalf("getLocalElement: %v", gerr)
	}
	svc := &atomosRemoteService{process: p}
	rsp, transportErr := svc.SyncMessagingByName(context.Background(), &CosmosRemoteSyncMessagingByNameReq{
		CallerId: callerInfo,
		To:       elem.GetIDInfo(),
		Message:  message,
	})
	if transportErr != nil {
		t.Fatalf("partial reply plus error must use transport OK: %v", transportErr)
	}
	if rsp == nil || rsp.Reply == nil || rsp.Error == nil {
		t.Fatalf("response must retain both partial reply and error: %v", rsp)
	}
	if rsp.Error.Code != ErrCodeAtomIsStopping {
		t.Fatalf("body error code = %d, want ErrCodeAtomIsStopping", rsp.Error.Code)
	}
	reply, err := rsp.Reply.UnmarshalNew()
	if err != nil {
		t.Fatalf("unmarshal partial reply: %v", err)
	}
	if got := reply.(*String).S; got != "partial" {
		t.Fatalf("partial reply = %q, want %q", got, "partial")
	}
}

func TestRuntimeReviewTryKillingValidationSkipsCleanup(t *testing.T) {
	p := newTestCosmosProcessWithoutCluster(t, "runtime_review_kill", "node")
	p.mutex.Lock()
	p.state = CosmosProcessStatePrepare
	p.mutex.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := &atomosRemoteService{process: p}
	rsp, err := svc.TryKilling(ctx, &CosmosRemoteTryKillingReq{})
	if err != nil {
		t.Fatalf("TryKilling transport error: %v", err)
	}
	if rsp == nil || rsp.Error == nil || rsp.Error.Code != ErrCodeCosmosProcessCannotStopPrepareState {
		t.Fatalf("TryKilling response = %v, want prepare-state validation error", rsp)
	}
	p.mutex.RLock()
	state := p.state
	p.mutex.RUnlock()
	if state != CosmosProcessStatePrepare {
		t.Fatalf("process state after rejected TryKilling = %v, want Prepare", state)
	}
	if !p.local.atomos.mailbox.isRunning() {
		t.Fatal("rejected TryKilling stopped the local mailbox")
	}
	if p.logging.logBox == nil || !p.logging.logBox.isRunning() {
		t.Fatal("rejected TryKilling stopped process logging")
	}
}
