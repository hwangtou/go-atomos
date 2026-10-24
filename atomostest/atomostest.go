// Package atomostest provides test helpers for the Atomos framework.
//
// Run is the test-mode process entry point, replacing the former
// atomos.MainForTest: logs go to t.Log instead of files, the PID-file check
// and daemon fork are skipped, and startup failures call t.Fatalf instead of
// os.Exit (os.Exit in a test binary would abort every other test).
//
// The package also provides in-memory fakes for unit-testing business logic
// without starting a process:
//
//   - FakeID implements atomos.ID with configurable per-message replies and
//     a recorded call history.
//   - FakeSelfID implements atomos.SelfID (embedding FakeID) with a fake
//     logger, a synchronously-executing fake task scheduler, and settable
//     CosmosMain/Config/AsyncCallbackPending.
//   - FakeLogging implements atomos.Logging by recording entries for
//     assertion.
//
// The fakes are stand-ins for testing user code. They are not
// framework-internal IDs: passing one into a framework kernel path (for
// example as the callerID of a real actor) fails explicitly with
// atomos.ErrFrameworkInvalidID, by design.
package atomostest

import (
	"os"
	"testing"

	atomos "github.com/hwangtou/go-atomos"
)

// Run starts an Atomos Cosmos process for a test. The runnable must already
// carry its Config (via SetConfig); logs are routed to t.Log and any startup
// failure fails the test via t.Fatalf.
//
// opts are accepted for symmetry with atomos.Run and reserved for future
// test-mode options; they are currently ignored.
func Run(t *testing.T, runnable *atomos.CosmosRunnable, opts ...atomos.RunOption) {
	t.Helper()
	t.Logf("Welcome to Atomos! pid=(%d)", os.Getpid())

	app, err := atomos.NewCosmosNodeAppWithTest(runnable.GetConfig(), t)
	if err != nil {
		t.Fatalf("App: Config is invalid. pid=(%d),err=(%v)", os.Getpid(), err)
	}

	process, err := app.NewCosmosProcess()
	if err != nil {
		t.Fatalf("App: Init cosmos process failed. pid=(%d),err=(%v)", os.Getpid(), err)
	}

	runnable.SetConfig(app.GetConfig())
	if err := process.Start(runnable); err != nil {
		t.Fatalf("App: Runnable starts failed, now exiting. err=(%v)", err)
	}
	t.Log("App: Started.")
}
