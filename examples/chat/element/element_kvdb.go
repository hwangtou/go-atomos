package element

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/chat/api"
	"github.com/syndtr/goleveldb/leveldb"
	"google.golang.org/protobuf/proto"
)

const (
	KvDbElementName = "DB"
	KvDbElementAtomInit = 1
)

// Element

type KvDbElement struct {
}

func (k *KvDbElement) Check() error {
	return nil
}

func (k *KvDbElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return KvDbElementName, 1, atomos.LogLevel_Debug, 1
}

func (k *KvDbElement) AtomConstructor() atomos.Atom {
	return &kvDbAtom{}
}

func (k *KvDbElement) AtomDataLoader(name string) (proto.Message, error) {
	return nil, nil
}

func (k *KvDbElement) AtomDataSaver(name string, data proto.Message) error {
	return nil
}

func (k *KvDbElement) AtomCanKill(id atomos.Id) bool {
	switch id.Name() {
	case atomos.MainAtomName:
		return true
	default:
		return false
	}
}

// Atom

type kvDbAtom struct {
	self atomos.AtomSelf
	arg  *api.KvDbSpawnArg
	db   *leveldb.DB
}

func (k *kvDbAtom) Spawn(self atomos.AtomSelf, arg *api.KvDbSpawnArg, data *api.KvDb) error {
	k.self = self
	if arg == nil {
		// Reload spawn.
	} else {
		k.arg = arg
	}
	db, err := leveldb.OpenFile(k.arg.DbPath, nil)
	if err != nil {
		return err
	}
	k.db = db
	return nil
}

func (k *kvDbAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	if err := k.db.Close(); err != nil {
		k.self.Log().Error("Db close error, err=%v", err)
	}
	k.db = nil
	return nil
}

func (k kvDbAtom) Get(from atomos.Id, in *api.DbGetReq) (*api.DbGetResp, error) {
	resp := &api.DbGetResp{}
	data, err := k.db.Get([]byte(in.Key), nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return resp, nil
		}
		return nil, err
	}
	resp.Has = true
	resp.Value = data
	return resp, nil
}

func (k kvDbAtom) Set(from atomos.Id, in *api.DbSetReq) (*api.DbSetResp, error) {
	resp := &api.DbSetResp{}
	err := k.db.Put([]byte(in.Key), in.Value, nil)
	if err != nil {
		return nil, err
	}
	resp.Succeed = true
	return resp, nil
}

func (k kvDbAtom) Del(from atomos.Id, in *api.DbDelReq) (*api.DbDelResp, error) {
	resp := &api.DbDelResp{}
	err := k.db.Delete([]byte(in.Key), nil)
	if err != nil {
		return nil, err
	}
	resp.Succeed = true
	return resp, nil
}
