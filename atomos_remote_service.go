package go_atomos

import (
	"context"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"time"
)

type atomosRemoteService struct {
	process *CosmosProcess
}

func (a *atomosRemoteService) ScaleGetAtomID(ctx context.Context, req *CosmosRemoteScaleGetAtomIDReq) (*CosmosRemoteScaleGetAtomIDRsp, error) {
	rsp := &CosmosRemoteScaleGetAtomIDRsp{}
	elem, err := a.process.local.getLocalElement(req.To.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}

	if elem.getCurFirstSyncCall() == req.CallerCurFirstSyncCall {
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SyncMessagingByName invalid caller cur first sync call. caller_cur_first_sync_call=(%v)", req.CallerCurFirstSyncCall)
		return rsp, nil
	}
	callerID := a.getFromCaller(req.CallerId, req.CallerCurFirstSyncCall)
	if callerID == nil {
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SyncMessagingByName invalid caller id. caller_id=(%v)", req.CallerId)
		return rsp, nil
	}

	in, er := anypb.UnmarshalNew(req.Args, proto.UnmarshalOptions{})
	if er != nil {
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: ScaleGetAtomID unmarshal args failed. err=(%v)", er)
		return rsp, nil
	}
	atom, tracker, err := elem.ScaleGetAtomID(callerID, req.Message, time.Duration(req.Timeout), in, req.FromTracker)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}

	a.process.setRemoteTracker(tracker)

	rsp.Id = atom.GetIDInfo()
	rsp.TrackerId = tracker.id
	return rsp, nil
}

func (a *atomosRemoteService) GetAtomID(ctx context.Context, req *CosmosRemoteGetAtomIDReq) (*CosmosRemoteGetAtomIDRsp, error) {
	rsp := &CosmosRemoteGetAtomIDRsp{}
	elem, err := a.process.local.getLocalElement(req.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}
	atom, tracker, err := elem.GetAtomID(req.Atom, req.FromTracker)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}

	a.process.setRemoteTracker(tracker)

	rsp.Id = atom.GetIDInfo()
	rsp.TrackerId = tracker.id
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
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: GetIDState invalid id type. id=(%v)", req.Id)
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
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: GetIDIdleTime invalid id type. id=(%v)", req.Id)
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
	atom, tracker, err := elem.SpawnAtom(req.Atom, req.Args, req.FromTracker)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}

	a.process.setRemoteTracker(tracker)

	rsp.Id = atom.GetIDInfo()
	rsp.TrackerId = tracker.id
	return rsp, nil
}

func (a *atomosRemoteService) SyncMessagingByName(ctx context.Context, req *CosmosRemoteSyncMessagingByNameReq) (*CosmosRemoteSyncMessagingByNameRsp, error) {
	rsp := &CosmosRemoteSyncMessagingByNameRsp{}
	elem, err := a.process.local.getLocalElement(req.To.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}
	in, er := anypb.UnmarshalNew(req.Args, proto.UnmarshalOptions{})
	if er != nil {
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SyncMessagingByName unmarshal args failed. err=(%v)", er)
		return rsp, nil
	}

	if elem.getCurFirstSyncCall() == req.CallerCurFirstSyncCall {
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SyncMessagingByName invalid caller cur first sync call. caller_cur_first_sync_call=(%v)", req.CallerCurFirstSyncCall)
		return rsp, nil
	}
	callerID := a.getFromCaller(req.To, req.CallerCurFirstSyncCall)

	out, err := elem.SyncMessagingByName(callerID, req.Message, time.Duration(req.Timeout), in)
	rsp.Reply, _ = anypb.New(out)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
	}
	return rsp, nil
}

func (a *atomosRemoteService) ReleaseID(ctx context.Context, req *CosmosRemoteReleaseIDReq) (*CosmosRemoteReleaseIDRsp, error) {
	rsp := &CosmosRemoteReleaseIDRsp{}

	a.process.cluster.remoteMutex.Lock()
	tracker, has := a.process.cluster.remoteTrackIDMap[req.TrackerId]
	if has {
		delete(a.process.cluster.remoteTrackIDMap, req.TrackerId)
	}
	a.process.cluster.remoteMutex.Unlock()
	tracker.Release()

	return rsp, nil
}

func (a *atomosRemoteService) ElementBroadcast(ctx context.Context, req *CosmosRemoteElementBroadcastReq) (*CosmosRemoteElementBroadcastRsp, error) {
	rsp := &CosmosRemoteElementBroadcastRsp{}

	callerID := a.getFromCaller(req.CallerId, "")
	err := a.process.local.ElementBroadcast(callerID, req.Key, req.ContentType, req.ContentBuffer)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
	}
	return rsp, nil
}

func (a *atomosRemoteService) mustEmbedUnimplementedAtomosRemoteServiceServer() {}

func (a *atomosRemoteService) getFromCaller(callerIDInfo *IDInfo, firstSyncCall string) SelfID {
	if callerIDInfo == nil {
		return nil
	}

	// Cosmos
	a.process.cluster.remoteMutex.RLock()
	cosmosNode, has := a.process.cluster.remoteCosmos[callerIDInfo.Cosmos]
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
