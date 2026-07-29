package atomos

import (
	"fmt"
	"log"
	"os"
	"path"
	"testing"
)

const (
	//pidPath = "/app.pid"
	// pidPerm is the file mode for the PID file. It must be owner-writable so
	// that repeated daemon() calls (e.g. MainForWorkingPath invokes LaunchApp
	// twice) can overwrite the file without needing to remove it first.
	pidPerm = 0644

	//UDSSocketPath = "/app.socket"
)

type App struct {
	config *Config

	env     *appEnv
	logging appLogging
}

func NewCosmosNodeAppWithWorkingPath(runnable CosmosRunnable, wd, cosmos, node string, logLevel LogLevel, customize map[string][]byte) (*App, *Error) {
	if len(os.Args) < 1 {
		return nil, NewErrorf(ErrRunnableConfigNotFound, "App: Args is invalid.").AddStack(nil)
	}
	// Check whether the working directory is existed, if exists, then use it; if not, then create it.
	if err := UtilFileEnsureDirectory(wd, os.ModePerm|os.ModeDir, true); err != nil {
		return nil, err.AddStack(nil)
	}
	// Join log, run, etc path with os separator.
	logPath := path.Join(wd, "log")
	runPath := path.Join(wd, "run")
	etcPath := path.Join(wd, "etc")
	if err := UtilFileEnsureDirectory(logPath, 0777, true); err != nil {
		return nil, err.AddStack(nil)
	}
	if err := UtilFileEnsureDirectory(runPath, 0777, true); err != nil {
		return nil, err.AddStack(nil)
	}
	if err := UtilFileEnsureDirectory(etcPath, 0777, true); err != nil {
		return nil, err.AddStack(nil)
	}
	logMaxSize := AppLoggingDefaultMaxSize
	if customize == nil {
		customize = map[string][]byte{}
	}
	config := &Config{
		Cosmos:     cosmos,
		Node:       node,
		LogLevel:   logLevel,
		LogPath:    logPath,
		LogMaxSize: logMaxSize,
		BuildPath:  "",
		BinPath:    os.Args[0],
		RunPath:    runPath,
		EtcPath:    etcPath,
		Customize:  customize,
	}
	// Apply env var overrides so ATOMOS_LOG_STDOUT and Docker detection work.
	applyEnvOverrides(config)

	// Choose logging backend: console for Docker/env-var-opt-in, file otherwise.
	var logging appLogging
	var err *Error
	if shouldLogToStdout(config) {
		logging = NewAppLoggingToConsole()
	} else {
		logMaxSize := config.LogMaxSize
		if logMaxSize == 0 {
			logMaxSize = AppLoggingDefaultMaxSize
		}
		logging, err = NewAppLoggingToFile(config.LogPath, logMaxSize, 10, func(err *Error) {
			log.Printf("App: Logging error. err=(%v)", err.AddStack(nil))
		})
		if err != nil {
			return nil, err.AddStack(nil)
		}
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
	// Choose logging backend: console for Docker/env-var-opt-in, file otherwise.
	var logging appLogging
	if shouldLogToStdout(conf) {
		logging = NewAppLoggingToConsole()
	} else {
		logSize := conf.LogMaxSize
		if logSize == 0 {
			logSize = AppLoggingDefaultMaxSize
		}
		logging, err = NewAppLoggingToFile(conf.LogPath, logSize, 10, func(err *Error) {
			log.Printf("App: Logging error. err=(%v)", err.AddStack(nil))
		})
		if err != nil {
			return nil, err.AddStack(nil)
		}
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
	//a.env.env = append(a.env.env, fmt.Sprintf("%s=%s", GetEnvAccessLogKey(), a.logging.getCurAccessLogName()))
	//a.env.env = append(a.env.env, fmt.Sprintf("%s=%s", GetEnvErrorLogKey(), a.logging.getCurErrorLogName()))

	// Fork Process
	attr := &os.ProcAttr{
		Dir:   a.env.workPath,
		Env:   a.env.env,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Sys:   createSysProcAttr(),
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
	a.logging.Close()
}

// Exit

func (a *App) ExitApp() {
	a.env.exitCh <- true
}

func (a *App) WaitExitApp() <-chan bool {
	return a.env.exitCh
}
