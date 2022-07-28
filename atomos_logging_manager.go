package go_atomos

// CHECKED!

import (
	"fmt"
)

type Logging interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	Fatal(format string, args ...interface{})
}

// Atomos日志管理器
// Atomos Logs Manager

type atomosLoggingManager struct {
	logging *LoggingAtomos
	atomos  *BaseAtomos
	level   LogLevel
}

// 初始化atomosLogsManager的内容。
// 没有构造和释构函数，因为atomosLogsManager是AtomCore内部使用的。
//
// Initialization of atomosLoggingManager.
// No New and Delete function because atomosLoggingManager is struct inner AtomCore.
func initAtomosLog(l *atomosLoggingManager, log *LoggingAtomos, a *BaseAtomos, lv LogLevel) {
	l.logging = log
	l.atomos = a
	l.level = lv
}

// 释放atomTasksManager对象的内容。
// Releasing atomTasksManager.
func releaseAtomosLog(l *atomosLoggingManager) {
	l.logging = nil
}

// 把Log以邮件的方式发送到Cosmos的Log实例处理。
// write Logs as Mails to Cosmos Log instance.
func (l *atomosLoggingManager) pushAtomosLog(id *IDInfo, level LogLevel, msg string) {
	l.logging.pushLogging(id, level, msg)
}

// 各种级别的日志函数。
// Log functions in difference levels.

func (l *atomosLoggingManager) Debug(format string, args ...interface{}) {
	if l.level > LogLevel_Debug {
		return
	}
	l.pushAtomosLog(l.atomos.id, LogLevel_Debug, fmt.Sprintf(format, args...))
}

func (l *atomosLoggingManager) Info(format string, args ...interface{}) {
	if l.level > LogLevel_Info {
		return
	}
	l.pushAtomosLog(l.atomos.id, LogLevel_Info, fmt.Sprintf(format, args...))
}

func (l *atomosLoggingManager) Warn(format string, args ...interface{}) {
	if l.level > LogLevel_Warn {
		return
	}
	l.pushAtomosLog(l.atomos.id, LogLevel_Warn, fmt.Sprintf(format, args...))
}

func (l *atomosLoggingManager) Error(format string, args ...interface{}) {
	if l.level > LogLevel_Error {
		return
	}
	l.pushAtomosLog(l.atomos.id, LogLevel_Error, fmt.Sprintf(format, args...))
}

func (l *atomosLoggingManager) Fatal(format string, args ...interface{}) {
	if l.level > LogLevel_Fatal {
		return
	}
	l.pushAtomosLog(l.atomos.id, LogLevel_Fatal, fmt.Sprintf(format, args...))
}
