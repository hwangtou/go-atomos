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
	Daemon(*Config) (chan struct{}, error)
	SendScript(*DaemonCommand)
	SendRunnable(CosmosRunnable)
	SendKill()
	Close()
}

func NewCosmosCycle() CosmosCycle {
	return newCosmosSelf()
}

func (c *CosmosSelf) SendRunnable(runnable CosmosRunnable) {
	c.SendScript(&DaemonCommand{
		Cmd:      DaemonCommandExecuteRunnable,
		Runnable: runnable,
	})
}

// 给Daemon中的Cosmos发送命令。
// write Daemon Command to a daemon cosmos.
func (c *CosmosSelf) SendScript(cmd *DaemonCommand) {
	c.daemonCmdCh <- cmd
}

func (c *CosmosSelf) SendKill() {
	c.SendScript(&DaemonCommand{
		Cmd: DaemonCommandExit,
	})
}

// DaemonWithRunnable会一直阻塞直至收到终结信号，或者脚本已经执行完毕。
// DaemonWithRunnable will block until stop signal received or script terminated.
func (c *CosmosSelf) Daemon(conf *Config) (chan struct{}, error) {
	if err := c.daemonInit(conf); err != nil {
		return nil, err
	}
	closeCh := make(chan struct{}, 1)
	go func() {
		defer func() {
			closeCh <- struct{}{}
		}()
		defer c.daemonExit()
		daemonCh := make(chan error, 1)
		for {
			select {
			case cmd := <-c.daemonCmdCh:
				switch cmd.Cmd {
				case DaemonCommandExit:
					func() {
						defer func() {
							if r := recover(); r != nil {
								c.logInfo("Cosmos.Daemon: Exit error, err=%s", r)
							}
						}()
						c.local.mainKillCh <- true
					}()
				case DaemonCommandExecuteRunnable:
					go func() {
						err := c.daemonCommandExecutor(cmd)
						if err != nil {
							c.logFatal("Cosmos.Daemon: Execute failed, err=%v", err)
						} else {
							c.logInfo("Cosmos.Daemon: Execute succeed")
						}
						daemonCh <- err
					}()
				}
			case s := <-c.signCh:
				exit, message := c.daemonSignalWatcher(s)
				if exit {
					c.logInfo("Cosmos.Daemon: Caught notice, exit=%v,message=%s", exit, message)
					c.local.mainKillCh <- true
				}
			case err := <-daemonCh:
				c.logInfo("Cosmos.Daemon: Exit, err=%v", err)
				return
			}
		}
	}()
	return closeCh, nil
}

// Daemon结束后Close。
// Close after Daemon run.
func (c *CosmosSelf) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.log != nil {
		c.log.WaitStop()
	}
}

// Daemon initialize.
func (c *CosmosSelf) daemonInit(conf *Config) (err error) {
	// Check
	if conf == nil {
		return errors.New("no configuration")
	}
	if err = conf.check(c); err != nil {
		return fmt.Errorf("config invalid, err=%v", err)
	}

	// Init
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.config != nil {
		return errors.New("config registered")
	}
	c.config = conf
	if c.clientCert, err = conf.getClientCertConfig(); err != nil {
		return err
	}
	if c.listenCert, err = conf.getListenCertConfig(); err != nil {
		return err
	}
	c.local = newCosmosLocal()
	c.remotes = newCosmosRemoteHelper(c)
	c.daemonCmdCh = make(chan *DaemonCommand)
	c.signCh = make(chan os.Signal)
	signal.Notify(c.signCh)

	return nil
}

// Daemon exit.
func (c *CosmosSelf) daemonExit() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.remotes.close()
}

// 后台驻留执行可执行命令。
// Daemon execute executable command.
func (c *CosmosSelf) daemonCommandExecutor(cmd *DaemonCommand) error {
	switch cmd.Cmd {
	case DaemonCommandExecuteRunnable:
		{
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
			defer c.deferRunnable()

			// 执行Runnable。
			// Execute runnable.
			if err := c.local.runRunnable(cmd.Runnable); err != nil {
				return err
			}
			return nil
		}
	case DaemonCommandExecuteDynamicRunnable:
		return errors.New("not supported")
	}
	return fmt.Errorf("unknown daemon command, cmd=%+v", cmd)
}

func (c *CosmosSelf) deferRunnable() {
	if r := recover(); r != nil {
		c.logFatal("Cosmos.Defer: SCRIPT CRASH! reason=%s,stack=%s", r, string(debug.Stack()))
	}
}

// 后台驻留检测系统信号。
// Daemon watching OS process signal.
func (c *CosmosSelf) daemonSignalWatcher(s os.Signal) (exit bool, message string) {
	switch s {
	case os.Interrupt:
		// Print more pretty in command line.
		fmt.Println()
		fallthrough
	case os.Kill:
		exit = true
		message = fmt.Sprintf("<ExitSignal:%s>", s)
	default:
		exit = false
		message = fmt.Sprintf("<Signal:%s>", s)
	}
	return
}

// 命令类型
// Command Type
type DaemonCommandType int

const (
	DaemonCommandExit = 0
	// 运行Runnable
	DaemonCommandExecuteRunnable = 1
	// TODO: Support in future version.
	DaemonCommandExecuteDynamicRunnable = 2
)

// 命令
// Command
type DaemonCommand struct {
	Cmd      DaemonCommandType
	Runnable CosmosRunnable
	//DynamicRunnable CosmosDynamicRunnable
}

// Runnable
type CosmosRunnable struct {
	interfaces      map[string]*ElementInterface
	interfaceOrder  []*ElementInterface
	implementations map[string]*ElementImplementation
	implementOrder  []*ElementImplementation
	script          Script
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
	if r.implementations == nil {
		r.implementations = map[string]*ElementImplementation{}
	}
	if _, has := r.implementations[i.Interface.Config.Name]; !has {
		r.implementations[i.Interface.Config.Name] = i
		r.implementOrder = append(r.implementOrder, i)
	}
	return r
}

// 添加虫洞
// Add wormhole.
func (r *CosmosRunnable) AddWormhole(w *ElementImplementation) {
	r.AddElementImplementation(w)
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
	if r.implementations == nil {
		return errors.New("config has no Interface yet")
	}
	if r.script == nil {
		return errors.New("config has no Script yet")
	}
	return nil
}

//// Dynamic Runnable
//type CosmosDynamicRunnable struct {
//	path     string
//	loaded   bool
//	runnable CosmosRunnable
//}
