package go_atomos

import (
	"fmt"
	"os"
	"path"
	"syscall"
	"testing"
)

const (
	//pidPath = "/app.pid"
	pidPerm = 0444

	//UDSSocketPath = "/app.socket"
)

type App struct {
	config *Config

	env     *appEnv
	logging appLoggingIntf
}

func NewCosmosNodeAppWithWorkingPath(runnable CosmosRunnable, wd, cosmos, node string, logLevel LogLevel, customize map[string][]byte) (*App, *Error) {
	if len(os.Args) < 1 {
		return nil, NewErrorf(ErrRunnableConfigInvalid, "App: Args is invalid.").AddStack(nil)
	}
	// Check whether the working directory is existed, if exists, then use it; if not, then create it.
	if err := ensureDirectory(wd); err != nil {
		return nil, err.AddStack(nil)
	}
	// Join log, run, etc path with os separator.
	logPath := path.Join(wd, "log")
	runPath := path.Join(wd, "run")
	etcPath := path.Join(wd, "etc")
	if err := ensureDirectory(logPath); err != nil {
		return nil, err.AddStack(nil)
	}
	if err := ensureDirectory(runPath); err != nil {
		return nil, err.AddStack(nil)
	}
	if err := ensureDirectory(etcPath); err != nil {
		return nil, err.AddStack(nil)
	}
	// Open Log.
	logMaxSize := defaultLogMaxSize
	logging, err := NewAppLogging(logPath, logMaxSize)
	if err != nil {
		err = err.AddStack(nil)
		return nil, err.AddStack(nil)
	}
	// Create App instance.
	if customize == nil {
		customize = map[string][]byte{}
	}
	config := &Config{
		Cosmos:         cosmos,
		Node:           node,
		LogLevel:       logLevel,
		LogPath:        logPath,
		LogMaxSize:     int64(logMaxSize),
		WorkingPath:    wd,
		BuildPath:      "",
		BinPath:        os.Args[0],
		RunPath:        runPath,
		EtcPath:        etcPath,
		EnableCluster:  nil,
		EnableElements: nil,
		Customize:      customize,
	}
	return &App{
		config: config,
		env: &appEnv{
			config:         config,
			executablePath: "",
			workPath:       "",
			args:           nil,
			env:            nil,
			pid:            0,
			exitCh:         make(chan bool, 1),
		},
		logging: logging,
	}, nil
}

func NewCosmosNodeAppWithConfigPath(configPath string, runnable *CosmosRunnable) (*App, *Error) {
	// Load Config.
	conf, err := NewCosmosNodeConfigFromYamlPath(configPath, runnable)
	if err != nil {
		return nil, err.AddStack(nil)
	}
	// Open Log.
	logSize := int(conf.LogMaxSize)
	if logSize == 0 {
		logSize = defaultLogMaxSize
	}
	logging, err := NewAppLogging(conf.LogPath, logSize)
	if err != nil {
		err = err.AddStack(nil)
		return nil, err.AddStack(nil)
	}
	return &App{
		config: conf,
		env: &appEnv{
			config:         conf,
			executablePath: "",
			workPath:       "",
			args:           nil,
			env:            nil,
			pid:            0,
			exitCh:         make(chan bool, 1),
		},
		logging: logging,
	}, nil
}

func NewCosmosNodeAppWithTest(config *Config, t *testing.T) (*App, *Error) {
	return &App{
		config: config,
		env: &appEnv{
			config:         config,
			executablePath: "",
			workPath:       "",
			args:           nil,
			env:            nil,
			pid:            0,
			exitCh:         make(chan bool, 1),
		},
		logging: &appLoggingForTest{t: t},
	}, nil
}

// Check

func (a *App) Check() (isRunning bool, processID int, err *Error) {
	// Env
	if isRunning, processID, err = a.env.check(); err != nil {
		return isRunning, processID, err.AddStack(nil)
	}
	return false, 0, nil
}

func (a *App) GetConfig() *Config {
	return a.config
}

// Parent & Child Process

func IsParentProcess() bool {
	return os.Getenv(GetEnvAppKey()) != "1"
}

func GetEnvAppKey() string {
	return fmt.Sprintf("_GO_ATOMOS_APP")
}

func GetEnvAccessLogKey() string {
	return fmt.Sprintf("_ACCESS_LOG")
}

func GetEnvErrorLogKey() string {
	return fmt.Sprintf("_ERROR_LOG")
}

// Parent

func (a *App) ForkAppProcess() *Error {
	var proc *os.Process
	execPath, er := os.Executable()
	if er != nil {
		return NewErrorf(ErrAppEnvGetExecutableFailed, "App: Launching, get executable path failed. err=(%v)", er).AddStack(nil)
	}
	wd, er := os.Getwd()
	if er != nil {
		return NewErrorf(ErrAppEnvGetExecutableFailed, "App: Launching, get work path failed. err=(%v)", er).AddStack(nil)
	}

	a.env.executablePath = execPath
	a.env.workPath = wd

	if len(a.env.args) == 0 {
		a.env.args = os.Args
	}

	a.env.env = os.Environ()
	a.env.env = append(a.env.env, fmt.Sprintf("%s=%s", GetEnvAppKey(), "1"))
	a.env.env = append(a.env.env, fmt.Sprintf("%s=%s", GetEnvAccessLogKey(), a.logging.getCurAccessLogName()))
	a.env.env = append(a.env.env, fmt.Sprintf("%s=%s", GetEnvErrorLogKey(), a.logging.getCurErrorLogName()))

	// Fork Process
	attr := &os.ProcAttr{
		Dir:   a.env.workPath,
		Env:   a.env.env,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	}
	proc, er = os.StartProcess(a.env.executablePath, a.env.args, attr)
	if er != nil {
		return NewErrorf(ErrAppEnvLaunchedFailed, "App: Launching, start process failed. err=(%v)", er).AddStack(nil)
	}
	a.env.pid = proc.Pid
	return nil
}

// Child

func (a *App) LaunchApp() *Error {
	if err := a.env.daemon(); err != nil {
		return err.AddStack(nil)
	}
	//if err := a.socket.daemon(); err != nil {
	//	return err.AddStack(nil)
	//}
	return nil
}

// Close

func (a *App) close() {
	//a.socket.close()
	a.env.close()
	//a.logging.Close()
}

// Exit

func (a *App) ExitApp() {
	a.env.exitCh <- true
}

func (a *App) WaitExitApp() <-chan bool {
	return a.env.exitCh
}

// Utils

func ensureDirectory(path string) *Error {
	pathStat, er := os.Stat(path)
	if os.IsNotExist(er) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return NewErrorf(ErrAppEnvCreateWorkDirFailed, "App: Create working directory failed. err=(%v)", err).AddStack(nil)
		}
	} else {
		if !pathStat.IsDir() {
			return NewErrorf(ErrAppEnvCreateWorkDirFailed, "App: Working directory is not a directory. path=(%s)", path).AddStack(nil)
		}
	}
	return nil
}
