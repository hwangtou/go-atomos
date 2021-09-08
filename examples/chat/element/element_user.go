package element

import (
	"errors"
	"google.golang.org/protobuf/proto"

	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/chat/api"
)

const (
	UserElementName = "User"
)

// Element

type UserElement struct {
}

func (u *UserElement) Load(mainId atomos.MainId) error {
	return nil
}

func (u *UserElement) Unload() {
}

func (u *UserElement) Persistence() atomos.ElementPersistence {
	return nil
}

func (u *UserElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return UserElementName, 1, atomos.LogLevel_Debug, 100
}

func (u *UserElement) AtomConstructor() atomos.Atom {
	return &userAtom{}
}

func (u *UserElement) AtomCanKill(id atomos.Id) bool {
	return id.Name() == UserManagerElementName
}

// Atom

type userAtom struct {
	self atomos.AtomSelf
	user *api.User
	friends map[int64]*api.UserBrief
}

func (u *userAtom) Spawn(self atomos.AtomSelf, arg *api.UserSpawnArg, data *api.User) error {
	self.Log().Info("Spawn")
	u.self = self
	u.user = data
	u.friends = map[int64]*api.UserBrief{}
	// TODO: Load from database
	return nil
}

func (u *userAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	u.self.Log().Info("Halt")
	return u.user
}

func (u *userAtom) UserInfo(from atomos.Id, in *api.UserInfoReq) (*api.UserInfoResp, error) {
	return &api.UserInfoResp{ User: u.user.Info }, nil
}

func (u *userAtom) GetFriends(from atomos.Id, in *api.GetFriendsReq) (resp *api.GetFriendsResp, err error) {
	return &api.GetFriendsResp{ Friends: u.friends, }, nil
}

func (u *userAtom) AddFriend(from atomos.Id, in *api.AddFriendReq) (resp *api.AddFriendResp, err error) {
	_, has := u.friends[in.User.Id]
	if has {
		resp = &api.AddFriendResp{
			Succeed: true,
		}
		err = errors.New("friend exists")
		return
	}
	u.friends[in.User.Id] = in.User
	return &api.AddFriendResp{ Succeed: true }, nil
}

func (u *userAtom) RoomMessage(from atomos.Id, in *api.RoomMessagePush) (*api.RoomMessagePushResp, error) {
	return nil, nil
}
