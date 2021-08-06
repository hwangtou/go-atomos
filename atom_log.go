package go_atomos

// CHECKED!

import (
	"fmt"
	"log"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultLogMailId = 0

// Atom日志管理器
// Atom Logs Manager

type atomLogsManager struct {
	atom *AtomCore
}

// 初始化atomLogsManager的内容。
// 没有构造和释构函数，因为atomLogsManager是AtomCore内部使用的。
//
// Initialization of atomLogsManager.
// No New and Delete function because atomLogsManager is struct inner AtomCore.
func initAtomLog(l *atomLogsManager, a *AtomCore) {
	l.atom = a
}

// 释放atomTasksManager对象的内容。
// Releasing atomTasksManager.
func releaseAtomLog(_ *atomLogsManager) {
}

// 把Log以邮件的方式发送到Cosmos的Log实例处理。
// write Logs as Mails to Cosmos Log instance.
func (l *atomLogsManager) pushAtomLog(id *AtomId, level LogLevel, msg string) {
	lm := logMailsPool.Get().(*LogMail)
	lm.Id = id
	lm.Time = timestamppb.Now()
	lm.Level = level
	lm.Message = msg
	m := NewMail(defaultLogMailId, lm)
	if ok := l.atom.element.cosmos.log.PushTail(m); !ok {
		log.Printf("atomLogs: Add log mail failed, id=%+v,level=%v,msg=%s", id, level, msg)
	}
}

// Log内存池
// Log Mails Pool
var logMailsPool = sync.Pool{
	New: func() interface{} {
		return &LogMail{}
	},
}

// 各种级别的日志函数。
// Log functions in difference levels.

func (l *atomLogsManager) Debug(format string, args ...interface{}) {
	if l.atom.element.current.Interface.Config.LogLevel > LogLevel_Debug {
		return
	}
	l.pushAtomLog(l.atom.atomId, LogLevel_Debug, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Info(format string, args ...interface{}) {
	if l.atom.element.current.Interface.Config.LogLevel > LogLevel_Info {
		return
	}
	l.pushAtomLog(l.atom.atomId, LogLevel_Info, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Warn(format string, args ...interface{}) {
	if l.atom.element.current.Interface.Config.LogLevel > LogLevel_Warn {
		return
	}
	l.pushAtomLog(l.atom.atomId, LogLevel_Warn, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Error(format string, args ...interface{}) {
	if l.atom.element.current.Interface.Config.LogLevel > LogLevel_Error {
		return
	}
	l.pushAtomLog(l.atom.atomId, LogLevel_Error, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Fatal(format string, args ...interface{}) {
	if l.atom.element.current.Interface.Config.LogLevel > LogLevel_Fatal {
		return
	}
	l.pushAtomLog(l.atom.atomId, LogLevel_Fatal, fmt.Sprintf(format, args...))
}
