package atomostest

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hwangtou/go-atomos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const faultLookupMethod = "/atomos.AtomosRemoteService/LookupAtom"

type faultLookupService struct {
	atomos.UnimplementedAtomosRemoteServiceServer
	mutex sync.Mutex
	calls []string
	seen  chan string
}

func (s *faultLookupService) LookupAtom(_ context.Context, req *atomos.CosmosRemoteLookupAtomReq) (*atomos.CosmosRemoteLookupAtomRsp, error) {
	s.mutex.Lock()
	s.calls = append(s.calls, req.Id.Atom)
	s.mutex.Unlock()
	if s.seen != nil {
		s.seen <- req.Id.Atom
	}
	return &atomos.CosmosRemoteLookupAtomRsp{State: atomos.AtomLifecycleState_AtomLifecycleStateAbsent}, nil
}

func (s *faultLookupService) count() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.calls)
}

func newFaultClient(t *testing.T, controller *FaultController, service *faultLookupService) atomos.AtomosRemoteServiceClient {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	atomos.RegisterAtomosRemoteServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(controller.UnaryClientInterceptor()),
		grpc.WithBlock(),
	)
	if err != nil {
		server.Stop()
		_ = listener.Close()
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	})
	return atomos.NewAtomosRemoteServiceClient(conn)
}

func faultLookupRequest(name string) *atomos.CosmosRemoteLookupAtomReq {
	return &atomos.CosmosRemoteLookupAtomReq{Id: &atomos.IDInfo{Type: atomos.IDType_Atom, Atom: name}}
}

func TestFaultControllerRequestResponseAndDuplicate(t *testing.T) {
	for _, tc := range []struct {
		name      string
		action    FaultAction
		wantCalls int
		wantError bool
	}{
		{name: "drop_request", action: FaultDropRequest, wantCalls: 0, wantError: true},
		{name: "drop_response", action: FaultDropResponse, wantCalls: 1, wantError: true},
		{name: "duplicate", action: FaultDuplicateRequest, wantCalls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controller := NewFaultController("a", "b", nil)
			service := &faultLookupService{}
			client := newFaultClient(t, controller, service)
			controller.AddRule(FaultRule{Method: faultLookupMethod, Action: tc.action, Once: true})
			_, err := client.LookupAtom(context.Background(), faultLookupRequest("atom"))
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError=%v", err, tc.wantError)
			}
			if tc.wantError && status.Code(err) != codes.Unavailable {
				t.Fatalf("error code = %v", status.Code(err))
			}
			if got := service.count(); got != tc.wantCalls {
				t.Fatalf("service calls = %d, want %d", got, tc.wantCalls)
			}
			if events := controller.Events(); len(events) != 1 || events[0].Action != tc.action {
				t.Fatalf("events = %v", events)
			}
		})
	}
}

func TestFaultControllerVirtualDelay(t *testing.T) {
	clock := NewVirtualClock()
	controller := NewFaultController("a", "b", clock)
	service := &faultLookupService{}
	client := newFaultClient(t, controller, service)
	controller.AddRule(FaultRule{Action: FaultDelay, Duration: time.Second, Once: true})
	done := make(chan error, 1)
	go func() {
		_, err := client.LookupAtom(context.Background(), faultLookupRequest("atom"))
		done <- err
	}()
	waitFaultEvents(t, controller, 1)
	select {
	case err := <-done:
		t.Fatalf("delay returned before virtual time advanced: %v", err)
	default:
	}
	clock.Advance(time.Second)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("delay did not finish")
	}
	if got := controller.Events()[0].Time; !got.Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("event time = %v, want injection time at virtual epoch", got)
	}
}

func TestFaultControllerHoldReorderAndHeal(t *testing.T) {
	controller := NewFaultController("a", "b", nil)
	service := &faultLookupService{seen: make(chan string, 2)}
	client := newFaultClient(t, controller, service)
	controller.AddRule(FaultRule{Action: FaultHold, Occurrence: 1})
	controller.AddRule(FaultRule{Action: FaultHold, Occurrence: 2})

	first := make(chan error, 1)
	go func() {
		_, err := client.LookupAtom(context.Background(), faultLookupRequest("first"))
		first <- err
	}()
	waitFaultEvents(t, controller, 1)
	second := make(chan error, 1)
	go func() {
		_, err := client.LookupAtom(context.Background(), faultLookupRequest("second"))
		second <- err
	}()
	events := waitFaultEvents(t, controller, 2)
	if !controller.Release(events[1].Seq) {
		t.Fatal("second hold was not released")
	}
	select {
	case got := <-service.seen:
		if got != "second" {
			t.Fatalf("first released call = %q, want second", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second call was not invoked")
	}
	if !controller.Release(events[0].Seq) {
		t.Fatal("first hold was not released")
	}
	for _, done := range []chan error{first, second} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("held call did not finish")
		}
	}

	controller.AddRule(FaultRule{Action: FaultHold, Once: true})
	held := make(chan error, 1)
	go func() {
		_, err := client.LookupAtom(context.Background(), faultLookupRequest("heal"))
		held <- err
	}()
	waitFaultEvents(t, controller, 3)
	controller.Heal()
	select {
	case err := <-held:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Heal did not release held call")
	}
}

func waitFaultEvents(t *testing.T, controller *FaultController, want int) []FaultEvent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for len(controller.Events()) < want {
		if time.Now().After(deadline) {
			t.Fatalf("fault events = %v, want %d", controller.Events(), want)
		}
		time.Sleep(time.Millisecond)
	}
	return controller.Events()
}
