package temp

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
)

// Cosmos生命周期，开发者定义和使用的部分。
// Cosmos Life Cycle, part for developer customize and use.

// CosmosProcess
// 这个才是进程的主循环。
type CosmosProcess struct {
	// CosmosCycle
	// Cosmos循环

	// Loads at NewCosmosCycle & Daemon.
	// State
	mutex   sync.Mutex
	running bool
	//// Config
	//config *Config
	// A channel focus on Daemon Command.
	daemonCmdCh chan DaemonCommand
	reloads     int

	////atomos *baseAtomos
	//sharedLog *LoggingAtomos

	//// CosmosRunnable & CosmosMainFn.
	//// 可运行Cosmos & Cosmos运行时。
	//
	//// Loads at DaemonWithRunnable or Runnable.
	//main *CosmosMainFn

	//// 集群助手，帮助访问远程的Cosmos。
	//// Cluster helper helps access to remote Cosmos.
	//remotes *cosmosRemoteServer
	//
	//// Telnet
	//telnet *cosmosTelnet
}

func newCosmosProcess() (*CosmosProcess, *ErrorInfo) {
	c := &CosmosProcess{
		mutex:       sync.Mutex{},
		running:     false,
		daemonCmdCh: make(chan DaemonCommand),
		reloads:     0,
		// Cosmos log is initialized once and available all the time.
		sharedLog: NewLoggingAtomos(),
		main:      nil,
		//remotes:     nil,
		//telnet:      nil,
	}
	//c.telnet = newCosmosTelnet(c)
	//if err := c.telnet.init(); err != nil {
	//	return nil, err
	//}

	return c, nil
}

// Cosmos生命周期
// Cosmos Life Cycle

// Daemon
// DaemonWithRunnable会一直阻塞直至收到终结信号，或者脚本已经执行完毕。
// DaemonWithRunnable will block until stop signal received or script terminated.
func (c *CosmosProcess) Daemon() (chan struct{}, *ErrorInfo) {
	closeCh := make(chan struct{}, 1)
	runCh := make(chan struct{}, 1)
	go func() {
		defer func() {
			c.logging(LogLevel_Info, "CosmosProcess: Exited, bye!")
		}()
		defer func() {
			closeCh <- struct{}{}
		}()
		daemonCh := make(chan *ErrorInfo, 1)
		for {
			exit := false
			runCh <- struct{}{}
			select {
			case cmd := <-c.daemonCmdCh:
				if cmd == nil {
					c.logging(LogLevel_Fatal, "CosmosProcess: Invalid runnable")
					break
				}
				if err := cmd.Check(); err != nil {
					c.logging(LogLevel_Fatal, "CosmosProcess: MainFn config check failed, err=(%v)", err.Message)
					break
				}
				switch cmd.Type() {
				case DaemonCommandExit:
					if !c.isRunning() {
						c.logging(LogLevel_Info, "CosmosProcess: Cannot stop runnable, because it's not running")
						break
					}
					select {
					case c.main.mainKillCh <- true:
						exit = true
						//c.main.pushKillMail(nil, true)
					default:
						c.logging(LogLevel_Info, "MainFn: Exit error, err=(Runnable is blocking)")
					}
				case DaemonCommandStopRunnable:
					if !c.isRunning() {
						c.logging(LogLevel_Info, "CosmosProcess: Cannot stop runnable, because it's not running")
						break
					}
					select {
					case c.main.mainKillCh <- true:
						//c.main.pushKillMail(nil, true)
					default:
						c.logging(LogLevel_Info, "MainFn: Exit error, err=(Runnable is blocking)")
					}
				case DaemonCommandExecuteRunnable:
					if !c.trySetRunning(true) {
						c.logging(LogLevel_Fatal, "CosmosProcess: Cannot execute runnable, because it's running")
						break
					}
					conf := cmd.GetConfig()
					runnable := cmd.GetRunnable()
					// Daemon initialize.
					// Check config.
					var err *ErrorInfo
					if err = conf.Check(); err != nil {
						c.logging(LogLevel_Fatal, "CosmosProcess: MainFn config check failed, err=(%v)", err.Message)
						c.trySetRunning(false)
						break
					}
					// Run main.
					c.main = newCosmosMainFn(c, conf, runnable)

					// 后台驻留执行可执行命令。
					// Daemon execute executable command.
					// 让本地的Cosmos去初始化Runnable中的各种内容，主要是Element相关信息的加载。
					// Make CosmosMainFn initial the content of Runnable, especially the Element information.
					err := c.main.initCosmosMainFn(conf, runnable)
					if err != nil {
						c.logging(LogLevel_Fatal, "CosmosProcess: MainFn init failed, err=(%v)", err.Message)
						c.trySetRunning(false)
						break
					}
					go func() {
						// 最后执行Runnable的清理相关动作，还原Cosmos的原状。
						// At last, clean up the Runnable, recover Cosmos.
						defer c.trySetRunning(false)
						defer c.main.pushKillMail(nil, true)

						// 防止Runnable中的Script崩溃导致程序崩溃。
						// To prevent panic from the Runnable Script.
						defer func() {
							if r := recover(); r != nil {
								c.logging(LogLevel_Fatal, fmt.Sprintf("Cosmos.Defer: SCRIPT CRASH! reason=(%s),stack=(%s)", r, string(debug.Stack())))
							}
						}()
						// 执行Runnable。
						// Execute runnable.
						c.logging(LogLevel_Info, "MainFn: NOW RUNNING!")
						runnable.mainScript(c.main, c.main.mainKillCh)
						c.logging(LogLevel_Info, "MainFn: Execute runnable succeed.")
						// Stopped
						daemonCh <- nil
					}()
				case DaemonCommandReloadRunnable:
					if !c.isRunning() {
						c.logging(LogLevel_Info, "CosmosProcess: Cannot execute runnable reload, because it's not running.")
						break
					}
					conf := cmd.GetConfig()
					runnable := cmd.GetRunnable()
					// Daemon initialize.
					// Check config.
					var err *ErrorInfo
					if err = conf.Check(); err != nil {
						c.logging(LogLevel_Fatal, "CosmosProcess: MainFn config check failed, err=(%v)", err.Message)
						c.trySetRunning(false)
						break
					}
					// Run main.
					err := c.main.initCosmosMainFn(conf, runnable)
					if err != nil {
						c.logging(LogLevel_Fatal, "CosmosProcess: MainFn init failed, err=(%v)", err.Message)
						c.trySetRunning(false)
						break
					}

					// Running
					c.reloads += 1
					err = c.main.pushReloadMail(nil, runnable, c.reloads)
					//err = c.main.reload(cmd.GetRunnable(), c.reloads)
					if err != nil {
						c.logging(LogLevel_Fatal, fmt.Sprintf("CosmosProcess: Execute runnable reload failed, err=(%v)", err))
					} else {
						c.logging(LogLevel_Info, "CosmosProcess: Execute runnable reload succeed")
					}
				}
			case err := <-daemonCh:
				if err != nil {
					c.logging(LogLevel_Error, fmt.Sprintf("CosmosProcess: Exited, err=(%v)", err))
				} else {
					c.logging(LogLevel_Info, "CosmosProcess: Exited")
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

func (c *CosmosProcess) Send(command DaemonCommand) *ErrorInfo {
	select {
	case c.daemonCmdCh <- command:
		return nil
	default:
		if c.daemonCmdCh == nil {
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
					c.logging(LogLevel_Fatal, "CosmosProcess: WaitKillSignal killed atomos failed, err=(%v)", err)
					continue
				}
				c.logging(LogLevel_Info, "CosmosProcess: WaitKillSignal killed atomos")
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
	c.daemonCmdCh = nil
}

//////////////////////////////////////////////////
////////////
// StartRunning

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

func (c *CosmosProcess) logging(level LogLevel, fmt string, args ...interface{}) {
	c.sharedLog.PushProcessLog(level, fmt, args)
}
