package atomos

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type elementRemoteSpawnTestServer struct {
	UnimplementedAtomosRemoteServiceServer
}

func (elementRemoteSpawnTestServer) SpawnAtom(_ context.Context, req *CosmosRemoteSpawnAtomReq) (*CosmosRemoteSpawnAtomRsp, error) {
	rsp := &CosmosRemoteSpawnAtomRsp{
		Error: NewError(ErrCodeAtomAlreadyExists, "spawn rejected"),
	}
	if req.Atom == "with-id" {
		rsp.Id = &IDInfo{Type: IDType_Atom, Element: req.Element, Atom: req.Atom}
	}
	return rsp, nil
}

func TestElementRemoteSpawnAtomReturnsResponseErrorWithOrWithoutID(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	RegisterAtomosRemoteServiceServer(server, elementRemoteSpawnTestServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	cosmos := &CosmosRemote{}
	info := &IDInfo{Type: IDType_Element, Element: ForTestAtomosName}
	element := &ElementRemote{
		cosmos: cosmos,
		remote: newBaseRemoteWithPinnedConn(cosmos, info, conn),
	}
	for _, name := range []string{"without-id", "with-id"} {
		t.Run(name, func(t *testing.T) {
			id, tracker, err := element.SpawnAtom(stubID{}, name, nil, nil)
			if got := CodeOf(err); got != ErrCodeAtomAlreadyExists {
				t.Fatalf("code = %v, want ErrCodeAtomAlreadyExists", got)
			}
			if id != nil || tracker != nil {
				t.Fatalf("id = %v, tracker = %v; want both nil on response error", id, tracker)
			}
		})
	}
}
