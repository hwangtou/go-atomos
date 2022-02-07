package go_atomos

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"time"
)

// Cosmos生命周期，开发者定义和使用的部分。
// Cosmos Life Cycle, part for developer customize and use.

func newCosmosSelf() *CosmosSelf {
	c := &CosmosSelf{}
	// Cosmos log is initialized once and available all the time.
	c.log = newMailBox(MailBoxHandler{
		OnReceive: c.onLogMessage,
		OnPanic:   c.onLogPanic,
		OnStop:    c.onLogStop,
	})
	c.log.Name = "logger"
	c.log.start()
	return c
}

// Runnable相关入口脚本
// Entrance script of runnable.
type Script func(cosmosSelf *CosmosSelf, mainId MainId, killNoticeChannel chan bool)

// Cosmos生命周期
// Cosmos Life Cycle

// DaemonWithRunnable会一直阻塞直至收到终结信号，或者脚本已经执行完毕。
// DaemonWithRunnable will block until stop signal received or script terminated.
func (c *CosmosSelf) Daemon(conf *Config) (chan struct{}, error) {
	if err := c.daemonInit(conf); err != nil {
		return nil, err
	}
	closeCh := make(chan struct{}, 1)
	runCh := make(chan struct{}, 1)
	go func() {
		defer func() {
			c.logInfo("Cosmos.Daemon: Exited, bye!")
		}()
		defer func() {
			closeCh <- struct{}{}
		}()
		daemonCh := make(chan error, 1)
		for {
			exit := false
			runCh <- struct{}{}
			select {
			case cmd := <-c.daemonCmdCh:
				if cmd == nil {
					c.logFatal("Cosmos.Daemon: Invalid runnable")
					break
				}
				if err := cmd.Check(); err != nil {
					c.logFatal("Cosmos.Daemon: Invalid runnable, err=%v", err)
					break
				}
				switch cmd.Type() {
				case DaemonCommandExit:
					if !c.isRunning() {
						c.logInfo("Cosmos.Daemon: Cannot stop runnable, because it's not running")
						break
					}
					if c.runtime.stop() {
						exit = true
					}
				case DaemonCommandStopRunnable:
					if !c.isRunning() {
						c.logInfo("Cosmos.Daemon: Cannot stop runnable, because it's not running")
						break
					}
					c.runtime.stop()
				case DaemonCommandExecuteRunnable:
					if c.isRunning() {
						c.logInfo("Cosmos.Daemon: Cannot execute runnable, because it's running")
						break
					}
					go func() {
						// Running
						err := c.daemonRunnableExecute(*cmd.GetRunnable())
						if err != nil {
							c.logFatal("Cosmos.Daemon: Execute runnable failed, err=%v", err)
						} else {
							c.logInfo("Cosmos.Daemon: Execute runnable succeed.")
						}
						// Stopped
						daemonCh <- err
					}()
				case DaemonCommandReloadRunnable:
					if !c.isRunning() {
						c.logInfo("Cosmos.Daemon: Cannot execute runnable upgrade, because it's not running.")
						break
					}
					// Running
					err := c.daemonRunnableUpgrade(*cmd.GetRunnable())
					if err != nil {
						c.logFatal("Cosmos.Daemon: Execute runnable upgrade failed, err=%v", err)
					} else {
						c.logInfo("Cosmos.Daemon: Execute runnable upgrade succeed")
					}
				}
			case err := <-daemonCh:
				if err != nil {
					c.logError("Cosmos.Daemon: Exited, err=%v", err)
				} else {
					c.logInfo("Cosmos.Daemon: Exited")
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

func (c *CosmosSelf) Send(command DaemonCommand) error {
	select {
	case c.daemonCmdCh <- command:
		return nil
	default:
		return ErrDaemonIsBusy
	}
}

func (c *CosmosSelf) WaitKillSignal() {
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
					LogWrite(LogFormatter(time.Now(), LogLevel_Fatal, fmt.Sprintf("WaitKillSignal killed atomos fails, err=%v", err)), true)
					continue
				}
				LogWrite(LogFormatter(time.Now(), LogLevel_Info, fmt.Sprintf("WaitKillSignal killed atomos")), false)
				return
			}
		}
	}
}

// Daemon结束后Close。
// Close after Daemon run.
func (c *CosmosSelf) Close() {
	if c.log != nil {
		c.log.waitStop()
		c.log = nil
	}
	c.daemonCmdCh = nil
	c.clientCert = nil
	c.listenCert = nil
	c.config = nil
}

//////////////////////////////////////////////////
////////////
// Daemon

// Daemon initialize.
func (c *CosmosSelf) daemonInit(conf *Config) (err error) {
	// Check
	if conf == nil {
		return errors.New("no configuration")
	}
	if err = conf.Check(); err != nil {
		return fmt.Errorf("config invalid, err=%v", err)
	}

	// Init
	if c.isRunning() {
		return errors.New("config registered")
	}
	c.config = conf
	if c.clientCert, err = conf.getClientCertConfig(); err != nil {
		return err
	}
	if c.listenCert, err = conf.getListenCertConfig(); err != nil {
		return err
	}
	c.daemonCmdCh = make(chan DaemonCommand)
	c.runtime = newCosmosRuntime()
	c.remotes = newCosmosRemoteHelper(c)
	c.telnet = newCosmosTelnet(c)

	return nil
}

// 后台驻留执行可执行命令。
// Daemon execute executable command.
func (c *CosmosSelf) daemonRunnableExecute(runnable CosmosRunnable) error {
	if !c.trySetRunning(true) {
		return ErrDaemonIsRunning
	}
	// 让本地的Cosmos去初始化Runnable中的各种内容，主要是Element相关信息的加载。
	// Make CosmosRuntime initial the content of Runnable, especially the Element information.
	if err := c.runtime.init(c, &runnable); err != nil {
		c.trySetRunning(false)
		return err
	}
	// 最后执行Runnable的清理相关动作，还原Cosmos的原状。
	// At last, clean up the Runnable, recover Cosmos.
	defer c.trySetRunning(false)
	defer c.runtime.close()

	// 防止Runnable中的Script崩溃导致程序崩溃。
	// To prevent panic from the Runnable Script.
	defer c.deferRunnable()

	// 执行Runnable。
	// Execute runnable.
	if err := c.runtime.run(&runnable); err != nil {
		return err
	}
	return nil
}

func (c *CosmosSelf) daemonRunnableUpgrade(runnable CosmosRunnable) error {
	c.upgradeCount += 1
	return c.runtime.upgrade(&runnable, c.upgradeCount)
}

func (c *CosmosSelf) deferRunnable() {
	if r := recover(); r != nil {
		c.logFatal("Cosmos.Defer: SCRIPT CRASH! reason=%s,stack=%s", r, string(debug.Stack()))
	}
}

func (c *CosmosSelf) trySetRunning(run bool) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.running == run {
		return false
	}
	c.running = run
	return true
}

func (c *CosmosSelf) isRunning() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.running
}

//////////////////////////////////////////////////
////////////
// Runnable

type CosmosRunnable struct {
	interfaces     map[string]*ElementInterface
	interfaceOrder []*ElementInterface
	implements     map[string]*ElementImplementation
	implementOrder []*ElementImplementation
	mainScript     Script
	upgradeScript  Script
}

func (r *CosmosRunnable) AddElementInterface(i *ElementInterface) *CosmosRunnable {
	if r.interfaces == nil {
		r.interfaces = map[string]*ElementInterface{}
	}
	if _, has := r.interfaces[i.Config.Name]; !has {
		r.interfaces[i.Config.Name] = i
		r.interfaceOrder = append(r.interfaceOrder, i)
	}
	return r
}

// CosmosRunnable构造器方法，用于添加Element。
// Construct method of CosmosRunnable, uses to add Element.
func (r *CosmosRunnable) AddElementImplementation(i *ElementImplementation) *CosmosRunnable {
	r.AddElementInterface(i.Interface)
	if r.implements == nil {
		r.implements = map[string]*ElementImplementation{}
	}
	if _, has := r.implements[i.Interface.Config.Name]; !has {
		r.implements[i.Interface.Config.Name] = i
		r.implementOrder = append(r.implementOrder, i)
	}
	return r
}

// CosmosRunnable构造器方法，用于设置Script。
// Construct method of CosmosRunnable, uses to set Script.
func (r *CosmosRunnable) SetScript(script Script) *CosmosRunnable {
	r.mainScript = script
	return r
}

func (r *CosmosRunnable) SetUpgradeScript(script Script) *CosmosRunnable {
	r.upgradeScript = script
	return r
}

// 命令
// Command

type DaemonCommand interface {
	Type() DaemonCommandType
	GetRunnable() *CosmosRunnable
	Check() error
}

// 命令类型
// Command Type
type DaemonCommandType int

const (
	DaemonCommandExit = 0
	// 运行Runnable
	DaemonCommandExecuteRunnable = 1
	DaemonCommandStopRunnable    = 2
	DaemonCommandReloadRunnable  = 3
)

var (
	ErrDaemonIsRunning = errors.New("cosmos daemon is running")
	ErrDaemonIsBusy    = errors.New("cosmos daemon is busy")
	ErrRunnableInvalid = errors.New("cosmos runnable invalid")
)

func NewRunnableCommand(runnable *CosmosRunnable) DaemonCommand {
	return &command{
		cmdType:  DaemonCommandExecuteRunnable,
		runnable: runnable,
	}
}

func NewRunnableUpdateCommand(runnable *CosmosRunnable) DaemonCommand {
	return &command{
		cmdType:  DaemonCommandReloadRunnable,
		runnable: runnable,
	}
}

func NewStopCommand() DaemonCommand {
	return &command{
		cmdType:  DaemonCommandStopRunnable,
		runnable: nil,
	}
}

func NewExitCommand() DaemonCommand {
	return &command{
		cmdType:  DaemonCommandExit,
		runnable: nil,
	}
}

// Command具体实现
type command struct {
	cmdType  DaemonCommandType
	runnable *CosmosRunnable
}

func (c command) Type() DaemonCommandType {
	return c.cmdType
}

func (c command) GetRunnable() *CosmosRunnable {
	return c.runnable
}

func (c command) Check() error {
	switch c.cmdType {
	case DaemonCommandExit, DaemonCommandStopRunnable:
		return nil
	case DaemonCommandExecuteRunnable:
		if c.runnable == nil {
			return ErrRunnableInvalid
		}
		if c.runnable.mainScript == nil {
			return ErrRunnableInvalid
		}
		return nil
	case DaemonCommandReloadRunnable:
		if c.runnable == nil {
			return ErrRunnableInvalid
		}
		if c.runnable.upgradeScript == nil {
			return ErrRunnableInvalid
		}
		return nil
	default:
		return ErrRunnableInvalid
	}
}
