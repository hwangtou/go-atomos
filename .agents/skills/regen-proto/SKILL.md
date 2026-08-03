---
name: regen-proto
description: >
  Regenerate Go code from atomos.proto (and any other .proto) in the go-atomos
  framework. Use whenever the user wants to generate, regenerate, rebuild, or
  recompile protobuf/gRPC code, or mentions proto generation issues like
  "package mismatch", "go_atomos vs atomos", "protoc", or "pb.go".
---

# regen-proto

Regenerate `atomos.pb.go` and `atomos_grpc.pb.go` from `atomos.proto`.

## Prerequisites

Three tools must be on `PATH`:

```bash
protoc --version                    # protobuf compiler (brew install protobuf, v25+)
which protoc-gen-go                 # google.golang.org/protobuf v1.36.10
which protoc-gen-go-grpc            # google.golang.org/grpc v1.4.0
```

If missing, install the Go plugins (protoc itself via `brew install protobuf`):

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.4.0
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Generate

Run from the **repository root** (`go-atomos/`):

```bash
protoc \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  atomos.proto
```

This regenerates:
- `atomos.pb.go` — message & enum definitions
- `atomos_grpc.pb.go` — gRPC service interface (AtomosRemoteService)

## Verify

After generation, always:

```bash
go build .        # must compile
go test . -count=1 -timeout 120s   # must pass
```

## Common pitfalls

### Package mismatch (`go_atomos` vs `atomos`)

If the generated `atomos.pb.go` has `package go_atomos` instead of `package atomos`, the `go_package` option in `atomos.proto` is wrong.

**Wrong** (protoc-gen-go derives package name from path → `go_atomos`):
```protobuf
option go_package = "github.com/hwangtou/go-atomos";
```

**Correct** (semicolon + explicit package name):
```protobuf
option go_package = "github.com/hwangtou/go-atomos;atomos";
```

The `;atomos` suffix overrides the derived name. Module path can keep its hyphen (`go-atomos`), but the Go package name must be `atomos`.

### Do NOT use protoc-gen-go-atomos for the framework's own proto

The custom `protoc-gen-go-atomos` plugin generates business-layer Actor IDL code (`*_atomos.pb.go`), not framework core messages. Its version is currently out of sync with the framework API — using it on `atomos.proto` produces compile errors. Only use `protoc-gen-go` + `protoc-gen-go-grpc` for `atomos.proto`.

### paths=source_relative is required

Without `paths=source_relative`, protoc creates nested directory structures based on the `go_package` import path. With it, the output files land flat in the current directory (`atomos.pb.go`, not `github.com/hwangtou/go-atomos/atomos.pb.go`).

### Wire compatibility

When modifying `atomos.proto`:
- **Never renumber** existing fields or enum values — they are serialized to etcd and transmitted over gRPC. Renumbering breaks wire compatibility.
- New fields are safe (proto3 ignores unknown fields).
- Removed fields leave gaps in numbering (e.g., field 2 missing) — this is normal and must be preserved.

## Versions

The plugin versions must match the dependencies in `go.mod`:

| Tool | Version | go.mod dependency |
|---|---|---|
| protoc-gen-go | v1.36.10 | `google.golang.org/protobuf v1.36.10` |
| protoc-gen-go-grpc | v1.4.0 | `google.golang.org/grpc v1.79.3` (compatible) |
| protoc | v25+ (v35.1 tested) | — |
