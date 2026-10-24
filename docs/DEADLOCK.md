# Sync-Call Deadlock: Detection and Handling

This document explains how sync-call deadlocks form in go-atomos, the three
defense layers (compile time, runtime detection, runtime handling), and the
engineering practices that keep a production process off the deadlock path.

## Why sync calls deadlock

Every Atom processes one mail at a time on its single mailbox goroutine. A
sync call (`SyncMessagingByName` / generated `id.SayHello(...)`-style clients)
blocks the *calling* goroutine until the target Atom's mailbox goroutine runs
the handler and replies. A "who-waits-for-whom" cycle therefore deadlocks:
every mailbox on the cycle is busy running a handler that is blocked waiting
for another busy mailbox.

The canonical shapes:

| Pattern | Example |
|---|---|
| Self-call | handler sync-calls its own atom (`GetFooAtomID(cosmos, self.GetIDInfo().Atom)`) |
| Two-atom cycle | A's handler sync-calls B while B's handler sync-calls A |
| Longer cycle | A→B→C→A, including hops across nodes (A@node1→B@node2→A@node1) |

Two "invisible" variants produce the same symptom without being wait-graph
cycles: a handler blocked on **user locks or slow I/O** (callers time out
behind it), and cycles routed through **user-owned goroutines/condition
variables**. These are covered only by timeouts, not cycle detection.

## Layer 1 — compile time (`syncdeadlock` analyzer)

```sh
go vet -vettool=$(which atomos-vet) ./...
```

The analyzer is type-based, keyed off generated-code shapes:

- Handler interfaces: named interfaces ending in `Atom`/`Element` that embed
  `atomos.Atomos` (e.g. `GreeterAtom`). Implementing types are found via
  `types.Implements`, including interfaces imported from other packages.
- Sync-call edges: method calls on generated ID clients
  (`*XxxAtomID`/`*XxxElementID`), generated `SpawnXxxAtom(...)` factories,
  and `SyncMessagingByName` with a literal message name. Async variants
  (`AsyncXxx`, callback-taking signatures) and calls inside `go` statements
  are excluded.
- Nodes are `(service, method)` pairs, so same-named methods on different
  services cannot create false edges. A handler calling the *same* handler on
  a *different* atom is not reported — atom names are runtime values.

Reports:

- **Self-call** (always a deadlock): the handler resolves an ID to its own
  atom via the documented `self.GetIDInfo().Atom` idiom.
- **Handler cycle** (A.Svc.M→B.Svc.N→…→A.Svc.M): "potential sync deadlock
  cycle" at the edge that closes it.

Known scope limits: handler bodies in *other* packages are not analyzed
(cross-package cycles are one-sided), and runtime-computed atom/message names
are invisible. The runtime wait-graph is the authoritative backstop.

## Layer 2 — runtime detection

### Local wait-graph (authoritative for in-process cycles)

Every sync push registers a wait edge `caller goroutine → target mailbox
goroutine` in a per-process graph before blocking
(`CosmosProcess.detectDeadlockAndWait`). If the new edge would close a cycle,
the call returns **`ErrIDFirstSyncCallDeadlock` (code 84) immediately** — no
timeout wait — with a readable chain:

```
Atomos: Sync call deadlock (cycle). caller=(n1::Elem::b) target=(n1::Elem::a)
    name=(Greeting) chain=g42→n1::Elem::b[g7] (cycle)
```

The check+register is atomic under a mutex, so two atoms simultaneously
sync-calling each other cannot both pass. Edges are removed on every exit
path, so the graph never retains stale entries. Self-calls
(`callerGoID == targetGoID`) are detected the same way. There are **no false
positives**: a detected cycle is provably stuck.

With `CosmosRunnable.SetSyncDeadlockDebug(true)` every edge additionally
captures the caller's stack; deadlock errors then embed the stacks of every
hop on the chain. Off by default (zero hot-path cost).

### Cross-node wait chain (`CosmosArgs.wait_chain`)

Remote sync calls propagate their wait chain in the request. On the sending
side the chain is `ambient chain of the executing mailbox + this caller's ID`;
on the receiving node the service entry checks whether the chain already
contains the target ID — if so, the call is rejected immediately with the
same `ErrIDFirstSyncCallDeadlock`, and the error travels back along the RPC
path, unblocking every member of the ring. The chain survives local hops: a
chain-carrying sync mail exposes its chain as the mailbox's ambient chain
while executing, so `A@n1 → B@n2 → (local) C@n2 → A@n1` is caught too.

Mixed-version clusters: nodes running older binaries ignore the new
`wait_chain` field (proto3 unknown-field skipping), so the chain breaks at
that node and protection degrades to timeouts until the fleet is upgraded.

### Timeouts (the universal backstop)

Timeouts catch everything cycle detection cannot see: slow handlers, user
locks, invisible cycles, and cross-node chains broken by old nodes.

- Per call: `atomos.ArgBaseAtomosTimeout(d)`.
- Process-wide default for calls without an explicit timeout:
  `CosmosRunnable.SetSyncCallDefaultTimeout(d)` (default 10s; also the base
  of the remote client-side deadline). Framework control mails (kill /
  wormhole) always keep a 10s floor so a short default cannot break Stop.
- Failure semantics: `ErrAtomosPushTimeoutReject` (mail still queued —
  removed) vs `ErrAtomosPushTimeoutHandling` (handler is executing — the
  mail stays and its reply is dropped).

Budget nesting rule: inner-call timeouts must be smaller than the outer
call's remaining budget, or the outer call fails first and the inner reply
arrives to nobody.

## Layer 3 — runtime handling

### What actually unblocks a deadlock

Go cannot preempt goroutines. A `Kill` mail to an atom whose mailbox is
blocked in a sync wait merely queues — killing cannot break a formed cycle.
The framework's detection is what unblocks things: the *closing* call returns
an error immediately, and as handlers propagate that error outward every
member of the ring wakes. For undetectable waits, timeouts do the same job,
slower.

Practical playbook once a deadlock/timeout is observed:

1. The error/chain (and debug stacks) tell you which handlers form the ring.
2. Fix the shape (see practices below) — the ring will re-form otherwise.
3. If an atom is wedged beyond recovery (e.g. handler stuck on a dead
   external resource), restart the *process*; there is no in-process unwedge
   for a blocked mailbox goroutine.

### `SetDeadlockHook` — ops and diagnostics outlet

```go
runnable.SetDeadlockHook(func(e *atomos.DeadlockEvent) {
    // e.Kind: Cycle | TimeoutReject | TimeoutHandling
    // e.Caller/e.Target/e.Name/e.Chain/e.Err
    metrics.Inc(e.Kind)
    if e.Kind == atomos.DeadlockEventCycle {
        _ = os.WriteFile("deadlock-stacks.txt", []byte(atomos.DumpStacks()), 0644)
    }
})
```

The hook fires on every local cycle, cross-node cycle, and sync timeout. It
runs synchronously on the reporting goroutine (often a mailbox): keep it
fast, panic-free, and free of sync calls into the framework. A panicking hook
is contained and logged, not propagated.

`atomos.DumpStacks()` returns all-goroutine stacks (panic-dump format) for
capture from the hook or an ops endpoint.

## Practices that prevent deadlocks

1. **Never sync-call your own atom** — it is the same goroutine; call the
   method directly.
2. **Keep the sync-call graph a DAG.** Organize services into layers and
   call downward; for anything upward or peer-level, use async messaging
   (`AsyncXxx` / `AsyncMessagingByName`) and reply via callback.
3. **Always carry a timeout on sync calls**, especially across nodes;
   nest budgets inner-smaller-than-outer.
4. **No blocking work in handlers**: no user mutexes that other handlers
   take, no long I/O — offload to tasks (`Task().AddToNamedQueue`) or make
   the call async.
5. **Run `atomos-vet` in CI** (`syncdeadlock` + `deferrelease`) so
   self-calls and in-package cycles never reach runtime.
6. **Enable `SetSyncDeadlockDebug` in staging** (or low-traffic production)
   to capture hop stacks, and wire `SetDeadlockHook` into metrics/alerting.

## Implementation map

| Piece | Where |
|---|---|
| Analyzer | `cmd/atomos-vet/syncdeadlock/` |
| Wait-graph | `cosmos_process.go` (`detectDeadlockAndWait`, `waitChain`) |
| Detection point | `base_atomos.go` `PushSyncMessage` |
| Timeout default | `base_atomos_mails.go` `waitReply`, `base_remote.go` `PushSyncMessage` |
| Cross-node chain | `atomos.proto` `CosmosArgs.wait_chain`, `base_remote.go`, `atomos_remote_service.go` |
| Hook + event | `cosmos.go` (`SetDeadlockHook`, `DeadlockEvent`), fire sites in detection/timeout paths |
| Stack dump | `util_go.go` `DumpStacks` |
| Tests | `sync_deadlock_test.go`, `sync_deadlock_crossnode_test.go` |
