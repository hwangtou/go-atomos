package element

import (
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"time"

	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/chat/api"
)

// Element

type UserElement struct {
	mainId  atomos.MainId
	persist *userPersistence
}

func (u *UserElement) Load(mainId atomos.MainId) error {
	u.mainId = mainId
	return nil
}

func (u *UserElement) Unload() {
}

func (u *UserElement) Persistence() atomos.ElementPersistence {
	if u.persist == nil {
		u.persist = &userPersistence{
			mainId: u.mainId,
		}
	}
	return u.persist
}

func (u *UserElement) Info() (version uint64, logLevel atomos.LogLevel, initNum int) {
	return 1, atomos.LogLevel_Debug, 100
}

func (u *UserElement) AtomConstructor() atomos.Atom {
	return &userAtom{}
}

func (u *UserElement) AtomCanKill(id atomos.Id) bool {
	return id.Name() == api.UserManagerName
}

// Element Persistence

type userPersistence struct {
	mainId atomos.MainId
	dbId   api.KvDbAtomId
}

func (c *userPersistence) GetAtomData(name string) (data proto.Message, err error) {
	if c.dbId == nil {
		if c.dbId, err = api.GetKvDbAtomId(c.mainId.Cosmos(), "DB"); err != nil {
			return nil, err
		}
	}
	name = fmt.Sprintf("User:%s", name)
	return c.dbId.Get(c.mainId, &api.DbGetReq{ Key: name })
}

func (c *userPersistence) SetAtomData(name string, data proto.Message) (err error) {
	if c.dbId == nil {
		if c.dbId, err = api.GetKvDbAtomId(c.mainId.Cosmos(), "DB"); err != nil {
			return err
		}
	}
	buf, err := proto.Marshal(data)
	if err != nil {
		return err
	}
	name = fmt.Sprintf("User:%s", name)
	_, err = c.dbId.Set(c.mainId, &api.DbSetReq{ Key: name, Value: buf })
	if err != nil {
		return err
	}
	return nil
}

// Atom

type userAtom struct {
	self atomos.AtomSelf
	data *api.UserData
}

func (u *userAtom) Spawn(self atomos.AtomSelf, arg *api.UserSpawnArg, data *api.UserData) error {
	self.Log().Info("Spawn")
	u.self = self
	if data == nil {
		// First initialize
		if arg == nil {
			return errors.New("no user initialize data")
		}
		u.data = &api.UserData{
			Info:      &api.UserBrief{
				UserId:   arg.UserId,
				Nickname: arg.Nickname,
			},
			Friends:   map[int64]*api.UserBrief{},
			Rooms:     map[int64]*api.ChatRoomBrief{},
			UpdatedAt: time.Now().Unix(),
		}
	} else {
		// Login
		u.data = data
	}
	return nil
}

func (u *userAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	u.self.Log().Info("Halt")
	return u.data
}

// 用户

// 用户信息
func (u *userAtom) UserInfo(from atomos.Id, in *api.Nil) (resp *api.UserInfoResp, err error) {
	return &api.UserInfoResp{ User: u.data.Info }, nil
}

// 好友

// 所有好友
func (u *userAtom) GetFriends(from atomos.Id, in *api.Nil) (resp *api.GetUserFriendResp, err error) {
	return &api.GetUserFriendResp{ Friends: u.data.Friends }, nil
}

// （主动方）添加好友，客户端请求
func (u *userAtom) AddFriend(from atomos.Id, in *api.AddUserFriendReq) (resp *api.AddUserFriendResp, err error) {
	if _, has := u.data.Friends[in.UserId]; has {
		return nil, errors.New("has already added")
	}
	um, err := api.GetUserManagerId(u.self.Cosmos(), api.UserManagerName)
	if err != nil {
		return nil, err
	}
	tarResp, err := um.FindUser(u.self, &api.FindUserReq{ UserId: in.UserId })
	if err != nil {
		return nil, err
	}
	tarUserId, err := api.GetUserId(u.self.Cosmos(), tarResp.Nickname)
	if err != nil {
		return nil, err
	}
	noticeResp, err := tarUserId.NoticeAddedFriend(u.self, &api.NoticeUserAddedFriendReq{ RequesterInfo: u.data.Info })
	if err != nil {
		return nil, err
	}
	u.data.Friends[noticeResp.RequestedInfo.UserId] = noticeResp.RequestedInfo
	u.data.UpdatedAt = time.Now().Unix()
	return &api.AddUserFriendResp{ Succeed: true }, nil
}

func (u *userAtom) DeleteFriend(from atomos.Id, in *api.DelUserFriendReq) (*api.DelUserFriendResp, error) {
	friend, has := u.data.Friends[in.UserId]
	if !has {
		return nil, errors.New("friend not exists")
	}
	tarUserId, err := api.GetUserId(u.self.Cosmos(), friend.Nickname)
	if err != nil {
		return nil, err
	}
	_, err = tarUserId.NoticeDeletedFriend(u.self, &api.NoticeUserDeletedFriendReq{ FriendInfo: u.data.Info })
	if err != nil {
		return nil, err
	}
	delete(u.data.Friends, friend.UserId)
	u.data.UpdatedAt = time.Now().Unix()
	return &api.DelUserFriendResp{ Succeed: true }, nil
}

func (u *userAtom) NoticeAddedFriend(from atomos.Id, in *api.NoticeUserAddedFriendReq) (*api.NoticeUserAddedFriendResp, error) {
	u.data.Friends[in.RequesterInfo.UserId] = in.RequesterInfo
	u.data.UpdatedAt = time.Now().Unix()
	return nil, nil
}

func (u *userAtom) NoticeUpdatedFriend(from atomos.Id, in *api.NoticeUserUpdatedFriendReq) (*api.Nil, error) {
	u.data.Friends[in.FriendInfo.UserId] = in.FriendInfo
	u.data.UpdatedAt = time.Now().Unix()
	return nil, nil
}

func (u *userAtom) NoticeDeletedFriend(from atomos.Id, in *api.NoticeUserDeletedFriendReq) (*api.Nil, error) {
	if _, has := u.data.Friends[in.FriendInfo.UserId]; has {
		delete(u.data.Friends, in.FriendInfo.UserId)
	}
	u.data.UpdatedAt = time.Now().Unix()
	return nil, nil
}

func (u *userAtom) GetRooms(from atomos.Id, in *api.Nil) (*api.GetUserRoomsResp, error) {
	return &api.GetUserRoomsResp{ Rooms: u.data.Rooms }, nil
}

func (u *userAtom) AddRoom(from atomos.Id, in *api.AddUserRoomReq) (*api.AddUserRoomResp, error) {
	roomId, err := api.GetChatRoomId(u.self.Cosmos(), fmt.Sprintf("%d", in.RoomId))
	if err != nil {
		return nil, err
	}
	roomResp, err := roomId.AddMember(u.self, &api.ChatRoomAddMemberReq{ Requester: u.data.Info })
	if err != nil {
		return nil, err
	}
	if !roomResp.Succeed {
		return nil, errors.New("room add member failed")
	}
	u.data.Rooms[in.RoomId] = roomResp.Room
	u.data.UpdatedAt = time.Now().Unix()
	return &api.AddUserRoomResp{ Succeed: true }, nil
}

func (u *userAtom) DelRoom(from atomos.Id, in *api.DelUserRoomReq) (*api.DelUserRoomResp, error) {
	roomId, err := api.GetChatRoomId(u.self.Cosmos(), fmt.Sprintf("%d", in.RoomId))
	if err != nil {
		return nil, err
	}
	roomResp, err := roomId.DelMember(u.self, &api.ChatRoomDelMemberReq{ UserId: u.data.Info.UserId })
	if err != nil {
		return nil, err
	}
	if !roomResp.Succeed {
		return nil, errors.New("room add member failed")
	}
	delete(u.data.Rooms, in.RoomId)
	u.data.UpdatedAt = time.Now().Unix()
	return &api.DelUserRoomResp{ Succeed: true }, nil
}

func (u *userAtom) RoomUpdated(from atomos.Id, in *api.RoomUpdatedPush) (*api.Nil, error) {
	if room, has := u.data.Rooms[in.RoomId]; has {
		room.Name = in.Name
	}
	return nil, nil
}

func (u *userAtom) RoomMemberUpdated(from atomos.Id, in *api.RoomMemberUpdatedPush) (*api.Nil, error) {
	room, has := u.data.Rooms[in.RoomId]
	if !has {
		return nil, nil
	}
	if in.NewMember {
		room.Members[in.Member.UserId] = in.Member
	} else {
		delete(room.Members, in.Member.UserId)
	}
	return nil, nil
}

func (u *userAtom) RoomMessaging(from atomos.Id, in *api.RoomMessagePush) (*api.Nil, error) {
	if _, has := u.data.Rooms[in.RoomId]; !has {
		return nil, errors.New("user not in room")
	}
	roomMessage, has := u.data.Messages[in.RoomId]
	if !has {
		roomMessage = &api.UserChatMessages{}
		u.data.Messages[in.RoomId] = roomMessage
	}
	roomMessage.UnreadCount += 1
	roomMessage.Messages = append(roomMessage.Messages, &api.ChatMessage{
		SenderUid: in.SenderId,
		Content:   in.Content,
		CreatedAt: in.CreatedAt,
	})
	return nil, nil
}
