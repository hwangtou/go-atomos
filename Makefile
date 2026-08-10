GOPATH ?= $(shell go env GOPATH)

.PHONY: vet build-vet test

# build-vet builds the combined go-atomos vet analyzer (multichecker: defer-release + sync-deadlock).
build-vet:
	go build -o cmd/atomos-vet/atomos-vet ./cmd/atomos-vet/

# vet runs all go-atomos analyzers over the whole module in a single pass.
# It skips test files (which deliberately use bare non-deferred Release).
vet: build-vet
	go vet -vettool=$(CURDIR)/cmd/atomos-vet/atomos-vet ./...

# test runs the standard Go test suite.
test:
	go test ./...
