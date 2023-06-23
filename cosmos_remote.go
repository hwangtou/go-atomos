package go_atomos

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"strconv"
	"sync"
	"time"
)

type CosmosRemote struct {
	process *CosmosProcess
	id      *IDInfo
	lock    *CosmosNodeVersionLock

	mutex   sync.RWMutex
	enable  bool
	current *cosmosRemoteVersion
	version map[string]*cosmosRemoteVersion

	elements map[string]*ElementRemote
	//*idTrackerManager
}

func newCosmosRemoteFromNodeInfo(process *CosmosProcess, info *CosmosNodeVersionInfo) *CosmosRemote {
	c := &CosmosRemote{
		process:  process,
		id:       info.Id,
		mutex:    sync.RWMutex{},
		enable:   false,
		current:  nil,
		version:  map[string]*cosmosRemoteVersion{},
		elements: map[string]*ElementRemote{},
	}
	return c
}

func newCosmosRemoteFromLockInfo(process *CosmosProcess, lock *CosmosNodeVersionLock) *CosmosRemote {
	c := &CosmosRemote{
		process:  process,
		id:       nil,
		mutex:    sync.RWMutex{},
		enable:   false,
		current:  nil,
		version:  map[string]*cosmosRemoteVersion{},
		elements: map[string]*ElementRemote{},
	}
	return c
}

// Use with mutex protect
func (c *CosmosRemote) refresh() {
	if c.lock == nil {
		c.enable = false
		return
	}
	if c.lock.Current == 0 {
		c.enable = false
		return
	}
	version, has := c.version[strconv.FormatInt(c.lock.Current, 10)]
	if !has {
		c.enable = false
		return
	}
	c.current = version
	c.enable = true
}

func (c *CosmosRemote) etcdUpdateLock(lock *CosmosNodeVersionLock) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.lock = lock
	c.refresh()
}

func (c *CosmosRemote) etcdDeleteLock() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.lock = nil
	c.refresh()
}

func (c *CosmosRemote) etcdCreateVersion(info *CosmosNodeVersionInfo, version string) {
	c.process.local.Log().Debug("CosmosRemote: Connect info version created. version=(%s)", version)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.id = info.Id
	c.version[version] = newCosmosRemoteVersion(c.process, info, version)
	if info.Elements != nil {
		for elemName, idInfo := range info.Elements {
			e, has := c.process.local.runnable.interfaces[elemName] // It's ok, because the interface will never be changed.
			if !has {
				c.process.local.Log().Error("CosmosRemote: Connect info element not supported. name=(%s)", elemName)
			}
			c.elements[elemName] = newElementRemote(c, idInfo, e, version)
		}
	}
	c.refresh()
}

func (c *CosmosRemote) etcdUpdateVersion(info *CosmosNodeVersionInfo, version string) {
	c.process.local.Log().Debug("CosmosRemote: Connect info version updated. version=(%s)", version)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	oldVersion, has := c.version[version]
	if !has {
		c.id = info.Id
		c.version[version] = newCosmosRemoteVersion(c.process, info, version)
		if info.Elements != nil {
			for elemName, idInfo := range info.Elements {
				e, has := c.process.local.runnable.interfaces[elemName] // It's ok, because the interface will never be changed.
				if !has {
					c.process.local.Log().Error("CosmosRemote: Connect info element not supported. name=(%s)", elemName)
				}
				c.elements[elemName] = newElementRemote(c, idInfo, e, version)
			}
		}
		c.refresh()
		return
	}
	if proto.Equal(info, oldVersion.info) {
		return
	}

	c.id = info.Id
	if info.Address != oldVersion.info.Address {
		oldVersion.setDisable()
		c.process.local.Log().Debug("CosmosRemote: Connect info version updated. version=(%s)", version)
		c.version[version] = newCosmosRemoteVersion(c.process, info, version)
	}

	// Compare old element and new element to know which element is added or removed.
	// If the element is removed, it will be disabled in the element list.
	// If the element is added, it will be added to the element list.
	// If the element is not changed, it will be ignored.
	if info.Elements != nil {
		for elemName, newInfo := range info.Elements {
			e, has := c.process.local.runnable.interfaces[elemName]
			if !has {
				c.process.local.Log().Error("CosmosRemote: Connect info element not supported. name=(%s)", elemName)
			}
			if oldElem, has := c.elements[elemName]; has {
				oldElem.setDisable()
			} else {
				c.elements[elemName] = newElementRemote(c, newInfo, e, version)
			}
		}
	}
	c.refresh()
}

func (c *CosmosRemote) etcdDeleteVersion(version string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	v, has := c.version[version]
	if has {
		delete(c.version, version)
		if c.current != nil && c.current.version == v.version {
			c.current = nil
			//for _, elem := range c.elements {
			//	elem.setDisable()
			//}
		}
	}
	c.refresh()
}

func (c *CosmosRemote) getCurrentClient() *grpc.ClientConn {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if !c.enable {
		return nil
	}
	if c.current == nil {
		return nil
	}
	return c.current.client
}

func (c *CosmosRemote) getElement(name string) (*ElementRemote, *Error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	elem, has := c.elements[name]
	if !has {
		return nil, NewErrorf(ErrCosmosRemoteElementNotFound, "CosmosRemote: Element not found. name=(%s)", name).AddStack(nil)
	}
	return elem, nil
}

// Implementation of ID

func (c *CosmosRemote) GetIDInfo() *IDInfo {
	if c == nil {
		return nil
	}
	return c.id
}

func (c *CosmosRemote) String() string {
	return c.id.Info()
}

func (c *CosmosRemote) Cosmos() CosmosNode {
	return c
}

func (c *CosmosRemote) State() AtomosState {
	cli := c.getCurrentClient()
	if cli == nil {
		return AtomosState(0)
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rsp, er := client.GetIDState(ctx, &CosmosRemoteGetIDStateReq{
		Id: c.id,
	})
	if er != nil {
		c.process.local.Log().Error("CosmosRemote: GetIDState failed. err=(%v)", er)
		return AtomosState(0)
	}

	return AtomosState(rsp.State)
}

func (c *CosmosRemote) IdleTime() time.Duration {
	cli := c.getCurrentClient()
	if cli == nil {
		return 0
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rsp, er := client.GetIDIdleTime(ctx, &CosmosRemoteGetIDIdleTimeReq{
		Id: c.id,
	})
	if er != nil {
		c.process.local.Log().Error("CosmosRemote: GetIDIdleTime failed. err=(%v)", er)
		return 0
	}

	return time.Duration(rsp.IdleTime)
}

func (c *CosmosRemote) SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error) {
	cli := c.getCurrentClient()
	if cli == nil {
		return nil, NewError(ErrCosmosRemoteConnectFailed, "CosmosRemote: SyncMessagingByName client error.").AddStack(nil)
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	arg, er := anypb.New(in)
	if er != nil {
		return nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "CosmosRemote: SyncMessagingByName arg error. err=(%v)", er).AddStack(nil)
	}

	rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId:               callerID.GetIDInfo(),
		CallerCurFirstSyncCall: callerID.getCurFirstSyncCall(),
		To:                     c.id,
		Timeout:                int64(timeout),
		Message:                name,
		Args:                   arg,
	})
	if er != nil {
		return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "CosmosRemote: SyncMessagingByName reply error. rsp=(%v),err=(%v)", rsp, er).AddStack(nil)
	}
	if rsp.Reply != nil {
		out, er = rsp.Reply.UnmarshalNew()
		if er != nil {
			return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "CosmosRemote: SyncMessagingByName reply unmarshal error. err=(%v)", er).AddStack(nil)
		}
	}
	if rsp.Error != nil {
		err = rsp.Error.AddStack(nil)
	}
	return out, err
}

func (c *CosmosRemote) AsyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message, callback func(out proto.Message, err *Error)) {
	cli := c.getCurrentClient()
	if cli == nil {
		callback(nil, NewError(ErrCosmosRemoteConnectFailed, "CosmosRemote: AsyncMessagingByName client error.").AddStack(nil))
		return
	}
	if callerID == nil {
		callback(nil, NewError(ErrFrameworkIncorrectUsage, "CosmosRemote: AsyncMessagingByName without fromID.").AddStack(nil))
		return
	}

	// 这种情况需要创建新的FirstSyncCall，因为这是一个新的调用链，调用的开端是push向的ID。
	callerIDInfo := callerID.GetIDInfo()
	firstSyncCall := callerID.nextFirstSyncCall()

	c.process.local.Parallel(func() {
		out, err := func() (out proto.Message, err *Error) {
			client := NewAtomosRemoteServiceClient(cli)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			arg, er := anypb.New(in)
			if er != nil {
				return nil, NewError(ErrCosmosRemoteRequestInvalid, "CosmosRemote: SyncMessagingByName arg error.").AddStack(nil)
			}
			rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
				CallerId:               callerIDInfo,
				CallerCurFirstSyncCall: firstSyncCall,
				To:                     c.id,
				Timeout:                int64(timeout),
				Message:                name,
				Args:                   arg,
			})
			if rsp.Reply != nil {
				out, er = rsp.Reply.UnmarshalNew()
				if er != nil {
					return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "CosmosRemote: SyncMessagingByName reply unmarshal error. err=(%v)", er).AddStack(nil)
				}
			}
			if er != nil {
				return nil, NewError(ErrCosmosRemoteResponseInvalid, "CosmosRemote: SyncMessagingByName reply error.").AddStack(nil)
			}
			if rsp.Error != nil {
				err = rsp.Error.AddStack(nil)
			}
			return out, err
		}()
		callerID.pushAsyncMessageCallbackMailAndWaitReply(name, out, err, callback)
	})
}

func (c *CosmosRemote) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	return nil, nil
}

func (c *CosmosRemote) Kill(callerID SelfID, timeout time.Duration) *Error {
	return NewError(ErrCosmosRemoteCannotKill, "CosmosRemote: Cannot kill remote.").AddStack(nil)
}

func (c *CosmosRemote) SendWormhole(callerID SelfID, timeout time.Duration, wormhole AtomosWormhole) *Error {
	return NewError(ErrCosmosRemoteCannotSendWormhole, "CosmosGlobal: Cannot send wormhole remote.").AddStack(nil)
}

//func (c *CosmosRemote) getIDTrackerManager() *idTrackerManager {
//	return c.idTrackerManager
//}

func (c *CosmosRemote) getGoID() uint64 {
	return c.id.GoId
}

// Implementation of CosmosNode

func (c *CosmosRemote) GetNodeName() string {
	return c.GetIDInfo().Node
}

func (c *CosmosRemote) CosmosIsLocal() bool {
	return false
}

func (c *CosmosRemote) CosmosGetElementID(elem string) (ID, *Error) {
	return c.getElement(elem)
}

func (c *CosmosRemote) CosmosGetAtomID(elem, name string) (ID, *IDTracker, *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	return element.GetAtomID(name, nil, false)
}

func (c *CosmosRemote) CosmosGetScaleAtomID(callerID SelfID, elem, message string, timeout time.Duration, args proto.Message) (ID ID, tracker *IDTracker, err *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	return element.ScaleGetAtomID(callerID, message, timeout, args, nil, false)
}

func (c *CosmosRemote) CosmosSpawnAtom(elem, name string, arg proto.Message) (ID, *IDTracker, *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err
	}
	return element.SpawnAtom(name, arg, nil, false)
}

func (c *CosmosRemote) ElementBroadcast(callerID ID, key, contentType string, contentBuffer []byte) (err *Error) {
	cli := c.getCurrentClient()
	if cli == nil {
		return NewError(ErrCosmosRemoteConnectFailed, "CosmosRemote: ElementBroadcast client error.").AddStack(nil)
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rsp, er := client.ElementBroadcast(ctx, &CosmosRemoteElementBroadcastReq{
		CallerId:      callerID.GetIDInfo(),
		Key:           key,
		ContentType:   contentType,
		ContentBuffer: contentBuffer,
	})

	if er != nil {
		return NewErrorf(ErrCosmosRemoteRequestInvalid, "CosmosRemote: ElementBroadcast error. err=(%v)", er).AddStack(nil)
	}
	if rsp.Error != nil {
		return rsp.Error.AddStack(nil)
	}
	return nil
}

// Remote

type cosmosRemoteVersion struct {
	process *CosmosProcess
	info    *CosmosNodeVersionInfo
	avail   bool
	client  *grpc.ClientConn
	version string
}

func newCosmosRemoteVersion(process *CosmosProcess, info *CosmosNodeVersionInfo, version string) *cosmosRemoteVersion {
	c := &cosmosRemoteVersion{
		process: process,
		info:    info,
		avail:   false,
		client:  nil,
	}
	c.check()
	return c
}

func (c *cosmosRemoteVersion) check() bool {
	if c.avail {
		return true
	}
	var er error
	if c.process.cluster.grpcDialOption == nil {
		c.client, er = grpc.Dial(c.info.Address, grpc.WithInsecure())
	} else {
		c.client, er = grpc.Dial(c.info.Address, *c.process.cluster.grpcDialOption)
	}
	if er != nil {
		c.process.local.Log().Fatal("CosmosRemote: Dial failed. err=(%v)", er)
		return false
	}
	c.avail = true
	return true
}

func (c *cosmosRemoteVersion) setDisable() {
	if c.client != nil {
		c.client.Close()
	}
}

func (c *CosmosRemote) tryKillingRemote() (err *Error) {
	var targetVersion *cosmosRemoteVersion
	c.mutex.RLock()
	switch len(c.version) {
	case 0:
	case 1:
		for s := range c.version {
			targetVersion = c.version[s]
		}
	default:
		err = NewError(ErrCosmosEtcdClusterVersionsCheckFailed, "CosmosRemote: Version invalid.").AddStack(nil)
	}
	c.mutex.RUnlock()

	if err != nil {
		return err.AddStack(nil)
	}
	if targetVersion == nil {
		return nil
	}
	cli := targetVersion.client
	if cli == nil {
		return NewError(ErrCosmosRemoteConnectFailed, "CosmosRemote: Client not found.").AddStack(nil)
	}

	client := NewAtomosRemoteServiceClient(cli)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, er := client.TryKilling(ctx, &CosmosRemoteTryKillingReq{})

	if er != nil {
		return NewErrorf(ErrCosmosRemoteRequestInvalid, "CosmosRemote: Try killing error. err=(%v)", er).AddStack(nil)
	}
	return nil
}

// remoteCosmosFakeSelfID is a fake self id for remote cosmos.

type remoteCosmosFakeSelfID struct {
	*CosmosRemote
	firstSyncCall string
}

func (c *CosmosRemote) newFakeCosmosSelfID(call string) *remoteCosmosFakeSelfID {
	return &remoteCosmosFakeSelfID{
		CosmosRemote:  c,
		firstSyncCall: call,
	}
}

func (r *remoteCosmosFakeSelfID) callerCounterDecr() {
}

func (r *remoteCosmosFakeSelfID) Log() Logging {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) Task() Task {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) getCurFirstSyncCall() string {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) setSyncMessageAndFirstCall(s string) *Error {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) unsetSyncMessageAndFirstCall() {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) nextFirstSyncCall() string {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) CosmosMain() *CosmosLocal {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) KillSelf() {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) Parallel(f func()) {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) Config() map[string][]byte {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) pushAsyncMessageCallbackMailAndWaitReply(name string, in proto.Message, err *Error, callback func(out proto.Message, err *Error)) {
	panic("not supported, should not be called")
}
