package atomostest

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ErrMemoryEtcdClosed is returned after a MemoryEtcd client is closed.
var ErrMemoryEtcdClosed = errors.New("atomostest: memory etcd is closed")

// ErrMemoryEtcdUnsupported is returned for operations outside the memory etcd contract.
var ErrMemoryEtcdUnsupported = errors.New("atomostest: unsupported memory etcd operation")

// EtcdClient is the narrow etcd seam shared by the memory implementation and
// production adapters. Put and transaction operations carry lease IDs
// explicitly because clientv3.Op does not expose its lease option.
type EtcdClient interface {
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Put(ctx context.Context, key, value string, lease clientv3.LeaseID) (*clientv3.PutResponse, error)
	Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error)
	Txn(ctx context.Context) EtcdTxn
	Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan
	Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error)
	KeepAlive(ctx context.Context, id clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error)
	Revoke(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error)
	Close() error
}

// EtcdTxn is the transaction subset implemented by MemoryEtcd.
type EtcdTxn interface {
	If(cmps ...clientv3.Cmp) EtcdTxn
	Then(ops ...EtcdOp) EtcdTxn
	Else(ops ...EtcdOp) EtcdTxn
	Commit() (*clientv3.TxnResponse, error)
}

// EtcdOp wraps a clientv3 operation with the lease information that clientv3
// keeps private. Lease must be zero for non-put operations.
type EtcdOp struct {
	Op    clientv3.Op
	Lease clientv3.LeaseID
}

// EtcdOpGet constructs a transaction Get operation.
func EtcdOpGet(key string, opts ...clientv3.OpOption) EtcdOp {
	return EtcdOp{Op: clientv3.OpGet(key, opts...)}
}

// EtcdOpPut constructs a transaction Put operation with an explicit lease.
func EtcdOpPut(key, value string, lease clientv3.LeaseID) EtcdOp {
	return EtcdOp{Op: clientv3.OpPut(key, value), Lease: lease}
}

// EtcdOpDelete constructs a transaction Delete operation.
func EtcdOpDelete(key string, opts ...clientv3.OpOption) EtcdOp {
	return EtcdOp{Op: clientv3.OpDelete(key, opts...)}
}

// MemoryEtcdOperation identifies an operation whose result can be fault-injected.
type MemoryEtcdOperation string

const (
	MemoryEtcdGet       MemoryEtcdOperation = "get"
	MemoryEtcdPut       MemoryEtcdOperation = "put"
	MemoryEtcdDelete    MemoryEtcdOperation = "delete"
	MemoryEtcdTxn       MemoryEtcdOperation = "txn"
	MemoryEtcdWatch     MemoryEtcdOperation = "watch"
	MemoryEtcdGrant     MemoryEtcdOperation = "grant"
	MemoryEtcdKeepAlive MemoryEtcdOperation = "keepalive"
	MemoryEtcdRevoke    MemoryEtcdOperation = "revoke"
)

// MemoryEtcd is a deterministic, revisioned in-memory etcd subset. Lease timers
// use the supplied VirtualClock; no wall-clock goroutine drives expiration.
type MemoryEtcd struct {
	mu sync.Mutex

	clock           *VirtualClock
	revision        int64
	compactRevision int64
	keys            map[string]*mvccpb.KeyValue
	snapshots       map[int64]map[string]*mvccpb.KeyValue
	history         []memoryEtcdBatch
	leases          map[clientv3.LeaseID]*memoryEtcdLease
	nextLeaseID     clientv3.LeaseID

	watchers      map[uint64]*memoryEtcdWatch
	nextWatcherID uint64
	faults        map[MemoryEtcdOperation]error
	nextFaults    map[MemoryEtcdOperation][]error
	closed        bool
}

type memoryEtcdBatch struct {
	revision int64
	events   []*clientv3.Event
}

type memoryEtcdLease struct {
	id         clientv3.LeaseID
	ttl        int64
	timer      memoryEtcdTimer
	keys       map[string]struct{}
	keepAlives map[*memoryEtcdKeepAlive]struct{}
}

type memoryEtcdKeepAlive struct {
	out      chan *clientv3.LeaseKeepAliveResponse
	finished chan struct{}
	timer    memoryEtcdTimer
	done     bool
}

type memoryEtcdTimer interface {
	Stop() bool
	Reset(time.Duration) bool
}

// NewMemoryEtcd creates an empty store at etcd's initial revision of 1.
func NewMemoryEtcd(clock *VirtualClock) *MemoryEtcd {
	if clock == nil {
		panic("atomostest: nil VirtualClock")
	}
	return &MemoryEtcd{
		clock:       clock,
		revision:    1,
		keys:        map[string]*mvccpb.KeyValue{},
		snapshots:   map[int64]map[string]*mvccpb.KeyValue{1: {}},
		leases:      map[clientv3.LeaseID]*memoryEtcdLease{},
		nextLeaseID: 1,
		watchers:    map[uint64]*memoryEtcdWatch{},
		faults:      map[MemoryEtcdOperation]error{},
		nextFaults:  map[MemoryEtcdOperation][]error{},
	}
}

// InjectError makes every subsequent operation of op fail with err. A nil err
// clears the persistent fault.
func (m *MemoryEtcd) InjectError(op MemoryEtcdOperation, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		delete(m.faults, op)
		return
	}
	m.faults[op] = err
}

// InjectNextError makes only the next operation of op fail with err. Multiple
// one-shot faults are consumed in injection order.
func (m *MemoryEtcd) InjectNextError(op MemoryEtcdOperation, err error) {
	if err == nil {
		panic("atomostest: nil injected error")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextFaults[op] = append(m.nextFaults[op], err)
}

// DisconnectWatches closes all active watch channels without a terminal
// response. A consumer can resume with WithRev(lastRevision+1).
func (m *MemoryEtcd) DisconnectWatches() {
	m.mu.Lock()
	watchers := m.allWatchersLocked()
	m.mu.Unlock()
	for _, watch := range watchers {
		m.stopWatch(watch, nil)
	}
}

// FailWatches terminates all active watches with a canceled watch response.
func (m *MemoryEtcd) FailWatches() {
	m.mu.Lock()
	watchers := m.allWatchersLocked()
	revision := m.revision
	m.mu.Unlock()
	for _, watch := range watchers {
		m.stopWatch(watch, &clientv3.WatchResponse{
			Header:   *memoryEtcdHeader(revision),
			Canceled: true,
		})
	}
}

// Compact discards event history through rev. Historical snapshots at rev are
// retained, while older snapshots are discarded.
func (m *MemoryEtcd) Compact(ctx context.Context, rev int64) (*clientv3.CompactResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.closedLocked(); err != nil {
		return nil, err
	}
	if rev > m.revision {
		return nil, rpctypes.ErrFutureRev
	}
	if rev <= m.compactRevision {
		return nil, rpctypes.ErrCompacted
	}
	m.compactRevision = rev
	for revision := range m.snapshots {
		if revision < rev {
			delete(m.snapshots, revision)
		}
	}
	kept := m.history[:0]
	for _, batch := range m.history {
		if batch.revision > rev {
			kept = append(kept, batch)
		}
	}
	m.history = kept
	return &clientv3.CompactResponse{Header: memoryEtcdHeader(m.revision)}, nil
}

// Revision returns the current global store revision.
func (m *MemoryEtcd) Revision() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.revision
}

// Get retrieves keys in ascending bytewise key order.
func (m *MemoryEtcd) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.operationErrorLocked(MemoryEtcdGet); err != nil {
		return nil, err
	}
	return m.getLocked(clientv3.OpGet(key, opts...))
}

// Put stores a key and optionally attaches it to lease.
func (m *MemoryEtcd) Put(ctx context.Context, key, value string, lease clientv3.LeaseID) (*clientv3.PutResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.operationErrorLocked(MemoryEtcdPut); err != nil {
		return nil, err
	}
	responses, _, err := m.commitLocked([]EtcdOp{EtcdOpPut(key, value, lease)})
	if err != nil {
		return nil, err
	}
	return (*clientv3.PutResponse)(responses[0].GetResponsePut()), nil
}

// Delete removes a key or range.
func (m *MemoryEtcd) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.operationErrorLocked(MemoryEtcdDelete); err != nil {
		return nil, err
	}
	responses, _, err := m.commitLocked([]EtcdOp{EtcdOpDelete(key, opts...)})
	if err != nil {
		return nil, err
	}
	return (*clientv3.DeleteResponse)(responses[0].GetResponseDeleteRange()), nil
}

// Txn starts an atomic transaction. All writes in the selected branch share one
// global revision.
func (m *MemoryEtcd) Txn(ctx context.Context) EtcdTxn {
	return &memoryEtcdTxn{ctx: ctx, client: m}
}

// Grant creates a lease whose expiration is driven by the shared VirtualClock.
func (m *MemoryEtcd) Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.operationErrorLocked(MemoryEtcdGrant); err != nil {
		return nil, err
	}
	if ttl <= 0 {
		return nil, errors.New("atomostest: lease TTL must be positive")
	}
	id := m.nextLeaseID
	m.nextLeaseID++
	lease := &memoryEtcdLease{
		id:         id,
		ttl:        ttl,
		keys:       map[string]struct{}{},
		keepAlives: map[*memoryEtcdKeepAlive]struct{}{},
	}
	m.leases[id] = lease
	lease.timer = m.clock.AfterFunc(time.Duration(ttl)*time.Second, func() {
		m.expireLease(id)
	})
	return &clientv3.LeaseGrantResponse{ResponseHeader: memoryEtcdHeader(m.revision), ID: id, TTL: ttl}, nil
}

// KeepAlive renews a lease immediately and then at one-third of its TTL. The
// returned channel is closed on cancellation, revocation, expiration, or Close.
func (m *MemoryEtcd) KeepAlive(ctx context.Context, id clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.operationErrorLocked(MemoryEtcdKeepAlive); err != nil {
		return nil, err
	}
	lease := m.leases[id]
	if lease == nil {
		closed := make(chan *clientv3.LeaseKeepAliveResponse)
		close(closed)
		return closed, nil
	}
	keepAlive := &memoryEtcdKeepAlive{
		out:      make(chan *clientv3.LeaseKeepAliveResponse, 16),
		finished: make(chan struct{}),
	}
	lease.keepAlives[keepAlive] = struct{}{}
	m.renewLeaseLocked(lease)
	m.sendKeepAliveLocked(keepAlive, lease)
	keepAlive.timer = m.clock.AfterFunc(memoryEtcdKeepAliveInterval(lease.ttl), func() {
		m.keepAliveTick(id, keepAlive)
	})
	if ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				m.mu.Lock()
				m.stopKeepAliveLocked(id, keepAlive)
				m.mu.Unlock()
			case <-keepAlive.finished:
			}
		}()
	}
	return keepAlive.out, nil
}

// Revoke removes a lease and all its attached keys in one revision.
func (m *MemoryEtcd) Revoke(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.operationErrorLocked(MemoryEtcdRevoke); err != nil {
		return nil, err
	}
	lease := m.leases[id]
	if lease == nil {
		return nil, rpctypes.ErrLeaseNotFound
	}
	m.removeLeaseLocked(lease)
	m.deleteLeaseKeysLocked(lease)
	return &clientv3.LeaseRevokeResponse{Header: memoryEtcdHeader(m.revision)}, nil
}

// Close cancels watches and keepalives and stops all lease timers.
func (m *MemoryEtcd) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	watchers := m.allWatchersLocked()
	leases := make([]*memoryEtcdLease, 0, len(m.leases))
	for _, lease := range m.leases {
		leases = append(leases, lease)
	}
	m.mu.Unlock()

	for _, watch := range watchers {
		watch.close(nil)
	}
	m.mu.Lock()
	for _, lease := range leases {
		m.removeLeaseLocked(lease)
	}
	m.mu.Unlock()
	return nil
}

// Watch replays retained events from WithRev (inclusive), then follows live
// events. WithRev(0) starts after the current revision.
func (m *MemoryEtcd) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	op := clientv3.OpGet(key, opts...)
	watch := newMemoryEtcdWatch(string(op.KeyBytes()), op.RangeBytes())
	m.mu.Lock()
	if err := m.operationErrorLocked(MemoryEtcdWatch); err != nil {
		watch.close(&clientv3.WatchResponse{Header: *memoryEtcdHeader(m.revision), Canceled: true})
		m.mu.Unlock()
		return watch.start(ctx, m)
	}
	start := op.Rev()
	if start == 0 {
		start = m.revision + 1
	}
	if start <= m.compactRevision {
		watch.close(&clientv3.WatchResponse{
			Header:          *memoryEtcdHeader(m.revision),
			CompactRevision: m.compactRevision,
			Canceled:        true,
		})
	} else {
		watch.startRevision = start
		m.nextWatcherID++
		watch.id = m.nextWatcherID
		m.watchers[watch.id] = watch
		for _, batch := range m.history {
			if batch.revision >= start {
				watch.enqueueBatch(batch)
			}
		}
	}
	m.mu.Unlock()
	return watch.start(ctx, m)
}

type memoryEtcdTxn struct {
	ctx    context.Context
	client *MemoryEtcd
	cmps   []clientv3.Cmp
	then   []EtcdOp
	els    []EtcdOp
}

func (t *memoryEtcdTxn) If(cmps ...clientv3.Cmp) EtcdTxn {
	t.cmps = append(t.cmps, cmps...)
	return t
}

func (t *memoryEtcdTxn) Then(ops ...EtcdOp) EtcdTxn {
	t.then = append(t.then, ops...)
	return t
}

func (t *memoryEtcdTxn) Else(ops ...EtcdOp) EtcdTxn {
	t.els = append(t.els, ops...)
	return t
}

func (t *memoryEtcdTxn) Commit() (*clientv3.TxnResponse, error) {
	if err := t.ctx.Err(); err != nil {
		return nil, err
	}
	m := t.client
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.operationErrorLocked(MemoryEtcdTxn); err != nil {
		return nil, err
	}
	succeeded, err := m.compareLocked(t.cmps)
	if err != nil {
		return nil, err
	}
	ops := t.els
	if succeeded {
		ops = t.then
	}
	responses, revision, err := m.commitLocked(ops)
	if err != nil {
		return nil, err
	}
	return &clientv3.TxnResponse{
		Header:    memoryEtcdHeader(revision),
		Succeeded: succeeded,
		Responses: responses,
	}, nil
}

func (m *MemoryEtcd) getLocked(op clientv3.Op) (*clientv3.GetResponse, error) {
	if len(op.KeyBytes()) == 0 {
		return nil, rpctypes.ErrEmptyKey
	}
	revision := op.Rev()
	keys := m.keys
	if revision > 0 {
		if revision > m.revision {
			return nil, rpctypes.ErrFutureRev
		}
		if revision < m.compactRevision {
			return nil, rpctypes.ErrCompacted
		}
		keys = m.snapshots[revision]
		if keys == nil {
			return nil, rpctypes.ErrCompacted
		}
	} else {
		revision = m.revision
	}
	matched := m.matchingKeys(keys, op.KeyBytes(), op.RangeBytes())
	filtered := matched[:0]
	for _, key := range matched {
		kv := keys[key]
		if op.MinModRev() != 0 && kv.ModRevision < op.MinModRev() ||
			op.MaxModRev() != 0 && kv.ModRevision > op.MaxModRev() ||
			op.MinCreateRev() != 0 && kv.CreateRevision < op.MinCreateRev() ||
			op.MaxCreateRev() != 0 && kv.CreateRevision > op.MaxCreateRev() {
			continue
		}
		filtered = append(filtered, key)
	}
	matched = filtered
	response := &clientv3.GetResponse{
		Header: memoryEtcdHeader(revision),
		Count:  int64(len(matched)),
	}
	if op.IsCountOnly() {
		return response, nil
	}
	limit := op.Limit()
	if limit > 0 && int64(len(matched)) > limit {
		response.More = true
		matched = matched[:limit]
	}
	response.Kvs = make([]*mvccpb.KeyValue, 0, len(matched))
	for _, key := range matched {
		kv := cloneMemoryEtcdKV(keys[key])
		if op.IsKeysOnly() {
			kv.Value = nil
		}
		response.Kvs = append(response.Kvs, kv)
	}
	return response, nil
}

func (m *MemoryEtcd) compareLocked(cmps []clientv3.Cmp) (bool, error) {
	for _, cmp := range cmps {
		keys := m.matchingKeys(m.keys, cmp.Key, cmp.RangeEnd)
		if len(keys) == 0 {
			if cmp.Target == pb.Compare_VALUE || !compareMemoryEtcdKV(nil, (*pb.Compare)(&cmp)) {
				return false, nil
			}
			continue
		}
		for _, key := range keys {
			kv := m.keys[key]
			if !compareMemoryEtcdKV(kv, (*pb.Compare)(&cmp)) {
				return false, nil
			}
		}
	}
	return true, nil
}

func compareMemoryEtcdKV(kv *mvccpb.KeyValue, cmp *pb.Compare) bool {
	var result int
	switch cmp.Target {
	case pb.Compare_VERSION:
		var version int64
		if kv != nil {
			version = kv.Version
		}
		result = compareInt64(version, cmp.GetVersion())
	case pb.Compare_CREATE:
		var create int64
		if kv != nil {
			create = kv.CreateRevision
		}
		result = compareInt64(create, cmp.GetCreateRevision())
	case pb.Compare_MOD:
		var mod int64
		if kv != nil {
			mod = kv.ModRevision
		}
		result = compareInt64(mod, cmp.GetModRevision())
	case pb.Compare_VALUE:
		var value []byte
		if kv != nil {
			value = kv.Value
		}
		result = bytes.Compare(value, cmp.GetValue())
	default:
		return false
	}
	switch cmp.Result {
	case pb.Compare_EQUAL:
		return result == 0
	case pb.Compare_NOT_EQUAL:
		return result != 0
	case pb.Compare_GREATER:
		return result > 0
	case pb.Compare_LESS:
		return result < 0
	default:
		return false
	}
}

func compareInt64(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func (m *MemoryEtcd) commitLocked(ops []EtcdOp) ([]*pb.ResponseOp, int64, error) {
	if err := m.validateOpsLocked(ops); err != nil {
		return nil, m.revision, err
	}
	revision := m.revision
	nextRevision := revision + 1
	events := make([]*clientv3.Event, 0)
	responses := make([]*pb.ResponseOp, 0, len(ops))
	for _, operation := range ops {
		op := operation.Op
		switch {
		case op.IsGet():
			response, err := m.getLocked(op)
			if err != nil {
				return nil, m.revision, err
			}
			responses = append(responses, &pb.ResponseOp{Response: &pb.ResponseOp_ResponseRange{
				ResponseRange: (*pb.RangeResponse)(response),
			}})
		case op.IsPut():
			event := m.putLocked(op, operation.Lease, nextRevision)
			events = append(events, event)
			responses = append(responses, &pb.ResponseOp{Response: &pb.ResponseOp_ResponsePut{
				ResponsePut: &pb.PutResponse{},
			}})
		case op.IsDelete():
			deleted, deleteEvents := m.deleteLocked(op.KeyBytes(), op.RangeBytes(), nextRevision)
			events = append(events, deleteEvents...)
			responses = append(responses, &pb.ResponseOp{Response: &pb.ResponseOp_ResponseDeleteRange{
				ResponseDeleteRange: &pb.DeleteRangeResponse{Deleted: deleted},
			}})
		default:
			return nil, m.revision, ErrMemoryEtcdUnsupported
		}
	}
	if len(events) > 0 {
		m.revision = nextRevision
		revision = nextRevision
		m.snapshots[revision] = cloneMemoryEtcdKeys(m.keys)
		m.publishLocked(memoryEtcdBatch{revision: revision, events: events})
	}
	for _, response := range responses {
		switch value := response.Response.(type) {
		case *pb.ResponseOp_ResponseRange:
			value.ResponseRange.Header = memoryEtcdHeader(revision)
		case *pb.ResponseOp_ResponsePut:
			value.ResponsePut.Header = memoryEtcdHeader(revision)
		case *pb.ResponseOp_ResponseDeleteRange:
			value.ResponseDeleteRange.Header = memoryEtcdHeader(revision)
		}
	}
	return responses, revision, nil
}

func (m *MemoryEtcd) validateOpsLocked(ops []EtcdOp) error {
	written := map[string]struct{}{}
	for _, operation := range ops {
		op := operation.Op
		if len(op.KeyBytes()) == 0 {
			return rpctypes.ErrEmptyKey
		}
		if !op.IsGet() && !op.IsPut() && !op.IsDelete() {
			return ErrMemoryEtcdUnsupported
		}
		if op.IsGet() {
			revision := op.Rev()
			if revision > m.revision {
				return rpctypes.ErrFutureRev
			}
			if revision > 0 && revision < m.compactRevision {
				return rpctypes.ErrCompacted
			}
			if revision > 0 && m.snapshots[revision] == nil {
				return rpctypes.ErrCompacted
			}
		}
		if op.IsPut() {
			if operation.Lease != 0 && m.leases[operation.Lease] == nil {
				return rpctypes.ErrLeaseNotFound
			}
			key := string(op.KeyBytes())
			if _, exists := written[key]; exists {
				return rpctypes.ErrDuplicateKey
			}
			written[key] = struct{}{}
		}
		if op.IsDelete() && op.RangeBytes() == nil {
			key := string(op.KeyBytes())
			if _, exists := written[key]; exists {
				return rpctypes.ErrDuplicateKey
			}
			written[key] = struct{}{}
		}
	}
	return nil
}

func (m *MemoryEtcd) putLocked(op clientv3.Op, leaseID clientv3.LeaseID, revision int64) *clientv3.Event {
	key := string(op.KeyBytes())
	previous := m.keys[key]
	if previous != nil {
		m.detachKeyLocked(previous)
	}
	kv := &mvccpb.KeyValue{
		Key:            append([]byte(nil), op.KeyBytes()...),
		Value:          append([]byte(nil), op.ValueBytes()...),
		CreateRevision: revision,
		ModRevision:    revision,
		Version:        1,
		Lease:          int64(leaseID),
	}
	if previous != nil {
		kv.CreateRevision = previous.CreateRevision
		kv.Version = previous.Version + 1
	}
	m.keys[key] = kv
	if leaseID != 0 {
		m.leases[leaseID].keys[key] = struct{}{}
	}
	return &clientv3.Event{Type: clientv3.EventTypePut, Kv: cloneMemoryEtcdKV(kv)}
}

func (m *MemoryEtcd) deleteLocked(key, end []byte, revision int64) (int64, []*clientv3.Event) {
	keys := m.matchingKeys(m.keys, key, end)
	events := make([]*clientv3.Event, 0, len(keys))
	for _, name := range keys {
		previous := m.keys[name]
		m.detachKeyLocked(previous)
		delete(m.keys, name)
		events = append(events, &clientv3.Event{
			Type: clientv3.EventTypeDelete,
			Kv: &mvccpb.KeyValue{
				Key:         append([]byte(nil), previous.Key...),
				ModRevision: revision,
			},
		})
	}
	return int64(len(events)), events
}

func (m *MemoryEtcd) deleteLeaseKeysLocked(lease *memoryEtcdLease) {
	if len(lease.keys) == 0 {
		return
	}
	keys := make([]string, 0, len(lease.keys))
	for key := range lease.keys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	revision := m.revision + 1
	events := make([]*clientv3.Event, 0, len(keys))
	for _, key := range keys {
		if m.keys[key] == nil {
			continue
		}
		_, deleted := m.deleteLocked([]byte(key), nil, revision)
		events = append(events, deleted...)
	}
	if len(events) == 0 {
		return
	}
	m.revision = revision
	m.snapshots[revision] = cloneMemoryEtcdKeys(m.keys)
	m.publishLocked(memoryEtcdBatch{revision: revision, events: events})
}

func (m *MemoryEtcd) publishLocked(batch memoryEtcdBatch) {
	m.history = append(m.history, batch)
	for _, watch := range m.watchers {
		watch.enqueueBatch(batch)
	}
}

func (m *MemoryEtcd) matchingKeys(keys map[string]*mvccpb.KeyValue, key, end []byte) []string {
	matched := make([]string, 0)
	for candidate := range keys {
		if memoryEtcdInRange([]byte(candidate), key, end) {
			matched = append(matched, candidate)
		}
	}
	sort.Strings(matched)
	return matched
}

func memoryEtcdInRange(candidate, key, end []byte) bool {
	if end == nil {
		return bytes.Equal(candidate, key)
	}
	if len(end) == 0 {
		return false
	}
	if len(end) == 1 && end[0] == 0 {
		return bytes.Compare(candidate, key) >= 0
	}
	return bytes.Compare(candidate, key) >= 0 && bytes.Compare(candidate, end) < 0
}

func (m *MemoryEtcd) detachKeyLocked(kv *mvccpb.KeyValue) {
	if kv.Lease == 0 {
		return
	}
	if lease := m.leases[clientv3.LeaseID(kv.Lease)]; lease != nil {
		delete(lease.keys, string(kv.Key))
	}
}

func (m *MemoryEtcd) expireLease(id clientv3.LeaseID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	lease := m.leases[id]
	if lease == nil {
		return
	}
	m.removeLeaseLocked(lease)
	m.deleteLeaseKeysLocked(lease)
}

func (m *MemoryEtcd) removeLeaseLocked(lease *memoryEtcdLease) {
	if m.leases[lease.id] == nil {
		return
	}
	delete(m.leases, lease.id)
	if lease.timer != nil {
		lease.timer.Stop()
	}
	for keepAlive := range lease.keepAlives {
		m.stopKeepAliveLocked(lease.id, keepAlive)
	}
}

func (m *MemoryEtcd) keepAliveTick(id clientv3.LeaseID, keepAlive *memoryEtcdKeepAlive) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lease := m.leases[id]
	if m.closed || lease == nil || keepAlive.done {
		return
	}
	m.renewLeaseLocked(lease)
	m.sendKeepAliveLocked(keepAlive, lease)
	keepAlive.timer.Reset(memoryEtcdKeepAliveInterval(lease.ttl))
}

func (m *MemoryEtcd) renewLeaseLocked(lease *memoryEtcdLease) {
	lease.timer.Reset(time.Duration(lease.ttl) * time.Second)
}

func (m *MemoryEtcd) sendKeepAliveLocked(keepAlive *memoryEtcdKeepAlive, lease *memoryEtcdLease) {
	select {
	case keepAlive.out <- &clientv3.LeaseKeepAliveResponse{
		ResponseHeader: memoryEtcdHeader(m.revision),
		ID:             lease.id,
		TTL:            lease.ttl,
	}:
	default:
	}
}

func memoryEtcdKeepAliveInterval(ttl int64) time.Duration {
	interval := time.Duration(ttl) * time.Second / 3
	if interval <= 0 {
		return time.Nanosecond
	}
	return interval
}

func (m *MemoryEtcd) stopKeepAliveLocked(id clientv3.LeaseID, keepAlive *memoryEtcdKeepAlive) {
	if keepAlive.done {
		return
	}
	keepAlive.done = true
	if keepAlive.timer != nil {
		keepAlive.timer.Stop()
	}
	if lease := m.leases[id]; lease != nil {
		delete(lease.keepAlives, keepAlive)
	}
	close(keepAlive.out)
	close(keepAlive.finished)
}

func (m *MemoryEtcd) operationErrorLocked(op MemoryEtcdOperation) error {
	if err := m.closedLocked(); err != nil {
		return err
	}
	if faults := m.nextFaults[op]; len(faults) > 0 {
		err := faults[0]
		m.nextFaults[op] = faults[1:]
		return err
	}
	return m.faults[op]
}

func (m *MemoryEtcd) closedLocked() error {
	if m.closed {
		return ErrMemoryEtcdClosed
	}
	return nil
}

func (m *MemoryEtcd) allWatchersLocked() []*memoryEtcdWatch {
	watchers := make([]*memoryEtcdWatch, 0, len(m.watchers))
	for _, watch := range m.watchers {
		watchers = append(watchers, watch)
	}
	return watchers
}

func (m *MemoryEtcd) stopWatch(watch *memoryEtcdWatch, response *clientv3.WatchResponse) {
	m.mu.Lock()
	delete(m.watchers, watch.id)
	m.mu.Unlock()
	watch.close(response)
}

type memoryEtcdWatch struct {
	id            uint64
	startRevision int64
	key           string
	end           []byte
	out           chan clientv3.WatchResponse
	notify        chan struct{}
	done          chan struct{}
	finished      chan struct{}

	mu      sync.Mutex
	queue   []clientv3.WatchResponse
	closed  bool
	aborted bool
}

func newMemoryEtcdWatch(key string, end []byte) *memoryEtcdWatch {
	var clonedEnd []byte
	if end != nil {
		clonedEnd = append([]byte{}, end...)
	}
	return &memoryEtcdWatch{
		key:      key,
		end:      clonedEnd,
		out:      make(chan clientv3.WatchResponse),
		notify:   make(chan struct{}, 1),
		done:     make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (w *memoryEtcdWatch) start(ctx context.Context, client *MemoryEtcd) clientv3.WatchChan {
	go w.run()
	if ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				client.stopWatch(w, nil)
			case <-w.finished:
			}
		}()
	}
	return w.out
}

func (w *memoryEtcdWatch) run() {
	defer close(w.finished)
	defer close(w.out)
	for {
		w.mu.Lock()
		if len(w.queue) > 0 {
			response := w.queue[0]
			w.queue = w.queue[1:]
			w.mu.Unlock()
			select {
			case w.out <- response:
			case <-w.done:
				return
			}
			continue
		}
		closed := w.closed
		w.mu.Unlock()
		if closed {
			return
		}
		select {
		case <-w.notify:
		case <-w.done:
			return
		}
	}
}

func (w *memoryEtcdWatch) close(response *clientv3.WatchResponse) {
	w.mu.Lock()
	if response == nil {
		if w.aborted {
			w.mu.Unlock()
			return
		}
		w.aborted = true
		w.closed = true
		w.queue = nil
		close(w.done)
		w.mu.Unlock()
		w.signal()
		return
	}
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.queue = append(w.queue, *response)
	w.mu.Unlock()
	w.signal()
}

func (w *memoryEtcdWatch) enqueueBatch(batch memoryEtcdBatch) {
	if batch.revision < w.startRevision {
		return
	}
	response := clientv3.WatchResponse{Header: *memoryEtcdHeader(batch.revision)}
	for _, event := range batch.events {
		if event.Kv != nil && memoryEtcdInRange(event.Kv.Key, []byte(w.key), w.end) {
			response.Events = append(response.Events, cloneMemoryEtcdEvent(event))
		}
	}
	if len(response.Events) == 0 {
		return
	}
	w.mu.Lock()
	if !w.closed {
		w.queue = append(w.queue, response)
	}
	w.mu.Unlock()
	w.signal()
}

func (w *memoryEtcdWatch) signal() {
	select {
	case w.notify <- struct{}{}:
	default:
	}
}

func memoryEtcdHeader(revision int64) *pb.ResponseHeader {
	return &pb.ResponseHeader{Revision: revision}
}

func cloneMemoryEtcdKV(kv *mvccpb.KeyValue) *mvccpb.KeyValue {
	if kv == nil {
		return nil
	}
	clone := *kv
	clone.Key = append([]byte(nil), kv.Key...)
	clone.Value = append([]byte(nil), kv.Value...)
	return &clone
}

func cloneMemoryEtcdEvent(event *clientv3.Event) *clientv3.Event {
	return &clientv3.Event{
		Type:   event.Type,
		Kv:     cloneMemoryEtcdKV(event.Kv),
		PrevKv: cloneMemoryEtcdKV(event.PrevKv),
	}
}

func cloneMemoryEtcdKeys(keys map[string]*mvccpb.KeyValue) map[string]*mvccpb.KeyValue {
	clone := make(map[string]*mvccpb.KeyValue, len(keys))
	for key, kv := range keys {
		clone[key] = cloneMemoryEtcdKV(kv)
	}
	return clone
}
