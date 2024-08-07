package go_atomos

import (
	"fmt"
	"log"
	"os"
	"strings"
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

type appLoggingIntf interface {
	WriteAccessLog(s string)
	WriteErrorLog(s string)
	Close()
	getCurAccessLogName() string
	getCurErrorLogName() string
}

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
		base, _ := os.Getwd()
		return nil, NewErrorf(ErrAppEnvLoggingPathInvalid, "invalid log path. base=(%s),path=(%s),err=(%v)", base, logPath, er).AddStack(nil)
	}
	if !stat.IsDir() {
		return nil, NewErrorf(ErrAppEnvLoggingPathInvalid, "log path is not directory").AddStack(nil)
	}

	l := &appLogging{
		logPath:    logPath,
		logMaxSize: logMaxSize,
	}

	// Test Log File.
	logTestPath := logPath + "/test"
	if er = os.WriteFile(logTestPath, []byte{}, 0644); er != nil {
		return nil, NewErrorf(ErrAppEnvLoggingPathInvalid, "log path cannot write, err=(%v)", er).AddStack(nil)
	}
	if er = os.Remove(logTestPath); er != nil {
		return nil, NewErrorf(ErrAppEnvLoggingPathInvalid, "log path test file cannot delete, err=(%v)", er).AddStack(nil)
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
	//os.Stdout = l.curAccessLog
	log.Default().SetOutput(l.curAccessLog)

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
	os.Stderr = l.curErrorLog

	return l, nil
}

func (l *appLogging) Close() {
	am := allocAtomosMail()
	initKillMail(am, nil, nil)
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
		return nil, NewErrorf(ErrAppEnvLoggingFileOpenFailed, "log open failed, path=(%s),err=(%v)", path, er).AddStack(nil)
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
			l.curAccessLogName = newName
			l.curAccessLog = f
			//os.Stdout = l.curAccessLog
			log.Default().SetOutput(l.curAccessLog)
			er := l.curAccessLog.Close()
			if er != nil {
				// TODO
			}
		}
		l.curAccessSize = 0
	}
}

func (l *appLogging) WriteErrorLog(s string) {
	l.WriteAccessLog(s)

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
			l.curErrorLogName = newName
			l.curErrorLog = f
			os.Stderr = l.curErrorLog
			er := l.curErrorLog.Close()
			if er != nil {
				// TODO
			}
		}
		l.curErrorSize = 0
	}
}

func (l *appLogging) getCurAccessLogName() string {
	return l.curAccessLogName
}

func (l *appLogging) getCurErrorLogName() string {
	return l.curErrorLogName
}

type appLoggingForTest struct {
	t *testing.T
}

func (l *appLoggingForTest) WriteAccessLog(s string) {
	l.t.Log(strings.TrimSuffix(s, "\n"))
}

func (l *appLoggingForTest) WriteErrorLog(s string) {
	l.t.Error(strings.TrimSuffix(s, "\n"))
}

func (l *appLoggingForTest) Close() {
}

func (l *appLoggingForTest) getCurAccessLogName() string {
	return ""
}

func (l *appLoggingForTest) getCurErrorLogName() string {
	return ""
}
