GOPATH ?= $(shell go env GOPATH)

.PHONY: vet build-vet test

# build-vet builds the defer-release IDTracker-leak analyzer.
build-vet:
	go build -o cmd/atomos-vet-deferrelease/atomos-vet-deferrelease ./cmd/atomos-vet-deferrelease/

# vet runs the framework's custom defer-release analyzer over the whole module.
# It skips test files (which deliberately use bare non-deferred Release).
vet: build-vet
	go vet -vettool=$(CURDIR)/cmd/atomos-vet-deferrelease/atomos-vet-deferrelease ./...

# test runs the standard Go test suite.
test:
	go test ./...
