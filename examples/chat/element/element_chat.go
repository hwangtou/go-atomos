package element

import (
	"errors"
	"fmt"
	"github.com/hwangtou/go-atomos/examples/chat/api"
	"google.golang.org/protobuf/proto"
	"time"

	atomos "github.com/hwangtou/go-atomos"
)

const (
	ChatElementAtomInit = 100
	ChatElementFmt = "Chat:%s"
)

// Element

type ChatRoomElement struct {
	mainId  atomos.MainId
	persist *chatPersistence
}

func (c *ChatRoomElement) Load(mainId atomos.MainId) error {
	c.mainId = mainId
	return nil
}

func (c *ChatRoomElement) Unload() {
}

func (c *ChatRoomElement) Persistence() atomos.ElementPersistence {
	if c.persist == nil {
		c.persist = &chatPersistence{
			mainId: c.mainId,
		}
	}
	return c.persist
}

func (c *ChatRoomElement) Info() (version uint64, logLevel atomos.LogLevel, initNum int) {
	return 1, atomos.LogLevel_Debug, ChatElementAtomInit
}

func (c *ChatRoomElement) AtomConstructor() atomos.Atom {
	return &chatRoomAtom{}
}

func (c *ChatRoomElement) AtomCanKill(id atomos.Id) bool {
	switch id.Name() {
	case api.ChatRoomManagerName, atomos.MainAtomName:
		return true
	default:
		return false
	}
}

// Element Persistence

type chatPersistence struct {
	mainId atomos.MainId
	dbId   api.KvDbAtomId
}

func (c *chatPersistence) GetAtomData(name string) (data proto.Message, err error) {
	if c.dbId == nil {
		if c.dbId, err = api.GetKvDbAtomId(c.mainId.Cosmos(), "DB"); err != nil {
			return nil, err
		}
	}
	name = fmt.Sprintf("Chat:%s", name)
	return c.dbId.Get(c.mainId, &api.DbGetReq{ Key: name })
}

func (c *chatPersistence) SetAtomData(name string, data proto.Message) (err error) {
	if c.dbId == nil {
		if c.dbId, err = api.GetKvDbAtomId(c.mainId.Cosmos(), "DB"); err != nil {
			return err
		}
	}
	buf, err := proto.Marshal(data)
	if err != nil {
		return err
	}
	name = fmt.Sprintf("Chat:%s", name)
	_, err = c.dbId.Set(c.mainId, &api.DbSetReq{ Key: name, Value: buf })
	if err != nil {
		return err
	}
	return nil
}

// Atom

type chatRoomAtom struct {
	self atomos.AtomSelf
	data *api.ChatRoomData
}

func (c *chatRoomAtom) Spawn(self atomos.AtomSelf, arg *api.ChatRoomSpawnArg, data *api.ChatRoomData) error {
	self.Log().Info("Spawn")
	c.self = self
	// Is it a new ChatRoom?
	if data == nil {
		// First initialize
		if arg == nil {
			return errors.New("no chat room initialize data")
		}
		c.data = &api.ChatRoomData{
			Info:    &api.ChatRoomBrief{
				RoomId:    arg.RoomId,
				Name:      arg.Name,
				Members:   map[int64]*api.UserBrief{},
				UpdatedAt: time.Now().Unix(),
			},
			Owner:   arg.Owner,
		}
	} else {
		c.data = data
	}
	return nil
}

func (c *chatRoomAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	return c.data
}

func (c *chatRoomAtom) Info(from atomos.Id, in *api.Nil) (*api.ChatRoomInfoResp, error) {
	return &api.ChatRoomInfoResp{ Room: c.data }, nil
}

func (c *chatRoomAtom) Update(from atomos.Id, in *api.ChatRoomUpdateReq) (*api.ChatRoomUpdateResp, error) {
	c.data.Info.Name = in.Name
	return &api.ChatRoomUpdateResp{ Succeed: true }, nil
}

func (c *chatRoomAtom) AddMember(from atomos.Id, in *api.ChatRoomAddMemberReq) (*api.ChatRoomAddMemberResp, error) {
	if from.Element().GetName() != api.UserName {
		return nil, errors.New("requester not user")
	}
	if from.Name() != fmt.Sprintf("%d", c.data.Owner.UserId) {
		return nil, errors.New("requester not chat room owner")
	}
	if in.Requester.UserId == c.data.Owner.UserId {
		return nil, errors.New("try adding owner as member")
	}
	if _, has := c.data.Info.Members[in.Requester.UserId]; has {
		return nil, errors.New("member exists")
	}
	c.data.Info.Members[in.Requester.UserId] = in.Requester
	return &api.ChatRoomAddMemberResp{
		Succeed: true,
		Room:    c.data.Info,
	}, nil
}

func (c *chatRoomAtom) DelMember(from atomos.Id, in *api.ChatRoomDelMemberReq) (*api.ChatRoomDelMemberResp, error) {
	if from.Element().GetName() != api.UserName {
		return nil, errors.New("requester not user")
	}
	if from.Name() != fmt.Sprintf("%d", c.data.Owner.UserId) {
		return nil, errors.New("requester not chat room owner")
	}
	if in.UserId == c.data.Owner.UserId {
		return nil, errors.New("try deleting owner as member")
	}
	if _, has := c.data.Info.Members[in.UserId]; !has {
		return nil, errors.New("member not exists")
	}
	delete(c.data.Info.Members, in.UserId)
	return &api.ChatRoomDelMemberResp{ Succeed: true }, nil
}

func (c *chatRoomAtom) SendMessage(from atomos.Id, in *api.ChatRoomSendMessageReq) (*api.ChatRoomSendMessageResp, error) {
	if _, has := c.data.Info.Members[in.SenderId]; in.SenderId != c.data.Owner.UserId && !has {
		return nil, errors.New("not room member")
	}
	if _, err := c.self.Task().Append(c.sendMessage, in); err != nil {
		return nil, err
	}
	return &api.ChatRoomSendMessageResp{ Succeed: true }, nil
}

func (c *chatRoomAtom) DelSelf(from atomos.Id, in *api.Nil) (*api.Nil, error) {
	c.data = nil
	c.self.KillSelf()
	return nil, nil
}

func (c *chatRoomAtom) sendMessage(taskId int64, msg *api.ChatRoomSendMessageReq) {
	push := &api.RoomMessagePush{
		RoomId:    c.data.Info.RoomId,
		SenderId:  msg.SenderId,
		Content:   msg.Content,
		CreatedAt: msg.CreatedAt,
	}
	for uid, _ := range c.data.Info.Members {
		userId, err := api.GetUserId(c.self.Cosmos(), fmt.Sprintf("%d", uid))
		if err != nil {
			c.self.Log().Error("User not found, uid=%d", uid)
			continue
		}
		if _, err = userId.RoomMessaging(c.self, push); err != nil {
			c.self.Log().Error("User cannot send, uid=%d,err=%v", uid, err)
		}
	}
}
