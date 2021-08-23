package element

import (
	"errors"
	"google.golang.org/protobuf/proto"

	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/chat/api"
)

const (
	UserManagerElementName = "UserManager"
)

// Element

type UserManagerElement struct {
}

func (u *UserManagerElement) Check() error {
	return nil
}

func (u *UserManagerElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return UserManagerElementName, 1, atomos.LogLevel_Debug, 1
}

func (u *UserManagerElement) AtomConstructor() atomos.Atom {
	return &userManagerAtom{}
}

func (u *UserManagerElement) AtomSaver(id atomos.Id, stateful atomos.AtomStateful) error {
	return nil
}

func (u *UserManagerElement) AtomCanKill(id atomos.Id) bool {
	return true
}

// Atom

type userManagerAtom struct {
	self atomos.AtomSelf
	*api.UserManager
}

func (u userManagerAtom) Spawn(self atomos.AtomSelf, arg proto.Message) error {
	self.Log().Info("Spawn")
	u.self = self
	u.UserManager = arg.(*api.UserManager)
	return nil
}

func (u userManagerAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) {
	u.self.Log().Info("Halt")
}

func (u *userManagerAtom) Register(from atomos.Id, in *api.UserRegisterReq) (*api.UserRegisterResp, error) {
	_, has := u.Users[in.User.Id]
	if has {
		return &api.UserRegisterResp{ Succeed: false }, errors.New("user exists")
	}
	u.Users[in.User.Id] = &api.User{
		Info:    in.User,
		Friends: map[int64]*api.UserBrief{},
	}
	return &api.UserRegisterResp{ Succeed: true }, nil
}

func (u *userManagerAtom) GetUser(from atomos.Id, in *api.GetUserReq) (*api.GetUserResp, error) {
	user, has := u.Users[in.Id]
	if !has {
		return nil, errors.New("user not exists")
	}
	return &api.GetUserResp{ User: user.Info }, nil
}
