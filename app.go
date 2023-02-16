package go_atomos

import (
	"fmt"
	"os"
	"sync"
	"syscall"
)

const (
	//pidPath = "/app.pid"
	pidPerm = 0444

	//UDSSocketPath = "/app.socket"
)

type App struct {
	config *Config

	env     *appEnv
	logging *appLogging
	socket  *appUDSServer
}

func NewCosmosNodeApp(configPath string) (*App, *Error) {
	// Load Config.
	conf, err := NewCosmosNodeConfigFromYamlPath(configPath)
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
		socket: &appUDSServer{
			config:         conf,
			logging:        logging,
			addr:           nil,
			listener:       nil,
			mutex:          sync.Mutex{},
			connID:         0,
			connMap:        map[int32]*AppUDSConn{},
			commandHandler: udsNodeCommandHandler,
		},
	}, nil
}

func NewCosmosSupervisorApp(configPath string) (*App, *Error) {
	// Load Config.
	conf, err := NewSupervisorConfigFromYaml(configPath)
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
		socket: &appUDSServer{
			config:         conf,
			logging:        logging,
			addr:           nil,
			listener:       nil,
			mutex:          sync.Mutex{},
			connID:         0,
			connMap:        map[int32]*AppUDSConn{},
			commandHandler: supervisorCommandHandlers,
		},
	}, nil
}

// Check

func (a *App) Check() (isRunning bool, processID int, err *Error) {
	// Env
	if isRunning, processID, err = a.env.check(); err != nil {
		return isRunning, processID, err.AddStack(nil)
	}
	// Socket
	if err = a.socket.check(); err != nil {
		return false, 0, err.AddStack(nil)
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
	a.env.env = append(a.env.env, fmt.Sprintf("%s=%s", GetEnvAccessLogKey(), a.logging.curAccessLogName))
	a.env.env = append(a.env.env, fmt.Sprintf("%s=%s", GetEnvErrorLogKey(), a.logging.curErrorLogName))

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
	if err := a.socket.daemon(); err != nil {
		return err.AddStack(nil)
	}
	return nil
}

// Close

func (a *App) close() {
	a.socket.close()
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
