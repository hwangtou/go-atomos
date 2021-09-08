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

func (c *ChatManagerElement) Load(mainId atomos.MainId) error {
	return nil
}

func (c *ChatManagerElement) Unload() {
}

func (c *ChatManagerElement) Persistence() atomos.ElementPersistence {
	return nil
}

func (c *ChatManagerElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return ChatManagerElementName, 1, atomos.LogLevel_Debug, 1
}

func (c *ChatManagerElement) AtomConstructor() atomos.Atom {
	return &chatRoomManagerAtom{}
}

func (c *ChatManagerElement) AtomCanKill(id atomos.Id) bool {
	return true
}

// Atom

type chatRoomManagerAtom struct {
	self atomos.AtomSelf
	data *api.ChatRoomManager
}

func (c *chatRoomManagerAtom) Spawn(self atomos.AtomSelf, arg *api.ChatRoomManagerSpawnArg, data *api.ChatRoomManager) error {
	self.Log().Info("Spawn")
	c.self = self
	c.data = data
	return nil
}

func (c *chatRoomManagerAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	c.self.Log().Info("Halt")
	return c.data
}

func (c *chatRoomManagerAtom) CreateRoom(from atomos.Id, in *api.CreateRoomReq) (*api.CreateRoomResp, error) {
	c.self.Log().Info("CreateRoom, in=%+v", in)
	return nil, nil
}

func (c *chatRoomManagerAtom) FindRoom(from atomos.Id, in *api.FindRoomReq) (*api.FindRoomResp, error) {
	c.self.Log().Info("FindRoom, in=%+v", in)
	return nil, nil
}
