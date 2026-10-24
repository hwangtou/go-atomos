# hello_world

The minimal Atomos application: one `Greeter` element with a `SayHello` atom
message, wired through the code generator and the `atomos.Run` entry point.

## Layout

- `api/hello.proto` — service definition. RPC options declare each method's
  role: `AtomosElementSpawn`, `AtomosAtomSpawn`, `AtomosAtomMethod`.
- `api/gen.sh` — regenerates `hello.pb.go` and `hello_atomos.pb.go`
  (requires `protoc`, `protoc-gen-go` and `protoc-gen-go-atomos` on `PATH`).
- `greeter.go` — the business logic: `greeterDeveloper` (the
  `atomos.ElementDeveloper` factory), `greeterElement` / `greeterAtom`
  (implementing the generated `api.GreeterElement` / `api.GreeterAtom`
  interfaces), and the `atomos.CosmosMainScript` lifecycle hooks.
- `main.go` — assembles the `atomos.CosmosRunnable` and calls `atomos.Run`.
- `hello_test.go` — a full in-process test using `atomostest.Run`.

## Run

```sh
go run ./examples/hello_world
```

The process starts, spawns a `world` atom from `OnStartUp`, sends it a
`SayHello` call and logs the reply. Logs go to
`$TMPDIR/atomos-hello_world/log`; set `ATOMOS_LOG_STDOUT=true` to log to
stdout instead. Stop with Ctrl-C.

This demo uses `atomos.WithWorkingPath` so no config file is needed.
Production apps usually keep the config in a YAML file and call plain
`atomos.Run(runnable)`, passing `--config path/to/config.yaml`.

## Test

```sh
go test ./examples/hello_world/
```

The test starts the real runtime via `atomostest.Run`, spawns an atom, makes
a sync `SayHello` call, verifies `GetGreeterAtomID` returns the running
instance, and kills the atom. For unit tests that do not need a process, see
the fakes in the `atomostest` package (`NewFakeID`, `NewFakeSelfID`,
`NewFakeLogging`).
