#!/bin/sh
# Build and install protoc-gen-go-atomos into GOBIN (or GOPATH/bin).
set -eu
cd "$(dirname "$0")"
go install .

## Test
#protoc -I. --go_out=. --go_opt=paths=source_relative \
#  --go-atomos_out=. --go-atomos_opt=paths=source_relative for_test.proto
