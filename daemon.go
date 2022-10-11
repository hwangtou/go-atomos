package go_atomos

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Node Watcher

type NodeWatcher struct {
	supervisorConfig *SupervisorConfig
	nodeConfig       *NodeConfig
	runPathWatcher   *fsnotify.Watcher

	curStatus WatcherStatus
	curProc   *os.Process
}

func NewNodeWatcher(cosmosName, nodeName string) (*NodeWatcher, *Error) {
	// Find Node
	supervisorConfig, err := LoadConfig(cosmosName)
	if err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	var nodeConfig *NodeConfig
	for _, config := range supervisorConfig.NodeConfigList {
		if config.NodeName == nodeName {
			nodeConfig = config
			break
		}
	}
	if nodeConfig == nil {
		return nil, NewErrorf(ErrCosmosNodeNameNotFound, "node name not found, name=(%s)", nodeName).AutoStack(nil, nil)
	}

	running, proc, err := nodeConfig.CheckDaemon(false)
	if err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	runPathWatcher, err := nodeConfig.GetVarRunWatcher()
	if err != nil {
		return nil, err.AutoStack(nil, nil)
	}

	watcher := &NodeWatcher{supervisorConfig: supervisorConfig, nodeConfig: nodeConfig, runPathWatcher: runPathWatcher}
	if running {
		watcher.curStatus = WatcherRun
		watcher.curProc = proc
	} else {
		watcher.curStatus = WatcherHalt
	}
	return watcher, nil
}

func (w *NodeWatcher) Close() {
	w.runPathWatcher.Close()
}

func (w *NodeWatcher) GetSupervisorConfig() *SupervisorConfig {
	return w.supervisorConfig
}

func (w *NodeWatcher) GetNodeConfig() *NodeConfig {
	return w.nodeConfig
}

func (w *NodeWatcher) OnChanged() <-chan WatcherStatus {
	ch := make(chan WatcherStatus, 1)
	go func() {
		switch w.curStatus {
		case WatcherHalt:
			for {
				select {
				case event := <-w.runPathWatcher.Events:
					switch event.Op {
					case fsnotify.Create:
						running, proc, err := w.nodeConfig.CheckDaemon(false)
						if err != nil || !running {
							ch <- WatcherError
							return
						}
						w.curStatus = WatcherRun
						w.curProc = proc
						ch <- WatcherRun
						return
					default:
						// TODO
					}
				case er := <-w.runPathWatcher.Errors:
					// TODO
					_ = er
					log.Println("error a", er)
				}
			}
		case WatcherRun:
			for {
				select {
				case event := <-w.runPathWatcher.Events:
					switch event.Op {
					case fsnotify.Remove:
						running, err := w.IsRunning()
						if err != nil || running {
							log.Println("error b", err)
							ch <- WatcherError
							return
						}
						w.curStatus = WatcherHalt
						w.curProc = nil
						ch <- WatcherHalt
						return
					default:
						// TODO
					}
				case er := <-w.runPathWatcher.Errors:
					// TODO
					_ = er
				case <-time.After(100 * time.Millisecond): // TODO
					running, err := w.IsRunning()
					if err != nil {
						log.Println("error c", err)
						ch <- WatcherError
						return
					}
					if running {
						continue
					}
					w.curStatus = WatcherHalt
					w.curProc = nil
					ch <- WatcherHalt
					return
				}
			}
		}
	}()
	return ch
}

func (w *NodeWatcher) IsRunning() (bool, *Error) {
	// Get Process ID.
	processID, err := w.nodeConfig.GetProcessID()
	if err != nil {
		return false, err.AutoStack(nil, nil)
	}
	// Check PID File.
	if processID != 0 {
		running, _, err := w.nodeConfig.IsProcessRunning(processID)
		if err != nil {
			return false, nil
		}
		return running, nil
	}
	return false, nil
}

func (w *NodeWatcher) LaunchProcess() *Error {
	binPath, err := w.nodeConfig.GetLatestBinary()
	if err != nil {
		return err.AutoStack(nil, nil)
	}
	if binPath == nil {
		return NewError(ErrCosmosConfigBinPathInvalid, "binary not found")
	}
	//wd, er := os.Getwd()
	//if er != nil {
	//	return NewError(ErrCosmosDaemonGetExecutableFailed, er.Error()).AutoStack(nil, nil)
	//}
	//
	//attr := &os.ProcAttr{
	//	Dir:   wd,
	//	Env:   os.Environ(),
	//	Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	//	Sys:   &syscall.SysProcAttr{Setsid: true},
	//}
	//proc, er := os.StartProcess(binPath.path, nil, attr)
	//if er != nil {
	//	return NewError(ErrCosmosDaemonStartProcessFailed, er.Error()).AutoStack(nil, nil)
	//}
	//state, er := proc.Wait()
	cmd := exec.Command(binPath.path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if er := cmd.Run(); er != nil {
		return NewError(ErrCosmosDaemonStartProcessFailed, er.Error()).AutoStack(nil, nil)
	}
	log.Println("Process Done")
	return nil
}

// Node Daemon

type NodeDaemon struct {
	nodeConfig *NodeConfig

	execPath string
	workPath string
	args     []string
	envs     []string

	socket  net.Listener
	command CosmosDaemonCommand
}

type CosmosDaemonCommand interface {
	Command(cmd string) (string, bool, *Error)
	Greeting() string
	PrintNow() string
	Bye() string
}

func NewNodeDaemon(cosmosName, nodeName string) (*NodeDaemon, *Error) {
	// Find Node
	supervisorConfig, err := LoadConfig(cosmosName)
	if err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	var nodeConfig *NodeConfig
	for _, config := range supervisorConfig.NodeConfigList {
		if config.NodeName == nodeName {
			nodeConfig = config
			break
		}
	}
	if nodeConfig == nil {
		return nil, NewErrorf(ErrCosmosNodeNameNotFound, "node name not found, name=(%s)", nodeName).AutoStack(nil, nil)
	}

	running, _, err := nodeConfig.CheckDaemon(true)
	if err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	if running {
		return nil, NewErrorf(ErrCosmosConfigRunPIDIsRunning, "node is already running")
	}

	return &NodeDaemon{nodeConfig: nodeConfig}, nil
}

func (d *NodeDaemon) BootOrDaemon() bool {
	return os.Getenv(d.GetEnvKey()) != "1"
}

func (d *NodeDaemon) BootProcess() (int, *Error) {
	execPath, er := os.Executable()
	if er != nil {
		return 0, NewError(ErrCosmosDaemonGetExecutableFailed, er.Error()).AutoStack(nil, nil)
	}
	wd, er := os.Getwd()
	if er != nil {
		return 0, NewError(ErrCosmosDaemonGetExecutableFailed, er.Error()).AutoStack(nil, nil)
	}

	d.execPath = execPath
	d.workPath = wd
	d.args = os.Args
	d.envs = os.Environ()
	d.envs = append(d.envs, fmt.Sprintf("%s=%s", d.GetEnvKey(), "1"))

	// Fork Process
	attr := &os.ProcAttr{
		Dir:   d.workPath,
		Env:   d.envs,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	}
	proc, er := os.StartProcess(d.execPath, d.args, attr)
	if er != nil {
		return 0, NewError(ErrCosmosDaemonStartProcessFailed, er.Error()).AutoStack(nil, nil)
	}
	//c.logFile.WriteAccessLog(fmt.Sprintf("Daemon Process, pid=(%d)\n", proc.Pid))
	return proc.Pid, nil
}

func (d *NodeDaemon) DaemonProcess() (int, *Error) {
	if err := d.nodeConfig.WritePIDFile(); err != nil {
		return 0, err.AutoStack(nil, nil)
	}
	if err := d.daemonRunPathSocket(); err != nil {
		return 0, err.AutoStack(nil, nil)
	}
	//go d.daemonNotify()
	return 0, nil
}

func (d *NodeDaemon) DaemonProcessClose() {
	if err := d.nodeConfig.RemoveProcessUnixDomainSocket(); err != nil {
		// TODO
	}
	if err := d.nodeConfig.RemovePIDFile(); err != nil {
		// TODO
	}
}

func (d *NodeDaemon) daemonRunPathSocket() *Error {
	l, er := net.Listen("unix", d.nodeConfig.GetUnixDomainSocketPath())
	if er != nil {
		return NewErrorf(ErrCosmosWriteUnixSocketFailed, "write unix socket failed, err=(%v)", er).AutoStack(nil, nil)
	}
	d.socket = l
	go func() {
		defer func() {
			if r := recover(); r != nil {
				//d.logFile.WriteErrorLog(fmt.Sprintf("Daemon Listener Recover, reason=(%v)\n", r))
				// TODO
			}
		}()
		for {
			conn, er := d.socket.Accept()
			if er != nil {
				//d.logFile.WriteErrorLog(fmt.Sprintf("Daemon Listener Accept Failed, err=(%v)\n", er))
				// TODO
				continue
			}
			go d.daemonSocketConnect(conn)
		}
	}()
	return nil
}

func (d *NodeDaemon) GetEnvKey() string {
	return fmt.Sprintf("_GO_ATOMOS_SUPERVISOR_%s", d.nodeConfig.CosmosName)
}

func (d *NodeDaemon) daemonSocketConnect(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			// TODO
			//d.logFile.WriteErrorLog(fmt.Sprintf("Daemon Connection Recover, reason=(%v)\n", r))
		}
	}()
	// TODO
	//d.logFile.WriteAccessLog(fmt.Sprintf("Daemon Connection, from=(%s)\n", conn.RemoteAddr().String()))
	var err *Error
	defer func(conn net.Conn) {
		if err != nil {
			_, er := conn.Write([]byte(fmt.Sprintf("Exit with error=(%v)\n", err)))
			if er != nil {
				// TODO
				//d.logFile.WriteErrorLog(fmt.Sprintf("Daemon Connection Write Failed, err=(%v)\n", er))
			}
		}
		_, er := conn.Write([]byte(d.command.Bye()))
		if er != nil {
			// TODO
			//d.logFile.WriteErrorLog(fmt.Sprintf("Daemon Connection Write Bye Failed, err=(%v)\n", er))
		}
		er = conn.Close()
		if er != nil {
			// TODO
			//d.logFile.WriteErrorLog(fmt.Sprintf("Daemon Connection Close Failed, err=(%v)\n", er))
		}
	}(conn)

	//now := d.command.PrintNow()
	//_, er := conn.Write([]byte(fmt.Sprintf("%s%s\n%s\n\n", hello, d.config.GetNodeFullName(), now)))
	_, er := conn.Write([]byte(fmt.Sprintf("%s\n", d.command.Greeting())))
	if er != nil {
		// TODO
		//d.logFile.WriteErrorLog(fmt.Sprintf("Daemon Connection Write Bye Failed, err=(%v)\n", er))
		return
	}
	buffer := bytes.Buffer{}
	reader := bufio.NewReader(conn)
	for {
		buf, prefix, er := reader.ReadLine()
		if er != nil {
			if errors.Is(er, io.EOF) {
				err = NewError(ErrCosmosUnixSocketConnEOF, "user exit").AutoStack(nil, nil)
				return
			}
			err = NewErrorf(ErrCosmosUnixSocketConnError, "conn read failed, err=(%v)", er).AutoStack(nil, nil)
			return
		}
		buffer.Write(buf)
		if !prefix {
			cmd := buffer.String()
			// TODO
			//d.logFile.WriteAccessLog(fmt.Sprintf("Daemon Connection Request, cmd=(%s)\n", cmd))
			resp, exit, err := d.command.Command(cmd)
			// TODO
			//d.logFile.WriteAccessLog(fmt.Sprintf("Daemon Connection Response, response=(%s),exit=(%v),err=(%v)\n", resp, exit, err))
			if err != nil {
				_, er := conn.Write([]byte(fmt.Sprintf("Command returns error, err=(%v)", err)))
				if er != nil {
					err = NewError(ErrCosmosUnixSocketCommandNotSupported, "command is not supported")
					return
				}
			}
			buffer.Reset()
			if exit {
				// TODO
				//d.logFile.WriteAccessLog(fmt.Sprintf("Daemon Connection Exit\n"))
				return
			}
			if _, er = conn.Write([]byte(resp)); er != nil {
				err = NewErrorf(ErrCosmosUnixSocketConnWriteError, "conn write failed, err=(%v)", er)
				return
			}
		}
	}
}

//func (d *NodeDaemon) daemonNotify() {
//	ch := make(chan os.Signal, 1)
//	signal.Notify(ch)
//	defer signal.Stop(ch)
//	for {
//		select {
//		case s := <-ch:
//			switch s {
//			case os.Interrupt, os.Kill:
//				// TODO
//				//d.logFile.WriteAccessLog("Cosmos Daemon Received Kill Signal")
//				d.ExitCh <- true
//				return
//			}
//		}
//	}
//}
