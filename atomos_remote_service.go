package go_atomos

import (
	"context"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"reflect"
	"time"
)

// AtomosRemoteService is the remote service of Atomos.
// It is used to communicate with the remote Atomos.
type atomosRemoteService struct {
	process *CosmosProcess
}

func (a *atomosRemoteService) TryKilling(ctx context.Context, req *CosmosRemoteTryKillingReq) (*CosmosRemoteTryKillingRsp, error) {
	rsp := &CosmosRemoteTryKillingRsp{}
	if err := a.process.stopFromOtherNode(); err != nil {
		rsp.Error = err.AddStack(a.process.local)
	}
	go func() {
		a.process.stopFromOtherNodeAfterResponse()
	}()
	return rsp, nil
}

// ScaleGetAtomID is the remote service of ScaleGetAtomID.
// It is used to communicate with the remote Atomos.
func (a *atomosRemoteService) ScaleGetAtomID(ctx context.Context, req *CosmosRemoteScaleGetAtomIDReq) (*CosmosRemoteScaleGetAtomIDRsp, error) {
	rsp := &CosmosRemoteScaleGetAtomIDRsp{}
	switch req.To.Type {
	case IDType_Atom:
		// Caller id.
		callerID := a.getFromCaller(req.CallerId, req.CallerCurFirstSyncCall)
		if callerID != nil {
			defer callerID.callerCounterDecr()
		}

		// Unmarshal args.
		in, er := anypb.UnmarshalNew(req.Args, proto.UnmarshalOptions{})
		if er != nil {
			rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: ScaleGetAtomID unmarshal args failed. err=(%v)", er).AddStack(nil)
			return rsp, nil
		}

		// Get element.
		elem, err := a.process.local.getLocalElement(req.To.Element)
		if err != nil {
			rsp.Error = err.AddStack(a.process.local)
			return rsp, nil
		}

		// Check caller cur first sync call.
		if elem.getCurFirstSyncCall() == req.CallerCurFirstSyncCall {
			rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: ScaleGetAtomID invalid caller cur first sync call. caller_cur_first_sync_call=(%v)", req.CallerCurFirstSyncCall).AddStack(nil)
			return rsp, nil
		}

		// Get atom.
		atom, _, err := elem.ScaleGetAtomID(callerID, req.Message, time.Duration(req.Timeout), in, nil, false)
		if err != nil {
			rsp.Error = err.AddStack(a.process.local)
			return rsp, nil
		}
		if reflect.ValueOf(atom).IsNil() {
			rsp.Error = NewErrorf(ErrAtomNotExists, "CosmosRemote: ScaleGetAtomID invalid atom. atom=(%v)", atom).AddStack(nil)
			return rsp, nil
		}

		rsp.Id = atom.GetIDInfo()
		return rsp, nil
	default:
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: ScaleGetAtomID invalid ToID type. to=(%v)", req.To).AddStack(nil)
		return rsp, nil
	}
}

func (a *atomosRemoteService) GetAtomID(ctx context.Context, req *CosmosRemoteGetAtomIDReq) (*CosmosRemoteGetAtomIDRsp, error) {
	rsp := &CosmosRemoteGetAtomIDRsp{}
	// Get element.
	elem, err := a.process.local.getLocalElement(req.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}
	// Get atom.
	atom, _, err := elem.GetAtomID(req.Atom, nil, false)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}

	rsp.Id = atom.GetIDInfo()
	return rsp, nil
}

func (a *atomosRemoteService) GetIDState(ctx context.Context, req *CosmosRemoteGetIDStateReq) (*CosmosRemoteGetIDStateRsp, error) {
	rsp := &CosmosRemoteGetIDStateRsp{}
	elem, err := a.process.local.getLocalElement(req.Id.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}
	switch req.Id.Type {
	case IDType_Atom:
		atom := elem.getAtom(req.Id.Atom)
		if atom != nil {
			rsp.State = int32(atom.State())
		} else {
			rsp.State = int32(AtomosHalt)
		}
	case IDType_Element:
		rsp.State = int32(elem.State())
	default:
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: GetIDState invalid id type. id=(%v)", req.Id).AddStack(nil)
	}
	return rsp, nil
}

func (a *atomosRemoteService) GetIDIdleTime(ctx context.Context, req *CosmosRemoteGetIDIdleTimeReq) (*CosmosRemoteGetIDIdleTimeRsp, error) {
	rsp := &CosmosRemoteGetIDIdleTimeRsp{}
	elem, err := a.process.local.getLocalElement(req.Id.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}
	switch req.Id.Type {
	case IDType_Atom:
		atom := elem.getAtom(req.Id.Atom)
		if atom != nil {
			rsp.IdleTime = int64(atom.IdleTime())
		} else {
			rsp.IdleTime = 0
		}
	case IDType_Element:
		rsp.IdleTime = int64(elem.IdleTime())
	default:
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: GetIDIdleTime invalid id type. id=(%v)", req.Id).AddStack(nil)
	}
	return rsp, nil
}

func (a *atomosRemoteService) GetElementInfo(ctx context.Context, req *CosmosRemoteGetElementInfoReq) (*CosmosRemoteGetElementInfoRsp, error) {
	elem, err := a.process.local.getLocalElement(req.Element)
	if err != nil {
		return nil, err.AddStack(a.process.local)
	}
	return &CosmosRemoteGetElementInfoRsp{
		AtomsNum:       uint64(elem.GetAtomsNum()),
		ActiveAtomsNum: uint64(elem.GetActiveAtomsNum()),
	}, nil
}

func (a *atomosRemoteService) SpawnAtom(ctx context.Context, req *CosmosRemoteSpawnAtomReq) (*CosmosRemoteSpawnAtomRsp, error) {
	rsp := &CosmosRemoteSpawnAtomRsp{}
	elem, err := a.process.local.getLocalElement(req.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}

	// Unmarshal args.
	in, er := anypb.UnmarshalNew(req.Args, proto.UnmarshalOptions{})
	if er != nil {
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SyncMessagingByName unmarshal args failed. err=(%v)", er).AddStack(nil)
		return rsp, nil
	}

	atom, _, err := elem.SpawnAtom(req.Atom, in, nil, false)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}

	rsp.Id = atom.GetIDInfo()
	return rsp, nil
}

// SyncMessagingByName sync messaging by name.
// CallerId is the caller id.
// CallerCurFirstSyncCall is the caller current first sync call.
// To is the target id.
// Args is the args.
// Rsp is the rsp.
// Error is the error.
func (a *atomosRemoteService) SyncMessagingByName(ctx context.Context, req *CosmosRemoteSyncMessagingByNameReq) (*CosmosRemoteSyncMessagingByNameRsp, error) {
	rsp := &CosmosRemoteSyncMessagingByNameRsp{}
	switch req.To.Type {
	case IDType_Atom, IDType_Element:
		// Caller id.
		callerID := a.getFromCaller(req.CallerId, req.CallerCurFirstSyncCall)
		if callerID != nil {
			defer callerID.callerCounterDecr()
		}

		// Unmarshal args.
		in, er := anypb.UnmarshalNew(req.Args, proto.UnmarshalOptions{})
		if er != nil {
			rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SyncMessagingByName unmarshal args failed. err=(%v)", er).AddStack(nil)
			return rsp, nil
		}

		var id SelfID
		// Get element.
		elem, err := a.process.local.getLocalElement(req.To.Element)
		if err != nil {
			rsp.Error = err.AddStack(a.process.local)
			return rsp, nil
		}
		if req.To.Type == IDType_Atom {
			atom := elem.getAtom(req.To.Atom)
			if atom != nil {
				id = atom
			} else {
				rsp.Error = NewErrorf(ErrAtomNotExists, "CosmosRemote: SyncMessagingByName invalid atom. atom=(%v)", req.To.Atom).AddStack(nil)
				return rsp, nil
			}
		} else {
			id = elem
		}
		if id == nil {
			rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SyncMessagingByName invalid id. id=(%v)", req.To).AddStack(nil)
			return rsp, nil
		}

		// Check caller cur first sync call.
		if id.getCurFirstSyncCall() == req.CallerCurFirstSyncCall {
			rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SyncMessagingByName invalid caller cur first sync call. caller_cur_first_sync_call=(%v)", req.CallerCurFirstSyncCall).AddStack(nil)
			return rsp, nil
		}

		// Sync messaging.
		out, err := id.SyncMessagingByName(callerID, req.Message, time.Duration(req.Timeout), in)
		if out != nil {
			rsp.Reply, _ = anypb.New(out)
		}
		if err != nil {
			rsp.Error = err.AddStack(a.process.local)
		}
		return rsp, nil
	default:
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SyncMessagingByName invalid ToID type. to=(%v)", req.To).AddStack(nil)
		return rsp, nil
	}
}

func (a *atomosRemoteService) KillAtom(ctx context.Context, req *CosmosRemoteKillAtomReq) (*CosmosRemoteKillAtomRsp, error) {
	rsp := &CosmosRemoteKillAtomRsp{}
	// Get element.
	elem, err := a.process.local.getLocalElement(req.Id.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}
	// Caller id.
	callerID := a.getFromCaller(req.CallerId, req.CallerCurFirstSyncCall)
	if callerID != nil {
		defer callerID.callerCounterDecr()
	}
	// Get atom.
	atom, _, err := elem.GetAtomID(req.Id.Atom, nil, false)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}
	// Kill atom.
	err = atom.Kill(callerID, time.Duration(req.Timeout))
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
	}
	return rsp, nil
}

func (a *atomosRemoteService) ElementBroadcast(ctx context.Context, req *CosmosRemoteElementBroadcastReq) (*CosmosRemoteElementBroadcastRsp, error) {
	rsp := &CosmosRemoteElementBroadcastRsp{}

	callerID := a.getFromCaller(req.CallerId, "")
	if callerID != nil {
		defer callerID.callerCounterDecr()
	}
	err := a.process.local.ElementBroadcast(callerID, req.Key, req.ContentType, req.ContentBuffer)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
	}
	return rsp, nil
}

func (a *atomosRemoteService) mustEmbedUnimplementedAtomosRemoteServiceServer() {}

func (a *atomosRemoteService) getFromCaller(callerIDInfo *IDInfo, firstSyncCall string) remoteFakeSelfID {
	if callerIDInfo == nil {
		return nil
	}

	// Cosmos
	a.process.cluster.remoteMutex.RLock()
	cosmosNode, has := a.process.cluster.remoteCosmos[callerIDInfo.Node]
	a.process.cluster.remoteMutex.RUnlock()
	if !has {
		return nil
	}
	if callerIDInfo.Type == IDType_Cosmos {
		return cosmosNode.newFakeCosmosSelfID(firstSyncCall)
	}

	// Element
	elem, err := cosmosNode.getElement(callerIDInfo.Element)
	if err != nil {
		return nil
	}
	if callerIDInfo.Type == IDType_Element {
		return elem.newRemoteElementFromCaller(callerIDInfo, firstSyncCall)
	}

	// Atom
	if callerIDInfo.Type != IDType_Atom {
		return nil
	}
	return elem.newRemoteAtomFromCaller(callerIDInfo, firstSyncCall)
}
