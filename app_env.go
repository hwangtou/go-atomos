package atomos

import (
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"strconv"
	"sync"
	"syscall"
)

type appEnv struct {
	config *Config

	executablePath string
	workPath       string
	args           []string
	env            []string

	pid    int
	exitCh chan bool

	// shutdownSignal records which signal triggered graceful shutdown.
	// 0 means "not triggered by a signal" (e.g. ExitApp() was called directly).
	// Read via ShutdownSignal(). Guarded by shutdownMutex because the signal
	// goroutine writes it while other goroutines may read it.
	shutdownSignal os.Signal
	shutdownMutex  sync.RWMutex
}

func (a *appEnv) check() (isRunning bool, processID int, err *Error) {
	if err = a.checkRunPath(); err != nil {
		return false, 0, err.AddStack(nil)
	}
	isRunning, processID, err = a.checkRunPathProcessID()
	if err != nil {
		return isRunning, processID, err.AddStack(nil)
	}
	return false, 0, nil
}

func (a *appEnv) checkRunPath() *Error {
	runPath := a.config.RunPath
	stat, er := os.Stat(runPath)
	if er != nil {
		return NewErrorf(ErrAppEnvRunPathInvalid, "App: Invalid run path. path=(%s),err=(%v)", runPath, er).AddStack(nil)
	}
	if !stat.IsDir() {
		return NewErrorf(ErrAppEnvRunPathInvalid, "App: Run path is not a directory. path=(%s)", runPath).AddStack(nil)
	}

	runTestPath := runPath + "/test"
	if er = ioutil.WriteFile(runTestPath, []byte{}, 0644); er != nil {
		return NewErrorf(ErrAppEnvRunPathInvalid, "App: Run path cannot write. path=(%s),err=(%v)", runTestPath, er).AddStack(nil)
	}
	if er = os.Remove(runTestPath); er != nil {
		return NewErrorf(ErrAppEnvRunPathInvalid, "App: Run path test file cannot delete. path=(%s),err=(%v)", runTestPath, er).AddStack(nil)
	}
	return nil
}

// checkRunPathProcessID reports whether a live instance already holds the PID
// file. It only PROBES — it does not delete stale files. Reclaiming a stale
// PID file is the job of daemon()'s O_EXCL create path, which keeps the
// "check then act" window closed.
func (a *appEnv) checkRunPathProcessID() (bool, int, *Error) {
	runPIDPath := path.Join(a.config.RunPath, a.config.Node+".pid")
	pidBuf, er := ioutil.ReadFile(runPIDPath)
	if er != nil {
		// No file (or unreadable) → no known live instance.
		return false, 0, nil
	}

	processID, er := strconv.ParseInt(string(pidBuf), 10, 64)
	if er != nil {
		// Corrupt PID file → not a live instance; daemon() will reclaim it.
		return false, 0, nil
	}

	proc, er := os.FindProcess(int(processID))
	if er != nil {
		return false, 0, nil
	}
	if er = proc.Signal(syscall.Signal(0)); er == nil {
		// Process responds → a real live instance. Refuse to start.
		return true, int(processID), NewErrorf(ErrAppEnvRunPathPIDIsRunning, "App: Run path pid is running. pid=(%d)", processID).AddStack(nil)
	}
	// No such process (ESRCH) or other error → stale file, not live.
	// Leave it in place; daemon() will reclaim it atomically.
	return false, 0, nil
}

func (a *appEnv) daemon() *Error {
	if !isRunningInDocker() {
		runPIDPath := path.Join(a.config.RunPath, a.config.Node+".pid")
		selfPID := os.Getpid()
		pidBuf := strconv.FormatInt(int64(selfPID), 10)
		// Acquire the PID file atomically with O_EXCL|O_CREATE. This is the
		// lock: whoever successfully creates the file owns the singleton slot.
		// Unlike the previous "Remove then WriteFile" sequence, there is no
		// check-then-act window in which two processes can both decide they're
		// the only instance.
		f, er := os.OpenFile(runPIDPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, pidPerm)
		if er != nil {
			if !os.IsExist(er) {
				return NewErrorf(ErrAppEnvRunPathWritePIDFileFailed, "App: Write pid file failed. path=(%s),err=(%v)", runPIDPath, er).AddStack(nil)
			}
			// File already exists. Three possibilities:
			//  (a) it records OUR OWN pid → daemon() was called again within the
			//      same process (LaunchApp is invoked twice in MainForWorkingPath
			//      / Main). Idempotent: nothing to do, the file is already ours.
			//  (b) it records a live OTHER process → a real concurrent instance,
			//      refuse to start.
			//  (c) it records a dead process → stale leftover, reclaim it.
			live, pid, probeErr := isPIDFileLive(runPIDPath)
			if probeErr != nil {
				return NewErrorf(ErrAppEnvRunPathWritePIDFileFailed, "App: Existing pid file is unreadable. path=(%s),err=(%v)", runPIDPath, probeErr).AddStack(nil)
			}
			if live && pid == selfPID {
				// (a) Idempotent re-entry: file already holds our pid. Skip.
				go a.daemonNotify()
				return nil
			}
			if live {
				// (b) Another live instance owns the slot.
				return NewErrorf(ErrAppEnvRunPathPIDIsRunning, "App: Another instance is running. pid=(%d)", pid).AddStack(nil)
			}
			// (c) Stale file: remove and retry once.
			if er = os.Remove(runPIDPath); er != nil {
				return NewErrorf(ErrAppEnvRunPathWritePIDFileFailed, "App: Stale pid file cannot be removed. path=(%s),err=(%v)", runPIDPath, er).AddStack(nil)
			}
			f, er = os.OpenFile(runPIDPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, pidPerm)
			if er != nil {
				return NewErrorf(ErrAppEnvRunPathWritePIDFileFailed, "App: Write pid file failed after reclaim. path=(%s),err=(%v)", runPIDPath, er).AddStack(nil)
			}
		}
		// Write our PID and close. File is created with O_EXCL so we are the
		// sole owner; writing the PID is best-effort on top of that lock.
		_, _ = f.WriteString(pidBuf)
		_ = f.Close()
	}
	go a.daemonNotify()
	return nil
}

// isPIDFileLive reads a PID file and reports whether the recorded process is
// still running. Returns (false, 0, nil) for a stale/empty file.
func isPIDFileLive(pidPath string) (live bool, pid int, err error) {
	pidBuf, er := ioutil.ReadFile(pidPath)
	if er != nil {
		return false, 0, nil // unreadable → treat as stale, caller will reclaim
	}
	processID, er := strconv.ParseInt(string(pidBuf), 10, 64)
	if er != nil {
		return false, 0, er
	}
	proc, er := os.FindProcess(int(processID))
	if er != nil {
		return false, 0, nil
	}
	if er = proc.Signal(syscall.Signal(0)); er == nil {
		return true, int(processID), nil
	}
	// ESRCH = no such process (stale). Anything else is ambiguous; treat as
	// not-live to allow reclaim, which matches the previous behavior.
	return false, 0, nil
}

func (a *appEnv) daemonNotify() {
	ch := make(chan os.Signal, 1)
	// Catch only graceful-shutdown signals. SIGKILL cannot be caught.
	// We intentionally exclude SIGWINCH, SIGURG, and other runtime signals
	// to avoid goroutine scheduling overhead from the signal channel.
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(ch)
	for {
		select {
		case s := <-ch:
			switch s {
			case syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT:
				// Record which signal triggered the shutdown so observers
				// (MainScript, ops tooling) can distinguish e.g. SIGTERM
				// (container stop) from SIGINT (Ctrl+C) from SIGHUP.
				a.shutdownMutex.Lock()
				a.shutdownSignal = s
				a.shutdownMutex.Unlock()
				// Trigger graceful shutdown: launchAndRun() will receive on
				// exitCh, call CosmosProcess.Stop(), and exit cleanly.
				a.exitCh <- true
				return
			}
		}
	}
}

// ShutdownSignal returns the signal that triggered graceful shutdown, or nil
// if shutdown was initiated by ExitApp() rather than a signal. Safe to call
// from any goroutine, typically during/after the shutdown sequence.
func (a *appEnv) ShutdownSignal() os.Signal {
	a.shutdownMutex.RLock()
	defer a.shutdownMutex.RUnlock()
	return a.shutdownSignal
}

func (a *appEnv) close() {
	if !isRunningInDocker() {
		runPIDPath := path.Join(a.config.RunPath, a.config.Node+".pid")
		_ = os.Remove(runPIDPath)
	}
}

// isRunningInDocker returns true if this process is running inside a Docker
// container. Docker creates the /.dockerenv marker file in every container.
func isRunningInDocker() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}
