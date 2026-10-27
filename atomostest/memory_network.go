package atomostest

import (
	"context"
	"errors"
	"net"
	"sync"

	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

// MemoryNetwork connects real CosmosProcess gRPC services through bufconn.
// Each node uses a separate Transport, while Fault returns the controller for
// one directed source/destination pair.
type MemoryNetwork struct {
	mu          sync.Mutex
	clock       *VirtualClock
	nodes       map[string]*memoryNetworkNode
	controllers map[string]*FaultController
	closed      bool
}

type memoryNetworkNode struct {
	listener *bufconn.Listener
}

// NewMemoryNetwork creates an empty in-memory cluster transport.
func NewMemoryNetwork(clock *VirtualClock) *MemoryNetwork {
	return &MemoryNetwork{clock: clock, nodes: map[string]*memoryNetworkNode{}, controllers: map[string]*FaultController{}}
}

// Transport returns the transport bound to one node name.
func (n *MemoryNetwork) Transport(node string) atomos.GRPCTransport {
	return &memoryNetworkTransport{network: n, node: node}
}

// Fault returns the directed fault controller used by connections from source
// to destination. Its rules affect subsequent RPCs on existing connections.
func (n *MemoryNetwork) Fault(source, destination string) *FaultController {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.faultLocked(source, destination)
}

func (n *MemoryNetwork) faultLocked(source, destination string) *FaultController {
	key := source + "\x00" + destination
	controller := n.controllers[key]
	if controller == nil {
		controller = NewFaultController(source, destination, n.clock)
		n.controllers[key] = controller
	}
	return controller
}

// Close releases held RPCs and closes all listeners. Processes should normally
// be stopped first so their gRPC services and connections shut down cleanly.
func (n *MemoryNetwork) Close() error {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return nil
	}
	n.closed = true
	nodes := make(map[string]*memoryNetworkNode, len(n.nodes))
	for name, node := range n.nodes {
		nodes[name] = node
	}
	controllers := make([]*FaultController, 0, len(n.controllers))
	for _, controller := range n.controllers {
		controllers = append(controllers, controller)
	}
	n.mu.Unlock()
	for _, controller := range controllers {
		controller.Heal()
	}
	var result error
	for _, node := range nodes {
		if err := node.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			result = errors.Join(result, err)
		}
	}
	return result
}

type memoryNetworkTransport struct {
	network *MemoryNetwork
	node    string
}

func (t *memoryNetworkTransport) Listen(string) (net.Listener, error) {
	n := t.network
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil, net.ErrClosed
	}
	if n.nodes[t.node] != nil {
		return nil, errors.New("atomostest: memory gRPC node already listening")
	}
	listener := bufconn.Listen(1024 * 1024)
	n.nodes[t.node] = &memoryNetworkNode{listener: listener}
	return &memoryNetworkListener{Listener: listener, network: n, node: t.node}, nil
}

func (t *memoryNetworkTransport) Dial(target string, options ...grpc.DialOption) (*grpc.ClientConn, error) {
	n := t.network
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return nil, net.ErrClosed
	}
	node := n.nodes[target]
	if node == nil {
		n.mu.Unlock()
		return nil, errors.New("atomostest: memory gRPC target not found")
	}
	listener := node.listener
	controller := n.faultLocked(t.node, target)
	n.mu.Unlock()

	options = append(options,
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithChainUnaryInterceptor(controller.UnaryClientInterceptor()),
	)
	return grpc.NewClient("passthrough:///"+target, options...)
}

type memoryNetworkAddr string

func (a memoryNetworkAddr) Network() string { return "memory" }
func (a memoryNetworkAddr) String() string  { return string(a) }

type memoryNetworkListener struct {
	*bufconn.Listener
	network *MemoryNetwork
	node    string
	once    sync.Once
}

func (l *memoryNetworkListener) Addr() net.Addr {
	return memoryNetworkAddr(l.node)
}

func (l *memoryNetworkListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() {
		l.network.mu.Lock()
		if current := l.network.nodes[l.node]; current != nil && current.listener == l.Listener {
			delete(l.network.nodes, l.node)
		}
		l.network.mu.Unlock()
	})
	return err
}
