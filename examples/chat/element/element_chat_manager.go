package element

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/chat/api"
	"google.golang.org/protobuf/proto"
)

const (
	ChatManagerElementName = "ChatManager"
)

// Element

type ChatManagerElement struct {
}

func (c *ChatManagerElement) Check() error {
	return nil
}

func (c *ChatManagerElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return ChatManagerElementName, 1, atomos.LogLevel_Debug, 1
}

func (c *ChatManagerElement) AtomConstructor() atomos.Atom {
	return &chatRoomManagerAtom{}
}

func (c *ChatManagerElement) AtomSaver(id atomos.Id, stateful atomos.AtomStateful) error {
	return nil
}

func (c *ChatManagerElement) AtomCanKill(id atomos.Id) bool {
	return true
}

// Atom

type chatRoomManagerAtom struct {
	self atomos.AtomSelf
	*api.ChatRoomManager
}

func (c *chatRoomManagerAtom) Spawn(self atomos.AtomSelf, arg proto.Message) error {
	self.Log().Info("Spawn")
	c.self = self
	c.ChatRoomManager = arg.(*api.ChatRoomManager)
	return nil
}

func (c *chatRoomManagerAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) {
	c.self.Log().Info("Halt")
}

func (c *chatRoomManagerAtom) CreateRoom(from atomos.Id, in *api.CreateRoomReq) (*api.CreateRoomResp, error) {
	c.self.Log().Info("CreateRoom, in=%+v", in)
	c.ChatRoomManager.Created[in]
}

func (c *chatRoomManagerAtom) FindRoom(from atomos.Id, in *api.FindRoomReq) (*api.FindRoomResp, error) {
	c.self.Log().Info("FindRoom, in=%+v", in)
}

func (c *chatRoomManagerAtom) DelRoom(from atomos.Id, in *api.DelRoomReq) (*api.DelRoomResp, error) {
	c.self.Log().Info("DelRoom, in=%+v", in)
}
