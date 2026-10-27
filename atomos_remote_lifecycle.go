package atomos

import (
	"context"
	"reflect"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func lifecycleManagerNotImplemented(operationID string) *Error {
	return lifecycleError(operationID, ErrCodeAtomNotImplemented, "CosmosRemote: controlled atom lifecycle requires PerAtomStrict")
}

func lifecycleRequestCaller(a *atomosRemoteService, info *IDInfo) ID {
	if info == nil {
		return a.process.local
	}
	caller := a.getRemoteInTargetProcessIDFromIDInfo(info, 0, 0)
	if caller == nil || reflect.ValueOf(caller).IsNil() {
		return a.process.local
	}
	return caller
}

func lifecycleUnmarshalArgs(arg *anypb.Any) (proto.Message, *Error) {
	if arg == nil {
		return nil, nil
	}
	msg, err := arg.UnmarshalNew()
	if err != nil {
		return nil, NewErrorf(ErrCodeCosmosRemoteServerInvalidArgs, "CosmosRemote: controlled spawn args: %v", err)
	}
	return msg, nil
}

func (a *atomosRemoteService) SpawnAtomControlled(_ context.Context, req *CosmosRemoteSpawnAtomControlledReq) (*CosmosRemoteSpawnAtomControlledRsp, error) {
	defer func() { Recover(a.process.local) }()
	rsp := &CosmosRemoteSpawnAtomControlledRsp{}
	if req == nil {
		rsp.Error = NewError(ErrCodeCosmosRemoteServerInvalidArgs, "CosmosRemote: SpawnAtomControlled missing request")
		return rsp, nil
	}
	if a.process.lifecycle == nil {
		rsp.Error = lifecycleManagerNotImplemented(req.OperationId)
		return rsp, nil
	}
	arg, err := lifecycleUnmarshalArgs(req.Args)
	if err != nil {
		rsp.Error = err
		return rsp, nil
	}
	result, opErr := a.process.lifecycle.spawn(context.Background(), lifecycleRequestCaller(a, req.CallerId), req.Element, req.Atom, arg, req.OperationId, req.Fingerprint, req.Retain, createExtForCosmosArgs(req.CosmosArgs)...)
	if opErr != nil {
		rsp.Error = opErr
	} else {
		rsp.Result = result
	}
	return rsp, nil
}

func (a *atomosRemoteService) HaltAtomControlled(_ context.Context, req *CosmosRemoteHaltAtomControlledReq) (*CosmosRemoteHaltAtomControlledRsp, error) {
	defer func() { Recover(a.process.local) }()
	rsp := &CosmosRemoteHaltAtomControlledRsp{}
	if req == nil || req.Id == nil {
		rsp.Error = NewError(ErrCodeCosmosRemoteServerInvalidArgs, "CosmosRemote: HaltAtomControlled missing ID")
		return rsp, nil
	}
	if a.process.lifecycle == nil {
		rsp.Error = lifecycleManagerNotImplemented(req.OperationId)
		return rsp, nil
	}
	result, err := a.process.lifecycle.halt(context.Background(), lifecycleRequestCaller(a, req.CallerId), req.Id, req.OperationId, req.Fingerprint, req.Retain, createExtForCosmosArgs(req.CosmosArgs)...)
	if err != nil {
		rsp.Error = err
	} else {
		rsp.Result = result
	}
	return rsp, nil
}

func (a *atomosRemoteService) TakeoverAtom(_ context.Context, req *CosmosRemoteTakeoverAtomReq) (*CosmosRemoteTakeoverAtomRsp, error) {
	defer func() { Recover(a.process.local) }()
	rsp := &CosmosRemoteTakeoverAtomRsp{}
	if req == nil || req.Id == nil || req.PreviousId == nil {
		rsp.Error = NewError(ErrCodeCosmosRemoteServerInvalidArgs, "CosmosRemote: TakeoverAtom requires id and previous_id")
		return rsp, nil
	}
	if a.process.lifecycle == nil {
		rsp.Error = lifecycleManagerNotImplemented(req.OperationId)
		return rsp, nil
	}
	arg, err := lifecycleUnmarshalArgs(req.Args)
	if err != nil {
		rsp.Error = err
		return rsp, nil
	}
	result, opErr := a.process.lifecycle.takeover(context.Background(), lifecycleRequestCaller(a, req.CallerId), req.Id, req.PreviousId, arg, req.OperationId, req.Fingerprint, req.Retain, createExtForCosmosArgs(req.CosmosArgs)...)
	if opErr != nil {
		rsp.Error = opErr
	} else {
		rsp.Result = result
	}
	return rsp, nil
}

func (a *atomosRemoteService) GetAtomOperation(_ context.Context, req *CosmosRemoteGetAtomOperationReq) (*CosmosRemoteGetAtomOperationRsp, error) {
	defer func() { Recover(a.process.local) }()
	rsp := &CosmosRemoteGetAtomOperationRsp{}
	if req == nil || req.OperationId == "" {
		rsp.Error = NewError(ErrCodeCosmosRemoteServerInvalidArgs, "CosmosRemote: GetAtomOperation missing operation ID")
		return rsp, nil
	}
	if a.process.lifecycle == nil {
		rsp.Error = lifecycleManagerNotImplemented(req.OperationId)
		return rsp, nil
	}
	op, _, err := a.process.lifecycle.loadOperation(req.OperationId)
	if err != nil {
		rsp.Error = err
	} else {
		rsp.Operation = op.Operation
	}
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
	if a.process.lifecycle != nil {
		lookup, err := a.process.lifecycle.lookup(req.Id.Element, req.Id.Atom)
		if err != nil {
			rsp.Error = err
			return rsp, nil
		}
		return lookup, nil
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
