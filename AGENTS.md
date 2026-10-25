# AGENTS.md

Guidance for AI agents and contributors working in this repository.

## Repository layout

- Root package `atomos` — the actor-model framework runtime.
- `atomostest/` — test helpers: `Run` (in-process test runtime) and fakes
  (`FakeID`, `FakeSelfID`, `FakeLogging`, `FakeTask`).
- `protoc-gen-go-atomos/` — the protoc plugin that generates element/atom
  interfaces, ID clients and decoders from `.proto` services.
- `cmd/atomos-vet/` — static analyzers (`syncdeadlock`, `deferrelease`).
- `examples/hello_world/` — minimal runnable application, in this module.
- `supervisor/` — standalone process supervisor; **separate Go module**.
- `docs/` — design documents (v3 API, gradual drain, generator backlog).
- `atomos.proto`, `for_test.proto` — wire protocol and framework-test protos.

## Build and test

Requires Go 1.24 or newer (see `go.mod`).

```sh
go build ./...
go vet ./...
go test . -count=1 -timeout 600s          # root package suite (~20s)
go test ./atomostest/ ./cmd/atomos-vet/... ./protoc-gen-go-atomos/ ./examples/...
make test-race                           # all packages in both modules, twice
make test-integration                    # embedded etcd integration suite
make test-integration-race               # integration suite with race detector
GOOS=windows go build ./...               # cross-compile gate
```

The `supervisor/` directory is a separate Unix-only module; build/test it from
its own directory. The Windows cross-compile gate covers the root module only
and intentionally excludes supervisor.

## Proto regeneration

- `atomos.proto` is the frozen wire protocol: **never renumber or repurpose
  existing fields**, and do not change `Error` codes' numeric values.
- `for_test.proto` backs the framework's own tests. Regenerate with:

  ```sh
  protoc -I. --go_out=. --go_opt=paths=source_relative \
    --go-atomos_out=. --go-atomos_opt=paths=source_relative for_test.proto
  ```

  then rename the outputs to `for_test_pb_test.go` and
  `for_test_atomos_pb_test.go` (they must stay test-only to keep the export
  surface clean; moving them to a subpackage would create an import cycle).
  Do not change its `go_package`.
- Service methods declare their role with the explicit
  `option (atomos.atomos_method) = AtomosElementSpawn | AtomosElementMethod |
  AtomosAtomSpawn | AtomosAtomMethod;` — always write the option explicitly.
- The generator has a golden-file test: `go test ./protoc-gen-go-atomos/`;
  refresh goldens with `go test ./protoc-gen-go-atomos/ -update` after
  intended generator changes.

## Conventions

- Public API returns `error` (framework failures are `*atomos.Error`); match
  with `errors.Is` against the sentinels in `error_sentinel.go`, read codes
  with `atomos.CodeOf`. Error codes are frozen — never change existing
  numeric values.
- Optional call parameters use variadic marker arguments
  (`ext ...atomos.ArgsForBaseAtomos`), e.g. `atomos.ArgBaseAtomosTimeout(d)`,
  `atomos.ArgBaseAtomosFromLocalOnly()`.
- Exported symbols carry English godoc; internal implementation comments may
  be in Chinese.
- Commits: English Conventional Commits; breaking changes use `!` and carry a
  migration note in the commit body.

## Common pitfalls

- **Self-call deadlock**: a sync handler that sync-calls its own atom (or a
  cycle across handlers) deadlocks the mailbox. Run
  `go vet -vettool=$(which atomos-vet) ./...` (`syncdeadlock` analyzer).
- **IDTracker leaks**: every tracker from `CosmosGetAtomID` /
  `CosmosSpawnAtom` / generated `Get*AtomID` / `Spawn*Atom` needs
  `defer tracker.Release()`, `atomos.WithID`, or `atomos.WithRebind`
  (`deferrelease` analyzer).
- **Fake IDs in kernel paths**: `atomostest` fakes are for testing user
  business logic only; the framework rejects them with
  `ErrFrameworkInvalidID` by design.
- **Commented-out tests**: `atom_local_test.go` and large parts of
  `nodes_test.go` / `simulate_test.go` are intentionally commented out — do
  not "fix" or delete them as part of unrelated work.
