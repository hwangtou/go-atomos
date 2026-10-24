GOPATH ?= $(shell go env GOPATH)

.PHONY: vet build-vet test test-root test-race test-integration bench cover cross-build

# build-vet builds the combined go-atomos vet analyzer (multichecker: defer-release + sync-deadlock).
build-vet:
	go build -o cmd/atomos-vet/atomos-vet ./cmd/atomos-vet/

# vet runs all go-atomos analyzers over the whole module in a single pass.
# It skips test files (which deliberately use bare non-deferred Release).
vet: build-vet
	go vet -vettool=$(CURDIR)/cmd/atomos-vet/atomos-vet ./...

# test runs the standard Go test suite (root + subpackages + supervisor module).
test:
	go test ./...
	cd supervisor && go test ./...

# test-race runs the root package under the race detector, twice: the second
# run catches state leaked across tests that only manifests on reuse
# (mailbox goroutines, package-level vars).
test-race:
	go test . -race -count=2 -timeout 900s

# test-integration runs the tests behind the "integration" build tag, against
# an embedded etcd (real lease/watch/txn semantics): version lock lifecycle
# and CAS races, drain broadcast, watch fault injection (server restart, lease
# expiry, dirty data), TLS handshake matrix, keepalive outage recovery. Needs
# no external etcd; everything runs in-process on ephemeral ports. The
# gRPC-level chaos tests live in the default suite (chaos_test.go).
test-integration:
	go test . -tags integration -count=1 -timeout 900s

# bench runs the root package benchmarks.
bench:
	go test . -run '^$$' -bench . -benchtime 1s

# cover writes root-package statement coverage to cover.out and prints a summary.
cover:
	go test . -count=1 -timeout 300s -coverprofile=cover.out
	go tool cover -func=cover.out | tail -1

# cross-build gate for Windows support.
cross-build:
	GOOS=windows go build ./...
