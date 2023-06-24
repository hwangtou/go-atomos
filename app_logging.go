package go_atomos

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

const (
	defaultLogMaxSize = 10000000

	logAccessPrefix = "access"
	logErrorPrefix  = "error"

	logNameFormatter = "2006-0102-150405"
	logNameSep       = "."
	logFilePerm      = 0644
)

var (
	appLoggingTestingT *testing.T
)

type appLogging struct {
	logPath    string
	logMaxSize int

	// Because Atomos Logging is thread-safe, so no lock is needed.

	curAccessLogName string
	curAccessLog     *os.File
	curAccessSize    int

	curErrorLogName string
	curErrorLog     *os.File
	curErrorSize    int
}

func NewAppLogging(logPath string, logMaxSize int) (*appLogging, *Error) {
	stat, er := os.Stat(logPath)
	if er != nil {
		return nil, NewErrorf(ErrAppLoggingPathInvalid, "invalid log path. path=(%s),err=(%v)", logPath, er).AddStack(nil)
	}
	if !stat.IsDir() {
		return nil, NewErrorf(ErrAppLoggingPathInvalid, "log path is not directory").AddStack(nil)
	}

	l := &appLogging{
		logPath:    logPath,
		logMaxSize: logMaxSize,
	}

	// Test Log File.
	logTestPath := logPath + "/test"
	if er = ioutil.WriteFile(logTestPath, []byte{}, 0644); er != nil {
		return nil, NewErrorf(ErrAppLoggingPathInvalid, "log path cannot write, err=(%v)", er).AddStack(nil)
	}
	if er = os.Remove(logTestPath); er != nil {
		return nil, NewErrorf(ErrAppLoggingPathInvalid, "log path test file cannot delete, err=(%v)", er).AddStack(nil)
	}

	// Open Log File.
	l.curAccessLogName = os.Getenv(GetEnvAccessLogKey())
	if l.curAccessLogName == "" {
		l.curAccessLogName = l.logFileFormatter(logAccessPrefix, "startup")
	}
	accessLogFile, err := l.openLogFile(l.curAccessLogName)
	if err != nil {
		return nil, err.AddStack(nil)
	}
	l.curAccessLog = accessLogFile

	l.curErrorLogName = os.Getenv(GetEnvErrorLogKey())
	if l.curErrorLogName == "" {
		l.curErrorLogName = l.logFileFormatter(logErrorPrefix, "startup")
	}
	errLogFile, err := l.openLogFile(l.curErrorLogName)
	if err != nil {
		l.curAccessLog.Close()
		return nil, err.AddStack(nil)
	}
	l.curErrorLog = errLogFile

	return l, nil
}

func (l *appLogging) Close() {
	am := allocAtomosMail()
	initKillMail(am, nil, "")
	sharedCosmosProcess.logging.logBox.pushHead(am.mail)
	<-am.waitCh
	_ = l.curAccessLog.Close()
	l.curAccessLog = nil
	_ = l.curErrorLog.Close()
	l.curErrorLog = nil
}

func (l *appLogging) openLogFile(path string) (*os.File, *Error) {
	f, er := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, logFilePerm)
	if er != nil {
		return nil, NewErrorf(ErrAppLoggingFileOpenFailed, "log open failed, path=(%s),err=(%v)", path, er).AddStack(nil)
	}
	return f, nil
}

func (l *appLogging) logFileFormatter(prefix, flag string) string {
	// {Name}-{DateTime}{-Flag}.log
	datetime := time.Now().Format(logNameFormatter)

	// Flag
	if len(flag) > 0 {
		flag = logNameSep + flag
	}
	return fmt.Sprintf("%s/%s%s%s%s.log", l.logPath, prefix, logNameSep, datetime, flag)
}

// Log

func (l *appLogging) WriteAccessLog(s string) {
	if LogStdout {
		_, _ = os.Stdout.WriteString(s)
	}
	if logTestOut && appLoggingTestingT != nil {
		appLoggingTestingT.Log(s)
	}
	n, er := l.curAccessLog.WriteString(s)
	if er != nil {
		// TODO: Write to System Error
		return
	}
	l.curAccessSize += n
	if l.curAccessSize >= l.logMaxSize {
		newName := l.logFileFormatter(logAccessPrefix, "")
		f, err := l.openLogFile(newName)
		if err != nil {
			// TODO
		} else {
			er := l.curAccessLog.Close()
			if er != nil {
				// TODO
			}
			l.curAccessLogName = newName
			l.curAccessLog = f
		}
		l.curAccessSize = 0
	}
}

func (l *appLogging) WriteErrorLog(s string) {
	l.WriteAccessLog(s)

	if LogStderr {
		_, _ = os.Stdout.WriteString(s)
		_, _ = os.Stderr.WriteString(s)
	}
	if logTestErr && appLoggingTestingT != nil {
		appLoggingTestingT.Error(s)
	}
	n, er := l.curErrorLog.WriteString(s)
	if er != nil {
		// TODO: Write to System Error
		return
	}
	l.curErrorSize += n
	if l.curErrorSize >= l.logMaxSize {
		newName := l.logFileFormatter(logErrorPrefix, "")
		f, err := l.openLogFile(newName)
		if err != nil {
			// TODO
		} else {
			er := l.curErrorLog.Close()
			if er != nil {
				// TODO
			}
			l.curErrorLogName = newName
			l.curErrorLog = f
		}
		l.curErrorSize = 0
	}
}
