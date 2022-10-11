package go_atomos

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
)

// Cosmos生命周期，开发者定义和使用的部分。
// Cosmos Life Cycle, part for developer customize and use.

// CosmosProcess
// 这个才是进程的主循环。

type CosmosProcess struct {
	sharedLog *LoggingAtomos

	// Loads at DaemonWithRunnable or Runnable.
	main *CosmosMain
}

// Cosmos生命周期
// Cosmos Life Cycle

func CosmosProcessMainFn(runnable CosmosRunnable) {
	if runnable.config == nil {
		log.Printf("CosmosProcess: No config")
		return
	}
	// Load runnable.
	cosmos := &CosmosProcess{}
	// Run
	if err := cosmos.Run(runnable); err != nil {
		os.Stderr.WriteString(fmt.Sprintf("CosmosProcess: Run failed, err=(%v)", err))
	}
}

func (c *CosmosProcess) Run(runnable CosmosRunnable) (err *ErrorInfo) {
	if err = runnable.config.Check(); err != nil {
		return err.AutoStack(nil, nil)
	}

	// Load Logger
	// Cosmos log is initialized once and available all the time.
	c.sharedLog, err = NewLoggingAtomos(runnable.config.LogPath)
	if err != nil {
		return err.AutoStack(nil, nil)
	}
	defer c.sharedLog.close()

	// Run main.
	c.main = newCosmosMain(c, &runnable)

	// 后台驻留执行可执行命令。
	// Daemon execute executable command.
	// 让本地的Cosmos去初始化Runnable中的各种内容，主要是Element相关信息的加载。
	// Make CosmosMain initial the content of Runnable, especially the Element information.
	err = c.main.onceLoad(&runnable)
	if err != nil {
		c.Logging(LogLevel_Fatal, "CosmosProcess: Main init failed") //, err=(%v)", err.Message)
		return
	}
	// 最后执行Runnable的清理相关动作，还原Cosmos的原状。
	// At last, clean up the Runnable, recover Cosmos.
	// Stopped
	defer func() {
		err = c.main.pushKillMail(nil, true)
		c.Logging(LogLevel_Info, "CosmosProcess: EXITED!")
		c.Logging(LogLevel_Info, "<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
		c.Logging(LogLevel_Info, "<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
	}()

	go c.waitKillSignal()

	// 防止Runnable中的Script崩溃导致程序崩溃。
	// To prevent panic from the Runnable Script.
	defer func() {
		if r := recover(); r != nil {
			c.Logging(LogLevel_Fatal, "CosmosProcess: Main script CRASH! reason=(%s),stack=(%s)", r, string(debug.Stack()))
		}
	}()
	// 执行Runnable。
	// Execute runnable.
	c.Logging(LogLevel_Info, "CosmosProcess: NOW RUNNING!")
	runnable.mainScript(c.main, c.main.waitProcessExitCh)
	c.Logging(LogLevel_Info, "CosmosProcess: Execute runnable succeed.")
	return nil
}

func (c *CosmosProcess) Stop() {
	main := c.main
	if main != nil {
		select {
		case main.waitProcessExitCh <- true:
		default:
			c.Logging(LogLevel_Info, "CosmosProcess: Exit error, err=(Runnable is blocking)")
		}
	}
}

func (c *CosmosProcess) waitKillSignal() {
	ch := make(chan os.Signal, 1)
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
				c.Stop()
				c.Logging(LogLevel_Info, "CosmosProcess: WaitKillSignal killed atomos")
				return
			}
		}
	}
}

func (c *CosmosProcess) Logging(level LogLevel, fmt string, args ...interface{}) {
	c.sharedLog.PushProcessLog(level, fmt, args...)
}
