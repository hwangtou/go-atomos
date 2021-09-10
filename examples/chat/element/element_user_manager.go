package element

import (
	"errors"
	"google.golang.org/protobuf/proto"
	"time"

	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/chat/api"
)

// Element

type UserManagerElement struct {
}

func (u *UserManagerElement) Load(mainId atomos.MainId) error {
	return nil
}

func (u *UserManagerElement) Unload() {
}

func (u *UserManagerElement) Persistence() atomos.ElementPersistence {
	return nil
}

func (u *UserManagerElement) Info() (version uint64, logLevel atomos.LogLevel, initNum int) {
	return 1, atomos.LogLevel_Debug, 1
}

func (u *UserManagerElement) AtomConstructor() atomos.Atom {
	return &userManagerAtom{}
}

func (u *UserManagerElement) AtomCanKill(id atomos.Id) bool {
	return true
}

// Atom

type userManagerAtom struct {
	self atomos.AtomSelf
	*api.UserManager
}

func (u *userManagerAtom) Spawn(self atomos.AtomSelf, arg *api.UserManagerSpawnArg, data *api.UserManager) error {
	self.Log().Info("Spawn")
	u.self = self
	u.UserManager = data
	return nil
}

func (u *userManagerAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	u.self.Log().Info("Halt")
	return u.UserManager
}

func (u *userManagerAtom) RegisterUser(from atomos.Id, in *api.RegisterUserReq) (*api.RegisterUserResp, error) {
	_, has := u.Users[in.User.Id]
	if has {
		return &api.RegisterUserResp{ Succeed: false }, errors.New("user exists")
	}
	u.Users[in.User.Id] = time.Now().Unix()
	return &api.RegisterUserResp{ Succeed: true }, nil
}

func (u *userManagerAtom) FindUser(from atomos.Id, in *api.FindUserReq) (*api.FindUserResp, error) {
	_, has := u.Users[in.Id]
	if has {
		return &api.FindUserResp{ Has: false }, errors.New("user exists")
	}
	u.Users[in.Id] = time.Now().Unix()
	return &api.FindUserResp{ Has: true }, nil
}
