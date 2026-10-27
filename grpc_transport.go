package atomos

import (
	"net"

	"google.golang.org/grpc"
)

// GRPCTransport supplies the listener and client connections for one cluster
// process. Implementations must be safe for concurrent Dial calls. The process
// closes listeners and client connections it obtains from the transport.
type GRPCTransport interface {
	Listen(address string) (net.Listener, error)
	Dial(target string, options ...grpc.DialOption) (*grpc.ClientConn, error)
}
