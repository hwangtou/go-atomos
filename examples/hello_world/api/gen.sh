#!/bin/sh
# Regenerate the Go bindings for hello.proto.
# Requires protoc, protoc-gen-go and protoc-gen-go-atomos on PATH.
# Install the plugins with:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install github.com/hwangtou/go-atomos/protoc-gen-go-atomos@latest
set -eu
cd "$(dirname "$0")"

# -I../../.. makes the framework's atomos.proto importable.
protoc -I. -I../../.. \
  --go_out=. --go_opt=paths=source_relative \
  --go-atomos_out=. --go-atomos_opt=paths=source_relative \
  hello.proto
