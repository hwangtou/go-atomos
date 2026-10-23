package atomos

import (
	"bytes"
	"fmt"
	"io"
)

// Logging is the public logging contract of the framework. It contains only
// exported methods, so external packages can implement it (e.g. mocks).
type Logging interface {
	io.Writer

	SetLevel(lv LogLevel)

	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warn(format string, args ...any)

	Error(format string, args ...any)
	Fatal(format string, args ...any)
}

// loggingInternal is the framework-internal extension of Logging with the
// core-level channels used for framework diagnostics. Framework-provided
// loggers always implement it; user code never needs to.
type loggingInternal interface {
	Logging

	coreInfo(format string, args ...any)
	coreError(format string, args ...any)
	coreFatal(format string, args ...any)
}

// Atomos日志管理器
// Atomos Logs Manager

type atomosLogging struct {
	id      *IDInfo
	level   LogLevel
	logging *loggingAtomos
}

// 初始化atomosLogsManager的内容。
// 没有构造和释构函数，因为atomosLogsManager是AtomCore内部使用的。
//
// Initialization of atomosLogging.
// No New and Delete function because atomosLogging is struct inner AtomCore.
func initAtomosLog(l *atomosLogging, id *IDInfo, lv LogLevel, logging *loggingAtomos) {
	l.id = id
	l.level = lv
	l.logging = logging
}

func (l *atomosLogging) Write(p []byte) (n int, err error) {
	// Remove return character, if any.
	p = bytes.TrimRight(p, "\n")
	l.logging.PushLogging(l.id, LogLevel_CoreInfo, string(p))
	return len(p), nil
}

// 设置日志级别。
// Set log level.

func (l *atomosLogging) SetLevel(lv LogLevel) {
	l.level = lv
}

// 把Log以邮件的方式发送到Cosmos的Log实例处理。
// write Logs as Mails to Cosmos Log instance.
func (l *atomosLogging) pushAtomosLog(id *IDInfo, level LogLevel, msg string) {
	l.logging.PushLogging(id, level, msg)
}

// 各种级别的日志函数。
// Log functions in difference levels.

func (l *atomosLogging) Debug(format string, args ...any) {
	if l.level > LogLevel_Debug {
		return
	}
	l.pushAtomosLog(l.id, LogLevel_Debug, fmt.Sprintf(format, args...))
}

func (l *atomosLogging) Info(format string, args ...any) {
	if l.level > LogLevel_Info {
		return
	}
	l.pushAtomosLog(l.id, LogLevel_Info, fmt.Sprintf(format, args...))
}

func (l *atomosLogging) coreInfo(format string, args ...any) {
	if l.level > LogLevel_CoreInfo {
		return
	}
	l.pushAtomosLog(l.id, LogLevel_CoreInfo, fmt.Sprintf(format, args...))
}

func (l *atomosLogging) Warn(format string, args ...any) {
	if l.level > LogLevel_Warn {
		return
	}
	l.pushAtomosLog(l.id, LogLevel_Warn, fmt.Sprintf(format, args...))
}

func (l *atomosLogging) Error(format string, args ...any) {
	if l.level > LogLevel_Err {
		return
	}
	l.pushAtomosLog(l.id, LogLevel_Err, fmt.Sprintf(format, args...))
}

func (l *atomosLogging) coreError(format string, args ...any) {
	if l.level > LogLevel_CoreErr {
		return
	}
	l.pushAtomosLog(l.id, LogLevel_CoreErr, fmt.Sprintf(format, args...))
}

func (l *atomosLogging) Fatal(format string, args ...any) {
	if l.level > LogLevel_Fatal {
		return
	}
	l.pushAtomosLog(l.id, LogLevel_Fatal, fmt.Sprintf(format, args...))
}

func (l *atomosLogging) coreFatal(format string, args ...any) {
	if l.level > LogLevel_CoreFatal {
		return
	}
	l.pushAtomosLog(l.id, LogLevel_CoreFatal, fmt.Sprintf(format, args...))
}
