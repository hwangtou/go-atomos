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

type UserManagerElement struct {
	mainId  atomos.MainId
	persist *userManagerPersistence
}

func (u *UserManagerElement) Load(mainId atomos.MainId) error {
	u.mainId = mainId
	return nil
}

func (u *UserManagerElement) Unload() {
}

func (u *UserManagerElement) Persistence() atomos.ElementPersistence {
	if u.persist == nil {
		u.persist = &userManagerPersistence{
			mainId: u.mainId,
		}
	}
	return u.persist
}

func (u *UserManagerElement) Info() (version uint64, logLevel atomos.LogLevel, initNum int) {
	return 1, atomos.LogLevel_Debug, 1
}

func (u *UserManagerElement) AtomConstructor() atomos.Atom {
	return &userManagerAtom{}
}

func (u *UserManagerElement) AtomCanKill(id atomos.Id) bool {
	return id.Name() == atomos.MainAtomName
}

// Element Persistence

type userManagerPersistence struct {
	mainId atomos.MainId
	dbId   api.KvDbAtomId
}

func (c *userManagerPersistence) GetAtomData(name string) (data proto.Message, err error) {
	if c.dbId == nil {
		if c.dbId, err = api.GetKvDbAtomId(c.mainId.Cosmos(), "DB"); err != nil {
			return nil, err
		}
	}
	name = fmt.Sprintf("UserManager:%s", name)
	return c.dbId.Get(c.mainId, &api.DbGetReq{ Key: name })
}

func (c *userManagerPersistence) SetAtomData(name string, data proto.Message) (err error) {
	if c.dbId == nil {
		if c.dbId, err = api.GetKvDbAtomId(c.mainId.Cosmos(), "DB"); err != nil {
			return err
		}
	}
	buf, err := proto.Marshal(data)
	if err != nil {
		return err
	}
	name = fmt.Sprintf("UserManager:%s", name)
	_, err = c.dbId.Set(c.mainId, &api.DbSetReq{ Key: name, Value: buf })
	if err != nil {
		return err
	}
	return nil
}

// Atom

type userManagerAtom struct {
	self atomos.AtomSelf
	data *api.UserManagerData
}

func (u *userManagerAtom) Spawn(self atomos.AtomSelf, arg *api.UserManagerSpawnArg, data *api.UserManagerData) error {
	self.Log().Info("Spawn")
	u.self = self
	u.data = data
	return nil
}

func (u *userManagerAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	u.self.Log().Info("Halt")
	return u.data
}

func (u *userManagerAtom) RegisterUser(from atomos.Id, in *api.RegisterUserReq) (resp *api.RegisterUserResp, err error) {
	resp = &api.RegisterUserResp{}
	// Check if the nickname has been already registered.
	if _, has := u.data.Users[in.Nickname]; has {
		err = errors.New("nickname registered")
		resp.Reason = err.Error()
		return
	}
	// Check if the password is valid.
	if len(in.Password) < 6 {
		err = errors.New("password too short")
		resp.Reason = err.Error()
		return
	}
	// Registration.
	u.data.CurId += 1
	uid := u.data.CurId
	u.data.Users[in.Nickname] = &api.UserRegistration{
		UserId:   uid,
		Nickname: in.Nickname,
		Password: in.Password,
		Updated:  time.Now().Unix(),
	}
	// Spawn.
	_, err = api.SpawnUser(u.self.Cosmos(), in.Nickname, &api.UserSpawnArg{ UserId: uid, Nickname: in.Nickname })
	if err != nil {
		resp.Reason = err.Error()
		return
	}
	resp.Succeed = true
	resp.Reason = "succeed"
	resp.UserId = uid
	return
}

func (u *userManagerAtom) LoginUser(from atomos.Id, in *api.LoginUserReq) (resp *api.LoginUserResp, err error) {
	resp = &api.LoginUserResp{}
	user, has := u.data.Users[in.Nickname]
	if !has {
		err = errors.New("nickname not found")
		return
	}
	if user.Password != in.Password {
		err = errors.New("password invalid")
		return
	}
	resp.Succeed = true
	resp.UserId = user.UserId
	resp.Nickname = user.Nickname
	return
}

func (u *userManagerAtom) FindUser(from atomos.Id, in *api.FindUserReq) (resp *api.FindUserResp, err error) {
	resp = &api.FindUserResp{}
	var user *api.UserRegistration
	if in.Nickname != "" {
		user = u.data.Users[in.Nickname]
		if user == nil {
			err = errors.New("nickname not found")
			return
		}
	} else if in.UserId != 0 {
		for _, u := range u.data.Users {
			if u.UserId == in.UserId {
				user = u
			}
		}
		if user == nil {
			err = errors.New("user id not found")
			return
		}
	} else {
		err = errors.New("find invalid")
		return
	}
	resp.Has = true
	resp.UserId = user.UserId
	resp.Nickname = user.Nickname
	return
}

func (u *userManagerAtom) DeleteUser(from atomos.Id, in *api.DeleteUserReq) (resp *api.DeleteUserResp, err error) {
	resp = &api.DeleteUserResp{}
	_, has := u.data.Users[in.Nickname]
	if !has {
		err = errors.New("nickname not found")
		return
	}
	// TODO: Think about how to delete an atom.
	return nil, err
}
