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

var (
	atomosGRPCTTL         = 1 * time.Second
	atomosGRPCDialTimeout = 3 * time.Second
	atomosGRPCTimeout     = 1 * time.Minute
)

func (a *atomosRemoteService) TryKilling(_ context.Context, _ *CosmosRemoteTryKillingReq) (*CosmosRemoteTryKillingRsp, error) {
	defer func() {
		Recover(a.process.local)
	}()
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
func (a *atomosRemoteService) ScaleGetAtomID(_ context.Context, req *CosmosRemoteScaleGetAtomIDReq) (*CosmosRemoteScaleGetAtomIDRsp, error) {
	defer func() {
		Recover(a.process.local)
	}()
	rsp := &CosmosRemoteScaleGetAtomIDRsp{}

	switch req.To.Type {
	case IDType_Atom:
		// Caller id.
		callerID := a.getFromCaller(req.CallerId, req.CallerContext)
		if callerID != nil {
			defer callerID.callerCounterRelease()
		}

		var in proto.Message
		var er error
		// Unmarshal args.
		if req.Args != nil {
			in, er = anypb.UnmarshalNew(req.Args, proto.UnmarshalOptions{})
			if er != nil {
				rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: ScaleGetAtomID unmarshal args failed. err=(%v)", er).AddStack(nil)
				return rsp, nil
			}
		}

		// Get element.
		elem, err := a.process.local.getLocalElement(req.To.Element)
		if err != nil {
			rsp.Error = err.AddStack(a.process.local)
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

func (a *atomosRemoteService) GetAtomID(_ context.Context, req *CosmosRemoteGetAtomIDReq) (*CosmosRemoteGetAtomIDRsp, error) {
	defer func() {
		Recover(a.process.local)
	}()
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

func (a *atomosRemoteService) GetIDState(_ context.Context, req *CosmosRemoteGetIDStateReq) (*CosmosRemoteGetIDStateRsp, error) {
	defer func() {
		Recover(a.process.local)
	}()
	rsp := &CosmosRemoteGetIDStateRsp{}
	elem, err := a.process.local.getLocalElement(req.Id.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}
	switch req.Id.Type {
	case IDType_Atom:
		atom, err := elem.getAtomFromRemote(req.Id.Atom)
		if err != nil {
			rsp.State = int32(AtomosHalt)
		} else {
			rsp.State = int32(atom.State())
		}
	case IDType_Element:
		rsp.State = int32(elem.State())
	default:
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: GetIDState invalid id type. id=(%v)", req.Id).AddStack(nil)
	}
	return rsp, nil
}

func (a *atomosRemoteService) GetIDIdleTime(_ context.Context, req *CosmosRemoteGetIDIdleTimeReq) (*CosmosRemoteGetIDIdleTimeRsp, error) {
	defer func() {
		Recover(a.process.local)
	}()
	rsp := &CosmosRemoteGetIDIdleTimeRsp{}
	elem, err := a.process.local.getLocalElement(req.Id.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}
	switch req.Id.Type {
	case IDType_Atom:
		atom, err := elem.getAtomFromRemote(req.Id.Atom)
		if err != nil {
			rsp.IdleTime = 0
		} else {
			rsp.IdleTime = int64(atom.IdleTime())
		}
	case IDType_Element:
		rsp.IdleTime = int64(elem.IdleTime())
	default:
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: GetIDIdleTime invalid id type. id=(%v)", req.Id).AddStack(nil)
	}
	return rsp, nil
}

func (a *atomosRemoteService) GetElementInfo(_ context.Context, req *CosmosRemoteGetElementInfoReq) (*CosmosRemoteGetElementInfoRsp, error) {
	defer func() {
		Recover(a.process.local)
	}()
	elem, err := a.process.local.getLocalElement(req.Element)
	if err != nil {
		return nil, err.AddStack(a.process.local)
	}
	return &CosmosRemoteGetElementInfoRsp{
		AtomsNum:       uint64(elem.GetAtomsNum()),
		ActiveAtomsNum: uint64(elem.GetActiveAtomsNum()),
	}, nil
}

func (a *atomosRemoteService) SpawnAtom(_ context.Context, req *CosmosRemoteSpawnAtomReq) (*CosmosRemoteSpawnAtomRsp, error) {
	defer func() {
		Recover(a.process.local)
	}()
	rsp := &CosmosRemoteSpawnAtomRsp{}
	callerID := a.getFromCaller(req.CallerId, req.CallerContext)
	if callerID != nil {
		defer callerID.callerCounterRelease()
	}

	elem, err := a.process.local.getLocalElement(req.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}

	var in proto.Message
	var er error
	// Unmarshal args.
	if req.Args != nil {
		in, er = anypb.UnmarshalNew(req.Args, proto.UnmarshalOptions{})
		if er != nil {
			rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SpawnAtom unmarshal args failed. err=(%v)", er).AddStack(nil)
			return rsp, nil
		}
	}

	atom, _, err := elem.SpawnAtom(callerID, req.Atom, in, nil, false)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}

	rsp.Id = atom.GetIDInfo()
	return rsp, nil
}

// SyncMessagingByName sync messaging by name.
// CallerID is the caller id.
// CallerContext is the caller context.
// To is the target id.
// Args is the args.
// Rsp is the rsp.
// Error is the error.
func (a *atomosRemoteService) SyncMessagingByName(_ context.Context, req *CosmosRemoteSyncMessagingByNameReq) (*CosmosRemoteSyncMessagingByNameRsp, error) {
	defer func() {
		Recover(a.process.local)
	}()
	a.process.local.Log().Debug("atomosRemoteService: SyncMessagingByName req=(%v)", req)
	rsp := &CosmosRemoteSyncMessagingByNameRsp{}

	switch req.To.Type {
	case IDType_Atom, IDType_Element:
		// Caller id.
		callerID := a.getFromCaller(req.CallerId, req.CallerContext)
		if callerID != nil {
			defer callerID.callerCounterRelease()
		}

		var in proto.Message
		var er error
		// Unmarshal args.
		if req.Args != nil {
			in, er = anypb.UnmarshalNew(req.Args, proto.UnmarshalOptions{})
			if er != nil {
				rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SyncMessagingByName unmarshal args failed. err=(%v)", er).AddStack(nil)
				return rsp, nil
			}
		}

		var id SelfID
		// Get element.
		elem, err := a.process.local.getLocalElement(req.To.Element)
		if err != nil {
			rsp.Error = err.AddStack(a.process.local)
			return rsp, nil
		}
		if req.To.Type == IDType_Atom {
			atom, err := elem.getAtomFromRemote(req.To.Atom)
			if err != nil {
				rsp.Error = err.AddStack(a.process.local)
				return rsp, nil
			}
			//if id == nil || reflect.ValueOf(id).IsNil() {
			//	rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: SyncMessagingByName invalid id. id=(%v)", req.To).AddStack(nil)
			//	return rsp, nil
			//}
			id = atom
		} else {
			id = elem
		}

		// Sync messaging.
		out, err := id.getAtomos().PushMessageMailAndWaitReply(callerID, req.Message, false, time.Duration(req.Timeout), in)
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

func (a *atomosRemoteService) AsyncMessagingByName(ctx context.Context, req *CosmosRemoteAsyncMessagingByNameReq) (*CosmosRemoteAsyncMessagingByNameRsp, error) {
	defer func() {
		Recover(a.process.local)
	}()
	a.process.local.Log().Debug("atomosRemoteService: AsyncMessagingByName req=(%v)", req)
	rsp := &CosmosRemoteAsyncMessagingByNameRsp{}

	switch req.To.Type {
	case IDType_Atom, IDType_Element:
		// Caller id.
		callerID := a.getFromCaller(req.CallerId, req.CallerContext)
		if callerID != nil {
			defer callerID.callerCounterRelease()
		}

		var in proto.Message
		var er error
		// Unmarshal args.
		if req.Args != nil {
			in, er = anypb.UnmarshalNew(req.Args, proto.UnmarshalOptions{})
			if er != nil {
				rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: AsyncMessagingByName unmarshal args failed. err=(%v)", er).AddStack(nil)
				return rsp, nil
			}
		}

		var id SelfID
		// Get element.
		elem, err := a.process.local.getLocalElement(req.To.Element)
		if err != nil {
			rsp.Error = err.AddStack(a.process.local)
			return rsp, nil
		}
		if req.To.Type == IDType_Atom {
			atom, err := elem.getAtomFromRemote(req.To.Atom)
			if err != nil {
				rsp.Error = err.AddStack(a.process.local)
				return rsp, nil
			}
			//if id == nil || reflect.ValueOf(id).IsNil() {
			//	rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: AsyncMessagingByName invalid id. id=(%v)", req.To).AddStack(nil)
			//	return rsp, nil
			//}
			id = atom
		} else {
			id = elem
		}

		// Async messaging.
		if req.NeedReply {
			out, err := id.getAtomos().PushMessageMailAndWaitReply(callerID, req.Message, true, time.Duration(req.Timeout), in)
			if out != nil {
				rsp.Reply, _ = anypb.New(out)
			}
			if err != nil {
				rsp.Error = err.AddStack(a.process.local)
			}
		} else {
			id.getAtomos().PushAsyncMessageMail(callerID, id, req.Message, time.Duration(req.Timeout), in, nil)
		}
		return rsp, nil
	default:
		rsp.Error = NewErrorf(ErrCosmosRemoteServerInvalidArgs, "CosmosRemote: AsyncMessagingByName invalid ToID type. to=(%v)", req.To).AddStack(nil)
		return rsp, nil
	}
}

func (a *atomosRemoteService) KillAtom(_ context.Context, req *CosmosRemoteKillAtomReq) (*CosmosRemoteKillAtomRsp, error) {
	defer func() {
		Recover(a.process.local)
	}()
	rsp := &CosmosRemoteKillAtomRsp{}

	// Get element.
	elem, err := a.process.local.getLocalElement(req.Id.Element)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
		return rsp, nil
	}
	// Caller id.
	callerID := a.getFromCaller(req.CallerId, req.CallerContext)
	if callerID != nil {
		defer callerID.callerCounterRelease()
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

func (a *atomosRemoteService) ElementBroadcast(_ context.Context, req *CosmosRemoteElementBroadcastReq) (*CosmosRemoteElementBroadcastRsp, error) {
	defer func() {
		Recover(a.process.local)
	}()
	rsp := &CosmosRemoteElementBroadcastRsp{}

	callerID := a.getFromCaller(req.CallerId, req.CallerContext)
	if callerID != nil {
		defer callerID.callerCounterRelease()
	}
	err := a.process.local.ElementBroadcast(callerID, req.Key, req.ContentType, req.ContentBuffer)
	if err != nil {
		rsp.Error = err.AddStack(a.process.local)
	}
	return rsp, nil
}

func (a *atomosRemoteService) mustEmbedUnimplementedAtomosRemoteServiceServer() {}

func (a *atomosRemoteService) getFromCaller(callerIDInfo *IDInfo, callerIDContextInfo *IDContextInfo) remoteFakeSelfID {
	defer func() {
		Recover(a.process.local)
	}()
	if callerIDInfo == nil {
		return nil
	}

	// Cosmos
	cosmosNode := a.process.local.GetCosmosNode(callerIDInfo.Node)
	if cosmosNode == nil {
		return nil
	}
	if callerIDInfo.Type == IDType_Cosmos {
		return cosmosNode.newFakeCosmosSelfID(callerIDInfo, callerIDContextInfo)
	}

	// Element
	elem, err := cosmosNode.getElement(callerIDInfo.Element)
	if err != nil {
		return nil
	}
	if callerIDInfo.Type == IDType_Element {
		return elem.newRemoteElementFromCaller(callerIDInfo, callerIDContextInfo)
	}

	// Atom
	if callerIDInfo.Type != IDType_Atom {
		return nil
	}
	return elem.newRemoteAtomFromCaller(callerIDInfo, callerIDContextInfo)
}
