package go_atomos

import (
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"time"
)

// Cosmos节点需要支持的接口内容
// 仅供生成器内部使用

type CosmosNode interface {
	IsLocal() bool
	// 获得某个Atom类型的Atom的引用
	GetAtomId(elem, name string) (Id, error)

	SpawnAtom(elem, name string, arg proto.Message) (Id, error)

	// 调用某个Atom类型的Atom的引用
	CallAtom(fromId, toId Id, message string, args proto.Message) (reply proto.Message, err error)

	// 关闭某个Atom类型的Atom
	KillAtom(fromId, toId Id) error
}

// Cosmos Cycle

type CosmosCycle interface {
	Daemon(*Config) error
	DaemonWithRunnable(*Config, CosmosRunnable) error
	Close()
}

func NewCosmosCycle() CosmosCycle {
	return newCosmosSelf()
}

// CosmosNode

type CosmosSelf struct {
	// Load at NewCosmosCycle
	config   *Config
	// Load at DaemonWithRunnable or Runnable
	local       *CosmosLocal
	cluster     *CosmosClusterHelper
	daemonCmdCh chan *DaemonCommand
	signCh      chan os.Signal
	mutex       sync.Mutex
	// Log
	log *MailBox
}

func newCosmosSelf() *CosmosSelf {
	c := &CosmosSelf{
		config:      nil,
		local:       nil,
		cluster:     nil,
		daemonCmdCh: nil,
		signCh:      nil,
		log:         nil,
	}
	// Cosmos log is initialized once and available all the time.
	c.log = NewMailBox(MailBoxHandler{
		OnReceive: c.onLogMessage,
		OnPanic:   c.onLogPanic,
		OnStop:    c.onLogStop,
		//OnRestart: c.onL, todo
	})
	c.log.Start()
	return c
}

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
				// todo
				c.Fatal("daemon execute failed")
			} else {
				// todo
				c.Info("daemon execute succeed")
			}
		case s := <-c.signCh:
			exit, message := c.daemonSign(s)
			c.Info("daemon caught notice, exit=%v,message=%s", exit, message)
			if exit {
				return nil
			}
		}
	}
}

func (c *CosmosSelf) SendScript(cmd *DaemonCommand) {
	c.daemonCmdCh <- cmd
}

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
				c.Fatal("daemon execute failed")
			} else {
				// todo
				c.Info("daemon execute succeed")
			}
		case s := <-c.signCh:
			exit, message := c.daemonSign(s)
			c.Info("daemon caught notice, exit=%v,message=%s", exit, message)
			if exit {
				return nil
			}
		}
	}
}

func (c *CosmosSelf) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.log != nil {
		c.log.Stop()
	}
}

func (c *CosmosSelf) daemonInit(conf *Config) error {
	// Check
	if conf == nil {
		// todo
		return errors.New("no config")
	}
	if err := conf.check(); err != nil {
		// todo
		return errors.New("config invalid")
	}

	// Init
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.config != nil {
		// todo
		return errors.New("registered")
	}
	c.config = conf
	c.local = newCosmosLocal()
	c.cluster = newCosmosClusterHelper()
	c.daemonCmdCh = make(chan *DaemonCommand)
	c.signCh = make(chan os.Signal)
	signal.Notify(c.signCh)

	return nil
}

func (c *CosmosSelf) daemonExit() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.config = nil
	c.local = nil
	c.cluster = nil
	c.daemonCmdCh = nil
	c.signCh = nil
}

func (c *CosmosSelf) daemonCommandExecutor(cmd *DaemonCommand) error {
	switch cmd.Cmd {
	case DaemonCommandExecuteRunnable:
		// Load Runnable
		if err := c.local.initRunnable(c, cmd.Runnable); err != nil {
			return err
		}
		defer c.local.exitRunnable()
		defer c.ScriptDefer()
		if err := c.local.runRunnable(cmd.Runnable); err != nil {
			return err
		}
		return nil
	}
	// todo
	return errors.New("unknown daemon type")
}

func (c *CosmosSelf) daemonSign(s os.Signal) (exit bool, message string) {
	switch s {
	case os.Interrupt, os.Kill:
		exit = true
		message = fmt.Sprintf("receive exit signal: signal=%s", s)
	default:
		exit = false
		message = fmt.Sprintf("receive signal: signal=%s", s)
	}
	return
}

// Interface

func (c *CosmosSelf) Local() *CosmosLocal {
	return c.local
}

func (c *CosmosSelf) GetName() string {
	return c.config.Node
}

func (c *CosmosSelf) ScriptDefer() {
	if r := recover(); r != nil {
		logFatal(time.Now(), fmt.Sprintf("Script => Script crash, reason=%s,stack=%s",
			r, string(debug.Stack())))
		// TODO: Notice error.
	}
}

type Script func(cosmosSelf *CosmosSelf, mainId Id, killNoticeChannel chan bool)
