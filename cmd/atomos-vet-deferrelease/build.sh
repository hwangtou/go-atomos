#!/bin/sh
# Build the defer-release vet analyzer into the cmd directory.
go build -o cmd/atomos-vet-deferrelease/atomos-vet-deferrelease ./cmd/atomos-vet-deferrelease/

# Optionally install into $GOPATH/bin so `go vet -vettool=$(which ...) ./...` finds it.
GOPATH="${GOPATH:-$(go env GOPATH)}"
if [ -n "$GOPATH" ] && [ -d "$GOPATH/bin" ]; then
	cp cmd/atomos-vet-deferrelease/atomos-vet-deferrelease "$GOPATH/bin/atomos-vet-deferrelease"
fi

## Usage:
# go vet -vettool=$(which atomos-vet-deferrelease) ./...
