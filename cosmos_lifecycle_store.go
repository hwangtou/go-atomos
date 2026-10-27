package atomos

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	lifecycleOperationWait = 30 * time.Second
	lifecyclePollInterval  = 10 * time.Millisecond
	lifecycleResultTTL     = time.Hour
)

type lifecycleOperationLock struct {
	mu   sync.Mutex
	refs int
}

type lifecycleManager struct {
	process       *CosmosProcess
	cli           EtcdClient
	leaseID       atomic.Int64
	healthy       atomic.Bool
	watchHealthy  atomic.Bool
	ownerMu       sync.RWMutex
	owners        map[string]lifecycleOwnerSnapshot
	shardMu       sync.RWMutex
	shards        map[string]lifecycleShardSnapshot
	watchCancel   context.CancelFunc
	watchDone     chan struct{}
	stop          chan struct{}
	stopped       chan struct{}
	operationMu   sync.Mutex
	operations    map[string]*lifecycleOperationLock
	janitorActive atomic.Bool
	closeOnce     sync.Once
	controlReads  atomic.Int64
}

func newLifecycleManager(p *CosmosProcess) *lifecycleManager {
	return &lifecycleManager{process: p, owners: map[string]lifecycleOwnerSnapshot{}, shards: map[string]lifecycleShardSnapshot{}, stop: make(chan struct{}), stopped: make(chan struct{})}
}

func (m *lifecycleManager) lockOperation(operationID string) func() {
	m.operationMu.Lock()
	if m.operations == nil {
		m.operations = map[string]*lifecycleOperationLock{}
	}
	lock := m.operations[operationID]
	if lock == nil {
		lock = &lifecycleOperationLock{}
		m.operations[operationID] = lock
	}
	lock.refs++
	m.operationMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		m.operationMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(m.operations, operationID)
		}
		m.operationMu.Unlock()
	}
}

func randomNonZeroUint64() (uint64, error) {
	var buf [8]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			return 0, err
		}
		if v := binary.BigEndian.Uint64(buf[:]); v != 0 {
			return v, nil
		}
	}
}

var processStartupFallback atomic.Uint64

func newProcessStartupID() uint64 {
	if id, err := randomNonZeroUint64(); err == nil {
		return id
	}
	id := uint64(time.Now().UnixNano()) ^ processStartupFallback.Add(1)
	if id == 0 {
		return 1
	}
	return id
}

func (m *lifecycleManager) publishLease(id clientv3.LeaseID, healthy bool) {
	if m == nil {
		return
	}
	m.leaseID.Store(int64(id))
	m.healthy.Store(healthy && id != 0)
}

func (m *lifecycleManager) currentLease() (clientv3.LeaseID, bool) {
	if m == nil {
		return 0, false
	}
	return clientv3.LeaseID(m.leaseID.Load()), m.healthy.Load()
}

func (m *lifecycleManager) startJanitor() {
	if m == nil || !m.janitorActive.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer close(m.stopped)
		ticker := m.process.getClock().NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C():
				m.deleteExpiredOperations(m.process.getClock().Now())
			case <-m.stop:
				return
			}
		}
	}()
}

func (m *lifecycleManager) close() {
	if m == nil {
		return
	}
	m.healthy.Store(false)
	m.watchHealthy.Store(false)
	m.closeOnce.Do(func() {
		close(m.stop)
		if m.watchCancel != nil {
			m.watchCancel()
		}
	})
	if m.janitorActive.Load() {
		<-m.stopped
	}
	if m.watchDone != nil {
		<-m.watchDone
	}
}

func lifecycleEscape(s string) string { return url.PathEscape(s) }

func (m *lifecycleManager) atomPrefix(element, atom string) string {
	return "/cosmos/" + lifecycleEscape(m.process.local.runnable.config.Cosmos) + "/lifecycle/atom/" + lifecycleEscape(element) + "/" + lifecycleEscape(atom)
}

func (m *lifecycleManager) recordKey(element, atom string) string {
	return m.atomPrefix(element, atom) + "/record"
}
func (m *lifecycleManager) ownerKey(element, atom string) string {
	return m.atomPrefix(element, atom) + "/owner"
}
func (m *lifecycleManager) operationPrefix() string {
	return "/cosmos/" + lifecycleEscape(m.process.local.runnable.config.Cosmos) + "/lifecycle/operation/"
}
func (m *lifecycleManager) operationKey(operationID string) string {
	return m.operationPrefix() + lifecycleEscape(operationID)
}

func lifecycleGet(cli EtcdClient, key string, out proto.Message) (int64, bool, *Error) {
	ctx, cancel := etcdContext()
	defer cancel()
	rsp, err := cli.Get(ctx, key)
	if err != nil {
		return 0, false, NewErrorf(ErrCodeCosmosEtcdGetFailed, "lifecycle: get %s: %v", key, err)
	}
	if len(rsp.Kvs) == 0 {
		return 0, false, nil
	}
	if err := proto.Unmarshal(rsp.Kvs[0].Value, out); err != nil {
		return 0, false, NewErrorf(ErrCodeCosmosEtcdGetFailed, "lifecycle: decode %s: %v", key, err)
	}
	return rsp.Kvs[0].ModRevision, true, nil
}

func lifecycleMessageBytes(msg proto.Message) []byte {
	buf, _ := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	return buf
}

func lifecycleBytes(msg proto.Message) (string, *Error) {
	buf, err := proto.Marshal(msg)
	if err != nil {
		return "", NewErrorf(ErrCodeFrameworkInternalError, "lifecycle: encode %T: %v", msg, err)
	}
	return string(buf), nil
}

func (m *lifecycleManager) loadRecord(element, atom string) (*AtomLifecycleRecord, int64, *Error) {
	record := &AtomLifecycleRecord{}
	mod, ok, err := lifecycleGet(m.process.cluster.etcdClient, m.recordKey(element, atom), record)
	if err != nil || !ok {
		return nil, mod, err
	}
	return record, mod, nil
}

func (m *lifecycleManager) loadOwner(element, atom string) (*AtomOwnershipRecord, int64, *Error) {
	m.controlReads.Add(1)
	owner := &AtomOwnershipRecord{}
	mod, ok, err := lifecycleGet(m.process.cluster.etcdClient, m.ownerKey(element, atom), owner)
	if err != nil || !ok {
		return nil, mod, err
	}
	return owner, mod, nil
}

func (m *lifecycleManager) loadOperation(operationID string) (*AtomOperationRecord, int64, *Error) {
	op := &AtomOperationRecord{}
	mod, ok, err := lifecycleGet(m.process.cluster.etcdClient, m.operationKey(operationID), op)
	if err != nil {
		return nil, 0, err
	}
	if !ok {
		return nil, 0, NewErrorf(ErrCodeOperationNotFound, "lifecycle: operation %q not found", operationID)
	}
	return op, mod, nil
}

func (m *lifecycleManager) lookup(element, atom string) (*CosmosRemoteLookupAtomRsp, *Error) {
	owner, _, err := m.loadOwner(element, atom)
	if err != nil {
		return nil, err
	}
	rsp := &CosmosRemoteLookupAtomRsp{State: AtomLifecycleState_AtomLifecycleStateAbsent}
	if owner == nil || owner.Ownership == nil {
		return rsp, nil
	}
	rsp.State = owner.Ownership.State
	rsp.Id = proto.Clone(owner.Ownership.Id).(*IDInfo)
	rsp.Ownership = proto.Clone(owner.Ownership).(*AtomOwnershipInfo)
	if owner.OperationId != "" {
		if op, _, opErr := m.loadOperation(owner.OperationId); opErr == nil {
			rsp.Operation = op.Operation
		}
	}
	return rsp, nil
}

func lifecycleSameIdentity(id *IDInfo, ownership *AtomOwnershipInfo) bool {
	return id != nil && ownership != nil && ownership.Id != nil &&
		id.Element == ownership.Id.Element && id.Atom == ownership.Id.Atom &&
		id.Incarnation != 0 && id.Incarnation == ownership.Incarnation &&
		id.StartupId != 0 && id.StartupId == ownership.StartupId &&
		id.Fence != 0 && id.Fence == ownership.Fence
}

func (m *lifecycleManager) authorize(id *IDInfo) *Error {
	if m == nil || !lifecycleStrictMode(m.process.lifecycleMode) || id == nil || id.Type != IDType_Atom {
		return nil
	}
	if id.Incarnation == 0 || id.StartupId == 0 || id.Fence == 0 {
		return NewErrorf(ErrCodeFencingRequired, "lifecycle: strict request lacks incarnation/startup/fence for %s", id.Info())
	}
	leaseID, healthy := m.currentLease()
	if !healthy {
		return NewErrorf(ErrCodeOwnershipLost, "lifecycle: process lease is unhealthy for %s", id.Info())
	}
	if !m.watchHealthy.Load() {
		return NewErrorf(ErrCodeOwnershipLost, "lifecycle: owner snapshot is not synchronized for %s", id.Info())
	}
	owner := m.cachedOwner(id.Element, id.Atom)
	if owner == nil || !lifecycleSameIdentity(id, owner.Ownership) {
		return NewErrorf(ErrCodeStaleFence, "lifecycle: stale identity for %s", id.Info())
	}
	if owner.LeaseId != int64(leaseID) || owner.Ownership.Id.Node != m.process.local.GetNodeName() {
		return NewErrorf(ErrCodeOwnershipLost, "lifecycle: ownership belongs to another lease for %s", id.Info())
	}
	if owner.Ownership.State != AtomLifecycleState_AtomLifecycleStateRunning {
		return NewErrorf(ErrCodeOperationStillRunning, "lifecycle: atom %s is %s", id.Info(), owner.Ownership.State)
	}
	if m.perShard() {
		if id.ShardId != m.shardID(id.Element, id.Atom) {
			return NewErrorf(ErrCodeFencingRequired, "lifecycle: atom %s has incorrect shard assignment", id.Info())
		}
		shard := m.cachedShard(id.ShardId)
		if shard == nil || shard.Fence != id.Fence || shard.LeaseId != int64(leaseID) || shard.Node != m.process.local.GetNodeName() || shard.StartupId != id.StartupId {
			return NewErrorf(ErrCodeOwnershipLost, "lifecycle: shard %d ownership is stale for %s", id.ShardId, id.Info())
		}
	}
	return nil
}

func lifecycleFingerprint(kind AtomOperationKind, element, atom string, args []byte, previous *IDInfo) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00", kind, element, atom)))
	h.Write(args)
	if previous != nil {
		h.Write([]byte(fmt.Sprintf("\x00%d\x00%d\x00%d", previous.Incarnation, previous.StartupId, previous.Fence)))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (m *lifecycleManager) existingOperation(operationID, fingerprint string, kind AtomOperationKind) (*AtomControlResult, bool, *Error) {
	if operationID == "" {
		return nil, false, nil
	}
	op, _, err := m.loadOperation(operationID)
	if err != nil {
		if CodeOf(err) == ErrCodeOperationNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	if op.Operation.Fingerprint != fingerprint || op.Operation.Kind != kind {
		return nil, false, NewErrorf(ErrCodeOperationConflict, "lifecycle: operation %q fingerprint/kind conflict", operationID)
	}
	if op.Result != nil {
		return proto.Clone(op.Result).(*AtomControlResult), true, nil
	}
	return &AtomControlResult{OperationId: operationID, Operation: op.Operation}, true, nil
}

func lifecycleRetention(now time.Time, accepted bool, retain bool) int64 {
	if accepted {
		return 0
	}
	ttl := lifecycleResultTTL
	if retain {
		ttl *= 24
	}
	return now.Add(ttl).UnixNano()
}

func newOperationInfo(now time.Time, id string, kind AtomOperationKind, fingerprint string, atomID *IDInfo) *AtomOperationInfo {
	return &AtomOperationInfo{
		OperationId: id, Kind: kind, State: AtomOperationState_AtomOperationStateRunning,
		Id: atomID, Fingerprint: fingerprint, StartTime: timestamppb.New(now),
	}
}

func (m *lifecycleManager) waitOperation(ctx context.Context, operationID, fingerprint string, kind AtomOperationKind) (*AtomControlResult, *Error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	timeout := m.process.getClock().AfterFunc(lifecycleOperationWait, cancel)
	defer timeout.Stop()
	for {
		op, _, err := m.loadOperation(operationID)
		if err != nil {
			return nil, err
		}
		if op.Operation.Fingerprint != fingerprint || op.Operation.Kind != kind {
			return nil, NewErrorf(ErrCodeOperationConflict, "lifecycle: operation %q conflict", operationID)
		}
		if op.Operation.State != AtomOperationState_AtomOperationStateRunning {
			return proto.Clone(op.Result).(*AtomControlResult), nil
		}
		timer := m.process.getClock().NewTimer(lifecyclePollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return &AtomControlResult{OperationId: operationID, Operation: op.Operation, Error: NewError(ErrCodeOperationStillRunning, "lifecycle: operation still running")}, nil
		case <-timer.C():
		}
	}
}

func (m *lifecycleManager) deleteExpiredOperations(now time.Time) {
	cli := m.process.cluster.etcdClient
	if cli == nil {
		return
	}
	ctx, cancel := etcdContext()
	rsp, err := cli.Get(ctx, m.operationPrefix(), clientv3.WithPrefix())
	cancel()
	if err != nil {
		return
	}
	for _, kv := range rsp.Kvs {
		op := &AtomOperationRecord{}
		if proto.Unmarshal(kv.Value, op) != nil || op.Operation == nil || op.Operation.State == AtomOperationState_AtomOperationStateRunning {
			continue
		}
		retain := op.Operation.RetainUntilUnixNano
		if retain == 0 || now.UnixNano() < retain {
			continue
		}
		ctx, cancel := etcdContext()
		_, _ = cli.Txn(ctx).If(clientv3.Compare(clientv3.ModRevision(string(kv.Key)), "=", kv.ModRevision)).Then(EtcdOpDelete(string(kv.Key))).Commit()
		cancel()
	}
}

func lifecycleError(operationID string, code int64, format string, args ...any) *Error {
	err := NewErrorf(code, format, args...)
	err.OperationId = operationID
	return err
}

func (m *lifecycleManager) requireUsable() *Error {
	if m == nil || !lifecycleStrictMode(m.process.lifecycleMode) || m.process.cluster.etcdClient == nil {
		return NewError(ErrCodeFencingRequired, "lifecycle: per-atom strict requires an etcd cluster")
	}
	return nil
}
