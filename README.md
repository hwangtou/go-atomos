# go-atomos

A Go actor-model framework. An **Atomos** is an actor: a serial mailbox, a
lifecycle (spawn / messaging / halt), supervised concurrency, and built-in
task scheduling. Actors are organized as **Elements** (actor types) and
**Atoms** (named actor instances), running inside a **Cosmos** process that
can stand alone or join an etcd-backed multi-node cluster.

## Features

- **Actor runtime** — every actor owns a serial mailbox; sync/async messaging
  with deadlock detection (local wait-graph + cross-node wait chains,
  `ErrIDFirstSyncCallDeadlock`), timeouts, and panic recovery hooks. See
  [docs/DEADLOCK.md](docs/DEADLOCK.md).
- **Code generation** — `protoc-gen-go-atomos` turns a `.proto` service into
  typed element/atom interfaces, ID clients, and message decoders.
- **Multi-node clusters** — etcd service discovery, cross-node messaging over
  gRPC, version-based hot upgrade and gradual drain (`DESIGN_DRAIN_GRADUAL.md`).
- **Lifecycle safety** — `IDTracker` reference counting with `Release` /
  `WithID` / `WithRebind`, enforced by the bundled `atomos-vet` analyzers.
- **Task scheduling** — deferred, timed and crontab tasks plus named serial
  and concurrent queues, all executed on the actor's mailbox goroutine.
- **Test kit** — `atomostest.Run` boots a real in-process runtime for tests
  (returning the `*CosmosProcess`, auto-stopped via `t.Cleanup`);
  `RunCluster` boots a real multi-node cluster over an embedded etcd and
  loopback gRPC; `Probe` records actor lifecycle events from the five
  runnable hooks; `Eventually` polls async actor state instead of sleeping;
  `TakeGoroutines().AssertNoLeak` catches leaked mailbox goroutines;
  `FakeID` / `FakeSelfID` / `FakeLogging` / `FakeTask` cover unit tests
  without a process.
- **Debug server** — an opt-in HTTP endpoint (`WithDebugServer("127.0.0.1:9099")`
  or the `_debug_server` customize key) serving `/elements` (element & atom
  inventory, mailbox depths, idle times, sync wait graph), `/metrics`
  (Prometheus text format), `/healthz`, and `/debug/pprof/*`.

Requires Go 1.24+.

## Quickstart

Install the plugins (protoc is required too):

```sh
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install github.com/hwangtou/go-atomos/protoc-gen-go-atomos@latest
```

Describe your actors in a `.proto` file. The `atomos_method` option declares
each RPC's role — element/atom spawner or message handler:

```proto
syntax = "proto3";
package hello;
option go_package = "example.com/hello/api";

import "atomos.proto"; // from the go-atomos repository root

message HelloSpawnArg { string name = 1; }
message HelloRequest  { string name = 1; }
message HelloReply    { string message = 1; }

service Greeter {
  rpc Spawn (HelloSpawnArg) returns (atomos.Nil) {
    option (atomos.atomos_method) = AtomosAtomSpawn;
  }
  rpc SayHello (HelloRequest) returns (HelloReply) {
    option (atomos.atomos_method) = AtomosAtomMethod;
  }
}
```

Generate the bindings:

```sh
protoc -I. -I<go-atomos repo root> \
  --go_out=. --go_opt=paths=source_relative \
  --go-atomos_out=. --go-atomos_opt=paths=source_relative \
  hello.proto
```

Implement the generated interfaces and run:

```go
type greeterAtom struct{ self atomos.AtomSelfID }

func (a *greeterAtom) Spawn(self atomos.AtomSelfID, arg *api.HelloSpawnArg, data *atomos.Nil, args ...atomos.ArgsForBaseAtomos) error {
	a.self = self
	return nil
}
func (a *greeterAtom) SayHello(from atomos.ID, in *api.HelloRequest) (*api.HelloReply, error) {
	return &api.HelloReply{Message: "Hello, " + in.GetName() + "!"}, nil
}
func (a *greeterAtom) String() string { return "greeter" }
func (a *greeterAtom) Halt(from atomos.ID, cancelled []uint64) (bool, proto.Message) {
	return false, nil
}

func main() {
	runnable := (&atomos.CosmosRunnable{}).
		SetMainScript(&myScript{}).
		AddElementImplementationWithSpawn(api.GetGreeterImplement(&greeterDeveloper{}))
	atomos.Run(runnable) // --config config.yaml, or atomos.WithWorkingPath(...)
}
```

A complete, runnable version of this walkthrough lives in
[examples/hello_world](examples/hello_world/) — including how to test it
with `atomostest`.

## Testing

```go
func TestGreeter(t *testing.T) {
	atomostest.Run(t, runnable) // real runtime, logs go to t.Log
	// ...spawn atoms, call messages, assert replies
}
```

For plain unit tests, `atomostest.NewFakeID`, `NewFakeSelfID`,
`NewFakeLogging` and `NewFakeTask` provide in-memory fakes of the framework
interfaces. Note the fakes are stand-ins for testing *your* code; passing one
into framework kernel paths fails with `atomos.ErrFrameworkInvalidID`.

Run the analyzers to catch actor-specific mistakes at build time:

```sh
go install github.com/hwangtou/go-atomos/cmd/atomos-vet@latest
go vet -vettool=$(which atomos-vet) ./...
```

- `syncdeadlock` — a sync message handler that sync-calls its own atom (or a
  typed handler call cycle) deadlocks its mailbox; the analyzer flags it at
  compile time using the generated `<Svc>Atom` interfaces and ID clients.
- `deferrelease` — every `IDTracker` from `CosmosGetAtomID` /
  `CosmosSpawnAtom` must be released via `defer tracker.Release()`,
  `atomos.WithID`, or `atomos.WithRebind`.

## Errors

All public APIs return `error`. Framework failures are `*atomos.Error` with a
frozen numeric code; match them with `errors.Is` against the sentinels
(`atomos.ErrAtomNotExists`, `atomos.ErrFrameworkInvalidID`, ...) or read the
code with `atomos.CodeOf(err)`.

## Repository layout

| Path | Contents |
|---|---|
| root package `atomos` | the framework runtime |
| `atomostest/` | test entry point and fakes |
| `protoc-gen-go-atomos/` | the protoc plugin |
| `cmd/atomos-vet/` | static analyzers (`syncdeadlock`, `deferrelease`) |
| `examples/hello_world/` | minimal runnable application |
| `supervisor/` | standalone process-supervisor utility |
| `docs/` | design documents |

## Documentation

- [docs/DESIGN_V3_API.md](docs/DESIGN_V3_API.md) — v3 API design and migration notes.
- [docs/DEADLOCK.md](docs/DEADLOCK.md) — sync-call deadlock model, three-layer defenses (analyzer, wait-graph, cross-node chains), and runtime handling (`SetDeadlockHook`, `DumpStacks`).
- [docs/DESIGN_DRAIN_GRADUAL.md](docs/DESIGN_DRAIN_GRADUAL.md) — gradual drain design.
- [docs/PROTO_GEN_IMPROVEMENTS.md](docs/PROTO_GEN_IMPROVEMENTS.md) — generator improvement backlog.
