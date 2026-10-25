# Changelog

All notable changes to go-atomos are documented here. The format loosely
follows [Keep a Changelog](https://keepachangelog.com/); versions follow the
framework's major-version lines (v2 = `master`/`preview`, v3 = the `v3`
branch).

## v3.0.0 (unreleased)

v3 is a source-incompatible redesign of the public API. The wire protocol is
**not** renegotiated: `atomos.proto` field numbers, the `Error` proto shape and
all 131 error-code numeric values are frozen and byte-compatible with v2, and
v2/v3 nodes can interoperate in a mixed cluster (see the compatibility notes
at the bottom). Full design rationale: `docs/DESIGN_V3_API.md`.

### Breaking changes & migration

Process entry & global state

- `Main` / `MainForWorkingPath` are deprecated wrappers; use
  `atomos.Run(runnable, opts...)` with `WithConfigFile` / `WithWorkingPath` /
  `WithStandalone` (and now `WithDebugServer`). `MainForConfigFile` is
  deleted. `MainForTest` moved to `atomostest.Run(t, runnable, ...)`.
- The `SharedCosmosProcess()` singleton and `InitCosmosProcess` are gone.
  Each `Run` creates its own `CosmosProcess` via `App.NewCosmosProcess`.
- Global setters (`SetMessageTimeoutTracer`, `MuteKeepaliveLog`,
  `SetIDTrackerDebug`) became `CosmosRunnable` builder methods applied at
  `Start`.

Error semantics

- All 130 `Err*` error-code constants are renamed `ErrCode*`
  (`ErrAtomNotExists` → `ErrCodeAtomNotExists`); values unchanged and now
  written as explicit literals (frozen against accidental renumbering).
- The `Err*` names are now **sentinel error variables**: match failures with
  `errors.Is(err, atomos.ErrAtomNotExists)` or `atomos.CodeOf(err)` instead
  of `err.Code == ...`.
- Public APIs return `error` instead of `*atomos.Error` throughout (ID,
  CosmosNode, Element, spawners, handlers, scripts, task/messenger APIs).
  Success paths return a genuine untyped nil.
- `protoc-gen-go-atomos` generates `error`-returning interfaces; business
  code must be regenerated with the v3 plugin. Generated `Async*` methods also
  return the pre-execution `error`, so callers no longer depend solely on a
  callback that may never be registered.

Call signatures

- Optional parameters are now variadic markers `ext ...ArgsForBaseAtomos`
  (`atomos.ArgBaseAtomosTimeout(d)`, `ArgBaseAtomosFromLocalOnly()`), the
  `[]ArgsForBaseAtomos` slice form is removed. An explicit `nil` is invalid.
- `Element.GetAtomID/SpawnAtom` drop the `fromLocalOrRemote bool`; pass
  `ArgBaseAtomosFromLocalOnly()` / `ArgBaseAtomosFromRemoteOnly()` for
  user-originated tracked calls.
- `CosmosRunnable.AddElementImplementation(impl, setSpawn bool)` split into
  `AddElementImplementation(impl)` and `AddElementImplementationWithSpawn(impl)`.
- `NewArgBaseAtomosWithAny(v)` → `ArgBaseAtomosAny(v)`;
  `IDTracker.ToString()` removed (use `String()`); `SelfID.CosmosMain()`
  returns `CosmosNode` instead of `*CosmosLocal`.

Export-surface convergence

- Public interfaces now contain only exported methods and are externally
  implementable (mock-friendly). Kernel-only methods moved to unexported
  interfaces; a user-made ID reaching a kernel path fails with
  `ErrFrameworkInvalidID`.
- Unexported: framework `On*` mailbox callbacks, remote RPC adapter types,
  `BaseAtomos` constructors, internal constants and util helpers, `Path`.
- `MapGo`/`MapGoWithRefCount` no longer embed `sync.RWMutex` (value copies
  no longer duplicate the lock); both are used through the `Map` interface.
- `ForTest*` proto fixtures are compiled into the test binary only
  (`for_test_pb_test.go`), leaving the production export surface.

Cluster RPC & TLS

- Call-class RPCs (`SyncMessagingByName`, `AsyncMessagingByName`,
  `SpawnAtom`, `KillAtom`, `GetAtomID`, `GetIDState`, `GetIDIdleTime`)
  double-write failures as non-OK gRPC status (code mapped from the
  framework error; full `*Error` proto in status details; body unchanged
  for v2 readers). **Migration**: raw gRPC stub callers must recover errors
  from status details instead of reading the body of a failed call. A Sync
  response carrying both a partial reply and an error remains OK-status with a
  body error, because non-OK gRPC responses cannot deliver that reply.
  Mixed-cluster behavior is documented in `docs/DESIGN_V3_API.md` §10.1.
- Mutual-TLS client certificates now load the **client** pair (previously
  every node presented the server certificate, making client auth
  meaningless). Half-written TLS material fails at startup with
  `ErrCodeCosmosEtcdClusterTLSInvalid` instead of silently degrading to
  plaintext. **Migration**: write TLS keys in complete pairs
  (`cluster_server_cert`+`cluster_server_key`, and for mutual TLS
  `cluster_client_cert`+`cluster_client_key`) or remove them entirely.
  Server-only TLS verifies CA/SAN by default; YAML certificate paths now feed
  the actual cluster listener/dialer, and `enable:false` stays standalone.

### Added

- Opt-in debug server: `Run(runnable, WithDebugServer("127.0.0.1:9099"))` or
  the `_debug_server` customize key serves `/elements` (element & atom
  inventory, mailbox depths, idle times, sync wait graph), `/metrics`
  (Prometheus text format, no client library), `/healthz`, `/debug/pprof/*`.
  Non-loopback listeners require `_debug_server_token`; whenever configured,
  that Bearer token is required on every endpoint. `atomosctl` reads
  `ATOMOS_DEBUG_TOKEN`.
- Runtime sync-deadlock detection: local wait-graph cycle detection returns
  `ErrIDFirstSyncCallDeadlock` before the timeout, plus cross-node wait
  chains (`CosmosArgs.wait_chain`, appended field 5) with
  `atomos.SetDeadlockHook` events. See `docs/DEADLOCK.md`.
- `atomos-vet` static analyzers: rewritten type-based `syncdeadlock`
  (self/cross-handler sync-call cycles) and `deferrelease` (IDTracker
  leaks), run via `go vet -vettool`.
- `atomostest` test doubles: `Run` (in-process runtime with `t.Log`
  routing, returning the `*CosmosProcess` and auto-stopping via
  `t.Cleanup`), `RunCluster` (real multi-node cluster: embedded etcd +
  loopback gRPC, default element-pinning router), `Probe` (five lifecycle
  hooks recorded and pollable), `Eventually`, `TakeGoroutines().AssertNoLeak`,
  `FakeID`, `FakeSelfID`, `FakeLogging`, `FakeTask`.
- Introspection round-out: `atomos_rpc_latency_seconds` histogram +
  `atomos_rpc_calls_total` + `atomos_remote_connections` metrics, `/schema`
  JSON endpoint (element message schema), `cmd/atomosctl` management CLI.
- `protoc-gen-go-atomos` emits a static `ElementConfig.Messages` fill
  (`atomos.AtomMessageConfigOf`); the framework fills it at runtime from
  decoders for hand-written implementations.
- Cluster nodes accept `_cluster_advertise_ip` in Customize (loopback
  advertisement for containers/single-host deployments);
  `CosmosRunnable.GetRouter()` read access.
- Supervisor: first test suite; `CreateFileIfNotExist` no longer overwrites
  existing configs on re-init.
- Error-code snapshot test locking all 131 numeric values; generator
  golden-file test (`go test ./protoc-gen-go-atomos/`, `-update` to
  refresh).
- `ArgBaseAtomosAnyValue` reader; generated Messenger fields are keyed.
- Makefile gates: `test-race` (-count=2), `test-integration` and
  `test-integration-race` (embedded etcd), `bench`, `cover`, `cross-build`,
  `vet` (atomos-vet wired in); GitHub Actions CI running all of them.

### Fixed

- Every one-shot etcd RPC is deadline-bounded. Lost keepalive streams trigger
  bounded re-registration and watch reconciliation; watch rebuilds resume from
  an authoritative snapshot revision and cancellation exits cleanly.
- Version locks use ModRevision CAS with bounded retries, stale-version
  reclamation and safe concurrent unlock; first-dial failures can be retried.
- DeathWatch-related test clusters leaked mailbox/logging goroutines that
  intermittently tripped the race detector; the shared cluster fixture now
  registers cleanup.

### Compatibility notes

- **Wire protocol frozen**: `atomos.proto` fields 1–4 of `CosmosArgs` and all
  existing message shapes are unchanged; v3 only appends (`wait_chain = 5`,
  `ErrFrameworkInvalidID = 130`). v2 readers skip unknown appended fields.
- **Error codes frozen**: all 130 v2 codes verified value-identical in v3
  (cross-checked against the v2 branch).
- **Mixed clusters**: v2→v3 calls are byte-compatible; v3→v2 call failures
  surface to v2 clients as `ErrCosmosRemoteResponseInvalid` (precise remote
  codes are masked by the status double-write — a known, documented
  trade-off; the step-by-step audit guide for projects rolling v3 into a
  v2 fleet lives in `docs/MIXED_CLUSTER_ERROR_CODE_MIGRATION.md`). Full
  matrix in `docs/DESIGN_V3_API.md` §10.
- **Generated code**: business code must be regenerated with the v3
  `protoc-gen-go-atomos`; v2-generated code will not compile against v3.

## v2 and earlier

No changelog was kept for v2 and earlier; see the git history on `master`
and the `preview/*` branches.
