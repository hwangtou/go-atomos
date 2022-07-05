package go_atomos

// CHECKED!

import (
	"fmt"
	"log"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultLogMailId = 0

// Atomos日志管理器
// Atomos Logs Manager

type atomosLogsManager struct {
	atomos *baseAtomos
	level  LogLevel
}

// 初始化atomosLogsManager的内容。
// 没有构造和释构函数，因为atomosLogsManager是AtomCore内部使用的。
//
// Initialization of atomosLogsManager.
// No New and Delete function because atomosLogsManager is struct inner AtomCore.
func initAtomosLog(l *atomosLogsManager, a *baseAtomos, lv LogLevel) {
	l.atomos = a
	l.level = lv
}

// 释放atomTasksManager对象的内容。
// Releasing atomTasksManager.
func releaseAtomosLog(l *atomosLogsManager) {
	l.atomos = nil
}

// 把Log以邮件的方式发送到Cosmos的Log实例处理。
// write Logs as Mails to Cosmos Log instance.
func (l *atomosLogsManager) pushAtomosLog(id *IDInfo, level LogLevel, msg string) {
	lm := logMailsPool.Get().(*LogMail)
	lm.Id = id
	lm.Time = timestamppb.Now()
	lm.Level = level
	lm.Message = msg
	m := newMail(defaultLogMailId, lm)
	if ok := l.atomos.cosmosLogMailbox.pushTail(m); !ok {
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

func (l *atomosLogsManager) Debug(format string, args ...interface{}) {
	if l.level > LogLevel_Debug {
		return
	}
	l.pushAtomosLog(l.atomos.id, LogLevel_Debug, fmt.Sprintf(format, args...))
}

func (l *atomosLogsManager) Info(format string, args ...interface{}) {
	if l.level > LogLevel_Info {
		return
	}
	l.pushAtomosLog(l.atomos.id, LogLevel_Info, fmt.Sprintf(format, args...))
}

func (l *atomosLogsManager) Warn(format string, args ...interface{}) {
	if l.level > LogLevel_Warn {
		return
	}
	l.pushAtomosLog(l.atomos.id, LogLevel_Warn, fmt.Sprintf(format, args...))
}

func (l *atomosLogsManager) Error(format string, args ...interface{}) {
	if l.level > LogLevel_Error {
		return
	}
	l.pushAtomosLog(l.atomos.id, LogLevel_Error, fmt.Sprintf(format, args...))
}

func (l *atomosLogsManager) Fatal(format string, args ...interface{}) {
	if l.level > LogLevel_Fatal {
		return
	}
	l.pushAtomosLog(l.atomos.id, LogLevel_Fatal, fmt.Sprintf(format, args...))
}
