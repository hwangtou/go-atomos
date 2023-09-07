package go_atomos

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"net"
	"strconv"
	"sync"
	"time"
)

type CosmosRemote struct {
	process *CosmosProcess
	context atomosIDContextRemote
	lock    *CosmosNodeVersionLock

	mutex   sync.RWMutex
	enable  bool
	current *cosmosRemoteVersion
	version map[string]*cosmosRemoteVersion

	elements map[string]*ElementRemote
}

func newCosmosRemoteFromNodeInfo(process *CosmosProcess, info *CosmosNodeVersionInfo) *CosmosRemote {
	c := &CosmosRemote{
		process:  process,
		context:  atomosIDContextRemote{},
		mutex:    sync.RWMutex{},
		enable:   false,
		current:  nil,
		version:  map[string]*cosmosRemoteVersion{},
		elements: map[string]*ElementRemote{},
	}
	initAtomosIDContextRemote(&c.context, info.Id)
	return c
}

func newCosmosRemoteFromLockInfo(process *CosmosProcess, lock *CosmosNodeVersionLock) *CosmosRemote {
	c := &CosmosRemote{
		process:  process,
		context:  atomosIDContextRemote{},
		mutex:    sync.RWMutex{},
		enable:   false,
		current:  nil,
		version:  map[string]*cosmosRemoteVersion{},
		elements: map[string]*ElementRemote{},
	}
	initAtomosIDContextRemote(&c.context, &IDInfo{})
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
	c.process.local.Log().coreInfo("CosmosRemote: Connect info version created. node=(%s),version=(%s),state=(%v),addr=(%s)",
		info.Node, version, info.State, info.Address)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.context.info = info.Id
	c.version[version] = newCosmosRemoteVersion(c.process, info, version)
	if info.Elements != nil {
		for elemName, idInfo := range info.Elements {
			//e, has := c.process.local.runnable.interfaces[elemName] // It's ok, because the interface will never be changed.
			e, has := c.process.local.runnable.implements[elemName] // It's ok, because the interface will never be changed.
			if !has {
				c.process.local.Log().coreError("CosmosRemote: Connect info element not supported. name=(%s)", elemName)
			} else {
				c.elements[elemName] = newElementRemote(c, idInfo, e.Interface, version)
			}
		}
	}
	c.refresh()
}

func (c *CosmosRemote) etcdUpdateVersion(info *CosmosNodeVersionInfo, version string) {
	c.process.local.Log().coreInfo("CosmosRemote: Connect info version updated. node=(%s),version=(%s),state=(%v),addr=(%s)",
		info.Node, version, info.State, info.Address)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	oldVersion, has := c.version[version]
	if !has {
		c.context.info = info.Id
		c.version[version] = newCosmosRemoteVersion(c.process, info, version)
		if info.Elements != nil {
			for elemName, idInfo := range info.Elements {
				//e, has := c.process.local.runnable.interfaces[elemName] // It's ok, because the interface will never be changed.
				e, has := c.process.local.runnable.implements[elemName] // It's ok, because the interface will never be changed.
				if !has {
					c.process.local.Log().coreError("CosmosRemote: Connect info element not supported. name=(%s)", elemName)
				} else {
					c.elements[elemName] = newElementRemote(c, idInfo, e.Interface, version)
				}
			}
		}
		c.refresh()
		return
	}
	if proto.Equal(info, oldVersion.info) {
		return
	}

	c.context.info = info.Id
	if info.Address != oldVersion.info.Address {
		oldVersion.setDisable()
		c.process.local.Log().coreInfo("CosmosRemote: Connect info version updated. version=(%s)", version)
		c.version[version] = newCosmosRemoteVersion(c.process, info, version)
	}

	// Compare old element and new element to know which element is added or removed.
	// If the element is removed, it will be disabled in the element list.
	// If the element is added, it will be added to the element list.
	// If the element is not changed, it will be ignored.
	if info.Elements != nil {
		for elemName, newInfo := range info.Elements {
			//e, has := c.process.local.runnable.interfaces[elemName]
			e, has := c.process.local.runnable.implements[elemName]
			if !has {
				c.process.local.Log().coreError("CosmosRemote: Connect info element not supported. name=(%s)", elemName)
			} else {
				if oldElem, has := c.elements[elemName]; has {
					oldElem.setDisable()
				} else {
					c.elements[elemName] = newElementRemote(c, newInfo, e.Interface, version)
				}
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

func (c *CosmosRemote) getCurrentClientWithTimeout(timeout time.Duration) (AtomosRemoteServiceClient, context.Context, context.CancelFunc, *Error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if !c.enable {
		return nil, nil, nil, NewError(ErrCosmosRemoteConnectFailed, "CosmosRemote: Not enabled.").AddStack(nil)
	}
	if c.current == nil {
		return nil, nil, nil, NewError(ErrCosmosRemoteConnectFailed, "CosmosRemote: No current remote.").AddStack(nil)
	}
	if timeout == 0 {
		// 0 means no timeout, but we need to set a timeout to avoid deadlock.
		timeout = atomosGRPCTimeout
	} else {
		timeout += atomosGRPCTTL
	}
	client := NewAtomosRemoteServiceClient(c.current.client)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return client, ctx, cancel, nil
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

func (c *CosmosRemote) GetIDContext() IDContext {
	return &c.context
}

func (c *CosmosRemote) GetIDInfo() *IDInfo {
	if c == nil {
		return nil
	}
	return c.context.info
}

func (c *CosmosRemote) String() string {
	return c.context.info.Info()
}

func (c *CosmosRemote) Cosmos() CosmosNode {
	return c
}

func (c *CosmosRemote) State() AtomosState {
	client, ctx, cancel, err := c.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		c.process.local.Log().Error("CosmosRemote: GetIDState failed. err=(%v)", err)
		return AtomosState(0)
	}
	defer cancel()

	rsp, er := client.GetIDState(ctx, &CosmosRemoteGetIDStateReq{Id: c.context.info})
	if er != nil {
		c.process.local.Log().Error("CosmosRemote: GetIDState failed. err=(%v)", er)
		return AtomosState(0)
	}

	return AtomosState(rsp.State)
}

func (c *CosmosRemote) IdleTime() time.Duration {
	client, ctx, cancel, err := c.getCurrentClientWithTimeout(atomosGRPCTTL)
	if err != nil {
		c.process.local.Log().Error("CosmosRemote: GetIDIdleTime failed. err=(%v)", err)
		return 0
	}
	defer cancel()

	rsp, er := client.GetIDIdleTime(ctx, &CosmosRemoteGetIDIdleTimeReq{
		Id: c.context.info,
	})
	if er != nil {
		c.process.local.Log().Error("CosmosRemote: GetIDIdleTime failed. err=(%v)", er)
		return 0
	}

	return time.Duration(rsp.IdleTime)
}

func (c *CosmosRemote) SyncMessagingByName(callerID SelfID, name string, timeout time.Duration, in proto.Message) (out proto.Message, err *Error) {
	if callerID == nil {
		return nil, NewError(ErrFrameworkIncorrectUsage, "CosmosRemote: SyncMessagingByName without fromID.").AddStack(nil)
	}

	var er error
	var arg *anypb.Any
	if in != nil {
		arg, er = anypb.New(in)
		if er != nil {
			return nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "CosmosRemote: SyncMessagingByName arg error. err=(%v)", er).AddStack(nil)
		}
	}

	client, ctx, cancel, err := c.getCurrentClientWithTimeout(timeout)
	if err != nil {
		return nil, err.AddStack(nil)
	}
	defer cancel()

	callerIdInfo := callerID.GetIDInfo()
	toIDInfo := c.context.info

	rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
		CallerId: callerIdInfo,
		CallerContext: &IDContextInfo{
			IdChain: append(callerID.GetIDContext().FromCallChain(), callerID.GetIDInfo().Info()),
		},
		To:        toIDInfo,
		Timeout:   int64(timeout),
		NeedReply: true,
		Message:   name,
		Args:      arg,
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
	if callerID == nil {
		if callback != nil {
			callback(nil, NewError(ErrFrameworkIncorrectUsage, "CosmosRemote: AsyncMessagingByName without fromID.").AddStack(nil))
		}
		c.process.local.Log().Error("CosmosRemote: AsyncMessagingByName without fromID.")
		return
	}

	var er error
	var arg *anypb.Any
	if in != nil {
		arg, er = anypb.New(in)
		if er != nil {
			if callback != nil {
				callback(nil, NewErrorf(ErrCosmosRemoteRequestInvalid, "CosmosRemote: AsyncMessagingByName arg error. err=(%v)", er).AddStack(nil))
			}
			c.process.local.Log().Error("CosmosRemote: AsyncMessagingByName arg error. err=(%v)", er)
			return
		}
	}

	client, ctx, cancel, err := c.getCurrentClientWithTimeout(timeout)
	if err != nil {
		if callback != nil {
			callback(nil, err.AddStack(nil))
		}
		c.process.local.Log().Error("CosmosRemote: AsyncMessagingByName client error. err=(%v)", err)
		return
	}
	defer cancel()

	callerIdInfo := callerID.GetIDInfo()
	toIDInfo := c.context.info
	needReply := callback != nil

	c.process.local.Parallel(func() {
		out, err := func() (out proto.Message, err *Error) {

			rsp, er := client.SyncMessagingByName(ctx, &CosmosRemoteSyncMessagingByNameReq{
				CallerId: callerIdInfo,
				CallerContext: &IDContextInfo{
					IdChain: []string{},
				},
				To:        toIDInfo,
				Timeout:   int64(timeout),
				NeedReply: needReply,
				Message:   name,
				Args:      arg,
			})
			if er != nil {
				return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "CosmosRemote: SyncMessagingByName reply error. rsp=(%v),err=(%v)", rsp, er).AddStack(nil)
			}
			if needReply {
				if rsp.Reply != nil {
					out, er = rsp.Reply.UnmarshalNew()
					if er != nil {
						return nil, NewErrorf(ErrCosmosRemoteResponseInvalid, "CosmosRemote: SyncMessagingByName reply unmarshal error. err=(%v)", er).AddStack(nil)
					}
				}
				if rsp.Error != nil {
					err = rsp.Error.AddStack(nil)
				}
			}

			return out, err
		}()

		if needReply {
			callerID.getAtomos().PushAsyncMessageCallbackMailAndWaitReply(callerID, name, out, err, callback)
		}
	})
}

func (c *CosmosRemote) DecoderByName(_ string) (MessageDecoder, MessageDecoder) {
	return nil, nil
}

func (c *CosmosRemote) Kill(_ SelfID, _ time.Duration) *Error {
	return NewError(ErrCosmosRemoteCannotKill, "CosmosRemote: Cannot kill remote.").AddStack(nil)
}

func (c *CosmosRemote) SendWormhole(_ SelfID, _ time.Duration, _ AtomosWormhole) *Error {
	return NewError(ErrCosmosRemoteCannotSendWormhole, "CosmosRemote: Cannot send wormhole to remote.").AddStack(nil)
}

func (c *CosmosRemote) getGoID() uint64 {
	//return c.id.GoId
	return 0
}

// Implementation of CosmosNode

func (c *CosmosRemote) GetNodeName() string {
	return c.GetIDInfo().Node
}

func (c *CosmosRemote) CosmosIsLocal() bool {
	return false
}

func (c *CosmosRemote) CosmosGetElementID(elem string) (ID, *Error) {
	id, err := c.getElement(elem)
	if err != nil {
		return nil, err.AddStack(nil)
	}
	return id, nil
}

func (c *CosmosRemote) CosmosGetAtomID(elem, name string) (ID, *IDTracker, *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	id, tracker, err := element.GetAtomID(name, nil, false)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	return id, tracker, nil
}

func (c *CosmosRemote) CosmosGetScaleAtomID(callerID SelfID, elem, message string, timeout time.Duration, args proto.Message) (id ID, tracker *IDTracker, err *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	id, tracker, err = element.ScaleGetAtomID(callerID, message, timeout, args, nil, false)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	return id, tracker, nil
}

func (c *CosmosRemote) CosmosSpawnAtom(callerID SelfID, elem, name string, arg proto.Message) (ID, *IDTracker, *Error) {
	element, err := c.getElement(elem)
	if err != nil {
		return nil, nil, err
	}
	id, tracker, err := element.SpawnAtom(callerID, name, arg, nil, false)
	if err != nil {
		return nil, nil, err.AddStack(nil)
	}
	return id, tracker, nil
}

func (c *CosmosRemote) ElementBroadcast(callerID SelfID, key, contentType string, contentBuffer []byte) (err *Error) {
	client, ctx, cancel, err := c.getCurrentClientWithTimeout(0)
	if err != nil {
		return err.AddStack(nil)
	}
	defer cancel()
	rsp, er := client.ElementBroadcast(ctx, &CosmosRemoteElementBroadcastReq{
		CallerId:      callerID.GetIDInfo(),
		CallerContext: nil,
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
	//if c.avail {
	//	return true
	//}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), atomosGRPCDialTimeout)
	defer cancel()

	var er error
	if c.process.cluster.grpcDialOption == nil {
		c.client, er = grpc.DialContext(ctx, c.info.Address, grpc.WithInsecure(), grpc.WithBlock())
	} else {
		c.client, er = grpc.DialContext(ctx, c.info.Address, *c.process.cluster.grpcDialOption, grpc.WithBlock())
	}
	if er != nil {
		dialTime := atomosGRPCDialTimeout
		if atomosGRPCDialTimeout < time.Second {
			dialTime = time.Second
		}
		conn, connEr := net.DialTimeout("tcp", c.info.Address, dialTime)
		if conn != nil {
			conn.Close()
		}
		c.process.local.Log().coreFatal("CosmosRemote: Dial failed. addr=(%s),err=(%v),conn=(%v),connEr=(%v)", c.info.Address, er, conn, connEr)
		return false
	}
	c.avail = true
	c.process.local.Log().coreInfo("CosmosRemote: Dial. addr=(%s)", c.info.Address)
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
	callerIDContext *IDContextInfo
}

func (c *CosmosRemote) newFakeCosmosSelfID(callerID *IDInfo, callerIDContext *IDContextInfo) *remoteCosmosFakeSelfID {
	return &remoteCosmosFakeSelfID{
		CosmosRemote:    c,
		callerIDContext: callerIDContext,
	}
}

func (r *remoteCosmosFakeSelfID) callerCounterRelease() {}

func (r *remoteCosmosFakeSelfID) GetIDContext() IDContext {
	return r
}

func (r *remoteCosmosFakeSelfID) FromCallChain() []string {
	return r.callerIDContext.IdChain
}

func (r *remoteCosmosFakeSelfID) Log() Logging {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) Task() Task {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) CosmosMain() *CosmosLocal {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) KillSelf() {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) Parallel(_ func()) {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) Config() map[string][]byte {
	panic("not supported, should not be called")
}

func (r *remoteCosmosFakeSelfID) getAtomos() *BaseAtomos {
	panic("not supported, should not be called")
}
