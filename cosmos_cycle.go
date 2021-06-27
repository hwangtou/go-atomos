package go_atomos

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
)

// Cosmos生命周期，开发者定义和使用的部分。
// Cosmos Life Cycle, part for developer customize and use.

// Runnable相关入口脚本
// Entrance script of runnable.
type Script func(cosmosSelf *CosmosSelf, mainId MainId, killNoticeChannel chan bool)

// Cosmos生命周期
// Cosmos Life Cycle

type CosmosCycle interface {
	Daemon(*Config) error
	DaemonWithRunnable(*Config, CosmosRunnable) error
	Close()
}

func NewCosmosCycle() CosmosCycle {
	return newCosmosSelf()
}

func (c *CosmosSelf) ScriptDefer() {
	if r := recover(); r != nil {
		c.logFatal("CosmosSelf.ScriptDefer: SCRIPT CRASH! reason=%s,stack=%s", r, string(debug.Stack()))
	}
}

// Daemon会一直阻塞直至收到终结信号。
// Daemon will block until stop signal received.
func (c *CosmosSelf) Daemon(conf *Config) error {
	//c.log.
	if err := c.daemonInit(conf); err != nil {
		return err
	}
	defer c.daemonExit()
	for {
		select {
		case cmd := <-c.daemonCmdCh:
			err := c.daemonCommandExecutor(cmd)
			if err != nil {
				c.logFatal("CosmosSelf.Daemon: Execute failed, err=%v", err)
			} else {
				c.logInfo("CosmosSelf.Daemon: Daemon execute succeed")
			}
		case s := <-c.signCh:
			exit, message := c.daemonSignalWatcher(s)
			c.logInfo("CosmosSelf.Daemon: Daemon caught notice, exit=%v,message=%s", exit, message)
			if exit {
				return nil
			}
		}
	}
}

// 给Daemon中的Cosmos发送命令。
// Send Daemon Command to a daemon cosmos.
func (c *CosmosSelf) SendScript(cmd *DaemonCommand) {
	c.daemonCmdCh <- cmd
}

// DaemonWithRunnable会一直阻塞直至收到终结信号，或者脚本已经执行完毕。
// DaemonWithRunnable will block until stop signal received or script terminated.
func (c *CosmosSelf) DaemonWithRunnable(conf *Config, runnable CosmosRunnable) error {
	if err := c.daemonInit(conf); err != nil {
		return err
	}
	defer c.daemonExit()
	go func() {
		// todo
		c.daemonCmdCh <- &DaemonCommand{
			Cmd:      DaemonCommandExecuteRunnable,
			Runnable: runnable,
		}
	}()
	for {
		select {
		case cmd := <-c.daemonCmdCh:
			err := c.daemonCommandExecutor(cmd)
			if err != nil {
				// todo
				c.logFatal("CosmosSelf.DaemonWithRunnable: Daemon execute failed, er=%v", err)
			} else {
				// todo
				c.logInfo("CosmosSelf.DaemonWithRunnable: Daemon execute succeed")
			}
		case s := <-c.signCh:
			exit, message := c.daemonSignalWatcher(s)
			c.logInfo("CosmosSelf.DaemonWithRunnable: Daemon caught notice, exit=%v,message=%s", exit, message)
			if exit {
				return nil
			}
		}
	}
}

// Daemon结束后Close。
// Close after Daemon run.
func (c *CosmosSelf) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.log != nil {
		c.log.Stop()
	}
}

// Daemon initialize.
func (c *CosmosSelf) daemonInit(conf *Config) error {
	// Check
	if conf == nil {
		return errors.New("no config")
	}
	if err := conf.check(c); err != nil {
		return fmt.Errorf("config invalid, err=%v", err)
	}

	// Init
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.config != nil {
		return errors.New("config registered")
	}
	c.config = conf
	c.local = newCosmosLocal()
	c.cluster = newCosmosClusterHelper()
	c.daemonCmdCh = make(chan *DaemonCommand)
	c.signCh = make(chan os.Signal)
	signal.Notify(c.signCh)

	return nil
}

// Daemon exit.
func (c *CosmosSelf) daemonExit() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.local.killNoticeCh != nil {
		c.local.killNoticeCh <- true
	}
	c.config = nil
	c.local = nil
	c.cluster = nil
	c.daemonCmdCh = nil
	c.signCh = nil
}

// 后台驻留执行可执行命令。
// Daemon execute executable command.
func (c *CosmosSelf) daemonCommandExecutor(cmd *DaemonCommand) error {
	switch cmd.Cmd {
	case DaemonCommandExecuteRunnable:
		// 让本地的Cosmos去初始化Runnable中的各种内容，主要是Element相关信息的加载。
		// Make CosmosLocal initial the content of Runnable, especially the Element information.
		if err := c.local.initRunnable(c, cmd.Runnable); err != nil {
			return err
		}
		// 最后执行Runnable的清理相关动作，还原Cosmos的原状。
		// At last, clean up the Runnable, recover Cosmos.
		defer c.local.exitRunnable()

		// 防止Runnable中的Script崩溃导致程序崩溃。
		// To prevent panic from the Runnable Script.
		defer c.ScriptDefer()

		// 执行Runnable。
		// Execute runnable.
		if err := c.local.runRunnable(cmd.Runnable); err != nil {
			return err
		}
		return nil
	}
	// todo
	return fmt.Errorf("unknown daemon command, cmd=%+v", cmd)
}

// 后台驻留检测系统信号。
// Daemon watching OS process signal.
func (c *CosmosSelf) daemonSignalWatcher(s os.Signal) (exit bool, message string) {
	switch s {
	case os.Interrupt, os.Kill:
		exit = true
		message = fmt.Sprintf("ExitSignal=%s", s)
	default:
		exit = false
		message = fmt.Sprintf("Signal=%s", s)
	}
	return
}

// 命令类型
// Command Type
type DaemonCommandType int

const (
	// 运行Runnable
	DaemonCommandExecuteRunnable = 1
	// TODO: Support in future version.
	DaemonCommandExecuteDynamicRunnable = 2
)

// 命令
// Command
type DaemonCommand struct {
	Cmd             DaemonCommandType
	Runnable        CosmosRunnable
	DynamicRunnable CosmosDynamicRunnable
}

// Runnable
type CosmosRunnable struct {
	terminateSign chan string
	defines       map[string]*ElementDefine
	script        Script
}

// CosmosRunnable构造器方法，用于添加Element。
// Construct method of CosmosRunnable, uses to add Element.
func (r *CosmosRunnable) AddElement(define *ElementDefine) *CosmosRunnable {
	if r.defines == nil {
		r.defines = map[string]*ElementDefine{}
	}
	r.defines[define.Config.Name] = define
	return r
}

// CosmosRunnable构造器方法，用于设置Script。
// Construct method of CosmosRunnable, uses to set Script.
func (r *CosmosRunnable) SetScript(script Script) *CosmosRunnable {
	r.script = script
	return r
}

// CosmosRunnable合法性检查。
// CosmosRunnable legal checker.
func (r *CosmosRunnable) Check() error {
	if r.defines == nil {
		return errors.New("config has no ElementDefine yet")
	}
	if r.script == nil {
		return errors.New("config has no Script yet")
	}
	return nil
}

// Dynamic Runnable
// TODO: Not implemented yet.
type CosmosDynamicRunnable struct {
	path     string
	loaded   bool
	runnable CosmosRunnable
}
