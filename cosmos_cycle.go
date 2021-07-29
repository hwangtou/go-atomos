package go_atomos

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
)

const (
	lCCSCrash           = "CosmosSelf.deferRunnable: SCRIPT CRASH! reason=%s,stack=%s"
	lCCDRExecuteFailed  = "CosmosSelf.Daemon: Execute failed, err=%v"
	lCCDRExecuteSucceed = "CosmosSelf.Daemon: Execute succeed"
	lCCDRNotice         = "CosmosSelf.Daemon: Caught notice, exit=%v,message=%s"
	lCCDRExit           = "CosmosSelf.Daemon: Exit, err=%v"
)

const (
	errNoConfig            = "no configuration"
	errConfigInvalid       = "config invalid, err=%v"
	errConfigRegistered    = "config registered"
	errDaemonCmdExecutor   = "unknown daemon command, cmd=%+v"
	errInvalidCosmosLocal  = "invalid CosmosLocal"
	errInvalidCosmosRemote = "invalid cosmosRemote"
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
								// TODO
							}
						}()
						c.local.killNoticeCh <- true
					}()
				case DaemonCommandExecuteRunnable:
					go func() {
						err := c.daemonCommandExecutor(cmd)
						if err != nil {
							// todo
							c.logFatal(lCCDRExecuteFailed, err)
						} else {
							// todo
							c.logInfo(lCCDRExecuteSucceed)
						}
						daemonCh <- err
					}()
				}
			case s := <-c.signCh:
				exit, message := c.daemonSignalWatcher(s)
				if exit {
					c.logInfo(lCCDRNotice, exit, message)
					c.local.killNoticeCh <- true
				}
			case err := <-daemonCh:
				c.logInfo(lCCDRExit, err)
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
func (c *CosmosSelf) daemonInit(conf *Config) error {
	// Check
	if conf == nil {
		return errors.New(errNoConfig)
	}
	if err := conf.check(c); err != nil {
		return fmt.Errorf(errConfigInvalid, err)
	}

	// Init
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.config != nil {
		return errors.New(errConfigRegistered)
	}
	c.config = conf
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
	// todo
	return fmt.Errorf(errDaemonCmdExecutor, cmd)
}

func (c *CosmosSelf) deferRunnable() {
	if r := recover(); r != nil {
		c.logFatal(lCCSCrash, r, string(debug.Stack()))
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
	implementations map[string]*ElementImplementation
	script          Script
}

func (r *CosmosRunnable) AddElementInterface(i *ElementInterface) *CosmosRunnable {
	if r.interfaces == nil {
		r.interfaces = map[string]*ElementInterface{}
	}
	r.interfaces[i.Config.Name] = i
	return r
}

// CosmosRunnable构造器方法，用于添加Element。
// Construct method of CosmosRunnable, uses to add Element.
func (r *CosmosRunnable) AddElementImplementation(i *ElementImplementation) *CosmosRunnable {
	if r.implementations == nil {
		r.implementations = map[string]*ElementImplementation{}
	}
	r.implementations[i.ElementInterface.Config.Name] = i
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
	if r.implementations == nil {
		return errors.New("config has no ElementInterface yet")
	}
	if r.script == nil {
		return errors.New("config has no Script yet")
	}
	return nil
}

//// Dynamic Runnable
//// TODO: Not implemented yet.
//type CosmosDynamicRunnable struct {
//	path     string
//	loaded   bool
//	runnable CosmosRunnable
//}

// Public

func NewAtomId(c CosmosNode, elemName, atomName string) (Id, error) {
	if c.IsLocal() {
		l, ok := c.(*CosmosLocal)
		if !ok {
			return nil, errors.New(errInvalidCosmosLocal)
		}
		element, err := l.getElement(elemName)
		if err != nil {
			return nil, err
		}
		return element.getAtomId(atomName)
	} else {
		r, ok := c.(*cosmosWatchRemote)
		if !ok {
			return nil, errors.New(errInvalidCosmosRemote)
		}
		element, err := r.getElement(elemName)
		if err != nil {
			return nil, err
		}
		return element.getAtomId(atomName)
	}
}
