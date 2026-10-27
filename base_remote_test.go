package atomos

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

func TestBaseRemote_LifeCycle(t *testing.T) {
	service := &baseRemoteTestService{
		stateRsp:    &CosmosRemoteGetIDStateRsp{State: int32(BaseAtomosWaiting)},
		idleTimeRsp: &CosmosRemoteGetIDIdleTimeRsp{IdleTime: int64(250 * time.Millisecond)},
		syncRsp:     &CosmosRemoteSyncMessagingByNameRsp{Reply: toAnyPb(t, &ForTestGreetingO{})},
		rsp:         &CosmosRemoteAsyncMessagingByNameRsp{},
		killRsp:     &CosmosRemoteKillAtomRsp{},
	}
	remote, process := newBaseRemoteTestRemote(t, service)

	state, err := remote.stateWithError()
	if err != nil || state != BaseAtomosWaiting {
		t.Fatalf("stateWithError = (%v, %v), want (Waiting, nil)", state, err)
	}
	if state := remote.State(); state != BaseAtomosWaiting {
		t.Fatalf("State = %v, want Waiting", state)
	}
	if idleTime := remote.IdleTime(); idleTime != 250*time.Millisecond {
		t.Fatalf("IdleTime = %v, want 250ms", idleTime)
	}
	out, err := remote.PushSyncMessage(process.local, "Greeting", &ForTestGreetingI{Mode: 7})
	if err != nil {
		t.Fatalf("PushSyncMessage failed: %v", err)
	}
	if _, ok := out.(*ForTestGreetingO); !ok {
		t.Fatalf("PushSyncMessage reply type = %T, want *ForTestGreetingO", out)
	}
	if err := remote.PushAsyncMessage(process.local, "Greeting", &ForTestGreetingI{Mode: 8}, nil); err != nil {
		t.Fatalf("PushAsyncMessage failed: %v", err)
	}
	if err := remote.Kill(process.local); err != nil {
		t.Fatalf("Kill failed: %v", err)
	}

	service.mutex.Lock()
	defer service.mutex.Unlock()
	if len(service.stateRequests) != 2 || len(service.idleTimeRequests) != 1 ||
		len(service.syncRequests) != 1 || len(service.requests) != 1 || len(service.killRequests) != 1 {
		t.Fatalf("unexpected request counts: state=%d idle=%d sync=%d async=%d kill=%d",
			len(service.stateRequests), len(service.idleTimeRequests), len(service.syncRequests), len(service.requests), len(service.killRequests))
	}
	if !proto.Equal(service.stateRequests[0].Id, remote.info) || !proto.Equal(service.idleTimeRequests[0].Id, remote.info) {
		t.Fatal("state or idle-time request carries the wrong target ID")
	}
	syncReq := service.syncRequests[0]
	if !proto.Equal(syncReq.To, remote.info) || syncReq.Message != "Greeting" || !proto.Equal(syncReq.CallerId, process.local.GetIDInfo()) {
		t.Fatalf("unexpected sync request: %v", syncReq)
	}
	var syncArg ForTestGreetingI
	if err := syncReq.Args.UnmarshalTo(&syncArg); err != nil || syncArg.Mode != 7 {
		t.Fatalf("sync argument = (%v, %v), want mode 7", &syncArg, err)
	}
	asyncReq := service.requests[0]
	if !proto.Equal(asyncReq.ToId, remote.info) || asyncReq.Message != "Greeting" || asyncReq.AsyncId != 0 {
		t.Fatalf("unexpected async request: %v", asyncReq)
	}
	if !proto.Equal(service.killRequests[0].Id, remote.info) || !proto.Equal(service.killRequests[0].CallerId, process.local.GetIDInfo()) {
		t.Fatalf("unexpected kill request: %v", service.killRequests[0])
	}
}

// TestBaseRemote_PinnedConnConcurrentAccess hammers validPinnedConn from
// multiple goroutines while the pinned connection is closed, forcing the
// Shutdown-detection clear path to race with concurrent reads. Before the
// pinnedConnBox guard this tripped -race (unsynchronized pinnedConn
// read/write); it must stay clean.
func TestBaseRemote_PinnedConnConcurrentAccess(t *testing.T) {
	// NewClient never connects by itself; we only exercise GetState/Close.
	conn, err := grpc.NewClient("passthrough:///127.0.0.1:1", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient failed: %v", err)
	}
	b := newBaseRemoteWithPinnedConn(nil, nil, conn)

	const readers = 8
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = b.validPinnedConn()
				_ = b.getPinnedConn()
			}
		}()
	}

	// Let readers hammer the live pin, then close the conn so the clear path
	// runs concurrently with the reads.
	time.Sleep(50 * time.Millisecond)
	if er := conn.Close(); er != nil {
		close(stop)
		wg.Wait()
		t.Fatalf("conn.Close failed: %v", er)
	}

	// Any reader may be the one to detect Shutdown and clear the pin.
	deadline := time.Now().Add(5 * time.Second)
	for b.getPinnedConn() != nil {
		if time.Now().After(deadline) {
			close(stop)
			wg.Wait()
			t.Fatal("pinned conn was not cleared after Close")
		}
		time.Sleep(time.Millisecond)
	}
	close(stop)
	wg.Wait()

	if c := b.validPinnedConn(); c != nil {
		t.Fatalf("validPinnedConn should stay nil after clear, got %v", c)
	}
}

// internal

func newBaseRemoteForTest(t *testing.T, cosmos *CosmosRemote, info *IDInfo) *BaseRemote {
	return &BaseRemote{
		cosmos: cosmos,
		info:   info,
		pinned: &pinnedConnBox{},
	}
}

type baseRemoteTestService struct {
	UnimplementedAtomosRemoteServiceServer
	mutex            sync.Mutex
	stateRsp         *CosmosRemoteGetIDStateRsp
	stateErr         error
	idleTimeRsp      *CosmosRemoteGetIDIdleTimeRsp
	idleTimeErr      error
	syncRsp          *CosmosRemoteSyncMessagingByNameRsp
	syncErr          error
	rsp              *CosmosRemoteAsyncMessagingByNameRsp
	err              error
	killRsp          *CosmosRemoteKillAtomRsp
	killErr          error
	failures         int
	calls            int
	stateRequests    []*CosmosRemoteGetIDStateReq
	idleTimeRequests []*CosmosRemoteGetIDIdleTimeReq
	syncRequests     []*CosmosRemoteSyncMessagingByNameReq
	requests         []*CosmosRemoteAsyncMessagingByNameReq
	killRequests     []*CosmosRemoteKillAtomReq
}

func (s *baseRemoteTestService) GetIDState(_ context.Context, req *CosmosRemoteGetIDStateReq) (*CosmosRemoteGetIDStateRsp, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.stateRequests = append(s.stateRequests, proto.Clone(req).(*CosmosRemoteGetIDStateReq))
	return s.stateRsp, s.stateErr
}

func (s *baseRemoteTestService) GetIDIdleTime(_ context.Context, req *CosmosRemoteGetIDIdleTimeReq) (*CosmosRemoteGetIDIdleTimeRsp, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.idleTimeRequests = append(s.idleTimeRequests, proto.Clone(req).(*CosmosRemoteGetIDIdleTimeReq))
	return s.idleTimeRsp, s.idleTimeErr
}

func (s *baseRemoteTestService) SyncMessagingByName(_ context.Context, req *CosmosRemoteSyncMessagingByNameReq) (*CosmosRemoteSyncMessagingByNameRsp, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.syncRequests = append(s.syncRequests, proto.Clone(req).(*CosmosRemoteSyncMessagingByNameReq))
	return s.syncRsp, s.syncErr
}

func (s *baseRemoteTestService) AsyncMessagingByName(_ context.Context, req *CosmosRemoteAsyncMessagingByNameReq) (*CosmosRemoteAsyncMessagingByNameRsp, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.calls++
	s.requests = append(s.requests, proto.Clone(req).(*CosmosRemoteAsyncMessagingByNameReq))
	if s.failures > 0 {
		s.failures--
		return nil, status.Error(codes.Unavailable, "injected response failure")
	}
	return s.rsp, s.err
}

func (s *baseRemoteTestService) KillAtom(_ context.Context, req *CosmosRemoteKillAtomReq) (*CosmosRemoteKillAtomRsp, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.killRequests = append(s.killRequests, proto.Clone(req).(*CosmosRemoteKillAtomReq))
	return s.killRsp, s.killErr
}

func newBaseRemoteTestProcess(t *testing.T, cosmosName string) *CosmosProcess {
	t.Helper()
	process, err := newCosmosProcess(cosmosName, "source", &reproLogger{})
	if err != nil {
		t.Fatalf("newCosmosProcess failed: %v", err)
	}
	runnable := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: cosmosName, Node: "source"})
	if err := process.Start(runnable); err != nil {
		t.Fatalf("process start failed: %v", err)
	}
	t.Cleanup(func() { _ = process.Stop() })
	return process
}

func newBaseRemoteTestRemote(t *testing.T, service *baseRemoteTestService) (*BaseRemote, *CosmosProcess) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	RegisterAtomosRemoteServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		server.Stop()
		_ = listener.Close()
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	})

	process := newBaseRemoteTestProcess(t, "base_remote_async")
	cosmos := newCosmosRemoteFromNodeInfo(process, &CosmosNodeVersionInfo{Id: &IDInfo{
		Type: IDType_Cosmos, Cosmos: "base_remote_async", Node: "target",
	}})
	cosmos.enable = true
	cosmos.current = &cosmosRemoteVersion{process: process, client: conn, avail: true}
	remote := newBaseRemoteForTest(t, cosmos, &IDInfo{
		Type: IDType_Element, Cosmos: "base_remote_async", Node: "target", Element: "element",
	})
	return remote, process
}

func TestBaseRemotePushAsyncMessageTerminatesCallbackOnFailure(t *testing.T) {
	tests := []struct {
		name        string
		service     *baseRemoteTestService
		wantCode    int64
		callbackPan bool
	}{
		{
			name: "body error with panicking callback",
			service: &baseRemoteTestService{rsp: &CosmosRemoteAsyncMessagingByNameRsp{
				Error: NewError(ErrCodeCosmosRemoteResponseFailed, "body failure"),
			}},
			wantCode:    ErrCodeCosmosRemoteResponseFailed,
			callbackPan: true,
		},
		{
			name: "status error with Atomos details",
			service: &baseRemoteTestService{
				err: grpcStatusFromError(NewError(ErrCodeAtomInstanceMismatch, "stale instance").AddStack(nil)),
			},
			wantCode: ErrCodeAtomInstanceMismatch,
		},
		{
			name: "transport error",
			service: &baseRemoteTestService{
				err: status.Error(codes.Unavailable, "transport failure"),
			},
			wantCode: ErrCodeCosmosRemoteStatusUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote, process := newBaseRemoteTestRemote(t, tt.service)
			callbackCalls := 0
			var callbackErr error
			returnedErr := remote.PushAsyncMessage(process.local, "message", nil, func(out proto.Message, err error) {
				callbackCalls++
				callbackErr = err
				if out != nil {
					t.Errorf("terminal callback reply = %v, want nil", out)
				}
				if tt.callbackPan {
					panic("callback panic")
				}
			})
			if CodeOf(returnedErr) != tt.wantCode {
				t.Fatalf("PushAsyncMessage error = %v, want code %d", returnedErr, tt.wantCode)
			}
			if callbackCalls != 1 {
				t.Fatalf("terminal callback calls = %d, want 1", callbackCalls)
			}
			if callbackErr != returnedErr {
				t.Fatalf("callback error = %v, returned error = %v, want the same error", callbackErr, returnedErr)
			}
			if pending := process.local.atomos.AsyncCallbackPending(); pending != 0 {
				t.Fatalf("pending callbacks = %d, want 0", pending)
			}
		})
	}
}

func TestBaseRemoteStateConnectionError(t *testing.T) {
	process := newBaseRemoteTestProcess(t, "base_remote_state")
	cosmos := newCosmosRemoteFromNodeInfo(process, &CosmosNodeVersionInfo{Id: &IDInfo{
		Type: IDType_Cosmos, Cosmos: "base_remote_state", Node: "target",
	}})
	remote := newBaseRemoteForTest(t, cosmos, &IDInfo{
		Type: IDType_Element, Cosmos: "base_remote_state", Node: "target", Element: "element",
	})
	if state := remote.State(); state != BaseAtomosInvalidState {
		t.Fatalf("State = %v, want Invalid", state)
	}
	state, err := remote.stateWithError()
	if state != BaseAtomosInvalidState || CodeOf(err) != ErrCodeCosmosRemoteConnectFailed {
		t.Fatalf("stateWithError = (%v, %v), want (Invalid, ConnectFailed)", state, err)
	}
}

func TestBaseRemoteStateAndIdleTimeErrors(t *testing.T) {
	tests := []struct {
		name     string
		bodyErr  *Error
		rpcErr   error
		wantCode int64
	}{
		{
			name:     "body error",
			bodyErr:  NewError(ErrCodeAtomIsStopping, "atom is stopping").AddStack(nil),
			wantCode: ErrCodeAtomIsStopping,
		},
		{
			name:     "status error with Atomos details",
			rpcErr:   grpcStatusFromError(NewError(ErrCodeAtomInstanceMismatch, "stale instance").AddStack(nil)),
			wantCode: ErrCodeAtomInstanceMismatch,
		},
		{
			name:     "bare transport status",
			rpcErr:   status.Error(codes.Unavailable, "connection unavailable"),
			wantCode: ErrCodeCosmosRemoteStatusUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &baseRemoteTestService{
				stateRsp:    &CosmosRemoteGetIDStateRsp{State: int32(BaseAtomosWaiting), Error: tt.bodyErr},
				stateErr:    tt.rpcErr,
				idleTimeRsp: &CosmosRemoteGetIDIdleTimeRsp{IdleTime: int64(time.Second), Error: tt.bodyErr},
				idleTimeErr: tt.rpcErr,
			}
			remote, _ := newBaseRemoteTestRemote(t, service)
			state, err := remote.stateWithError()
			if state != BaseAtomosInvalidState || CodeOf(err) != tt.wantCode {
				t.Fatalf("stateWithError = (%v, %v), want (Invalid, code %d)", state, err, tt.wantCode)
			}
			if idleTime := remote.IdleTime(); idleTime != 0 {
				t.Fatalf("IdleTime = %v, want 0 on failure", idleTime)
			}
		})
	}
}

func TestBaseRemotePushSyncMessageErrors(t *testing.T) {
	malformedReply := toAnyPb(t, &ForTestGreetingO{})
	malformedReply.TypeUrl = "type.googleapis.com/atomos.DoesNotExist"
	tests := []struct {
		name      string
		rsp       *CosmosRemoteSyncMessagingByNameRsp
		rpcErr    error
		wantCode  int64
		wantReply bool
	}{
		{
			name: "body error preserves partial reply",
			rsp: &CosmosRemoteSyncMessagingByNameRsp{
				Reply: toAnyPb(t, &ForTestGreetingO{}),
				Error: NewError(ErrCodeAtomIsStopping, "atom is stopping").AddStack(nil),
			},
			wantCode:  ErrCodeAtomIsStopping,
			wantReply: true,
		},
		{
			name:     "status error with Atomos details",
			rpcErr:   grpcStatusFromError(NewError(ErrCodeAtomInstanceMismatch, "stale instance").AddStack(nil)),
			wantCode: ErrCodeAtomInstanceMismatch,
		},
		{
			name:     "bare transport status",
			rpcErr:   status.Error(codes.Unavailable, "connection unavailable"),
			wantCode: ErrCodeCosmosRemoteStatusUnavailable,
		},
		{
			name:     "malformed reply",
			rsp:      &CosmosRemoteSyncMessagingByNameRsp{Reply: malformedReply},
			wantCode: ErrCodeCosmosRemoteResponseInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote, process := newBaseRemoteTestRemote(t, &baseRemoteTestService{syncRsp: tt.rsp, syncErr: tt.rpcErr})
			out, err := remote.PushSyncMessage(process.local, "Greeting", &ForTestGreetingI{Mode: 1})
			if CodeOf(err) != tt.wantCode {
				t.Fatalf("PushSyncMessage error = %v, want code %d", err, tt.wantCode)
			}
			if len(asError(err).CallStacks) == 0 {
				t.Fatal("PushSyncMessage error lost its call stack")
			}
			if (out != nil) != tt.wantReply {
				t.Fatalf("PushSyncMessage reply = %v, wantReply=%v", out, tt.wantReply)
			}
		})
	}
}

func TestBaseRemoteKillErrors(t *testing.T) {
	tests := []struct {
		name     string
		rsp      *CosmosRemoteKillAtomRsp
		rpcErr   error
		wantCode int64
	}{
		{
			name:     "body error",
			rsp:      &CosmosRemoteKillAtomRsp{Error: NewError(ErrCodeAtomIsStopping, "atom is stopping").AddStack(nil)},
			wantCode: ErrCodeAtomIsStopping,
		},
		{
			name:     "status error with Atomos details",
			rpcErr:   grpcStatusFromError(NewError(ErrCodeAtomInstanceMismatch, "stale instance").AddStack(nil)),
			wantCode: ErrCodeAtomInstanceMismatch,
		},
		{
			name:     "bare transport status",
			rpcErr:   status.Error(codes.Unavailable, "connection unavailable"),
			wantCode: ErrCodeCosmosRemoteStatusUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote, process := newBaseRemoteTestRemote(t, &baseRemoteTestService{killRsp: tt.rsp, killErr: tt.rpcErr})
			err := remote.Kill(process.local)
			if CodeOf(err) != tt.wantCode {
				t.Fatalf("Kill error = %v, want code %d", err, tt.wantCode)
			}
			if len(asError(err).CallStacks) == 0 {
				t.Fatal("Kill error lost its call stack")
			}
		})
	}
}

func TestBaseRemoteRetryableAsyncMessage(t *testing.T) {
	service := &baseRemoteTestService{rsp: &CosmosRemoteAsyncMessagingByNameRsp{}, failures: 1}
	remote, process := newBaseRemoteTestRemote(t, service)
	process.retry.maxAttempts = 3
	process.retry.backoff = func(int) time.Duration { return 0 }
	callback := make(chan error, 1)
	if err := remote.PushAsyncMessage(process.local, "retryable", nil, func(_ proto.Message, err error) {
		callback <- err
	}, ArgBaseAtomosRetryable("message-1")); err != nil {
		t.Fatalf("submit retryable message: %v", err)
	}
	requests := waitBaseRemoteAsyncRequests(t, service, 2)
	if requests[0].MessageId != "message-1" || requests[1].MessageId != "message-1" {
		t.Fatalf("message IDs changed across attempts: %q, %q", requests[0].MessageId, requests[1].MessageId)
	}
	if requests[0].AsyncId == 0 || requests[0].AsyncId != requests[1].AsyncId || requests[0].StartupId != requests[1].StartupId {
		t.Fatalf("callback identity changed across attempts: %v, %v", requests[0], requests[1])
	}
	select {
	case err := <-callback:
		t.Fatalf("callback terminated before the business reply: %v", err)
	default:
	}
	if pending := process.local.atomos.AsyncCallbackPending(); pending != 1 {
		t.Fatalf("pending callbacks = %d, want 1", pending)
	}
	if err := process.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-callback:
		if CodeOf(err) != ErrCodeAsyncCallbackCancelledByHalt {
			t.Fatalf("callback error = %v, want cancellation by Halt", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("callback was not terminated by process Stop")
	}
}

func TestBaseRemoteRetryableAsyncMessageExhaustion(t *testing.T) {
	service := &baseRemoteTestService{rsp: &CosmosRemoteAsyncMessagingByNameRsp{}, failures: 2}
	remote, process := newBaseRemoteTestRemote(t, service)
	process.retry.maxAttempts = 2
	process.retry.backoff = func(int) time.Duration { return 0 }
	callback := make(chan error, 1)
	if err := remote.PushAsyncMessage(process.local, "retryable", nil, func(_ proto.Message, err error) {
		callback <- err
	}, ArgBaseAtomosRetryable("message-2")); err != nil {
		t.Fatalf("submit retryable message: %v", err)
	}
	select {
	case err := <-callback:
		if CodeOf(err) != ErrCodeCosmosRemoteStatusUnavailable {
			t.Fatalf("terminal error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("retry exhaustion did not terminate callback")
	}
	if got := waitBaseRemoteAsyncRequests(t, service, 2); len(got) != 2 {
		t.Fatalf("attempts = %d, want 2", len(got))
	}
	if pending := process.local.atomos.AsyncCallbackPending(); pending != 0 {
		t.Fatalf("pending callbacks = %d, want 0", pending)
	}
}

func TestBaseRemoteAsyncMessageNotRetriedByDefault(t *testing.T) {
	service := &baseRemoteTestService{rsp: &CosmosRemoteAsyncMessagingByNameRsp{}, failures: 1}
	remote, process := newBaseRemoteTestRemote(t, service)
	callback := make(chan error, 1)
	if err := remote.PushAsyncMessage(process.local, "ordinary", nil, func(_ proto.Message, err error) {
		callback <- err
	}); CodeOf(err) != ErrCodeCosmosRemoteStatusUnavailable {
		t.Fatalf("ordinary async error = %v", err)
	}
	select {
	case err := <-callback:
		if CodeOf(err) != ErrCodeCosmosRemoteStatusUnavailable {
			t.Fatalf("callback error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ordinary async callback was not terminated")
	}
	if got := waitBaseRemoteAsyncRequests(t, service, 1); len(got) != 1 || got[0].MessageId != "" {
		t.Fatalf("ordinary requests = %v", got)
	}
}

func waitBaseRemoteAsyncRequests(t *testing.T, service *baseRemoteTestService, want int) []*CosmosRemoteAsyncMessagingByNameReq {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		service.mutex.Lock()
		requests := append([]*CosmosRemoteAsyncMessagingByNameReq(nil), service.requests...)
		service.mutex.Unlock()
		if len(requests) >= want {
			return requests
		}
		if time.Now().After(deadline) {
			t.Fatalf("async requests = %d, want %d", len(requests), want)
		}
		time.Sleep(time.Millisecond)
	}
}
