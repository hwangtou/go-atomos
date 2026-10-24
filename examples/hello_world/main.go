// Command hello_world is the minimal Atomos application: one Greeter element
// with a SayHello atom message, started through the atomos.Run entry point.
//
// Run it with:
//
//	go run ./examples/hello_world
//
// Logs (including the SayHello reply) go to $TMPDIR/atomos-hello_world/log;
// set ATOMOS_LOG_STDOUT=true to log to stdout instead.
package main

import (
	"os"
	"path/filepath"

	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
)

func main() {
	// WithWorkingPath starts the process directly (no daemon fork) with a
	// programmatically built config. Production apps usually keep the config
	// in a YAML file and call plain atomos.Run(runnable) with --config.
	workDir := filepath.Join(os.TempDir(), "atomos-hello_world")
	atomos.Run(newRunnable(&mainScript{}),
		atomos.WithWorkingPath(workDir, "hello", "hello_node", atomos.LogLevel_Debug, nil))
}

// newRunnable assembles the application: main script plus the generated
// Greeter element implementation, marked as a spawn entry so the element is
// spawned at process start.
func newRunnable(script atomos.CosmosMainScript) *atomos.CosmosRunnable {
	return (&atomos.CosmosRunnable{}).
		SetMainScript(script).
		AddElementImplementationWithSpawn(api.GetGreeterImplement(&greeterDeveloper{}))
}
