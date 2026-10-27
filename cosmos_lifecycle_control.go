package atomos

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type lifecycleSpawnClaim struct {
	operation *AtomOperationRecord
	owner     *AtomOwnershipRecord
	result    *AtomControlResult
	shard     *ShardOwnershipRecord
	retain    bool
}

func (m *lifecycleManager) claimSpawn(ctx context.Context, operationID, fingerprint string, kind AtomOperationKind, element, atom string, previous *IDInfo, takeover bool) (*lifecycleSpawnClaim, *Error) {
	if result, exists, err := m.existingOperation(operationID, fingerprint, kind); err != nil {
		return nil, err
	} else if exists {
		if result.Operation != nil && result.Operation.State == AtomOperationState_AtomOperationStateRunning {
			result, err = m.waitOperation(ctx, operationID, fingerprint, kind)
			if err != nil {
				return nil, err
			}
		}
		return &lifecycleSpawnClaim{result: result}, nil
	}
	leaseID, healthy := m.currentLease()
	if !healthy {
		return nil, lifecycleError(operationID, ErrCodeOwnershipLost, "lifecycle: cannot claim without a healthy process lease")
	}
	shard, err := m.ensureShardOwnership(ctx, element, atom, takeover)
	if err != nil {
		return nil, lifecycleError(operationID, CodeOf(err), "lifecycle: shard ownership: %s", err.Message)
	}

	for attempt := 0; attempt < 32; attempt++ {
		record, recordMod, err := m.loadRecord(element, atom)
		if err != nil {
			return nil, err
		}
		owner, ownerMod, err := m.loadOwner(element, atom)
		if err != nil {
			return nil, err
		}
		if !takeover && owner != nil && owner.Ownership != nil && owner.Ownership.State != AtomLifecycleState_AtomLifecycleStateHalted {
			code := int64(ErrCodeOperationStillRunning)
			if owner.Ownership.State == AtomLifecycleState_AtomLifecycleStateRunning {
				code = ErrCodeAtomSpawningAnExistedAtom
			}
			return nil, lifecycleError(operationID, code, "lifecycle: atom %s::%s is %s", element, atom, owner.Ownership.State)
		}
		if takeover {
			if previous == nil || owner == nil || !lifecycleSameIdentity(previous, owner.Ownership) {
				return nil, lifecycleError(operationID, ErrCodeStaleFence, "lifecycle: takeover previous_id does not match current owner")
			}
		}
		fence := uint64(1)
		if shard != nil {
			fence = shard.Fence
		} else if record != nil && record.NextFence > 0 {
			fence = record.NextFence
		}
		incarnation, er := randomNonZeroUint64()
		if er != nil {
			return nil, lifecycleError(operationID, ErrCodeFrameworkInternalError, "lifecycle: random incarnation: %v", er)
		}
		atomID := &IDInfo{
			Type: IDType_Atom, Cosmos: m.process.local.runnable.config.Cosmos, Node: m.process.local.GetNodeName(),
			Element: element, Atom: atom, Incarnation: incarnation, StartupId: m.process.startupID, Fence: fence,
		}
		if shard != nil {
			atomID.ShardId = shard.ShardId
		}
		op := newOperationInfo(m.process.getClock().Now(), operationID, kind, fingerprint, atomID)
		ownership := &AtomOwnershipInfo{Id: atomID, Incarnation: incarnation, StartupId: m.process.startupID, Fence: fence, State: AtomLifecycleState_AtomLifecycleStateSpawning}
		owner = &AtomOwnershipRecord{Ownership: ownership, LeaseId: int64(leaseID), OperationId: operationID}
		if record == nil {
			record = &AtomLifecycleRecord{Element: element, Atom: atom}
		}
		if shard == nil {
			record.NextFence = fence + 1
		}
		operation := &AtomOperationRecord{Operation: op, Result: &AtomControlResult{OperationId: operationID, Id: atomID, Ownership: ownership, Operation: op}}
		recordBuf, err := lifecycleBytes(record)
		if err != nil {
			return nil, err
		}
		ownerBuf, err := lifecycleBytes(owner)
		if err != nil {
			return nil, err
		}
		opBuf, err := lifecycleBytes(operation)
		if err != nil {
			return nil, err
		}

		recordKey, ownerKey, opKey := m.recordKey(element, atom), m.ownerKey(element, atom), m.operationKey(operationID)
		txn := m.process.cluster.etcdClient.Txn(ctx).If(
			clientv3.Compare(clientv3.ModRevision(recordKey), "=", recordMod),
			clientv3.Compare(clientv3.ModRevision(ownerKey), "=", ownerMod),
			clientv3.Compare(clientv3.Version(opKey), "=", 0),
		).Then(
			EtcdOpPut(recordKey, recordBuf, 0),
			EtcdOpPut(ownerKey, ownerBuf, leaseID),
			EtcdOpPut(opKey, opBuf, 0),
		)
		rsp, er := txn.Commit()
		if er != nil {
			return nil, lifecycleError(operationID, ErrCodeCosmosEtcdUpdateFailed, "lifecycle: claim: %v", er)
		}
		if rsp.Succeeded {
			m.cacheOwnerPut(ownerKey, owner, rsp.Header.Revision)
			return &lifecycleSpawnClaim{operation: operation, owner: owner, result: operation.Result, shard: shard}, nil
		}
		if result, exists, err := m.existingOperation(operationID, fingerprint, kind); err != nil {
			return nil, err
		} else if exists {
			result, err = m.waitOperation(ctx, operationID, fingerprint, kind)
			return &lifecycleSpawnClaim{result: result}, err
		}
	}
	return nil, lifecycleError(operationID, ErrCodeOperationConflict, "lifecycle: claim contention")
}

func (m *lifecycleManager) finishSpawn(operationID string, claim *lifecycleSpawnClaim, businessErr *Error) (*AtomControlResult, *Error) {
	opRecord, opMod, err := m.loadOperation(operationID)
	if err != nil {
		return nil, err
	}
	owner, ownerMod, err := m.loadOwner(claim.owner.Ownership.Id.Element, claim.owner.Ownership.Id.Atom)
	if err != nil {
		return nil, err
	}
	if owner == nil || !lifecycleSameIdentity(claim.owner.Ownership.Id, owner.Ownership) || owner.OperationId != operationID {
		return nil, lifecycleError(operationID, ErrCodeOwnershipLost, "lifecycle: spawn completion lost ownership")
	}
	owner.Ownership.Id.InstanceId = claim.owner.Ownership.Id.InstanceId
	opRecord.Operation.Id.InstanceId = claim.owner.Ownership.Id.InstanceId
	if opRecord.Result != nil && opRecord.Result.Id != nil {
		opRecord.Result.Id.InstanceId = claim.owner.Ownership.Id.InstanceId
	}
	result := proto.Clone(claim.result).(*AtomControlResult)
	result.Operation = opRecord.Operation
	result.Ownership = proto.Clone(owner.Ownership).(*AtomOwnershipInfo)
	result.Error = businessErr
	opRecord.Operation.State = AtomOperationState_AtomOperationStateSucceeded
	opRecord.Operation.FinishTime = timestamppb.New(m.process.getClock().Now())
	opRecord.Operation.Error = businessErr
	opRecord.Operation.RetainUntilUnixNano = lifecycleRetention(m.process.getClock().Now(), false, claim.retain)
	if businessErr != nil {
		opRecord.Operation.State = AtomOperationState_AtomOperationStateFailed
	}
	opRecord.Result = result
	result.Operation = opRecord.Operation

	opBuf, err := lifecycleBytes(opRecord)
	if err != nil {
		return nil, err
	}
	ownerKey := m.ownerKey(owner.Ownership.Id.Element, owner.Ownership.Id.Atom)
	leaseID, healthy := m.currentLease()
	if !healthy || leaseID != clientv3.LeaseID(owner.LeaseId) {
		return nil, lifecycleError(operationID, ErrCodeOwnershipLost, "lifecycle: lease changed before spawn completion")
	}
	_, shardMod, guardErr := m.shardWriteGuard(claim.owner.Ownership.Id)
	if guardErr != nil {
		return nil, lifecycleError(operationID, CodeOf(guardErr), "lifecycle: spawn shard guard: %s", guardErr.Message)
	}
	comparisons := []clientv3.Cmp{
		clientv3.Compare(clientv3.ModRevision(m.operationKey(operationID)), "=", opMod),
		clientv3.Compare(clientv3.ModRevision(ownerKey), "=", ownerMod),
	}
	if m.perShard() {
		comparisons = append(comparisons, clientv3.Compare(clientv3.ModRevision(m.shardOwnerKey(claim.owner.Ownership.Id.ShardId)), "=", shardMod))
	}
	txn := m.process.cluster.etcdClient.Txn(context.Background()).If(comparisons...)
	if businessErr == nil {
		owner.Ownership.State = AtomLifecycleState_AtomLifecycleStateRunning
		result.Ownership = proto.Clone(owner.Ownership).(*AtomOwnershipInfo)
		opRecord.Result.Ownership = result.Ownership
		opBuf, err = lifecycleBytes(opRecord)
		if err != nil {
			return nil, err
		}
		ownerBuf, encodeErr := lifecycleBytes(owner)
		if encodeErr != nil {
			return nil, encodeErr
		}
		txn = txn.Then(EtcdOpPut(m.operationKey(operationID), opBuf, 0), EtcdOpPut(ownerKey, ownerBuf, leaseID))
	} else {
		txn = txn.Then(EtcdOpPut(m.operationKey(operationID), opBuf, 0), EtcdOpDelete(ownerKey))
	}
	rsp, er := txn.Commit()
	if er != nil {
		return nil, lifecycleError(operationID, ErrCodeCosmosEtcdUpdateFailed, "lifecycle: finish spawn: %v", er)
	}
	if !rsp.Succeeded {
		return nil, lifecycleError(operationID, ErrCodeOwnershipLost, "lifecycle: spawn completion CAS failed")
	}
	if businessErr == nil {
		m.cacheOwnerPut(ownerKey, owner, rsp.Header.Revision)
	} else {
		m.cacheOwnerDelete(ownerKey, rsp.Header.Revision)
	}
	return result, nil
}

func (m *lifecycleManager) spawn(ctx context.Context, callerID ID, element, atom string, arg proto.Message, operationID, fingerprint string, retain bool, args ...ArgsForBaseAtomos) (*AtomControlResult, *Error) {
	if err := m.requireUsable(); err != nil {
		return nil, err
	}
	if err := m.process.beginAtomSpawn(); err != nil {
		return &AtomControlResult{OperationId: operationID, Error: err}, nil
	}
	defer m.process.endAtomSpawn()
	if operationID == "" {
		var er error
		operationID, er = NewOperationID()
		if er != nil {
			return nil, NewErrorf(ErrCodeFrameworkInternalError, "lifecycle: operation ID: %v", er)
		}
	}
	if fingerprint == "" {
		var buf []byte
		if arg != nil {
			buf = lifecycleMessageBytes(arg)
		}
		fingerprint = lifecycleFingerprint(AtomOperationKind_AtomOperationKindSpawn, element, atom, buf, nil)
	}
	claim, err := m.claimSpawn(ctx, operationID, fingerprint, AtomOperationKind_AtomOperationKindSpawn, element, atom, nil, false)
	if err != nil {
		return &AtomControlResult{OperationId: operationID, Error: err}, nil
	}
	if claim.operation == nil {
		return claim.result, nil
	}
	claim.retain = retain
	if callerID == nil {
		callerID = m.process.local
	}
	elem, localErr := m.process.local.getLocalElement(element)
	if localErr != nil {
		localErr.OperationId = operationID
		return m.finishSpawn(operationID, claim, localErr)
	}
	beforeReady := func(actualID *IDInfo) *Error {
		claim.owner.Ownership.Id.InstanceId = actualID.InstanceId
		claim.operation.Operation.Id.InstanceId = actualID.InstanceId
		claim.result.Id.InstanceId = actualID.InstanceId
		result, finishErr := m.finishSpawn(operationID, claim, nil)
		if finishErr != nil {
			return finishErr
		}
		claim.result = result
		return nil
	}
	marker := &argBaseAtomosLifecycle{identity: claim.owner.Ownership.Id, beforeReady: beforeReady}
	spawnArgs := append(append([]ArgsForBaseAtomos{}, args...), marker)
	atomID, _, spawnErr := elem.SpawnAtom(callerID, atom, arg, nil, spawnArgs...)
	if spawnErr != nil {
		businessErr := asError(spawnErr)
		businessErr.OperationId = operationID
		result, finishErr := m.finishSpawn(operationID, claim, businessErr)
		if finishErr != nil {
			return nil, finishErr
		}
		return result, nil
	}
	if claim.result == nil || claim.result.Operation == nil || claim.result.Operation.State != AtomOperationState_AtomOperationStateSucceeded {
		return nil, lifecycleError(operationID, ErrCodeOperationUnknown, "lifecycle: local spawn returned without a completed operation")
	}
	claim.result.Id = atomID.GetIDInfo()
	return claim.result, nil
}

func (m *lifecycleManager) halt(ctx context.Context, callerID ID, id *IDInfo, operationID, fingerprint string, retain bool, args ...ArgsForBaseAtomos) (*AtomControlResult, *Error) {
	if err := m.requireUsable(); err != nil {
		return nil, err
	}
	if id == nil {
		return nil, NewError(ErrCodeCosmosRemoteServerInvalidArgs, "lifecycle: halt missing ID")
	}
	if operationID == "" {
		var er error
		operationID, er = NewOperationID()
		if er != nil {
			return nil, NewErrorf(ErrCodeFrameworkInternalError, "lifecycle: operation ID: %v", er)
		}
	}
	if fingerprint == "" {
		fingerprint = lifecycleFingerprint(AtomOperationKind_AtomOperationKindHalt, id.Element, id.Atom, nil, id)
	}
	if result, exists, err := m.existingOperation(operationID, fingerprint, AtomOperationKind_AtomOperationKindHalt); err != nil {
		return nil, err
	} else if exists {
		if result.Operation != nil && result.Operation.State == AtomOperationState_AtomOperationStateRunning {
			return m.waitOperation(ctx, operationID, fingerprint, AtomOperationKind_AtomOperationKindHalt)
		}
		return result, nil
	}
	owner, _, loadErr := m.loadOwner(id.Element, id.Atom)
	if loadErr != nil {
		return nil, loadErr
	}
	if owner != nil && lifecycleSameIdentity(id, owner.Ownership) && owner.Ownership.State == AtomLifecycleState_AtomLifecycleStateStopping && owner.OperationId != operationID {
		return m.joinStoppingHalt(ctx, operationID, fingerprint, retain, owner)
	}
	if err := m.authorize(id); err != nil {
		return &AtomControlResult{OperationId: operationID, Id: id, Error: err}, nil
	}
	_, shardMod, guardErr := m.shardWriteGuard(id)
	if guardErr != nil {
		return &AtomControlResult{OperationId: operationID, Id: id, Error: guardErr}, nil
	}

	var claim *AtomOwnershipRecord
	for attempt := 0; attempt < 16; attempt++ {
		owner, ownerMod, err := m.loadOwner(id.Element, id.Atom)
		if err != nil {
			return nil, err
		}
		if owner == nil || !lifecycleSameIdentity(id, owner.Ownership) {
			return &AtomControlResult{OperationId: operationID, Id: id, Error: lifecycleError(operationID, ErrCodeStaleFence, "lifecycle: halt stale identity")}, nil
		}
		op := newOperationInfo(m.process.getClock().Now(), operationID, AtomOperationKind_AtomOperationKindHalt, fingerprint, id)
		ownership := proto.Clone(owner.Ownership).(*AtomOwnershipInfo)
		ownership.State = AtomLifecycleState_AtomLifecycleStateStopping
		operation := &AtomOperationRecord{Operation: op, Result: &AtomControlResult{OperationId: operationID, Id: id, Ownership: ownership, Operation: op}}
		newOwner := &AtomOwnershipRecord{Ownership: ownership, LeaseId: owner.LeaseId, OperationId: operationID}
		ownerBuf, err := lifecycleBytes(newOwner)
		if err != nil {
			return nil, err
		}
		opBuf, err := lifecycleBytes(operation)
		if err != nil {
			return nil, err
		}
		comparisons := []clientv3.Cmp{
			clientv3.Compare(clientv3.ModRevision(m.ownerKey(id.Element, id.Atom)), "=", ownerMod),
			clientv3.Compare(clientv3.Version(m.operationKey(operationID)), "=", 0),
		}
		if m.perShard() {
			comparisons = append(comparisons, clientv3.Compare(clientv3.ModRevision(m.shardOwnerKey(id.ShardId)), "=", shardMod))
		}
		ctxTxn, cancel := etcdContext()
		rsp, er := m.process.cluster.etcdClient.Txn(ctxTxn).If(comparisons...).Then(
			EtcdOpPut(m.ownerKey(id.Element, id.Atom), ownerBuf, clientv3.LeaseID(owner.LeaseId)),
			EtcdOpPut(m.operationKey(operationID), opBuf, 0),
		).Commit()
		cancel()
		if er != nil {
			return nil, asError(er)
		}
		if rsp.Succeeded {
			m.cacheOwnerPut(m.ownerKey(id.Element, id.Atom), newOwner, rsp.Header.Revision)
			claim = newOwner
			break
		}
		if _, exists, err := m.existingOperation(operationID, fingerprint, AtomOperationKind_AtomOperationKindHalt); err == nil && exists {
			return m.waitOperation(ctx, operationID, fingerprint, AtomOperationKind_AtomOperationKindHalt)
		}
	}
	if claim == nil {
		return nil, lifecycleError(operationID, ErrCodeOperationConflict, "lifecycle: halt claim contention")
	}

	elem, err := m.process.local.getLocalElement(id.Element)
	if err != nil {
		return nil, err
	}
	atom, err := elem.getAtomFromRemote(id.Atom, id.InstanceId)
	var businessErr *Error
	if err != nil {
		businessErr = err
	} else {
		if callerID == nil {
			callerID = m.process.local
		}
		killArgs := append([]ArgsForBaseAtomos{}, args...)
		waitKilled := false
		for _, arg := range args {
			if arg != nil && arg.getArgType() == ArgTypeBaseAtomosWaitKilled {
				waitKilled = true
			}
		}
		if !waitKilled {
			killArgs = append(killArgs, ArgBaseAtomosWaitKilled())
		}
		killArgs = append(killArgs, &argBaseAtomosLifecycle{identity: id, internal: true})
		businessErr = asError(atom.atomos.PushKillMail(callerID, killArgs...))
	}
	return m.finishHalt(operationID, claim, businessErr, retain)
}

func (m *lifecycleManager) joinStoppingHalt(ctx context.Context, operationID, fingerprint string, retain bool, owner *AtomOwnershipRecord) (*AtomControlResult, *Error) {
	joined, _, err := m.loadOperation(owner.OperationId)
	if err != nil {
		return nil, err
	}
	joinedResult, err := m.waitOperation(ctx, owner.OperationId, joined.Operation.Fingerprint, joined.Operation.Kind)
	if err != nil {
		return nil, err
	}
	now := timestamppb.New(m.process.getClock().Now())
	op := &AtomOperationInfo{
		OperationId: operationID, Kind: AtomOperationKind_AtomOperationKindHalt,
		State: joinedResult.Operation.State, Id: owner.Ownership.Id, Fingerprint: fingerprint,
		StartTime: now, FinishTime: now, Error: joinedResult.Error,
		RetainUntilUnixNano: lifecycleRetention(m.process.getClock().Now(), false, retain),
	}
	result := proto.Clone(joinedResult).(*AtomControlResult)
	result.OperationId = operationID
	result.Operation = op
	record := &AtomOperationRecord{Operation: op, Result: result}
	buf, encodeErr := lifecycleBytes(record)
	if encodeErr != nil {
		return nil, encodeErr
	}
	ctxTxn, cancel := etcdContext()
	rsp, er := m.process.cluster.etcdClient.Txn(ctxTxn).If(clientv3.Compare(clientv3.Version(m.operationKey(operationID)), "=", 0)).Then(EtcdOpPut(m.operationKey(operationID), buf, 0)).Commit()
	cancel()
	if er != nil {
		return nil, asError(er)
	}
	if !rsp.Succeeded {
		return m.waitOperation(ctx, operationID, fingerprint, AtomOperationKind_AtomOperationKindHalt)
	}
	return result, nil
}

func (m *lifecycleManager) finishHalt(operationID string, owner *AtomOwnershipRecord, businessErr *Error, retain bool) (*AtomControlResult, *Error) {
	id := owner.Ownership.Id
	op, opMod, err := m.loadOperation(operationID)
	if err != nil {
		return nil, err
	}
	currentOwner, ownerMod, err := m.loadOwner(id.Element, id.Atom)
	if err != nil {
		return nil, err
	}
	if currentOwner == nil || currentOwner.OperationId != operationID || !lifecycleSameIdentity(id, currentOwner.Ownership) {
		return nil, lifecycleError(operationID, ErrCodeOwnershipLost, "lifecycle: halt completion lost ownership")
	}
	leaseID, healthy := m.currentLease()
	if !healthy || leaseID != clientv3.LeaseID(currentOwner.LeaseId) {
		return nil, lifecycleError(operationID, ErrCodeOwnershipLost, "lifecycle: lease changed before halt completion")
	}
	_, shardMod, guardErr := m.shardWriteGuard(id)
	if guardErr != nil {
		return nil, lifecycleError(operationID, CodeOf(guardErr), "lifecycle: halt shard guard: %s", guardErr.Message)
	}
	result := &AtomControlResult{OperationId: operationID, Id: id, Ownership: owner.Ownership, Operation: op.Operation, Error: businessErr}
	op.Operation.State = AtomOperationState_AtomOperationStateSucceeded
	op.Operation.Error = businessErr
	op.Operation.FinishTime = timestamppb.New(m.process.getClock().Now())
	op.Operation.RetainUntilUnixNano = lifecycleRetention(m.process.getClock().Now(), false, retain)
	if businessErr != nil {
		op.Operation.State = AtomOperationState_AtomOperationStateFailed
	}
	op.Result = result
	op.Result.Operation = op.Operation
	opBuf, err := lifecycleBytes(op)
	if err != nil {
		return nil, err
	}
	record, recordMod, err := m.loadRecord(id.Element, id.Atom)
	if err != nil {
		return nil, err
	}
	if record == nil {
		record = &AtomLifecycleRecord{Element: id.Element, Atom: id.Atom, NextFence: id.Fence + 1}
	}
	if businessErr == nil {
		record.Terminal = AtomLifecycleState_AtomLifecycleStateHalted
		record.TerminalId = id
	}
	recordBuf, err := lifecycleBytes(record)
	if err != nil {
		return nil, err
	}
	comparisons := []clientv3.Cmp{
		clientv3.Compare(clientv3.ModRevision(m.operationKey(operationID)), "=", opMod),
		clientv3.Compare(clientv3.ModRevision(m.ownerKey(id.Element, id.Atom)), "=", ownerMod),
		clientv3.Compare(clientv3.ModRevision(m.recordKey(id.Element, id.Atom)), "=", recordMod),
	}
	if m.perShard() {
		comparisons = append(comparisons, clientv3.Compare(clientv3.ModRevision(m.shardOwnerKey(id.ShardId)), "=", shardMod))
	}
	ctx, cancel := etcdContext()
	rsp, er := m.process.cluster.etcdClient.Txn(ctx).If(comparisons...).Then(EtcdOpPut(m.operationKey(operationID), opBuf, 0), EtcdOpPut(m.recordKey(id.Element, id.Atom), recordBuf, 0), EtcdOpDelete(m.ownerKey(id.Element, id.Atom))).Commit()
	cancel()
	if er != nil {
		return nil, asError(er)
	}
	if !rsp.Succeeded {
		return nil, lifecycleError(operationID, ErrCodeOwnershipLost, "lifecycle: halt completion CAS failed")
	}
	m.cacheOwnerDelete(m.ownerKey(id.Element, id.Atom), rsp.Header.Revision)
	return result, nil
}

func (m *lifecycleManager) supersedeOperation(operationID, takeoverID string) {
	op, mod, err := m.loadOperation(operationID)
	if err != nil || op.Operation.State != AtomOperationState_AtomOperationStateRunning {
		return
	}
	op.Operation.State = AtomOperationState_AtomOperationStateFailed
	op.Operation.FinishTime = timestamppb.New(m.process.getClock().Now())
	op.Operation.Error = lifecycleError(operationID, ErrCodeOwnershipLost, "lifecycle: operation superseded by takeover %s", takeoverID)
	op.Operation.RetainUntilUnixNano = lifecycleRetention(m.process.getClock().Now(), false, false)
	if op.Result == nil {
		op.Result = &AtomControlResult{OperationId: operationID, Id: op.Operation.Id}
	}
	op.Result.Error = op.Operation.Error
	op.Result.Operation = op.Operation
	buf, err := lifecycleBytes(op)
	if err != nil {
		return
	}
	ctx, cancel := etcdContext()
	_, _ = m.process.cluster.etcdClient.Txn(ctx).If(clientv3.Compare(clientv3.ModRevision(m.operationKey(operationID)), "=", mod)).Then(EtcdOpPut(m.operationKey(operationID), buf, 0)).Commit()
	cancel()
}

func (m *lifecycleManager) takeover(ctx context.Context, callerID ID, id, previous *IDInfo, arg proto.Message, operationID, fingerprint string, retain bool, args ...ArgsForBaseAtomos) (*AtomControlResult, *Error) {
	if err := m.requireUsable(); err != nil {
		return nil, err
	}
	if err := m.process.beginAtomSpawn(); err != nil {
		return &AtomControlResult{OperationId: operationID, Error: err}, nil
	}
	defer m.process.endAtomSpawn()
	if previous == nil {
		return nil, NewError(ErrCodeCosmosRemoteServerInvalidArgs, "lifecycle: takeover previous_id is required")
	}
	if operationID == "" {
		var er error
		operationID, er = NewOperationID()
		if er != nil {
			return nil, NewErrorf(ErrCodeFrameworkInternalError, "lifecycle: operation ID: %v", er)
		}
	}
	var buf []byte
	if arg != nil {
		buf = lifecycleMessageBytes(arg)
	}
	if fingerprint == "" {
		fingerprint = lifecycleFingerprint(AtomOperationKind_AtomOperationKindTakeover, id.Element, id.Atom, buf, previous)
	}
	previousOwner, _, err := m.loadOwner(id.Element, id.Atom)
	if err != nil {
		return nil, err
	}
	if previousOwner == nil || !lifecycleSameIdentity(previous, previousOwner.Ownership) {
		return &AtomControlResult{OperationId: operationID, Id: previous, Error: lifecycleError(operationID, ErrCodeStaleFence, "lifecycle: takeover previous_id does not match current owner")}, nil
	}
	if previous.Node == m.process.local.GetNodeName() {
		elem, localErr := m.process.local.getLocalElement(id.Element)
		if localErr != nil {
			return nil, localErr
		}
		oldAtom, localErr := elem.getAtomFromRemote(id.Atom, previous.InstanceId)
		if localErr != nil {
			return nil, localErr
		}
		if callerID == nil {
			callerID = m.process.local
		}
		if haltErr := oldAtom.atomos.PushKillMail(callerID, ArgBaseAtomosWaitKilled(), &argBaseAtomosLifecycle{identity: previous, internal: true}); haltErr != nil {
			return nil, asError(haltErr)
		}
	}
	claim, err := m.claimSpawn(ctx, operationID, fingerprint, AtomOperationKind_AtomOperationKindTakeover, id.Element, id.Atom, previous, true)
	if err != nil {
		return &AtomControlResult{OperationId: operationID, Error: err}, nil
	}
	if claim.operation == nil {
		return claim.result, nil
	}
	claim.retain = retain
	if previousOwner != nil && previousOwner.OperationId != "" {
		m.supersedeOperation(previousOwner.OperationId, operationID)
	}
	if claim.owner.Ownership.Fence <= previous.Fence || claim.owner.Ownership.Incarnation == previous.Incarnation {
		return nil, lifecycleError(operationID, ErrCodeFrameworkInternalError, "lifecycle: invalid takeover identity")
	}
	if callerID == nil {
		callerID = m.process.local
	}
	elem, localErr := m.process.local.getLocalElement(id.Element)
	if localErr != nil {
		localErr.OperationId = operationID
		return m.finishSpawn(operationID, claim, localErr)
	}
	beforeReady := func(actualID *IDInfo) *Error {
		claim.owner.Ownership.Id.InstanceId = actualID.InstanceId
		claim.operation.Operation.Id.InstanceId = actualID.InstanceId
		claim.result.Id.InstanceId = actualID.InstanceId
		result, finishErr := m.finishSpawn(operationID, claim, nil)
		if finishErr == nil {
			claim.result = result
		}
		return finishErr
	}
	marker := &argBaseAtomosLifecycle{identity: claim.owner.Ownership.Id, beforeReady: beforeReady}
	atom, _, spawnErr := elem.SpawnAtom(callerID, id.Atom, arg, nil, append(args, marker)...)
	if spawnErr != nil {
		businessErr := asError(spawnErr)
		businessErr.OperationId = operationID
		return m.finishSpawn(operationID, claim, businessErr)
	}
	claim.result.Id = atom.GetIDInfo()
	return claim.result, nil
}
