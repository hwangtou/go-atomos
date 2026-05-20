package atomos

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

type CosmosRemote struct {
	process *CosmosProcess
	remote  BaseRemote
	lock    *CosmosNodeVersionLock

	mutex   sync.RWMutex
	enable  bool
	current *cosmosRemoteVersion
	version map[string]*cosmosRemoteVersion

	elements map[string]*ElementRemoteFromSource
}

func newCosmosRemoteFromNodeInfo(process *CosmosProcess, info *CosmosNodeVersionInfo) *CosmosRemote {
	c := &CosmosRemote{
		process:  process,
		mutex:    sync.RWMutex{},
		enable:   false,
		current:  nil,
		version:  map[string]*cosmosRemoteVersion{},
		elements: map[string]*ElementRemoteFromSource{},
	}
	c.remote = newBaseRemote(c, info.Id)
	return c
}

func newCosmosRemoteFromLockInfo(process *CosmosProcess, lock *CosmosNodeVersionLock) *CosmosRemote {
	c := &CosmosRemote{
		process:  process,
		mutex:    sync.RWMutex{},
		enable:   false,
		current:  nil,
		version:  map[string]*cosmosRemoteVersion{},
		elements: map[string]*ElementRemoteFromSource{},
	}
	c.remote = newBaseRemote(c, nil)
	return c
}

type CosmosRemoteInTargetProcess struct {
	*CosmosRemote
	startupID uint64
	asyncID   uint64
}

func (c *CosmosRemoteInTargetProcess) asyncSet(callback func(out proto.Message, err *Error)) (startupID, callbackID uint64) {
	return c.startupID, c.asyncID
}

func newCosmosRemoteInTargetProcess(cosmos *CosmosRemote, startupID, asyncID uint64) *CosmosRemoteInTargetProcess {
	return &CosmosRemoteInTargetProcess{
		CosmosRemote: cosmos,
		startupID:    startupID,
		asyncID:      asyncID,
	}
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

	c.remote.info = info.Id
	c.version[version] = newCosmosRemoteVersion(c.process, info, version)
	if info.Elements != nil {
		for elemName, idInfo := range info.Elements {
			//e, has := c.process.local.runnable.interfaces[elemName] // It's ok, because the interface will never be changed.
			e, has := c.process.local.runnable.implements[elemName] // It's ok, because the interface will never be changed.
			if !has {
				c.process.local.Log().coreError("CosmosRemote: Connect info element not supported. name=(%s)", elemName)
			} else {
				c.elements[elemName] = newElementRemoteFromSource(c, idInfo, e.Interface, version)
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
		c.remote.info = info.Id
		c.version[version] = newCosmosRemoteVersion(c.process, info, version)
		if info.Elements != nil {
			for elemName, idInfo := range info.Elements {
				//e, has := c.process.local.runnable.interfaces[elemName] // It's ok, because the interface will never be changed.
				e, has := c.process.local.runnable.implements[elemName] // It's ok, because the interface will never be changed.
				if !has {
					c.process.local.Log().coreError("CosmosRemote: Connect info element not supported. name=(%s)", elemName)
				} else {
					c.elements[elemName] = newElementRemoteFromSource(c, idInfo, e.Interface, version)
				}
			}
		}
		c.refresh()
		return
	}
	if proto.Equal(info, oldVersion.info) {
		return
	}

	c.remote.info = info.Id
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
					c.elements[elemName] = newElementRemoteFromSource(c, newInfo, e.Interface, version)
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

func (c *CosmosRemote) getElement(name string) (*ElementRemoteFromSource, *Error) {
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
	return c.remote.info
}

func (c *CosmosRemote) String() string {
	return c.GetIDInfo().Info()
}

func (c *CosmosRemote) Cosmos() CosmosNode {
	return c
}

func (c *CosmosRemote) State() BaseAtomosState {
	return c.remote.State()
}

func (c *CosmosRemote) IdleTime() time.Duration {
	return c.remote.IdleTime()
}

func (c *CosmosRemote) SyncMessagingByName(callerID ID, name string, in proto.Message, ext []ArgsForBaseAtomos) (out proto.Message, err *Error) {
	return c.remote.PushSyncMessage(callerID, name, in, ext)
}

func (c *CosmosRemote) AsyncMessagingByName(callerID ID, name string, in proto.Message, callback func(out proto.Message, err *Error), ext []ArgsForBaseAtomos) (errBeforeExec *Error) {
	return c.remote.PushAsyncMessage(callerID, name, in, callback, ext)
}

func (c *CosmosRemote) asyncSet(callback func(out proto.Message, err *Error)) (startupID, callbackID uint64) {
	// BUG: asyncSet should never be called on a CosmosRemote.
	// It exists only to satisfy the ID interface. Returning zeros so callers
	// get a safe no-op rather than a process crash.
	return 0, 0
}

func (c *CosmosRemote) asyncCallback(callerID ID, name string, startupID, asyncID uint64, reply proto.Message, err *Error) {
	c.remote.PushAsyncMessageCallback(c, callerID, name, startupID, asyncID, reply, err)
}

func (c *CosmosRemote) DecoderByName(name string) (MessageDecoder, MessageDecoder) {
	return nil, nil
}

func (c *CosmosRemote) Kill(callerID ID, ext []ArgsForBaseAtomos) *Error {
	return NewError(ErrCosmosRemoteCannotKill, "CosmosRemote: Cannot kill remote.").AddStack(nil)
}

func (c *CosmosRemote) SendWormhole(callerID ID, wormhole BaseAtomosWormhole, ext []ArgsForBaseAtomos) *Error {
	return NewError(ErrCosmosRemoteCannotSendWormhole, "CosmosGlobal: Cannot send wormhole remote.").AddStack(nil)
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

func (c *CosmosRemote) CosmosGetElementID(elem string, args ...ArgsForBaseAtomos) (ID, *Error) {
	id, err := c.getElement(elem)
	if err != nil {
		return nil, err.AddStack(nil)
	}
	return id, nil
}

func (c *CosmosRemote) CosmosGetAtomID(elem, name string, args ...ArgsForBaseAtomos) (ID, *IDTracker, *Error) {
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

func (c *CosmosRemote) CosmosSpawnAtom(callerID SelfID, elem, name string, arg proto.Message, args ...ArgsForBaseAtomos) (ID, *IDTracker, *Error) {
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

func (c *CosmosRemote) ElementBroadcast(callerID ID, key, contentType string, contentBuffer []byte) (err *Error) {
	cli, ctx, cancel, err := c.remote.getCli(atomosClientTimeout)
	if err != nil {
		return err.AddStack(nil)
	}
	defer cancel()

	rsp, er := cli.ElementBroadcast(ctx, &CosmosRemoteElementBroadcastReq{
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
	//if c.avail {
	//	return true
	//}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1) // TODO: timeout
	defer cancel()

	var er error
	var client *grpc.ClientConn
	if c.process.cluster.grpcDialOption == nil {
		client, er = grpc.DialContext(ctx, c.info.Address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	} else {
		client, er = grpc.DialContext(ctx, c.info.Address, *c.process.cluster.grpcDialOption, grpc.WithBlock())
	}
	if er != nil {
		conn, connEr := net.DialTimeout("tcp", c.info.Address, time.Second*1)
		if conn != nil {
			conn.Close()
		}
		c.process.local.Log().coreFatal("CosmosRemote: Dial failed. addr=(%s),err=(%v),conn=(%v),connEr=(%v)", c.info.Address, er, conn, connEr)
		return false
	}
	c.client = client
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
