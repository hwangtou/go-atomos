package atomos

import "context"

func lifecycleManagerNotImplemented(operationID string) *Error {
	err := NewError(ErrCodeAtomNotImplemented, "CosmosRemote: controlled atom lifecycle manager is not enabled")
	err.OperationId = operationID
	return err
}

func (a *atomosRemoteService) SpawnAtomControlled(_ context.Context, req *CosmosRemoteSpawnAtomControlledReq) (*CosmosRemoteSpawnAtomControlledRsp, error) {
	defer func() { Recover(a.process.local) }()
	rsp := &CosmosRemoteSpawnAtomControlledRsp{}
	if req == nil {
		rsp.Error = NewError(ErrCodeCosmosRemoteServerInvalidArgs, "CosmosRemote: SpawnAtomControlled missing request")
		return rsp, nil
	}
	rsp.Error = lifecycleManagerNotImplemented(req.OperationId)
	return rsp, nil
}

func (a *atomosRemoteService) HaltAtomControlled(_ context.Context, req *CosmosRemoteHaltAtomControlledReq) (*CosmosRemoteHaltAtomControlledRsp, error) {
	defer func() { Recover(a.process.local) }()
	rsp := &CosmosRemoteHaltAtomControlledRsp{}
	if req == nil {
		rsp.Error = NewError(ErrCodeCosmosRemoteServerInvalidArgs, "CosmosRemote: HaltAtomControlled missing request")
		return rsp, nil
	}
	rsp.Error = lifecycleManagerNotImplemented(req.OperationId)
	return rsp, nil
}

func (a *atomosRemoteService) TakeoverAtom(_ context.Context, req *CosmosRemoteTakeoverAtomReq) (*CosmosRemoteTakeoverAtomRsp, error) {
	defer func() { Recover(a.process.local) }()
	rsp := &CosmosRemoteTakeoverAtomRsp{}
	if req == nil {
		rsp.Error = NewError(ErrCodeCosmosRemoteServerInvalidArgs, "CosmosRemote: TakeoverAtom missing request")
		return rsp, nil
	}
	rsp.Error = lifecycleManagerNotImplemented(req.OperationId)
	return rsp, nil
}

func (a *atomosRemoteService) GetAtomOperation(_ context.Context, req *CosmosRemoteGetAtomOperationReq) (*CosmosRemoteGetAtomOperationRsp, error) {
	defer func() { Recover(a.process.local) }()
	rsp := &CosmosRemoteGetAtomOperationRsp{}
	if req == nil {
		rsp.Error = NewError(ErrCodeCosmosRemoteServerInvalidArgs, "CosmosRemote: GetAtomOperation missing request")
		return rsp, nil
	}
	rsp.Error = lifecycleManagerNotImplemented(req.OperationId)
	return rsp, nil
}

func (a *atomosRemoteService) LookupAtom(_ context.Context, req *CosmosRemoteLookupAtomReq) (*CosmosRemoteLookupAtomRsp, error) {
	defer func() { Recover(a.process.local) }()
	rsp := &CosmosRemoteLookupAtomRsp{State: AtomLifecycleState_AtomLifecycleStateUnknown}
	if req == nil || req.Id == nil {
		rsp.Error = NewError(ErrCodeCosmosRemoteServerInvalidArgs, "CosmosRemote: LookupAtom missing ID")
		return rsp, nil
	}
	if req.Id.Type != IDType_Atom || req.Id.Element == "" || req.Id.Atom == "" {
		rsp.Error = NewError(ErrCodeCosmosRemoteServerInvalidArgs, "CosmosRemote: LookupAtom requires an atom ID")
		return rsp, nil
	}
	elem, err := a.process.local.getLocalElement(req.Id.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}

	elem.lock.RLock()
	atom := elem.atoms[req.Id.Atom]
	elem.lock.RUnlock()
	if atom == nil {
		rsp.State = AtomLifecycleState_AtomLifecycleStateAbsent
		return rsp, nil
	}
	rsp.Id = atom.GetIDInfo()
	switch atom.State() {
	case BaseAtomosSpawning:
		rsp.State = AtomLifecycleState_AtomLifecycleStateSpawning
	case BaseAtomosWaiting, BaseAtomosBusy:
		rsp.State = AtomLifecycleState_AtomLifecycleStateRunning
	case BaseAtomosStopping:
		rsp.State = AtomLifecycleState_AtomLifecycleStateStopping
	case BaseAtomosHalt:
		rsp.State = AtomLifecycleState_AtomLifecycleStateHalted
	}
	return rsp, nil
}
