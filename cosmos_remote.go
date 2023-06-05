package go_atomos

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"sync"
	"time"
)

type CosmosRemote struct {
	process *CosmosProcess
	info    *CosmosProcessInfo

	mutex   sync.RWMutex
	enable  bool
	current *cosmosRemoteVersion
	version map[string]*cosmosRemoteVersion

	elements map[string]*ElementRemote
	*idTrackerManager
}

func newCosmosRemote(process *CosmosProcess, info *CosmosProcessInfo, version string) *CosmosRemote {
	c := &CosmosRemote{
		process:  process,
		info:     info,
		mutex:    sync.RWMutex{},
		enable:   true,
		current:  nil,
		version:  map[string]*cosmosRemoteVersion{},
		elements: map[string]*ElementRemote{},
	}
	c.version[version] = newCosmosRemoteVersion(process, info)

	if info.Elements != nil {
		for elemName, idInfo := range info.Elements {
			e, has := process.local.runnable.interfaces[elemName]
			if !has {
				process.local.Log().Error("CosmosRemote: Connect info element not supported. name=(%s)", elemName)
			}
			c.elements[elemName] = newElementRemote(c, idInfo, e)
		}
	}

	return c
}

func (c *CosmosRemote) setCurrent(version string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	v, has := c.version[version]
	if !has {
		// TODO: error
		return
	}
	c.current = v
}

func (c *CosmosRemote) unsetCurrent() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.current = nil
}

func (c *CosmosRemote) updateVersion(info *CosmosProcessInfo, version string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.enable = true

	// 目前不支持同一个version的更新
	if _, has := c.version[version]; has {
		return
	}

	c.version[version] = newCosmosRemoteVersion(c.process, info)

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
				c.elements[elemName] = newElementRemote(c, newInfo, e)
			}
		}
	}
}

func (c *CosmosRemote) setDeleted() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, elem := range c.elements {
		elem.setDisable()
	}
	c.enable = false
	c.current = nil
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
	return c.info.Id
}

func (c *CosmosRemote) String() string {
	return c.info.Id.Info()
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
		Id: c.info.Id,
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
		Id: c.info.Id,
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
		To:                     c.info.Id,
		Timeout:                int64(timeout),
		Message:                name,
		Args:                   arg,
	})
	out, er = rsp.Reply.UnmarshalNew()
	if er != nil {
		return nil, NewError(ErrCosmosRemoteResponseInvalid, "CosmosRemote: SyncMessagingByName reply error.").AddStack(nil)
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
				To:                     c.info.Id,
				Timeout:                int64(timeout),
				Message:                name,
				Args:                   arg,
			})
			out, er = rsp.Reply.UnmarshalNew()
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

func (c *CosmosRemote) getIDTrackerManager() *idTrackerManager {
	return c.idTrackerManager
}

func (c *CosmosRemote) getGoID() uint64 {
	return c.info.Id.GoId
}

// Implementation of CosmosNode

func (c *CosmosRemote) GetNodeName() string {
	return c.GetIDInfo().Cosmos
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
	return element.GetAtomID(name, NewIDTrackerInfoFromLocalGoroutine(3))
}

func (c *CosmosRemote) CosmosGetScaleAtomID(callerID SelfID, elem, message string, timeout time.Duration, args proto.Message) (ID ID, tracker *IDTracker, err *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	return element.ScaleGetAtomID(callerID, message, timeout, args, NewIDTrackerInfoFromLocalGoroutine(3))
}

func (c *CosmosRemote) CosmosSpawnAtom(elem, name string, arg proto.Message) (ID, *IDTracker, *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err
	}
	return element.SpawnAtom(name, arg, NewIDTrackerInfoFromLocalGoroutine(3))
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
	info    *CosmosProcessInfo
	avail   bool
	client  *grpc.ClientConn
}

func newCosmosRemoteVersion(process *CosmosProcess, info *CosmosProcessInfo) *cosmosRemoteVersion {
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
	if c.process.cluster.dialOption == nil {
		c.client, er = grpc.Dial(c.info.Address, grpc.WithInsecure())
	} else {
		c.client, er = grpc.Dial(c.info.Address, *c.process.cluster.dialOption)
	}
	if er != nil {
		c.process.local.Log().Fatal("CosmosRemote: Dial failed. err=(%v)", er)
		return false
	}
	c.avail = true
	return true
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
