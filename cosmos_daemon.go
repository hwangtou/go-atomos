package go_atomos

import (
	"net"
)

//const (
//	pidPath = "/process.pid"
//	pidPerm = 0444
//
//	socketPath = "/process.socket"
//)

type process struct {
	executablePath string
	workPath       string

	env  []string
	args []string
}

type CosmosDaemon struct {
	config  *Config
	logFile *LogFile

	// Unix Socket
	socket  net.Listener
	command CosmosDaemonCommand

	process *process
	ExitCh  chan bool
}

//type CosmosDaemonCommand interface {
//	Command(cmd string) (string, bool, *Error)
//	Greeting() string
//	PrintNow() string
//	Bye() string
//}

func NewCosmosDaemon(config *Config, logFile *LogFile, command CosmosDaemonCommand) *CosmosDaemon {
	return &CosmosDaemon{
		config:  config,
		logFile: logFile,
		socket:  nil,
		command: command,
		process: &process{},
		ExitCh:  make(chan bool),
	}
}

// Parent & Child Process

//func (c *CosmosDaemon) CreateProcess() (daemon bool, pid int, err *Error) {
//	var proc *os.Process
//	if os.Getenv(c.getEnvKey()) != "1" {
//		execPath, er := os.Executable()
//		if er != nil {
//			return false, pid, NewError(ErrCosmosDaemonGetExecutableFailed, er.Error()).AutoStack(nil, nil)
//		}
//		wd, er := os.Getwd()
//		if er != nil {
//			return false, pid, NewError(ErrCosmosDaemonGetExecutableFailed, er.Error()).AutoStack(nil, nil)
//		}
//
//		c.process.executablePath = execPath
//		c.process.workPath = wd
//
//		if len(c.process.args) == 0 {
//			c.process.args = os.Args
//		}
//
//		if len(c.process.env) == 0 {
//			c.process.env = os.Environ()
//		}
//		c.process.env = append(c.process.env, fmt.Sprintf("%s=%s", c.getEnvKey(), "1"))
//
//		// Fork Process
//		attr := &os.ProcAttr{
//			Dir:   c.process.workPath,
//			Env:   c.process.env,
//			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
//			Sys:   &syscall.SysProcAttr{Setsid: true},
//		}
//		proc, er = os.StartProcess(c.process.executablePath, c.process.args, attr)
//		if er != nil {
//			return false, pid, NewError(ErrCosmosDaemonStartProcessFailed, er.Error()).AutoStack(nil, nil)
//		}
//		c.logFile.WriteAccessLog(fmt.Sprintf("Daemon Process, pid=(%d)\n", proc.Pid))
//		return false, proc.Pid, nil
//	} else {
//		return true, pid, nil
//	}
//}

//func ForkProcess(args, env []string) *Error {
//	//execPath, er := os.Executable()
//	//if er != nil {
//	//	return NewError(ErrCosmosDaemonGetExecutableFailed, er.Error()).AutoStack(nil, nil)
//	//}
//	//wd, er := os.Getwd()
//	//if er != nil {
//	//	return NewError(ErrCosmosDaemonGetExecutableFailed, er.Error()).AutoStack(nil, nil)
//	//}
//	//
//	////c.process.executablePath = execPath
//	////c.process.workPath = wd
//	//
//	////if len(c.process.args) == 0 {
//	////	c.process.args = os.Args
//	////}
//	//
//	////if len(c.process.env) == 0 {
//	////	c.process.env = os.Environ()
//	////}
//	////c.process.env = append(c.process.env, fmt.Sprintf("%s=%s", c.getEnvKey(), "1"))
//	//
//	//// Fork Process
//	//attr := &os.ProcAttr{
//	//	Dir:   wd,  //c.process.workPath,
//	//	Env:   env, //c.process.env,
//	//	Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
//	//	Sys:   &syscall.SysProcAttr{Setsid: true},
//	//}
//	//proc, er := os.StartProcess(execPath, args, attr)
//	//if er != nil {
//	//	return nil
//	//}
//	////c.logFile.WriteAccessLog(fmt.Sprintf("Daemon Process, pid=(%d)\n", proc.Pid))
//	return nil
//}

//func (c *CosmosDaemon) getEnvKey() string {
//	return fmt.Sprintf("_GO_ATOMOS_SUPERVISOR_%s", c.config.Cosmos)
//}

//// Check
//
//func (c *CosmosDaemon) Check() (hasRun bool, err *Error) {
//	if err := c.checkRunPath(); err != nil {
//		return false, err.AutoStack(nil, nil)
//	}
//	hasRun, err = c.checkRunPathProcessID()
//	if err != nil {
//		return hasRun, err.AutoStack(nil, nil)
//	}
//	if err := c.checkRunPathDaemonSocket(); err != nil {
//		return false, err.AutoStack(nil, nil)
//	}
//
//	return false, nil
//}
//
//func (c *CosmosDaemon) checkRunPath() *Error {
//	runPath := c.config.RunPath
//	stat, er := os.Stat(runPath)
//	if er != nil {
//		return NewErrorf(ErrCosmosConfigRunPathInvalid, "invalid run path, err=(%v)", er).AutoStack(nil, nil)
//	}
//	if !stat.IsDir() {
//		return NewErrorf(ErrCosmosConfigRunPathInvalid, "run path is not directory").AutoStack(nil, nil)
//	}
//
//	runTestPath := runPath + "/test"
//	if er = ioutil.WriteFile(runTestPath, []byte{}, 0644); er != nil {
//		return NewErrorf(ErrCosmosConfigRunPathInvalid, "run path cannot write, err=(%v)", er).AutoStack(nil, nil)
//	}
//	if er = os.Remove(runTestPath); er != nil {
//		return NewErrorf(ErrCosmosConfigRunPathInvalid, "run path test file cannot delete, err=(%v)", er).AutoStack(nil, nil)
//	}
//
//	return nil
//}
//
//func (c *CosmosDaemon) checkRunPathProcessID() (bool, *Error) {
//	runPIDPath := c.config.RunPath + pidPath
//	pidBuf, er := ioutil.ReadFile(runPIDPath)
//	if er != nil {
//		return false, nil
//	}
//
//	processID, er := strconv.ParseInt(string(pidBuf), 10, 64)
//	if er != nil {
//		return false, NewErrorf(ErrCosmosConfigRunPIDInvalid, "run pid invalid, err=(%v)", er).AutoStack(nil, nil)
//	}
//
//	var ok bool
//	var errno syscall.Errno
//	proc, er := os.FindProcess(int(processID))
//	if er != nil {
//		// TODO
//		goto removePID
//	}
//	er = proc.Signal(syscall.Signal(0))
//	if er == nil {
//		return true, NewErrorf(ErrCosmosConfigRunPIDIsRunning, "run pid process is running, pid=(%d),err=(%v)", processID, er).AutoStack(nil, nil)
//	}
//	if er.Error() == "os: process already finished" {
//		goto removePID
//	}
//	errno, ok = er.(syscall.Errno)
//	if !ok {
//		goto removePID
//	}
//	switch errno {
//	case syscall.ESRCH:
//		goto removePID
//	case syscall.EPERM:
//		return true, NewErrorf(ErrCosmosConfigRunPIDIsRunning, "run pid process EPERM, pid=(%d),err=(%v)", processID, er).AutoStack(nil, nil)
//	}
//
//removePID:
//	if er = os.Remove(runPIDPath); er != nil {
//		return false, NewErrorf(ErrCosmosConfigRemovePIDPathFailed, "run pid file remove failed, pid=(%d),err=(%v)", processID, er).AutoStack(nil, nil)
//	}
//	return false, nil
//}
//
//func (c *CosmosDaemon) checkRunPathDaemonSocket() *Error {
//	runSocketPath := c.config.RunPath + socketPath
//	_, er := os.Stat(runSocketPath)
//	if er != nil && !os.IsNotExist(er) {
//		return NewErrorf(ErrCosmosConfigRunPIDPathInvalid, "check run path daemon socket failed, path=(%s),err=(%v)", runSocketPath, er).AutoStack(nil, nil)
//	}
//	if er == nil {
//		if er = os.Remove(runSocketPath); er != nil {
//			return NewErrorf(ErrCosmosConfigRunPIDPathInvalid, "run socket path remove failed, path=(%s),err=(%v)", runSocketPath, er).AutoStack(nil, nil)
//		}
//	}
//	return nil
//}

// Daemon

//func (c *CosmosDaemon) Daemon() *Error {
//	if err := c.daemonRunPathProcess(); err != nil {
//		return err.AutoStack(nil, nil)
//	}
//	if err := c.daemonRunPathSocket(); err != nil {
//		return err.AutoStack(nil, nil)
//	}
//	go c.daemonNotify()
//	return nil
//}

//func (c *CosmosDaemon) daemonRunPathProcess() *Error {
//	runPIDPath := c.config.RunPath + pidPath
//	pidBuf := strconv.FormatInt(int64(os.Getpid()), 10)
//	if er := ioutil.WriteFile(runPIDPath, []byte(pidBuf), pidPerm); er != nil {
//		return NewErrorf(ErrCosmosWritePIDFileFailed, "write pid file failed, err=(%v)", er).AutoStack(nil, nil)
//	}
//	return nil
//}

//func (c *CosmosDaemon) daemonRunPathSocket() *Error {
//	runSocketPath := c.config.RunPath + socketPath
//	l, er := net.Listen("unix", runSocketPath)
//	if er != nil {
//		return NewErrorf(ErrCosmosWriteUnixSocketFailed, "write unix socket failed, err=(%v)", er).AutoStack(nil, nil)
//	}
//	c.socket = l
//	go c.daemonSocketListener()
//	return nil
//}
//
//func (c *CosmosDaemon) daemonSocketListener() {
//	defer func() {
//		if r := recover(); r != nil {
//			c.logFile.WriteErrorLog(fmt.Sprintf("Daemon Listener Recover, reason=(%v)\n", r))
//		}
//	}()
//	for {
//		conn, er := c.socket.Accept()
//		if er != nil {
//			c.logFile.WriteErrorLog(fmt.Sprintf("Daemon Listener Accept Failed, err=(%v)\n", er))
//			continue
//		}
//		go c.daemonSocketConnect(conn)
//	}
//}
//
//func (c *CosmosDaemon) daemonSocketConnect(conn net.Conn) {
//	defer func() {
//		if r := recover(); r != nil {
//			c.logFile.WriteErrorLog(fmt.Sprintf("Daemon Connection Recover, reason=(%v)\n", r))
//		}
//	}()
//	c.logFile.WriteAccessLog(fmt.Sprintf("Daemon Connection, from=(%s)\n", conn.RemoteAddr().String()))
//	var err *Error
//	defer func(conn net.Conn) {
//		if err != nil {
//			_, er := conn.Write([]byte(fmt.Sprintf("Exit with error=(%v)\n", err)))
//			if er != nil {
//				c.logFile.WriteErrorLog(fmt.Sprintf("Daemon Connection Write Failed, err=(%v)\n", er))
//			}
//		}
//		_, er := conn.Write([]byte(c.command.Bye()))
//		if er != nil {
//			c.logFile.WriteErrorLog(fmt.Sprintf("Daemon Connection Write Bye Failed, err=(%v)\n", er))
//		}
//		er = conn.Close()
//		if er != nil {
//			c.logFile.WriteErrorLog(fmt.Sprintf("Daemon Connection Close Failed, err=(%v)\n", er))
//		}
//	}(conn)
//
//	hello := c.command.Greeting()
//	now := c.command.PrintNow()
//	_, er := conn.Write([]byte(fmt.Sprintf("%s%s\n%s\n\n", hello, c.config.GetNodeFullName(), now)))
//	if er != nil {
//		c.logFile.WriteErrorLog(fmt.Sprintf("Daemon Connection Write Bye Failed, err=(%v)\n", er))
//		return
//	}
//	buffer := bytes.Buffer{}
//	reader := bufio.NewReader(conn)
//	for {
//		buf, prefix, er := reader.ReadLine()
//		if er != nil {
//			if errors.Is(er, io.EOF) {
//				err = NewError(ErrCosmosUnixSocketConnEOF, "user exit").AutoStack(nil, nil)
//				return
//			}
//			err = NewErrorf(ErrCosmosUnixSocketConnError, "conn read failed, err=(%v)", er).AutoStack(nil, nil)
//			return
//		}
//		buffer.Write(buf)
//		if !prefix {
//			cmd := buffer.String()
//			c.logFile.WriteAccessLog(fmt.Sprintf("Daemon Connection Request, cmd=(%s)\n", cmd))
//			resp, exit, err := c.command.Command(cmd)
//			c.logFile.WriteAccessLog(fmt.Sprintf("Daemon Connection Response, response=(%s),exit=(%v),err=(%v)\n", resp, exit, err))
//			if err != nil {
//				_, er := conn.Write([]byte(fmt.Sprintf("Command returns error, err=(%v)", err)))
//				if er != nil {
//					err = NewError(ErrCosmosUnixSocketCommandNotSupported, "command is not supported")
//					return
//				}
//			}
//			buffer.Reset()
//			if exit {
//				c.logFile.WriteAccessLog(fmt.Sprintf("Daemon Connection Exit\n"))
//				return
//			}
//			if _, er = conn.Write([]byte(resp)); er != nil {
//				err = NewErrorf(ErrCosmosUnixSocketConnWriteError, "conn write failed, err=(%v)", er)
//				return
//			}
//		}
//	}
//}

//func (c *CosmosDaemon) daemonNotify() {
//	ch := make(chan os.Signal, 1)
//	signal.Notify(ch)
//	defer signal.Stop(ch)
//	for {
//		select {
//		case s := <-ch:
//			switch s {
//			case os.Interrupt, os.Kill:
//				c.logFile.WriteAccessLog("Cosmos Daemon Received Kill Signal")
//				c.ExitCh <- true
//				return
//			}
//		}
//	}
//}

//// Close
//
//func (c *CosmosDaemon) Close() {
//	c.closeSocket()
//	c.deletePIDFile()
//}
//
//func (c *CosmosDaemon) closeSocket() {
//	runSocketPath := c.config.RunPath + socketPath
//	er := os.Remove(runSocketPath)
//	if er != nil {
//		// TODO
//	}
//}
//
//func (c *CosmosDaemon) deletePIDFile() {
//	runPIDPath := c.config.RunPath + pidPath
//	er := os.Remove(runPIDPath)
//	if er != nil {
//		// TODO
//	}
//}

// Exit

func (c *CosmosDaemon) Exit() {
	c.ExitCh <- true
}
