package go_atomos

import (
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"strconv"
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
		return NewErrorf(ErrAppEnvRunPathInvalid, "App: Run path is not a directory. err=(%v)", er).AddStack(nil)
	}

	runTestPath := runPath + "/test"
	if er = ioutil.WriteFile(runTestPath, []byte{}, 0644); er != nil {
		return NewErrorf(ErrAppEnvRunPathInvalid, "App: Run path cannot writ. err=(%v)", er).AddStack(nil)
	}
	if er = os.Remove(runTestPath); er != nil {
		return NewErrorf(ErrAppEnvRunPathInvalid, "App: Run path test file cannot delete. err=(%v)", er).AddStack(nil)
	}
	return nil
}

func (a *appEnv) checkRunPathProcessID() (bool, int, *Error) {
	runPIDPath := path.Join(a.config.RunPath, a.config.Node+".pid")
	pidBuf, er := ioutil.ReadFile(runPIDPath)
	if er != nil {
		return false, 0, nil
	}

	processID, er := strconv.ParseInt(string(pidBuf), 10, 64)
	if er != nil {
		return false, 0, NewErrorf(ErrAppEnvRunPathPIDFileInvalid, "App: Run path pid file invalid. err=(%v)", er).AddStack(nil)
	}

	var ok bool
	var errno syscall.Errno
	proc, er := os.FindProcess(int(processID))
	if er != nil {
		// TODO
		goto removePID
	}
	er = proc.Signal(syscall.Signal(0))
	if er == nil {
		return true, int(processID), NewErrorf(ErrAppEnvRunPathPIDIsRunning, "App: Run path pid is running. pid=(%d),err=(%v)", processID, er).AddStack(nil)
	}
	if er.Error() == "os: app already finished" {
		goto removePID
	}
	errno, ok = er.(syscall.Errno)
	if !ok {
		goto removePID
	}
	switch errno {
	case syscall.ESRCH:
		goto removePID
	case syscall.EPERM:
		return true, int(processID), NewErrorf(ErrAppEnvRunPathPIDIsRunning, "App: Run path pid is EPERM. pid=(%d),err=(%v)", processID, er).AddStack(nil)
	}

removePID:
	if er = os.Remove(runPIDPath); er != nil {
		return false, 0, NewErrorf(ErrAppEnvRunPathRemovePIDFailed, "App: Run path pid file removes failed. pid=(%d),err=(%v)", processID, er).AddStack(nil)
	}
	return false, 0, nil
}

func (a *appEnv) daemon() *Error {
	runPIDPath := path.Join(a.config.RunPath, a.config.Node+".pid")
	pidBuf := strconv.FormatInt(int64(os.Getpid()), 10)
	if er := ioutil.WriteFile(runPIDPath, []byte(pidBuf), pidPerm); er != nil {
		return NewErrorf(ErrAppEnvRunPathWritePIDFileFailed, "App: Write pid file failed. err=(%v)", er).AddStack(nil)
	}
	go a.daemonNotify()
	return nil
}

func (a *appEnv) daemonNotify() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch)
	defer signal.Stop(ch)
	for {
		select {
		case s := <-ch:
			switch s {
			case os.Interrupt, os.Kill:
				// TODO
				a.exitCh <- true
				return
			}
		}
	}
}

func (a *appEnv) close() {
	runPIDPath := path.Join(a.config.RunPath, a.config.Node+".pid")
	er := os.Remove(runPIDPath)
	if er != nil {
		// TODO
	}
}
