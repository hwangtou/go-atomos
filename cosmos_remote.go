package atomos

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
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

	// watchers are DeathWatch callbacks fired when this remote node departs
	// (its version key is deleted from etcd). Guarded by mutex. Entries may be
	// tombstoned (set nil) by cancel; dispatch skips nil.
	watchers []DeathWatchCallback
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

// DeathWatchCallback is invoked (asynchronously, outside any framework lock)
// when the remote node's version key is deleted from etcd — i.e. the node
// left, crashed, or its lease expired. The callback receives the node name and
// the dead version's info (including StartupId, so the caller can distinguish a
// same-address restart from a true departure). Use it to drop cached remote IDs
// pointing at this node; the framework itself holds no such cache. The callback
// must be idempotent — a single departure may produce multiple delete events.
type DeathWatchCallback func(ev NodeDeathEvent)

// NodeDeathEvent describes a remote node departure delivered to DeathWatch
// callbacks.
type NodeDeathEvent struct {
	// Node is the departed remote node's name.
	Node string
	// Info is the CosmosNodeVersionInfo of the dead version (carries Address,
	// StartupId, State, Elements). Nil if unavailable.
	Info *CosmosNodeVersionInfo
}

// AddDeathWatch registers a callback fired when this remote node departs.
// Returns a cancel func to detach the watcher (mirrors IDTracker.Release style).
// Safe to call from any goroutine. Callbacks fire asynchronously outside
// framework locks, so they may call back into the framework without deadlocking.
func (c *CosmosRemote) AddDeathWatch(cb DeathWatchCallback) (cancel func()) {
	c.mutex.Lock()
	c.watchers = append(c.watchers, cb)
	idx := len(c.watchers) - 1
	c.mutex.Unlock()
	return func() {
		c.mutex.Lock()
		if idx < len(c.watchers) {
			c.watchers[idx] = nil // tombstone; dispatch skips nil
		}
		c.mutex.Unlock()
	}
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
	// Prefer a non-Draining version as the routing target. The lock.Current
	// points at the "designated" version, but during a drain that version is
	// Draining (stop accepting new atoms). In that case, fall back to any other
	// version that is Started, so new traffic goes to the upgraded node instead.
	// Existing calls are unaffected — they use pinned connections (see BaseRemote).
	currentKey := strconv.FormatInt(c.lock.Current, 10)
	currentVersion, has := c.version[currentKey]
	if has && currentVersion.getInfo().GetState() != ClusterNodeState_Draining {
		c.current = currentVersion
		c.enable = true
		return
	}
	// Current is Draining (or missing): look for any Started version.
	for key, v := range c.version {
		if v.getInfo().GetState() == ClusterNodeState_Started {
			c.current = v
			c.enable = true
			c.process.local.Log().coreInfo("CosmosRemote: refresh picked Started version=(%s) over Draining/Stopping current=(%s).", key, currentKey)
			return
		}
	}
	// No Started version available. If the current exists (even if Draining),
	// keep using it so calls don't hard-fail — the caller will get errors from
	// the draining node, which is better than no route at all.
	if has {
		c.current = currentVersion
		c.enable = true
		return
	}
	c.enable = false
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
	oldInfo := oldVersion.getInfo()
	if proto.Equal(info, oldInfo) {
		return
	}

	c.remote.info = info.Id
	if info.Address != oldInfo.Address || info.StartupId != oldInfo.StartupId {
		// The address changed, OR the same address now serves a NEW process
		// generation (the node restarted; startup_id differs). The old
		// connection and version state belong to the dead process — disable and
		// rebuild, otherwise gRPC would silently reconnect to the new process
		// while we keep state bound to the old generation.
		oldVersion.setDisable()
		c.process.local.Log().coreInfo("CosmosRemote: Connect info version replaced. version=(%s),addr=(%s=>%s),startup=(%d=>%d)",
			version, oldInfo.Address, info.Address, oldInfo.StartupId, info.StartupId)
		c.version[version] = newCosmosRemoteVersion(c.process, info, version)
	} else {
		// Same process generation: refresh the metadata in place so state
		// transitions (Started→Draining→Stopping) reach refresh(); otherwise
		// routing keeps using the stale state captured at creation time.
		oldVersion.setInfo(info)
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
	v, has := c.version[version]
	var deadInfo *CosmosNodeVersionInfo
	var snapshot []DeathWatchCallback
	if has {
		delete(c.version, version)
		deadInfo = v.info
		if c.current != nil && c.current.version == v.version {
			c.current = nil
			//for _, elem := range c.elements {
			//	elem.setDisable()
			//}
		}
		// Close the dead version's gRPC connection. Previously setDisable() was
		// only called on the PUT (address/startup-id change) path, which leaked
		// the conn on a plain delete (node leave / lease expire). Closing here
		// pairs conn teardown with version removal.
		v.setDisable()
		// Snapshot watchers under the lock so dispatch can run lock-free.
		if len(c.watchers) > 0 {
			snapshot = append(snapshot, c.watchers...)
		}
	}
	c.refresh()
	c.mutex.Unlock()

	// Dispatch DeathWatch callbacks asynchronously. This runs outside c.mutex
	// AND outside p.cluster.remoteMutex (held by the upstream caller
	// etcdDeleteClusterVersionNodeInfo), so callbacks may safely call back into
	// the framework. Async dispatch also bounds the etcd watcher goroutine's
	// exposure to slow/panicking user callbacks.
	if len(snapshot) > 0 && deadInfo != nil {
		ev := NodeDeathEvent{Node: c.GetNodeName(), Info: deadInfo}
		go c.dispatchDeathWatch(ev, snapshot)
	}
}

// dispatchDeathWatch invokes each (non-tombstoned) DeathWatch callback,
// recovering from panics so one bad callback cannot kill the dispatch.
func (c *CosmosRemote) dispatchDeathWatch(ev NodeDeathEvent, callbacks []DeathWatchCallback) {
	for _, cb := range callbacks {
		if cb == nil {
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					if c.process != nil && c.process.logging != nil {
						c.process.logging.pushFrameworkErrorLog(
							"DeathWatch: callback panicked. node=(%s) err=(%v)", ev.Node, r)
					}
				}
			}()
			cb(ev)
		}()
	}
}

func (c *CosmosRemote) getCurrentClient() *grpc.ClientConn {
	c.mutex.RLock()
	if !c.enable || c.current == nil {
		c.mutex.RUnlock()
		return nil
	}
	current := c.current
	c.mutex.RUnlock()
	// Read the connection through dialMu-guarded accessor: check() publishes
	// and setDisable() clears client under dialMu, so a direct field read here
	// would race with both.
	client := current.getClient()
	// Lazily dial on first use (or after a setDisable reset). check() guards
	// itself with dialMu so concurrent callers don't redial, and runs without
	// the CosmosRemote lock so a slow dial cannot stall other cross-node calls.
	if client == nil {
		current.check()
		c.mutex.RLock()
		if c.current == current {
			client = current.getClient()
		}
		c.mutex.RUnlock()
	}
	return client
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
	// dialMu + dialed guard the lazy dial so check() runs at most once per
	// "dial cycle". Unlike sync.Once, this can be reset by setDisable() to
	// allow redial after a connection is closed. The dial itself happens
	// *outside* the CosmosRemote mutex (which can be held by getCurrentClient
	// callers), preventing a slow (~1s) connect from blocking all
	// cross-node calls.
	dialMu  sync.Mutex
	dialed  bool
}

func newCosmosRemoteVersion(process *CosmosProcess, info *CosmosNodeVersionInfo, version string) *cosmosRemoteVersion {
	c := &cosmosRemoteVersion{
		process: process,
		info:    info,
		avail:   false,
		client:  nil,
	}
	// The gRPC dial is deferred to the first getCurrentClient call. Dialing here
	// would block while the caller holds the CosmosRemote write lock, stalling
	// every concurrent cross-node RPC for up to the dial timeout.
	return c
}

// getClient returns the current dialed connection (nil if not dialed or
// disabled). All reads/writes of client and avail go through dialMu so that
// check(), setDisable() and callers never race on these fields.
func (c *cosmosRemoteVersion) getClient() *grpc.ClientConn {
	c.dialMu.Lock()
	defer c.dialMu.Unlock()
	return c.client
}

// getInfo returns a snapshot of the version registration info. check() reads
// the address without holding the CosmosRemote mutex, while etcdUpdateVersion
// may replace the info in place (state refresh) — so info access is serialized
// through dialMu as well.
func (c *cosmosRemoteVersion) getInfo() *CosmosNodeVersionInfo {
	c.dialMu.Lock()
	defer c.dialMu.Unlock()
	return c.info
}

// setInfo replaces the registration info in place (same process generation,
// e.g. a state transition). Callers must hold the CosmosRemote write lock.
func (c *cosmosRemoteVersion) setInfo(info *CosmosNodeVersionInfo) {
	c.dialMu.Lock()
	defer c.dialMu.Unlock()
	c.info = info
}

func (c *cosmosRemoteVersion) check() bool {
	c.dialMu.Lock()
	if c.dialed {
		// Already dialed in this cycle (success or failure); return current state.
		avail := c.avail
		c.dialMu.Unlock()
		return avail
	}
	c.dialed = true
	c.dialMu.Unlock()

	// Snapshot the registration info: etcdUpdateVersion may replace c.info in
	// place while we dial without holding the CosmosRemote mutex.
	info := c.getInfo()

	var er error
	var client *grpc.ClientConn
	if c.process.cluster.grpcDialOption == nil {
		client, er = grpc.NewClient(info.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		client, er = grpc.NewClient(info.Address, *c.process.cluster.grpcDialOption)
	}
	if er != nil {
		c.process.local.Log().coreFatal("CosmosRemote: NewClient failed. addr=(%s),err=(%v)", info.Address, er)
		return false
	}
	// grpc.NewClient connects lazily and ignores grpc.WithBlock. Preserve the
	// old blocking-dial fail-fast semantics — calls to a down peer fail here
	// within ~1s instead of hanging until each RPC's own deadline — by kicking
	// off the connection and waiting for Ready (TLS handshake included).
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1) // TODO: timeout
	defer cancel()
	client.Connect()
	for {
		st := client.GetState()
		if st == connectivity.Ready {
			break
		}
		if !client.WaitForStateChange(ctx, st) {
			// Timed out waiting for readiness.
			client.Close()
			conn, connEr := net.DialTimeout("tcp", info.Address, time.Second*1)
			if conn != nil {
				conn.Close()
			}
			c.process.local.Log().coreFatal("CosmosRemote: Connect failed. addr=(%s),state=(%v),conn=(%v),connEr=(%v)", info.Address, st, conn, connEr)
			return false
		}
	}
	// Publish the connection under dialMu. If setDisable() ran while we were
	// dialing (it resets dialed=false), this version is already disabled: close
	// the freshly dialed connection instead of publishing it, otherwise the
	// connection would leak (nothing would ever Close it again).
	c.dialMu.Lock()
	if !c.dialed {
		c.dialMu.Unlock()
		client.Close()
		c.process.local.Log().coreInfo("CosmosRemote: Dial finished after disable, closing. addr=(%s)", info.Address)
		return false
	}
	c.client = client
	c.avail = true
	c.dialMu.Unlock()
	c.process.local.Log().coreInfo("CosmosRemote: Dial. addr=(%s)", info.Address)
	return true
}

func (c *cosmosRemoteVersion) setDisable() {
	c.dialMu.Lock()
	defer c.dialMu.Unlock()
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
	c.avail = false
	// Reset the dial guard so a subsequent getCurrentClient can redial.
	c.dialed = false
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
	cli := targetVersion.getClient()
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
