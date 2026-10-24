package atomos

// App-level lifecycle coverage (PID-file management, Check, ExitApp) that
// the commented-out v2 TestAppLogging/TestCosmosMain used to carry: the
// daemon paths that don't need a real fork are exercised in-process; the
// fork/daemon child flow is covered end-to-end by examples/hello_world's
// process-level test.

import (
	"os"
	"os/exec"
	"path"
	"strconv"
	"testing"
	"time"
)

func newWorkingPathApp(t *testing.T) *App {
	t.Helper()
	app, err := NewCosmosNodeAppWithWorkingPath(CosmosRunnable{}, t.TempDir(), "appTest", "appNode", LogLevel_Debug, nil)
	if err != nil {
		t.Fatalf("NewCosmosNodeAppWithWorkingPath: %v", err)
	}
	return app
}

func TestApp_LaunchCheckExitRoundTrip(t *testing.T) {
	if isRunningInDocker() {
		t.Skip("PID-file management is disabled inside Docker")
	}
	app := newWorkingPathApp(t)

	// Launch writes our own PID and starts the notify loop.
	if err := app.LaunchApp(); err != nil {
		t.Fatalf("LaunchApp: %v", err)
	}
	pidPath := path.Join(app.config.RunPath, app.config.Node+".pid")
	buf, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	if got, want := string(buf), strconv.Itoa(os.Getpid()); got != want {
		t.Fatalf("pid file = %q, want %q", got, want)
	}

	// Check must see the (live) PID as running: isRunning=true with the
	// PIDIsRunning error as the signal.
	isRunning, pid, err := app.Check()
	if !isRunning || pid != os.Getpid() || CodeOf(err) != ErrCodeAppEnvRunPathPIDIsRunning {
		t.Fatalf("Check = (%v,%d,%v), want (true,%d,ErrCodeAppEnvRunPathPIDIsRunning)", isRunning, pid, err, os.Getpid())
	}

	// Double launch is idempotent (working-path mode calls LaunchApp twice).
	if err := app.LaunchApp(); err != nil {
		t.Fatalf("second LaunchApp: %v", err)
	}

	// ExitApp wakes WaitExitApp.
	app.ExitApp()
	select {
	case <-app.WaitExitApp():
	case <-time.After(2 * time.Second):
		t.Fatal("WaitExitApp did not fire after ExitApp")
	}

	// close removes the PID file; Check then reports not running.
	app.close()
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file still present after close: %v", err)
	}
	isRunning, _, err = app.Check()
	if err != nil || isRunning {
		t.Fatalf("Check after close = (%v,_,%v), want (false,_,nil)", isRunning, err)
	}
}

func TestApp_CheckStalePIDIsCleaned(t *testing.T) {
	if isRunningInDocker() {
		t.Skip("PID-file management is disabled inside Docker")
	}
	app := newWorkingPathApp(t)

	// Produce a PID that is guaranteed dead: run a trivial child and wait
	// it out.
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatalf("spawn sacrificial process: %v", err)
	}
	pidPath := path.Join(app.config.RunPath, app.config.Node+".pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(dead.Process.Pid)), 0644); err != nil {
		t.Fatalf("write stale pid: %v", err)
	}

	isRunning, _, err := app.Check()
	if err != nil || isRunning {
		t.Fatalf("Check with stale pid = (%v,_,%v), want (false,_,nil)", isRunning, err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatal("stale pid file was not removed by Check")
	}
}

func TestApp_CheckGarbagePIDFails(t *testing.T) {
	if isRunningInDocker() {
		t.Skip("PID-file management is disabled inside Docker")
	}
	app := newWorkingPathApp(t)
	pidPath := path.Join(app.config.RunPath, app.config.Node+".pid")
	if err := os.WriteFile(pidPath, []byte("not-a-pid"), 0644); err != nil {
		t.Fatalf("write garbage pid: %v", err)
	}

	_, _, err := app.Check()
	if err == nil || CodeOf(err) != ErrCodeAppEnvRunPathPIDFileInvalid {
		t.Fatalf("Check with garbage pid = %v, want ErrCodeAppEnvRunPathPIDFileInvalid", err)
	}
}
