package element

import (
	"errors"
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/chat/api"
	"google.golang.org/protobuf/proto"
	"time"
)

// Element

type ChatManagerElement struct {
	mainId  atomos.MainId
	persist *chatManagerPersistence
}

func (c *ChatManagerElement) Load(mainId atomos.MainId) error {
	c.mainId = mainId
	return nil
}

func (c *ChatManagerElement) Unload() {
}

func (c *ChatManagerElement) Persistence() atomos.ElementPersistence {
	if c.persist == nil {
		c.persist = &chatManagerPersistence{
			mainId: c.mainId,
		}
	}
	return c.persist
}

func (c *ChatManagerElement) Info() (version uint64, logLevel atomos.LogLevel, initNum int) {
	return 1, atomos.LogLevel_Debug, 1
}

func (c *ChatManagerElement) AtomConstructor() atomos.Atom {
	return &chatRoomManagerAtom{}
}

func (c *ChatManagerElement) AtomCanKill(id atomos.Id) bool {
	return true
}

// Element Persistence

type chatManagerPersistence struct {
	mainId atomos.MainId
	dbId   api.KvDbAtomId
}

func (c *chatManagerPersistence) GetAtomData(name string) (data proto.Message, err error) {
	if c.dbId == nil {
		if c.dbId, err = api.GetKvDbAtomId(c.mainId.Cosmos(), "DB"); err != nil {
			return nil, err
		}
	}
	name = fmt.Sprintf("ChatManager:%s", name)
	return c.dbId.Get(c.mainId, &api.DbGetReq{ Key: name })
}

func (c *chatManagerPersistence) SetAtomData(name string, data proto.Message) (err error) {
	if c.dbId == nil {
		if c.dbId, err = api.GetKvDbAtomId(c.mainId.Cosmos(), "DB"); err != nil {
			return err
		}
	}
	buf, err := proto.Marshal(data)
	if err != nil {
		return err
	}
	name = fmt.Sprintf("ChatManager:%s", name)
	_, err = c.dbId.Set(c.mainId, &api.DbSetReq{ Key: name, Value: buf })
	if err != nil {
		return err
	}
	return nil
}

// Atom

type chatRoomManagerAtom struct {
	self atomos.AtomSelf
	data *api.ChatRoomManagerData
}

func (c *chatRoomManagerAtom) Spawn(self atomos.AtomSelf, arg *api.ChatRoomManagerSpawnArg, data *api.ChatRoomManagerData) error {
	self.Log().Info("Spawn")
	c.self = self
	c.data = data
	return nil
}

func (c *chatRoomManagerAtom) GetRooms(from atomos.Id, in *api.Nil) (*api.GetRoomsResp, error) {
	rooms := map[int64]string{}
	for _, room := range c.data.Rooms {
		rooms[room.RoomId] = room.Name
	}
	return &api.GetRoomsResp{ Rooms: rooms }, nil
}

func (c *chatRoomManagerAtom) DelRoom(from atomos.Id, in *api.DelRoomReq) (*api.DelRoomResp, error) {
	if _, has := c.data.Rooms[in.RoomId]; !has {
		return nil, errors.New("room not exists")
	}
	// Check user.
	if from.Element().GetName() != api.UserName {
		return nil, errors.New("only user can delete room")
	}
	// Get room.
	roomId, err := api.GetChatRoomId(c.self.Cosmos(), fmt.Sprintf("%d", in.RoomId))
	if err != nil {
		return nil, err
	}
	if _, err = roomId.DelSelf(c.self, &api.Nil{}); err != nil {
		return nil, err
	}
	delete(c.data.Rooms, in.RoomId)
	return &api.DelRoomResp{ Succeed: true }, nil
}

func (c *chatRoomManagerAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	c.self.Log().Info("Halt")
	return c.data
}

func (c *chatRoomManagerAtom) CreateRoom(from atomos.Id, in *api.CreateRoomReq) (*api.CreateRoomResp, error) {
	c.self.Log().Info("CreateRoom, in=%+v", in)
	// Check user.
	if from.Element().GetName() != api.UserName {
		return nil, errors.New("only user can delete room")
	}
	c.data.CurId += 1
	curRoomId := c.data.CurId
	roomAtomName := fmt.Sprintf("%d", curRoomId)
	_, err := api.SpawnChatRoom(c.self.Cosmos(), roomAtomName, &api.ChatRoomSpawnArg{
		RoomId: curRoomId,
		Name:   in.Name,
		Owner:  in.Owner,
	})
	if err != nil {
		return nil, err
	}
	c.data.Rooms[curRoomId] = &api.RoomRegistration{
		RoomId:  c.data.CurId,
		Name:    roomAtomName,
		Updated: time.Now().Unix(),
	}
	return nil, nil
}

func (c *chatRoomManagerAtom) FindRoom(from atomos.Id, in *api.FindRoomReq) (*api.FindRoomResp, error) {
	c.self.Log().Info("FindRoom, in=%+v", in)
	if _, has := c.data.Rooms[in.RoomId]; !has {
		return nil, errors.New("room not exists")
	}
	// Get room.
	roomId, err := api.GetChatRoomId(c.self.Cosmos(), fmt.Sprintf("%d", in.RoomId))
	if err != nil {
		return nil, err
	}
	roomInfoResp, err := roomId.Info(c.self, &api.Nil{})
	if err != nil {
		return nil, err
	}
	return &api.FindRoomResp{ Room: roomInfoResp.Room.Info }, nil
}
