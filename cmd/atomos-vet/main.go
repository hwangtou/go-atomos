// Command atomos-vet is a go vet driver combining all go-atomos static
// analyzers. Run via:
//
//	go vet -vettool=$(which atomos-vet) ./...
//
// It runs each analyzer on every non-test, non-generated package. The analyzers
// are independent packages under cmd/atomos-vet/.
package main

import (
	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/hwangtou/go-atomos/cmd/atomos-vet/deferrelease"
	"github.com/hwangtou/go-atomos/cmd/atomos-vet/syncdeadlock"
)

func main() {
	multichecker.Main(
		deferrelease.Analyzer,
		syncdeadlock.Analyzer,
	)
}
