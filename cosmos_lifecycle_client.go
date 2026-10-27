package atomos

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// CosmosLifecycleReader exposes side-effect-free strict lifecycle queries.
// CosmosLocal and CosmosRemote implement it; CosmosNode does not require it.
type CosmosLifecycleReader interface {
	CosmosLookupAtom(element, atom string) (*CosmosRemoteLookupAtomRsp, error)
	CosmosGetAtomOperation(operationID string) (*AtomOperationInfo, error)
}

func lifecycleOperationArg(args []ArgsForBaseAtomos) string {
	for _, arg := range args {
		switch value := arg.(type) {
		case argBaseAtomosOperationID:
			return value.operationID
		case *argBaseAtomosOperationID:
			if value != nil {
				return value.operationID
			}
		}
	}
	return ""
}

func lifecycleTransportError(operationID string, err error) *Error {
	return lifecycleError(operationID, ErrCodeOperationUnknown, "lifecycle: transport outcome unknown: %v", err)
}

func (e *ElementRemote) spawnControlled(callerID ID, name string, arg proto.Message, args ...ArgsForBaseAtomos) (ID, *Error) {
	operationID := lifecycleOperationArg(args)
	if operationID == "" {
		var err error
		operationID, err = NewOperationID()
		if err != nil {
			return nil, asError(err)
		}
	}
	var anyArg *anypb.Any
	var fingerprint string
	if arg != nil {
		var err error
		anyArg, err = anypb.New(arg)
		if err != nil {
			return nil, asError(err)
		}
		buf := lifecycleMessageBytes(arg)
		fingerprint = lifecycleFingerprint(AtomOperationKind_AtomOperationKindSpawn, e.remote.info.Element, name, buf, nil)
	}
	cli, conn, ctx, cancel, err := e.remote.getCliWithConn(atomosClientTimeout)
	if err != nil {
		return nil, err
	}
	defer cancel()
	rsp, rpcErr := cli.SpawnAtomControlled(ctx, &CosmosRemoteSpawnAtomControlledReq{
		CallerId: callerID.GetIDInfo(), Element: e.remote.info.Element, Atom: name, Args: anyArg,
		OperationId: operationID, Fingerprint: fingerprint, CosmosArgs: helperRemoteArgs(args),
	})
	if rpcErr != nil {
		return nil, lifecycleTransportError(operationID, rpcErr)
	}
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	if rsp.Result == nil {
		return nil, lifecycleError(operationID, ErrCodeOperationUnknown, "lifecycle: empty controlled spawn result")
	}
	if rsp.Result.Error != nil {
		if rsp.Result.Id != nil {
			return newAtomRemoteInSourceProcessWithPinnedConn(e, rsp.Result.Id, conn), rsp.Result.Error
		}
		return nil, rsp.Result.Error
	}
	if rsp.Result.Id == nil {
		return nil, lifecycleError(operationID, ErrCodeOperationUnknown, "lifecycle: successful spawn has no ID")
	}
	return newAtomRemoteInSourceProcessWithPinnedConn(e, rsp.Result.Id, conn), nil
}

func (e *ElementRemote) takeoverControlled(callerID ID, name string, previous *IDInfo, arg proto.Message, args ...ArgsForBaseAtomos) (ID, *Error) {
	operationID := lifecycleOperationArg(args)
	if operationID == "" {
		var err error
		operationID, err = NewOperationID()
		if err != nil {
			return nil, asError(err)
		}
	}
	var anyArg *anypb.Any
	var buf []byte
	if arg != nil {
		var err error
		anyArg, err = anypb.New(arg)
		if err != nil {
			return nil, asError(err)
		}
		buf = lifecycleMessageBytes(arg)
	}
	id := &IDInfo{Type: IDType_Atom, Cosmos: e.remote.info.Cosmos, Node: e.remote.info.Node, Element: e.remote.info.Element, Atom: name}
	cli, conn, ctx, cancel, err := e.remote.getCliWithConn(atomosClientTimeout)
	if err != nil {
		return nil, err
	}
	defer cancel()
	rsp, rpcErr := cli.TakeoverAtom(ctx, &CosmosRemoteTakeoverAtomReq{
		CallerId: callerID.GetIDInfo(), Id: id, PreviousId: previous, Args: anyArg, OperationId: operationID,
		Fingerprint: lifecycleFingerprint(AtomOperationKind_AtomOperationKindTakeover, id.Element, name, buf, previous), CosmosArgs: helperRemoteArgs(args),
	})
	if rpcErr != nil {
		return nil, lifecycleTransportError(operationID, rpcErr)
	}
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	if rsp.Result == nil {
		return nil, lifecycleError(operationID, ErrCodeOperationUnknown, "lifecycle: empty takeover result")
	}
	if rsp.Result.Error != nil {
		return nil, rsp.Result.Error
	}
	return newAtomRemoteInSourceProcessWithPinnedConn(e, rsp.Result.Id, conn), nil
}

func helperRemoteArgs(args []ArgsForBaseAtomos) *CosmosArgs {
	remote := make([]ArgsForBaseAtomos, 0, len(args))
	for _, arg := range args {
		if arg == nil {
			continue
		}
		switch arg.getArgType() {
		case ArgTypeBaseAtomosTimeout, ArgTypeBaseAtomosAppendToHead, ArgTypeBaseAtomosWaitKilled:
			remote = append(remote, arg)
		}
	}
	if len(remote) == 0 {
		return nil
	}
	return createBaseAtomosHelper(BaseAtomosMailSync, remote).getRemoteArg()
}

func (e *ElementRemote) lookupControlled(name string) (*CosmosRemoteLookupAtomRsp, *grpc.ClientConn, *Error) {
	cli, conn, ctx, cancel, err := e.remote.getCliWithConn(atomosClientTimeout)
	if err != nil {
		return nil, nil, err
	}
	defer cancel()
	rsp, rpcErr := cli.LookupAtom(ctx, &CosmosRemoteLookupAtomReq{Id: &IDInfo{Type: IDType_Atom, Element: e.remote.info.Element, Atom: name}})
	if rpcErr != nil {
		return nil, nil, grpcErrorToError(rpcErr)
	}
	if rsp.Error != nil {
		return nil, nil, rsp.Error
	}
	return rsp, conn, nil
}

func (e *ElementRemote) haltControlled(callerID ID, id *IDInfo, args ...ArgsForBaseAtomos) *Error {
	operationID := lifecycleOperationArg(args)
	if operationID == "" {
		var err error
		operationID, err = NewOperationID()
		if err != nil {
			return asError(err)
		}
	}
	cli, _, ctx, cancel, err := e.remote.getCliWithConn(atomosClientTimeout)
	if err != nil {
		return err
	}
	defer cancel()
	rsp, rpcErr := cli.HaltAtomControlled(ctx, &CosmosRemoteHaltAtomControlledReq{
		CallerId: callerID.GetIDInfo(), Id: id, OperationId: operationID,
		Fingerprint: lifecycleFingerprint(AtomOperationKind_AtomOperationKindHalt, id.Element, id.Atom, nil, id),
		CosmosArgs:  helperRemoteArgs(args),
	})
	if rpcErr != nil {
		return lifecycleTransportError(operationID, rpcErr)
	}
	if rsp.Error != nil {
		return rsp.Error
	}
	if rsp.Result == nil {
		return lifecycleError(operationID, ErrCodeOperationUnknown, "lifecycle: empty halt result")
	}
	return rsp.Result.Error
}

func (c *CosmosLocal) strictSpawn(callerID SelfID, element Element, elemName, name string, arg proto.Message, args ...ArgsForBaseAtomos) (ID, *IDTracker, error) {
	switch elem := element.(type) {
	case *ElementLocal:
		operationID := lifecycleOperationArg(args)
		result, err := c.process.lifecycle.spawn(context.Background(), callerID, elemName, name, arg, operationID, "", false, args...)
		if err != nil {
			return nil, nil, err
		}
		if result == nil {
			return nil, nil, NewError(ErrCodeOperationUnknown, "lifecycle: empty spawn result")
		}
		if result.Error != nil {
			return nil, nil, result.Error
		}
		if result.Id == nil {
			return nil, nil, NewError(ErrCodeOperationUnknown, "lifecycle: successful spawn has no ID")
		}
		atom, localErr := elem.getAtomFromRemote(name, result.Id.InstanceId)
		if localErr != nil && result.Error == nil {
			return nil, nil, localErr
		}
		if atom == nil {
			return nil, nil, errOrNil(result.Error)
		}
		return atom, elem.addRefAtom(atom, c.newLocalIDTrackerInfo()), errOrNil(result.Error)
	case *elementRemoteFromSource:
		id, err := elem.ElementRemote.spawnControlled(callerID, name, arg, args...)
		return id, nil, errOrNil(err)
	default:
		return nil, nil, NewErrorf(ErrCodeFrameworkInternalError, "lifecycle: unsupported element %T", element)
	}
}

func (c *CosmosLocal) strictGet(elemName, name string, args ...ArgsForBaseAtomos) (ID, *IDTracker, error) {
	lookup, err := c.process.lifecycle.lookup(elemName, name)
	if err != nil {
		return nil, nil, err
	}
	switch lookup.State {
	case AtomLifecycleState_AtomLifecycleStateRunning:
		if lookup.Id.Node == c.GetNodeName() {
			elem, localErr := c.getLocalElement(elemName)
			if localErr != nil {
				return nil, nil, localErr
			}
			atom, localErr := elem.getAtomFromRemote(name, lookup.Id.InstanceId)
			if localErr != nil {
				return nil, nil, localErr
			}
			return atom, elem.addRefAtom(atom, c.newLocalIDTrackerInfo()), nil
		}
		c.process.cluster.remoteMutex.RLock()
		remote := c.process.cluster.remoteCosmos[lookup.Id.Node]
		c.process.cluster.remoteMutex.RUnlock()
		if remote == nil {
			return nil, nil, NewErrorf(ErrCodeCosmosRemoteElementNotFound, "lifecycle: owner node %q not discovered", lookup.Id.Node)
		}
		elem, remoteErr := remote.getElement(elemName)
		if remoteErr != nil {
			return nil, nil, remoteErr
		}
		remoteLookup, conn, remoteErr := elem.ElementRemote.lookupControlled(name)
		if remoteErr != nil {
			return nil, nil, remoteErr
		}
		if remoteLookup.State != AtomLifecycleState_AtomLifecycleStateRunning || !lifecycleSameIdentity(lookup.Id, remoteLookup.Ownership) {
			return nil, nil, NewError(ErrCodeStaleFence, "lifecycle: owner changed during Get")
		}
		return newAtomRemoteInSourceProcessWithPinnedConn(elem.ElementRemote, remoteLookup.Id, conn), nil, nil
	case AtomLifecycleState_AtomLifecycleStateAbsent:
		impl := c.runnable.implements[elemName]
		if impl == nil {
			return nil, nil, NewErrorf(ErrCodeMainElementNotFound, "lifecycle: element %q not registered", elemName)
		}
		persistence, ok := impl.Developer.(AutoData)
		if !ok || persistence == nil {
			return nil, nil, NewError(ErrCodeAtomNotExists, "lifecycle: atom absent and AutoData is unavailable")
		}
		element, localErr := c.getGlobalElement(elemName, name)
		if localErr != nil {
			return nil, nil, localErr
		}
		return c.strictSpawn(c, element, elemName, name, nil, args...)
	default:
		return nil, nil, NewErrorf(ErrCodeOperationStillRunning, "lifecycle: atom is %s", lookup.State)
	}
}

func (c *CosmosLocal) CosmosLookupAtom(element, atom string) (*CosmosRemoteLookupAtomRsp, error) {
	if c.process.lifecycle == nil {
		return nil, lifecycleManagerNotImplemented("")
	}
	rsp, err := c.process.lifecycle.lookup(element, atom)
	return rsp, errOrNil(err)
}

func (c *CosmosLocal) CosmosGetAtomOperation(operationID string) (*AtomOperationInfo, error) {
	if c.process.lifecycle == nil {
		return nil, lifecycleManagerNotImplemented(operationID)
	}
	op, _, err := c.process.lifecycle.loadOperation(operationID)
	if err != nil {
		return nil, err
	}
	return op.Operation, nil
}

func (c *CosmosRemote) CosmosLookupAtom(element, atom string) (*CosmosRemoteLookupAtomRsp, error) {
	elem, err := c.getElement(element)
	if err != nil {
		return nil, err
	}
	rsp, _, remoteErr := elem.ElementRemote.lookupControlled(atom)
	return rsp, errOrNil(remoteErr)
}

func (c *CosmosRemote) CosmosGetAtomOperation(operationID string) (*AtomOperationInfo, error) {
	cli, ctx, cancel, err := c.remote.getCli(atomosClientTimeout)
	if err != nil {
		return nil, err
	}
	defer cancel()
	rsp, rpcErr := cli.GetAtomOperation(ctx, &CosmosRemoteGetAtomOperationReq{OperationId: operationID})
	if rpcErr != nil {
		return nil, grpcErrorToError(rpcErr)
	}
	if rsp.Error != nil {
		return nil, rsp.Error
	}
	return rsp.Operation, nil
}

// CosmosTakeoverAtom explicitly replaces previous with a higher-fence
// incarnation on this node. previous must identify the current owner exactly.
func (c *CosmosLocal) CosmosTakeoverAtom(callerID SelfID, element, atom string, previous *IDInfo, arg proto.Message, args ...ArgsForBaseAtomos) (ID, *IDTracker, error) {
	if c.process.lifecycle == nil {
		return nil, nil, lifecycleManagerNotImplemented("")
	}
	id := &IDInfo{Type: IDType_Atom, Cosmos: c.runnable.config.Cosmos, Node: c.GetNodeName(), Element: element, Atom: atom}
	result, err := c.process.lifecycle.takeover(context.Background(), callerID, id, previous, arg, lifecycleOperationArg(args), "", false, args...)
	if err != nil {
		return nil, nil, err
	}
	if result.Error != nil {
		return nil, nil, result.Error
	}
	elem, localErr := c.getLocalElement(element)
	if localErr != nil {
		return nil, nil, localErr
	}
	localAtom, localErr := elem.getAtomFromRemote(atom, result.Id.InstanceId)
	if localErr != nil {
		return nil, nil, localErr
	}
	return localAtom, elem.addRefAtom(localAtom, c.newLocalIDTrackerInfo()), nil
}

// CosmosTakeoverAtom explicitly replaces previous with a higher-fence
// incarnation on the remote node. previous must identify the current owner.
func (c *CosmosRemote) CosmosTakeoverAtom(callerID SelfID, element, atom string, previous *IDInfo, arg proto.Message, args ...ArgsForBaseAtomos) (ID, *IDTracker, error) {
	elem, err := c.getElement(element)
	if err != nil {
		return nil, nil, err
	}
	id, remoteErr := elem.ElementRemote.takeoverControlled(callerID, atom, previous, arg, args...)
	return id, nil, errOrNil(remoteErr)
}
