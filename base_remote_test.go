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

// TestBaseRemote_LifeCycle

func TestBaseRemote_LifeCycle(t *testing.T) {

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

type baseRemoteAsyncTestService struct {
	UnimplementedAtomosRemoteServiceServer
	rsp *CosmosRemoteAsyncMessagingByNameRsp
	err error
}

func (s *baseRemoteAsyncTestService) AsyncMessagingByName(context.Context, *CosmosRemoteAsyncMessagingByNameReq) (*CosmosRemoteAsyncMessagingByNameRsp, error) {
	return s.rsp, s.err
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

func newBaseRemoteAsyncTestRemote(t *testing.T, service *baseRemoteAsyncTestService) (*BaseRemote, *CosmosProcess) {
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
		service     *baseRemoteAsyncTestService
		wantCode    int64
		callbackPan bool
	}{
		{
			name: "body error with panicking callback",
			service: &baseRemoteAsyncTestService{rsp: &CosmosRemoteAsyncMessagingByNameRsp{
				Error: NewError(ErrCodeCosmosRemoteResponseFailed, "body failure"),
			}},
			wantCode:    ErrCodeCosmosRemoteResponseFailed,
			callbackPan: true,
		},
		{
			name: "transport error",
			service: &baseRemoteAsyncTestService{
				err: status.Error(codes.Unavailable, "transport failure"),
			},
			wantCode: ErrCodeCosmosRemoteConnectFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote, process := newBaseRemoteAsyncTestRemote(t, tt.service)
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
