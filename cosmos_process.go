package go_atomos

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
)

// Cosmos生命周期，开发者定义和使用的部分。
// Cosmos Life Cycle, part for developer customize and use.

// CosmosProcess
// 这个才是进程的主循环。

type CosmosProcess struct {
	sharedLog *LoggingAtomos

	//// Telnet
	//telnet *cosmosTelnet

	// A channel focus on Daemon Command.
	cmdCh chan Command

	// CosmosRunnable & CosmosMain.
	// 可运行Cosmos & Cosmos运行时。

	// Loads at NewCosmosCycle & Daemon.
	// State
	mutex   sync.Mutex
	running bool

	// Loads at DaemonWithRunnable or Runnable.
	main *CosmosMain
}

// Cosmos生命周期
// Cosmos Life Cycle

func NewCosmosProcess() (*CosmosProcess, *ErrorInfo) {
	c := &CosmosProcess{
		// Cosmos log is initialized once and available all the time.
		sharedLog: NewLoggingAtomos(),
		//telnet:      nil,
		cmdCh:   make(chan Command),
		mutex:   sync.Mutex{},
		running: false,
		main:    nil,
	}
	//c.telnet = newCosmosTelnet(c)
	//if err := c.telnet.init(); err != nil {
	//	return nil, err
	//}

	return c, nil
}

// Daemon
// DaemonWithRunnable会一直阻塞直至收到终结信号，或者脚本已经执行完毕。
// DaemonWithRunnable will block until stop signal received or script terminated.
func (c *CosmosProcess) Daemon() (chan struct{}, *ErrorInfo) {
	closeCh := make(chan struct{}, 1)
	runCh := make(chan struct{}, 1)
	go func() {
		defer c.Logging(LogLevel_Info, "CosmosProcess: Exited, bye!")
		defer func() {
			closeCh <- struct{}{}
		}()
		mainExitCh := make(chan *ErrorInfo, 1)
		exit := false
		for {
			runCh <- struct{}{}
			select {
			case cmd := <-c.cmdCh:
				if cmd == nil {
					c.Logging(LogLevel_Fatal, "CosmosProcess: Invalid runnable")
					break
				}
				if err := cmd.Check(); err != nil {
					c.Logging(LogLevel_Fatal, "CosmosProcess: Main check failed, err=(%v)", err.Message)
					break
				}
				switch cmd.Type() {
				case CommandExit:
					if !c.isRunning() {
						c.Logging(LogLevel_Info, "CosmosProcess: Cannot stop runnable, because it's not running")
						break
					}
					main := c.main
					if main != nil {
						select {
						case main.mainKillCh <- true:
							exit = true
							//c.main.pushKillMail(nil, true)
						default:
							c.Logging(LogLevel_Info, "Main: Exit error, err=(Runnable is blocking)")
						}
					}
				case CommandStopRunnable:
					if !c.isRunning() {
						c.Logging(LogLevel_Info, "CosmosProcess: Cannot stop runnable, because it's not running")
						break
					}
					main := c.main
					if main != nil {
						select {
						case main.mainKillCh <- true:
							//c.main.pushKillMail(nil, true)
						default:
							c.Logging(LogLevel_Info, "Main: Exit error, err=(Runnable is blocking)")
						}
					}
				case CommandExecuteRunnable:
					if !c.trySetRunning(true) {
						c.Logging(LogLevel_Fatal, "CosmosProcess: Cannot execute runnable, because it's running")
						break
					}
					conf := cmd.GetConfig()
					runnable := cmd.GetRunnable()
					// Daemon initialize.
					// Check config.
					var err *ErrorInfo
					if err = conf.Check(); err != nil {
						c.Logging(LogLevel_Fatal, "CosmosProcess: Main config check failed, err=(%v)", err.Message)
						c.trySetRunning(false)
						break
					}
					// Run main.
					c.main = newCosmosMain(c, runnable)

					// 后台驻留执行可执行命令。
					// Daemon execute executable command.
					// 让本地的Cosmos去初始化Runnable中的各种内容，主要是Element相关信息的加载。
					// Make CosmosMain initial the content of Runnable, especially the Element information.
					err = c.main.onceLoad(runnable)
					if err != nil {
						c.Logging(LogLevel_Fatal, "CosmosProcess: Main init failed") //, err=(%v)", err.Message)
						c.trySetRunning(false)
						break
					}
					go func() {
						// 最后执行Runnable的清理相关动作，还原Cosmos的原状。
						// At last, clean up the Runnable, recover Cosmos.
						defer c.trySetRunning(false)
						// Stopped
						defer func() {
							mainExitCh <- nil
						}()
						defer func() {
							err = c.main.pushKillMail(nil, true)
							c.Logging(LogLevel_Info, "Main: EXITED!")
						}()

						// 防止Runnable中的Script崩溃导致程序崩溃。
						// To prevent panic from the Runnable Script.
						defer func() {
							if r := recover(); r != nil {
								c.Logging(LogLevel_Fatal, "Main: Main script CRASH! reason=(%s)", r)
							}
						}()
						// 执行Runnable。
						// Execute runnable.
						c.Logging(LogLevel_Info, "Main: NOW RUNNING!")
						runnable.mainScript(c.main, c.main.mainKillCh)
						c.Logging(LogLevel_Info, "Main: Execute runnable succeed.")
					}()
				case CommandReloadRunnable:
					if !c.isRunning() {
						c.Logging(LogLevel_Info, "CosmosProcess: Cannot execute runnable reload, because it's not running.")
						break
					}
					conf := cmd.GetConfig()
					runnable := cmd.GetRunnable()
					// Daemon initialize.
					// Check config.
					var err *ErrorInfo
					if err = conf.Check(); err != nil {
						c.Logging(LogLevel_Fatal, "CosmosProcess: Main config check failed, err=(%v)", err.Message)
						c.trySetRunning(false)
						break
					}
					// Run main.
					main := c.main
					if main != nil {
						err = c.main.pushReloadMail(nil, runnable, c.main.atomos.reloads+1)
						//err = c.main.reload(cmd.GetRunnable(), c.reloads)
						if err != nil {
							c.Logging(LogLevel_Fatal, fmt.Sprintf("CosmosProcess: Execute runnable reload failed, err=(%v)", err))
						} else {
							c.Logging(LogLevel_Info, "CosmosProcess: Execute runnable reload succeed")
						}
					}
				}
			case err := <-mainExitCh:
				if err != nil {
					c.Logging(LogLevel_Error, fmt.Sprintf("CosmosProcess: Exited, err=(%v)", err))
				} else {
					c.Logging(LogLevel_Info, "CosmosProcess: Exited")
				}
				if exit {
					return
				}
			}
		}
	}()
	<-runCh
	return closeCh, nil
}

func (c *CosmosProcess) Send(command Command) *ErrorInfo {
	select {
	case c.cmdCh <- command:
		return nil
	default:
		if c.cmdCh == nil {
			return NewErrorf(ErrCosmosIsClosed, "Send to close, cmd=(%v)", command)
		} else {
			return NewErrorf(ErrCosmosIsBusy, "Send to busy, cmd=(%v)", command)
		}
	}
}

func (c *CosmosProcess) WaitKillSignal() {
	ch := make(chan os.Signal)
	signal.Notify(ch)
	defer signal.Stop(ch)
	for {
		select {
		case s := <-ch:
			switch s {
			case os.Interrupt:
				fmt.Println()
				fallthrough
			case os.Kill:
				if err := c.Send(NewExitCommand()); err != nil {
					c.Logging(LogLevel_Fatal, "CosmosProcess: WaitKillSignal killed atomos failed, err=(%v)", err)
					continue
				}
				c.Logging(LogLevel_Info, "CosmosProcess: WaitKillSignal killed atomos")
				return
			}
		}
	}
}

// Daemon结束后Close。
// Close after Daemon run.
func (c *CosmosProcess) Close() {
	//if c.log != nil {
	//	c.log.log.waitStop()
	//	c.log = nil
	//}
	c.cmdCh = nil
}

func (c *CosmosProcess) trySetRunning(run bool) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.running == run {
		return false
	}
	c.running = run
	return true
}

func (c *CosmosProcess) isRunning() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.running
}

func (c *CosmosProcess) Logging(level LogLevel, fmt string, args ...interface{}) {
	c.sharedLog.PushProcessLog(level, fmt, args...)
}

// 命令
// Command

type Command interface {
	Type() CommandType
	GetConfig() *Config
	GetRunnable() *CosmosRunnable
	Check() *ErrorInfo
}

// 命令类型
// Command Type
type CommandType int

const (
	CommandExit = 0
	// 运行Runnable
	CommandExecuteRunnable = 1
	CommandStopRunnable    = 2
	CommandReloadRunnable  = 3
)

func NewRunnableCommand(runnable *CosmosRunnable) Command {
	return &command{
		cmdType:  CommandExecuteRunnable,
		runnable: runnable,
	}
}

func NewRunnableReloadCommand(runnable *CosmosRunnable) Command {
	return &command{
		cmdType:  CommandReloadRunnable,
		runnable: runnable,
	}
}

func NewStopCommand() Command {
	return &command{
		cmdType:  CommandStopRunnable,
		runnable: nil,
	}
}

func NewExitCommand() Command {
	return &command{
		cmdType:  CommandExit,
		runnable: nil,
	}
}

// Command具体实现
type command struct {
	cmdType  CommandType
	runnable *CosmosRunnable
}

func (c command) Type() CommandType {
	return c.cmdType
}

func (c command) GetRunnable() *CosmosRunnable {
	return c.runnable
}

func (c command) GetConfig() *Config {
	return c.runnable.config
}

func (c command) Check() *ErrorInfo {
	switch c.cmdType {
	case CommandExit, CommandStopRunnable:
		return nil
	case CommandExecuteRunnable:
		if c.runnable == nil {
			return NewError(ErrProcessRunnableInvalid, "Command: Runnable is nil")
		}
		if c.runnable.mainScript == nil {
			return NewError(ErrProcessRunnableInvalid, "Command: Main script is nil")
		}
		return nil
	case CommandReloadRunnable:
		if c.runnable == nil {
			return NewError(ErrProcessRunnableInvalid, "Command: Runnable is nil")
		}
		if c.runnable.reloadScript == nil {
			return NewError(ErrProcessRunnableInvalid, "Command: Reload script is nil")
		}
		return nil
	default:
		return NewError(ErrProcessRunnableInvalid, "Command: Unknown command type")
	}
}
