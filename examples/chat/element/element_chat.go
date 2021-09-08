package element

import (
	"errors"
	"fmt"
	"github.com/hwangtou/go-atomos/examples/chat/api"
	"google.golang.org/protobuf/proto"

	atomos "github.com/hwangtou/go-atomos"
)

const (
	ChatElementName = "Chat"
	ChatElementAtomInit = 100
	ChatElementFmt = "Chat:%s"
)

// Element

type ChatRoomElement struct {
	mainId atomos.Id
}

func (c *ChatRoomElement) Load(mainId atomos.MainId) error {
	return nil
}

func (c *ChatRoomElement) Unload() {
}

func (c *ChatRoomElement) Persistence() atomos.ElementPersistence {
	return nil
}

func (c *ChatRoomElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return ChatElementName, 1, atomos.LogLevel_Debug, ChatElementAtomInit
}

func (c *ChatRoomElement) AtomConstructor() atomos.Atom {
	return &chatRoomAtom{}
}

func (c *ChatRoomElement) AtomCanKill(id atomos.Id) bool {
	switch id.Name() {
	case ChatManagerElementName, atomos.MainAtomName:
		return true
	default:
		return false
	}
}

func (c *ChatRoomElement) AtomDataLoader(name string) (proto.Message, error) {
	dbId, err := api.GetKvDbAtomId(c.mainId.Cosmos(), KvDbElementName)
	if err != nil {
		return nil, err
	}
	chatDataKey := fmt.Sprintf(ChatElementFmt, name)
	get, err := dbId.Get(c.mainId, &api.DbGetReq{Key: chatDataKey})
	if err != nil {
		return nil, err
	}
	if !get.Has {
		return nil, nil
	}

	var data api.ChatRoom
	if err = proto.Unmarshal(get.Value, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (c *ChatRoomElement) AtomDataSaver(name string, data proto.Message) error {
	dbId, err := api.GetKvDbAtomId(c.mainId.Cosmos(), KvDbElementName)
	if err != nil {
		return err
	}
	chatDataKey := fmt.Sprintf(ChatElementFmt, name)
	chatData, err := proto.Marshal(data)
	if err != nil {
		return err
	}
	_, err = dbId.Set(c.mainId, &api.DbSetReq{Key: chatDataKey, Value: chatData})
	if err != nil {
		return err
	}
	return nil
}

// Atom

type chatRoomAtom struct {
	self atomos.AtomSelf
	data *api.ChatRoom
}

func (c *chatRoomAtom) Spawn(self atomos.AtomSelf, arg *api.ChatRoomSpawnArg, data *api.ChatRoom) error {
	c.self = self
	// Is it a new ChatRoom?
	if data == nil {
		if arg == nil {
			return fmt.Errorf("spawn room without arg and data, name=%+v", self.Name())
		}
		data = &api.ChatRoom{
			Info:    &api.ChatRoomBrief{
				Name:      arg.Name,
				UpdatedAt: 0,
			},
			Owner:   arg.Owner,
			Members: map[int64]*api.UserBrief{},
		}
	}
	c.data = data
	return nil
}

func (c *chatRoomAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	return c.data
}

func (c *chatRoomAtom) Info(from atomos.Id, in *api.ChatRoomInfoReq) (*api.ChatRoomInfoResp, error) {
	return &api.ChatRoomInfoResp{ Room: c.data }, nil
}

func (c *chatRoomAtom) AddMember(from atomos.Id, in *api.AddMemberReq) (*api.AddMemberResp, error) {
	if in.User.Id == c.data.Owner.Id {
		return nil, errors.New("need not add owner")
	}
	if _, has := c.data.Members[in.User.Id]; has {
		return nil, errors.New("member exists")
	}
	return &api.AddMemberResp{ Succeed: true }, nil
}

func (c *chatRoomAtom) DelMember(from atomos.Id, in *api.DelMemberReq) (*api.DelMemberResp, error) {
	if in.UserId == c.data.Owner.Id {
		return nil, errors.New("cannot delete owner")
	}
	if _, has := c.data.Members[in.UserId]; !has {
		return nil, errors.New("member not exists")
	}
	delete(c.data.Members, in.UserId)
	return &api.DelMemberResp{ Succeed: true }, nil
}

func (c *chatRoomAtom) SendMessage(from atomos.Id, in *api.SendMessageReq) (*api.SendMessageResp, error) {
	if _, has := c.data.Members[in.SenderId]; !has && c.data.Owner.Id != in.SenderId {
		return &api.SendMessageResp{ Succeed: false }, errors.New("sender not in room")
	}
	_, err := c.self.Task().Append(c.sendMessage, in)
	if err != nil {
		return &api.SendMessageResp{ Succeed: false }, err
	}
	return &api.SendMessageResp{ Succeed: true }, nil
}

func (c *chatRoomAtom) sendMessage(int64, string) {
}
