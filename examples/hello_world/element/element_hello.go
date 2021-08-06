package element

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"google.golang.org/protobuf/proto"
)

type GreeterElement struct {
}

func (h *GreeterElement) Check() error {
	return nil
}

func (h *GreeterElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return "Greeter", 1, atomos.LogLevel_Debug, 100
}

func (h *GreeterElement) AtomConstructor() atomos.Atom {
	return &helloAtom{}
}

func (h *GreeterElement) AtomSaver(id atomos.Id, stateful atomos.AtomStateful) error {
	panic("implement me")
}

func (h *GreeterElement) AtomCanKill(id atomos.Id) bool {
	// todo
	return true
}

type helloAtom struct {
	self atomos.AtomSelf
	arg proto.Message
}

func (h *helloAtom) Spawn(self atomos.AtomSelf, arg proto.Message) error {
	self.Log().Info("Spawn")
	h.self = self
	h.arg = arg
	return nil
}

func (h *helloAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) {
	h.self.Log().Info("AtomHalt")
}

func (h *helloAtom) SayHello(from atomos.Id, in *api.HelloRequest) (*api.HelloReply, error) {
	h.self.Log().Info("SayHello")
	return &api.HelloReply{ Message: "Ok" }, nil
}
