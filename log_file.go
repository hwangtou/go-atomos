package go_atomos

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"
)

const (
	logMaxSize = 10000000

	logAccessPrefix = "access"
	logErrorPrefix  = "error"

	logNameSep  = "_"
	logFilePerm = 0644
)

type LogFile struct {
	logPath string

	// Because Atomos Logging is thread-safe, so no lock is needed.

	curAccessLog  *os.File
	curAccessSize int

	curErrorLog  *os.File
	curErrorSize int
}

func NewLogFile(logPath string) (*LogFile, *Error) {
	stat, er := os.Stat(logPath)
	if er != nil {
		return nil, NewErrorf(ErrCosmosConfigLogPathInvalid, "invalid log path, err=(%v)", er).AutoStack(nil, nil)
	}
	if !stat.IsDir() {
		return nil, NewErrorf(ErrCosmosConfigLogPathInvalid, "log path is not directory").AutoStack(nil, nil)
	}

	l := &LogFile{
		logPath: logPath,
	}

	// Test Log File.
	logTestPath := logPath + "/test"
	if er = ioutil.WriteFile(logTestPath, []byte{}, 0644); er != nil {
		return nil, NewErrorf(ErrCosmosConfigLogPathInvalid, "log path cannot write, err=(%v)", er).AutoStack(nil, nil)
	}
	if er = os.Remove(logTestPath); er != nil {
		return nil, NewErrorf(ErrCosmosConfigLogPathInvalid, "log path test file cannot delete, err=(%v)", er).AutoStack(nil, nil)
	}

	// Open Log File.
	accessLogFile, err := l.openLogFile(l.logFileFormatter(logAccessPrefix, "startup"))
	if err != nil {
		return nil, err.AutoStack(nil, nil)
	}

	errFile, err := l.openLogFile(l.logFileFormatter(logErrorPrefix, "startup"))
	if err != nil {
		accessLogFile.Close()
		return nil, err.AutoStack(nil, nil)
	}
	l.curAccessLog = accessLogFile
	l.curErrorLog = errFile

	return l, nil
}

func (l *LogFile) Close() {
	er := l.curAccessLog.Close()
	if er != nil {
		// TODO
	}
	l.curAccessLog = nil
	er = l.curErrorLog.Close()
	if er != nil {
		// TODO
	}
	l.curErrorLog = nil
}

func (l *LogFile) openLogFile(path string) (*os.File, *Error) {
	f, er := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, logFilePerm)
	if er != nil {
		return nil, NewErrorf(ErrCosmosLogOpenFailed, "log open failed, path=(%s),err=(%v)", path, er).AutoStack(nil, nil)
	}
	return f, nil
}

func (l *LogFile) logFileFormatter(prefix, flag string) string {
	// {Name}-{DateTime}{-Flag}.log
	datetime := time.Now().Format("2006-0102-150405")

	// Flag
	if len(flag) > 0 {
		flag = logNameSep + flag
	}
	return fmt.Sprintf("%s/%s%s%s%s.log", l.logPath, prefix, logNameSep, datetime, flag)
}

// Log

func (l *LogFile) WriteAccessLog(s string) {
	n, er := l.curAccessLog.WriteString(s)
	if er != nil {
		// TODO: Write to System Error
		return
	}
	l.curAccessSize += n
	if l.curAccessSize >= logMaxSize {
		f, err := l.openLogFile(l.logFileFormatter(logAccessPrefix, ""))
		if err != nil {
			// TODO
		} else {
			er := l.curAccessLog.Close()
			if er != nil {
				// TODO
			}
			l.curAccessLog = f
		}
		l.curAccessSize = 0
	}
}

func (l *LogFile) WriteErrorLog(s string) {
	l.WriteAccessLog(s)

	n, er := l.curErrorLog.WriteString(s)
	if er != nil {
		// TODO: Write to System Error
		return
	}
	l.curErrorSize += n
	if l.curErrorSize >= logMaxSize {
		f, err := l.openLogFile(l.logFileFormatter(logErrorPrefix, ""))
		if err != nil {
			// TODO
		} else {
			er := l.curErrorLog.Close()
			if er != nil {
				// TODO
			}
			l.curErrorLog = f
		}
		l.curErrorSize = 0
	}
}
