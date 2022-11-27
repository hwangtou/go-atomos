package go_atomos

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"
)

const (
	pidPath = "/process.pid"
	pidPerm = 0444

	socketPath = "/process.socket"

	logMaxSize   = 100 //10000000
	logDaemon    = "daemon"
	logPrefix    = "access"
	logErrPrefix = "error"
	logNameSep   = "_"
	logFilePerm  = 0644
)

type cosmosDaemon struct {
	configPath string

	config *Config

	// Log
	// Because Atomos Logging is thread-safe, so no lock is needed.
	curLogFile *os.File
	curLogSize int
	curErrFile *os.File
	curErrSize int

	// Unix Socket
	socket net.Listener
}

func NewCosmosDaemon(configPath string) *cosmosDaemon {
	return &cosmosDaemon{
		configPath: configPath,
		config:     nil,
	}
}

func (c *cosmosDaemon) Check() *Error {
	if err := c.checkConfigPath(); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.checkLogPath(); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.checkRunPath(); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.checkRunPathProcessID(); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.checkRunPathDaemonSocket(); err != nil {
		return err.AutoStack(nil, nil)
	}

	return nil
}

func (c *cosmosDaemon) checkConfigPath() *Error {
	conf, err := ConfigFromYaml(c.configPath)
	if err != nil {
		return err.AutoStack(nil, err)
	}
	if err = conf.Check(); err != nil {
		return err.AutoStack(nil, err)
	}
	c.config = conf

	return nil
}

func (c *cosmosDaemon) checkLogPath() *Error {
	logPath := c.config.LogPath
	stat, er := os.Stat(logPath)
	if er != nil {
		return NewErrorf(ErrCosmosConfigLogPathInvalid, "invalid log path, err=(%v)", er).AutoStack(nil, nil)
	}
	if !stat.IsDir() {
		return NewErrorf(ErrCosmosConfigLogPathInvalid, "log path is not directory").AutoStack(nil, nil)
	}

	logTestPath := logPath + "/test"
	if er = ioutil.WriteFile(logTestPath, []byte{}, 0644); er != nil {
		return NewErrorf(ErrCosmosConfigLogPathInvalid, "log path cannot write, err=(%v)", er).AutoStack(nil, nil)
	}
	if er = os.Remove(logTestPath); er != nil {
		return NewErrorf(ErrCosmosConfigLogPathInvalid, "log path test file cannot delete, err=(%v)", er).AutoStack(nil, nil)
	}

	return nil
}

func (c *cosmosDaemon) checkRunPath() *Error {
	runPath := c.config.RunPath
	stat, er := os.Stat(runPath)
	if er != nil {
		return NewErrorf(ErrCosmosConfigRunPathInvalid, "invalid run path, err=(%v)", er).AutoStack(nil, nil)
	}
	if !stat.IsDir() {
		return NewErrorf(ErrCosmosConfigRunPathInvalid, "run path is not directory").AutoStack(nil, nil)
	}

	runTestPath := runPath + "/test"
	if er = ioutil.WriteFile(runTestPath, []byte{}, 0644); er != nil {
		return NewErrorf(ErrCosmosConfigRunPathInvalid, "run path cannot write, err=(%v)", er).AutoStack(nil, nil)
	}
	if er = os.Remove(runTestPath); er != nil {
		return NewErrorf(ErrCosmosConfigRunPathInvalid, "run path test file cannot delete, err=(%v)", er).AutoStack(nil, nil)
	}

	return nil
}

func (c *cosmosDaemon) checkRunPathProcessID() *Error {
	runPIDPath := c.config.RunPath + pidPath
	pidBuf, er := ioutil.ReadFile(runPIDPath)
	if er != nil {
		return nil
	}

	processID, er := strconv.ParseInt(string(pidBuf), 10, 64)
	if er != nil {
		return NewErrorf(ErrCosmosConfigRunPIDInvalid, "run pid invalid, err=(%v)", er).AutoStack(nil, nil)
	}

	var ok bool
	var errno syscall.Errno
	proc, er := os.FindProcess(int(processID))
	if er != nil {
		// TODO
		goto removePID
	}
	er = proc.Signal(syscall.Signal(0))
	if er == nil {
		return NewErrorf(ErrCosmosConfigRunPIDIsRunning, "run pid process is running, pid=(%d),err=(%v)", processID, er).AutoStack(nil, nil)
	}
	if er.Error() == "os: process already finished" {
		goto removePID
	}
	errno, ok = er.(syscall.Errno)
	if !ok {
		goto removePID
	}
	switch errno {
	case syscall.ESRCH:
		goto removePID
	case syscall.EPERM:
		return NewErrorf(ErrCosmosConfigRunPIDIsRunning, "run pid process is running, pid=(%d),err=(%v)", processID, er).AutoStack(nil, nil)
	}

removePID:
	if er = os.Remove(runPIDPath); er != nil {
		return NewErrorf(ErrCosmosConfigRemovePIDPathFailed, "run pid file remove failed, pid=(%d),err=(%v)", processID, er).AutoStack(nil, nil)
	}
	return nil
}

func (c *cosmosDaemon) checkRunPathDaemonSocket() *Error {
	runSocketPath := c.config.RunPath + socketPath
	_, er := os.Stat(runSocketPath)
	if er != nil && !os.IsNotExist(er) {
		return NewErrorf(ErrCosmosConfigRunPIDPathInvalid, "check run path daemon socket failed, path=(%s),err=(%v)", runSocketPath, er).AutoStack(nil, nil)
	}
	if er == nil {
		if er = os.Remove(runSocketPath); er != nil {
			return NewErrorf(ErrCosmosConfigRunPIDPathInvalid, "run socket path remove failed, path=(%s),err=(%v)", runSocketPath, er).AutoStack(nil, nil)
		}
	}
	return nil
}

func (c *cosmosDaemon) Daemon() *Error {
	if err := c.daemonLogPath(); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.daemonRunPathProcessID(); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.daemonRunPathDaemonSocket(); err != nil {
		return err.AutoStack(nil, nil)
	}
	return nil
}

func (c *cosmosDaemon) daemonLogPath() *Error {
	logFile, err := c.openLogFile(c.logFileFormatter(true, false, "startup"))
	if err != nil {
		return err.AutoStack(nil, nil)
	}
	c.curLogFile = logFile

	errFile, err := c.openLogFile(c.logFileFormatter(false, true, "startup"))
	if err != nil {
		c.curLogFile.Close()
		return err.AutoStack(nil, nil)
	}
	c.curErrFile = errFile

	return nil
}

func (c *cosmosDaemon) daemonRunPathProcessID() *Error {
	runPIDPath := c.config.RunPath + pidPath
	pidBuf := strconv.FormatInt(int64(os.Getpid()), 10)
	if er := ioutil.WriteFile(runPIDPath, []byte(pidBuf), pidPerm); er != nil {
		return NewErrorf(ErrCosmosWritePIDFileFailed, "write pid file failed, err=(%v)", er).AutoStack(nil, nil)
	}
	return nil
}

func (c *cosmosDaemon) daemonRunPathDaemonSocket() *Error {
	runSocketPath := c.config.RunPath + socketPath
	l, er := net.Listen("unix", runSocketPath)
	if er != nil {
		return NewErrorf(ErrCosmosWriteUnixSocketFailed, "write unix socket failed, err=(%v)", er).AutoStack(nil, nil)
	}
	c.socket = l
	go c.daemonSocketListener()
	return nil
}

func (c *cosmosDaemon) daemonSocketListener() {
	defer func() {
		if r := recover(); r != nil {
			// TODO
		}
	}()
	for {
		conn, er := c.socket.Accept()
		if er != nil {
			// TODO
			continue
		}
		go c.daemonSocketConnect(conn)
	}
}

func (c *cosmosDaemon) daemonSocketConnect(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			// TODO
		}
	}()
	var err *Error
	defer func(conn net.Conn) {
		if err != nil {
			conn.Write([]byte(fmt.Sprintf("Exit with error=(%v)\n", err)))
		}
		conn.Write([]byte("Bye!\n"))
		err := conn.Close()
		if err != nil {
			// TODO
		}
	}(conn)

	hello := "Hello Atomos!"
	now := "Now:\t" + time.Now().Format("2006-01-02 15:04:05 MST -07:00")
	_, er := conn.Write([]byte(fmt.Sprintf("%s\n%s\n%s\n\n", hello, c.config.GetNodeFullName(), now)))
	if er != nil {
		return
	}
	buffer := bytes.Buffer{}
	reader := bufio.NewReader(conn)
	for {
		buf, prefix, er := reader.ReadLine()
		if er != nil {
			if errors.Is(er, io.EOF) {
				err = NewError(ErrCosmosUnixSocketConnEOF, "user exit")
				return
			}
			err = NewErrorf(ErrCosmosUnixSocketConnError, "conn read failed, err=(%v)", er)
			return
		}
		buffer.Write(buf)
		if !prefix {
			response, exit := c.handleCommand(buffer.Bytes())
			buffer.Reset()
			if exit {
				err = NewError(ErrCosmosUnixSocketConnShouldQuit, "conn should quit")
				return
			}
			if _, er = conn.Write(response); er != nil {
				err = NewErrorf(ErrCosmosUnixSocketConnWriteError, "conn write failed, err=(%v)", er)
				return
			}
		}
	}
}

func (c *cosmosDaemon) handleCommand(buf []byte) ([]byte, bool) {
	b := bytes.Split(buf, []byte(":"))
	switch string(b[0]) {
	case "exit":
		return []byte("will exit\n"), true
	case "write_test":
		for i := 0; i < 100; i += 1 {
			c.writeLog("abcdefghij\n")
			time.Sleep(100 * time.Millisecond)
		}
		//for i := 0; i < 10000; i += 1 {
		//	c.writeErr("1234567890")
		//}
		return []byte("OK"), false
	default:
		return []byte("test\n"), false
	}
}

func (c *cosmosDaemon) Exit() {
	c.closeSocket()
	c.deletePIDFile()
	c.closeLogFile()
}

func (c *cosmosDaemon) closeSocket() {
	runSocketPath := c.config.RunPath + socketPath
	er := os.Remove(runSocketPath)
	if er != nil {
		// TODO
	}
}

func (c *cosmosDaemon) deletePIDFile() {
	runPIDPath := c.config.RunPath + pidPath
	er := os.Remove(runPIDPath)
	if er != nil {
		// TODO
	}
}

func (c *cosmosDaemon) closeLogFile() {
	er := c.curLogFile.Close()
	if er != nil {
		// TODO
	}
	c.curLogFile = nil
	er = c.curErrFile.Close()
	if er != nil {
		// TODO
	}
	c.curErrFile = nil
}

// Log

func (c *cosmosDaemon) writeLog(s string) {
	n, er := c.curLogFile.WriteString(s)
	if er != nil {
		// TODO: Write to System Error
		return
	}
	c.curLogSize += n
	log.Println("size =", c.curLogSize)
	if c.curLogSize >= logMaxSize {
		log.Println("new log file")
		f, err := c.openLogFile(c.logFileFormatter(true, false, ""))
		if err != nil {
			// TODO
			log.Println("new log file err=", err)
		} else {
			er := c.curLogFile.Close()
			if er != nil {
				// TODO
				log.Println("new log file close err=", er)
			}
			c.curLogFile = f
			log.Println("new log file f=", f.Name())
		}
		c.curLogSize = 0
	}
}

func (c *cosmosDaemon) writeErr(s string) {
	n, er := c.curErrFile.WriteString(s)
	if er != nil {
		// TODO: Write to System Error
		return
	}
	c.curErrSize += n
	log.Println("size =", c.curErrSize)
	if c.curErrSize >= logMaxSize {
		log.Println("new error file")
		f, err := c.openLogFile(c.logFileFormatter(false, true, ""))
		if err != nil {
			// TODO
		} else {
			er := c.curErrFile.Close()
			if er != nil {
				// TODO
			}
			c.curErrFile = f
		}
		c.curErrSize = 0
	}
}

func (c *cosmosDaemon) logFileFormatter(isLog, isError bool, flag string) string {
	// Dir Path
	dirPath := c.config.LogPath

	// {Name}-{DateTime}{-Flag}.log
	var name, datetime string

	// Name
	if isLog {
		name = logPrefix
	} else if isError {
		name = logErrPrefix
	} else {
		name = logDaemon
	}

	// Datetime
	datetime = time.Now().Format("2006-01-02-15-04-05")

	// Flag
	if len(flag) > 0 {
		flag = logNameSep + flag
	}
	return fmt.Sprintf("%s/%s%s%s%s.log", dirPath, name, logNameSep, datetime, flag)
}

func (c *cosmosDaemon) openLogFile(path string) (*os.File, *Error) {
	f, er := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, logFilePerm)
	if er != nil {
		return nil, NewErrorf(ErrCosmosLogOpenFailed, "log open failed, path=(%s),err=(%v)", path, er).AutoStack(nil, nil)
	}
	return f, nil
}
