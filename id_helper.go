package atomos

// WithRebind performs a (typically remote) call, re-resolving the ID and
// retrying once if the call fails with a rebind-triggering error — i.e. an
// error indicating the caller's ID is stale and a fresh resolution may yield a
// usable target:
//   - ErrAtomInstanceMismatch (M5-0: target respawned under the same name)
//   - ErrAtomNotExists (target gone, no live instance)
//   - ErrCosmosRemoteConnectFailed (target node unreachable / moved)
//   - ErrCosmosNodeDraining (target node is draining; re-resolve to a Started node)
//
// resolve produces a fresh ID (wrap a generated Get<Svc>AtomID/Spawn<Svc>Atom
// factory). call performs the actual RPC against the resolved ID. At most one
// retry is attempted (the call runs at most twice) to bound latency; callers
// needing more can nest WithRebind.
//
// Tracker lifecycle: WithRebind releases the tracker of any ID it resolves
// (including the re-resolved one) once the call completes — both via the
// embedded *IDTracker for generated IDs (Release is idempotent and nil-safe)
// and as a no-op for remote IDs whose tracker is nil. Callers therefore do
// NOT need to Release the ID returned by resolve themselves.
//
// Example (typical remote call with automatic rebind):
//
//	out, err := atomos.WithRebind(func() (atomos.ID, *atomos.Error) {
//	    id, e := api.GetXxxAtomID(node, name)
//	    return id, e
//	}, func(id atomos.ID) (*XxxO, *atomos.Error) {
//	    return id.(*api.XxxAtomID).Method(caller, in)
//	})
func WithRebind[R any](resolve func() (ID, *Error), call func(ID) (R, *Error)) (R, *Error) {
	var zero R
	id, err := resolve()
	if err != nil {
		return zero, err.AddStack(nil)
	}
	defer releaseID(id)
	out, err := call(id)
	if err == nil || !shouldRebind(err.Code) {
		return out, err
	}
	// Rebind: re-resolve and retry once. The deferred releaseID(id) above still
	// runs for the stale ID; this call uses a freshly resolved one.
	id2, rerr := resolve()
	if rerr != nil {
		return zero, rerr.AddStack(nil)
	}
	defer releaseID(id2)
	return call(id2)
}

// shouldRebind reports whether the error code indicates the caller's ID is
// stale and a re-resolve may yield a usable target.
func shouldRebind(code int64) bool {
	switch code {
	case ErrAtomInstanceMismatch, ErrAtomNotExists,
		ErrCosmosRemoteConnectFailed, ErrCosmosNodeDraining:
		return true
	}
	return false
}

// releaseID releases the IDTracker carried by an ID, if any. Generated IDs
// (*<Svc>AtomID/*<Svc>ElementID) embed *IDTracker (promoting Release), so they
// satisfy ReleasableID; remote IDs (AtomRemoteInSourceProcess, etc.) do not,
// and their tracker is nil — Release is a no-op there. Safe to call on any ID.
func releaseID(id ID) {
	if r, ok := id.(ReleasableID); ok {
		r.Release()
	}
}
