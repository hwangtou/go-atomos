package element

import (
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"google.golang.org/protobuf/proto"
	"time"
)

// Firstly, create a struct to implement ElementDeveloper interface.

type RemoteBoothElement struct {
	mainId atomos.MainId
}

func (r *RemoteBoothElement) Load(mainId atomos.MainId) error {
	r.mainId = mainId
	r.mainId.Log().Info("RemoteBoothElement is loading")
	return nil
}

func (r *RemoteBoothElement) Unload() {
	r.mainId.Log().Info("RemoteBoothElement is loading")
}

func (r *RemoteBoothElement) Info() (version uint64, logLevel atomos.LogLevel, initNum int) {
	return 1, atomos.LogLevel_Debug, 1
}

func (r *RemoteBoothElement) AtomConstructor() atomos.Atom {
	return &RemoteBoothAtom{}
}

func (r *RemoteBoothElement) Persistence() atomos.ElementPersistence {
	return nil
}

func (r *RemoteBoothElement) AtomCanKill(id atomos.Id) bool {
	return true
}

// Secondly, create a struct to implement RemoteBoothAtom.

type RemoteBoothAtom struct {
	self         atomos.AtomSelf
	localWatches map[string]api.LocalBoothId
	otherWatches map[string]atomos.Id
}

func (r *RemoteBoothAtom) Spawn(self atomos.AtomSelf, arg *api.RemoteBoothSpawnArg, data *api.RemoteBoothData) error {
	r.self = self
	r.localWatches = map[string]api.LocalBoothId{}
	tid, err := r.self.Task().AddAfter(3 * time.Second, r.Looper, nil)
	r.self.Log().Info("Add timer, tid=%d,err=%v", tid, err)
	return nil
}

func (r *RemoteBoothAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	return nil
}

func (r *RemoteBoothAtom) SayHello(from atomos.Id, in *api.RemoteSayHelloReq) (*api.RemoteSayHelloResp, error) {
	r.self.Log().Info("SayHello, from=%s,id=%d,message=%s", r.id2Str(from), in.Id, in.Message)
	return &api.RemoteSayHelloResp{Id: in.Id}, nil
}

func (r *RemoteBoothAtom) Watch(from atomos.Id, in *api.RemoteWatchReq) (*api.RemoteWatchResp, error) {
	r.self.Log().Info("Watch, from=%s,in=%+V", r.id2Str(from), in)
	localId, ok := from.(api.LocalBoothId)
	if !ok {
		return nil, api.ErrRemoteUnsupportedWatcher
	}
	r.localWatches[r.id2Str(from)] = localId
	return &api.RemoteWatchResp{}, nil
}

func (r *RemoteBoothAtom) Unwatch(from atomos.Id, in *api.RemoteUnwatchReq) (*api.RemoteUnwatchResp, error) {
	r.self.Log().Info("Unwatch, from=%s,in=%+V", r.id2Str(from), in)
	key := r.id2Str(from)
	if _, has := r.localWatches[key]; !has {
		return nil, api.ErrRemoteUnwatchedWatcher
	}
	delete(r.localWatches, key)
	return &api.RemoteUnwatchResp{}, nil
}

func (r *RemoteBoothAtom) Looper(taskId uint64) {
	if taskId == 10 {
		r.self.Log().Info("Exit looper, kill self.")
		r.self.KillSelf()
		return
	}
	r.self.Log().Info("Looper, taskId=%d", taskId)
	for key, watcherId := range r.localWatches {
		if _, err := watcherId.RemoteNotice(r.self, &api.LocalRemoteNoticeReq{}); err != nil {
			delete(r.localWatches, key)
			r.self.Log().Error("Lost contact with remote watcher and deleted, id=%+V,err=%v", key, err)
		}
		r.self.Log().Info("Notice remote watcher, id=%+V", key)
	}
	if _, err := r.self.Task().AddAfter(3 * time.Second, r.Looper, nil); err != nil {
		r.self.Log().Error("Loop add task failed, err=%v", err)
	}
}

func (r *RemoteBoothAtom) id2Str(id atomos.Id) string {
	return fmt.Sprintf("%s:%s:%s", id.Cosmos().GetNodeName(), id.Element().GetName(), id.Name())
}
