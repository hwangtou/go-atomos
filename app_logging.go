package go_atomos

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	// AppLoggingDefaultMaxSize Default log max size is 10MB.
	AppLoggingDefaultMaxSize = 10 * 1024 * 1024
	// AppLoggingAutoCleanupRatio Default auto cleanup size is 300MB.
	AppLoggingAutoCleanupRatio = int64(30)
	AppLoggingAutoCleanupOff   = -1

	AppLoggingAccessPrefix  = "access"
	AppLoggingErrorPrefix   = "error"
	AppLoggingNameFormatter = "2006-0102-150405"
	AppLoggingNameSep       = "."
	AppLoggingPathPerm      = os.FileMode(0774)
	AppLoggingFilePerm      = os.FileMode(0664)
)

type appLogging interface {
	WriteAccessLog(s string)
	WriteErrorLog(s string)
}

// AppLoggingToFile is the logging to file implementation.

type AppLoggingToFile struct {
	logPath        string
	logFileMaxSize int64
	logPathMaxSize int64
	// Auto Cleanup
	autoCleanup bool
	// Error Handler
	errHandler func(*Error)

	// Because Atomos Logging is thread-safe, so no lock is needed.

	curAccessLogName string
	curAccessLog     *os.File
	curAccessSize    int64

	curErrorLogName string
	curErrorLog     *os.File
	curErrorSize    int64
}

// NewAppLoggingToFile creates a new AppLoggingToFile instance.
// logPath: the path to store log files.
// logFileMaxSize: the max size of a log file. If it is 0, then it is 10MB.
// logPathMaxRatio: the max size of the log path. If it is 0, then it is 30 times of logFileMaxSize. If it is -1, then auto cleanup is off. Use a number more than 2 is recommended.
// errHandler: the error handler. If it is nil, then the error will be ignored.
func NewAppLoggingToFile(logPath string, logFileMaxSize, logPathMaxRatio int64, errHandler func(*Error)) (*AppLoggingToFile, *Error) {
	if err := UtilFileEnsureDirectory(logPath, AppLoggingPathPerm, true); err != nil {
		return nil, err.AddStack(nil)
	}

	// Log File Max Size
	if logFileMaxSize <= 0 {
		logFileMaxSize = AppLoggingDefaultMaxSize
	}
	// Auto Cleanup
	autoCleanup := true
	logPathMaxSize := int64(0)
	if logPathMaxRatio == AppLoggingAutoCleanupOff {
		autoCleanup = false
	} else if logPathMaxRatio == 0 {
		logPathMaxSize = logFileMaxSize * AppLoggingAutoCleanupRatio
	} else {
		logPathMaxSize = logFileMaxSize * logPathMaxRatio
	}

	l := &AppLoggingToFile{
		logPath:        logPath,
		logFileMaxSize: logFileMaxSize,
		logPathMaxSize: logPathMaxSize,
		autoCleanup:    autoCleanup,
		errHandler:     errHandler,
	}

	// Open Log File.
	l.curAccessLogName = l.logFileFormatter(AppLoggingAccessPrefix, "startup")
	accessLogFile, err := l.openLogFile(l.curAccessLogName)
	if err != nil {
		return nil, err.AddStack(nil)
	}
	l.curAccessLog = accessLogFile

	l.curErrorLogName = l.logFileFormatter(AppLoggingErrorPrefix, "startup")
	errLogFile, err := l.openLogFile(l.curErrorLogName)
	if err != nil {
		_ = l.curAccessLog.Close()
		return nil, err.AddStack(nil)
	}
	l.curErrorLog = errLogFile

	l.redirectStd()

	return l, nil
}

func (l *AppLoggingToFile) openLogFile(path string) (*os.File, *Error) {
	f, er := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, AppLoggingFilePerm)
	if er != nil {
		return nil, NewErrorf(ErrAppEnvLoggingFileOpenFailed, "log open failed, path=(%s),err=(%v)", path, er).AddStack(nil)
	}
	return f, nil
}

func (l *AppLoggingToFile) logFileFormatter(prefix, flag string) string {
	// {Name}-{DateTime}{-Flag}.log
	datetime := time.Now().Format(AppLoggingNameFormatter)

	// Flag
	if len(flag) > 0 {
		flag = AppLoggingNameSep + flag
	}
	return fmt.Sprintf("%s/%s%s%s%s.log", l.logPath, prefix, AppLoggingNameSep, datetime, flag)
}

// redirectStd redirects the stdout and stderr to the current log file.
// NOTICE: stdout and stderr cannot count the size of the log file.
func (l *AppLoggingToFile) redirectStd() {
	os.Stdout = l.curAccessLog
	os.Stderr = l.curErrorLog

	log.SetOutput(l.curAccessLog)
}

// checkLogPathSize checks the log path size and remove old log files if needed.
func (l *AppLoggingToFile) checkLogPathSize() *Error {
	if !l.autoCleanup {
		return nil
	}
	pathSize, err := UtilFileGetDirectorySize(l.logPath)
	if err != nil {
		return err.AddStack(nil)
	}
	if pathSize <= l.logPathMaxSize {
		return nil
	}

	// Remove old log files.
	// Walk through the log path and sort by time.
	sizeNeeded := pathSize - l.logPathMaxSize
	fileInfoList := make([]os.FileInfo, 0)
	er := filepath.Walk(l.logPath, func(path string, info os.FileInfo, er error) error {
		if er != nil {
			return er
		}
		if info.IsDir() {
			return nil
		}
		fileInfoList = append(fileInfoList, info)
		return nil
	})
	if er != nil {
		return NewErrorf(ErrAppEnvLoggingPathInvalid, "check log path size failed, err=(%v)", er).AddStack(nil)
	}
	sort.Slice(fileInfoList, func(i, j int) bool {
		return fileInfoList[i].ModTime().Before(fileInfoList[j].ModTime())
	})
	for _, fileInfo := range fileInfoList {
		if sizeNeeded <= 0 {
			break
		}
		er := os.Remove(filepath.Join(l.logPath, fileInfo.Name()))
		if er != nil {
			return NewErrorf(ErrAppEnvLoggingPathInvalid, "check log path size failed, err=(%v)", er).AddStack(nil)
		}
		sizeNeeded -= fileInfo.Size()
	}
	return nil
}

func (l *AppLoggingToFile) onError(err *Error) {
	if l.errHandler != nil {
		l.errHandler(err)
	}
}

// Log

func (l *AppLoggingToFile) WriteAccessLog(s string) {
	n, er := l.curAccessLog.WriteString(s)
	if er != nil {
		if err := l.checkLogPathSize(); err != nil {
			l.onError(err.AddStack(nil))
			return
		} else {
			if _, er := l.curAccessLog.WriteString(s); er != nil {
				l.onError(NewErrorf(ErrAppEnvLoggingFileWriteFailed, "AppLoggingToFile: Write access log failed, err=(%v)", er).AddStack(nil))
				return
			}
		}
	}

	// Check log file size.
	l.curAccessSize += int64(n)
	if l.curAccessSize >= l.logFileMaxSize {
		newName := l.logFileFormatter(AppLoggingAccessPrefix, "")
		newFile, err := l.openLogFile(newName)
		if err != nil {
			l.onError(err.AddStack(nil))
			return
		}

		// Close the old log file.
		er := l.curAccessLog.Close()
		if er != nil {
			l.onError(NewErrorf(ErrAppEnvLoggingFileCloseFailed, "AppLoggingToFile: Close access log failed, err=(%v)", er).AddStack(nil))
		}

		// Switch to the new log file.
		l.curAccessLogName = newName
		l.curAccessLog = newFile
		l.curAccessSize = 0

		l.redirectStd()

		if err := l.checkLogPathSize(); err != nil {
			l.onError(err.AddStack(nil))
			return
		}
	}
}

func (l *AppLoggingToFile) WriteErrorLog(s string) {
	l.WriteAccessLog(s)

	n, er := l.curErrorLog.WriteString(s)
	if er != nil {
		l.onError(NewErrorf(ErrAppEnvLoggingFileWriteFailed, "AppLoggingToFile: Write error log failed, err=(%v)", er).AddStack(nil))
		return
	}
	l.curErrorSize += int64(n)
	if l.curErrorSize >= l.logFileMaxSize {
		newName := l.logFileFormatter(AppLoggingErrorPrefix, "")
		newFile, err := l.openLogFile(newName)
		if err != nil {
			l.onError(err.AddStack(nil))
			return
		}

		// Close the old log file.
		er := l.curErrorLog.Close()
		if er != nil {
			l.onError(NewErrorf(ErrAppEnvLoggingFileCloseFailed, "AppLoggingToFile: Close error log failed, err=(%v)", er).AddStack(nil))
		}

		// Switch to the new log file.
		l.curErrorLogName = newName
		l.curErrorLog = newFile
		l.curErrorSize = 0

		l.redirectStd()
	}
}

// For Test

type appLoggingForTest struct {
	t *testing.T

	ignoreError bool
}

func (l *appLoggingForTest) WriteAccessLog(s string) {
	l.t.Log(strings.TrimSuffix(s, "\n"))
}

func (l *appLoggingForTest) WriteErrorLog(s string) {
	if l.ignoreError {
		l.t.Log(strings.TrimSuffix(s, "\n"))
	} else {
		l.t.Error(strings.TrimSuffix(s, "\n"))
	}
}

// For Test to string

type appLoggingForTestToString struct {
	access bytes.Buffer
	error  bytes.Buffer
}

func (l *appLoggingForTestToString) WriteAccessLog(s string) {
	l.access.WriteString(s)
}

func (l *appLoggingForTestToString) WriteErrorLog(s string) {
	l.error.WriteString(s)
}

// For Benchmark

type appLoggingForBenchmark struct {
	b *testing.B
}

func (l *appLoggingForBenchmark) WriteAccessLog(s string) {
	l.b.Log(strings.TrimSuffix(s, "\n"))
}

func (l *appLoggingForBenchmark) WriteErrorLog(s string) {
	l.b.Error(strings.TrimSuffix(s, "\n"))
}

// For Benchmark to string

type appLoggingForBenchmarkToString struct {
	access bytes.Buffer
	error  bytes.Buffer
}

func (l *appLoggingForBenchmarkToString) WriteAccessLog(s string) {
	l.access.WriteString(s)
}

func (l *appLoggingForBenchmarkToString) WriteErrorLog(s string) {
	l.error.WriteString(s)
}
