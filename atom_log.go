package go_atomos

import (
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	"sync"
)

// TODO: Config & write file
const defaultLogMailId = 0

// AtomLog
type atomLogsManager struct {
	*AtomCore
}

// No New and Delete because atomLogsManager is struct in AtomCore.

func initAtomLog(l *atomLogsManager, a *AtomCore) {
	l.AtomCore = a
}

func releaseAtomLog(l *atomLogsManager) {
	l.AtomCore = nil
}

func (l *atomLogsManager) pushAtomLog(id *AtomId, level LogLevel, msg string) {
	lm := logMailsPool.Get().(*LogMail)
	lm.Id = id
	lm.Time = timestamppb.Now()
	lm.Level = level
	lm.Message = msg
	m := NewMail(defaultLogMailId, lm)
	if ok := l.AtomCore.element.cosmos.log.PushTail(m); !ok {
		// todo
		log.Println("Atom Log Mail failed", id, level, msg)
	}
}

var logMailsPool = sync.Pool{
	New: func() interface{} {
		return &LogMail{}
	},
}

func (l *atomLogsManager) Debug(format string, args ...interface{}) {
	if l.element.define.Config.LogLevel > LogLevel_Debug {
		return
	}
	l.pushAtomLog(l.AtomCore.atomId, LogLevel_Debug, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Info(format string, args ...interface{}) {
	if l.element.define.Config.LogLevel > LogLevel_Info {
		return
	}
	l.pushAtomLog(l.AtomCore.atomId, LogLevel_Info, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Warn(format string, args ...interface{}) {
	if l.element.define.Config.LogLevel > LogLevel_Warn {
		return
	}
	l.pushAtomLog(l.AtomCore.atomId, LogLevel_Warn, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Error(format string, args ...interface{}) {
	if l.element.define.Config.LogLevel > LogLevel_Error {
		return
	}
	l.pushAtomLog(l.AtomCore.atomId, LogLevel_Error, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Fatal(format string, args ...interface{}) {
	if l.element.define.Config.LogLevel > LogLevel_Fatal {
		return
	}
	l.pushAtomLog(l.AtomCore.atomId, LogLevel_Fatal, fmt.Sprintf(format, args...))
}

// Element defines MailBoxHandler to support logging.

func (c *CosmosSelf) onLogMessage(mail *Mail) {
	lm := mail.Content.(*LogMail)
	c.logging(lm)
	logMailsPool.Put(lm)
	DelMail(mail)
}

func (c *CosmosSelf) onLogPanic(mail *Mail, trace string) {
	lm := mail.Content.(*LogMail)
	c.logging(&LogMail{
		Id:      lm.Id,
		Time:    lm.Time,
		Level:   LogLevel_Fatal,
		Message: trace,
	})
}

func (c *CosmosSelf) onLogStop(killMail, remainMails *Mail, num uint32) {
	for curMail := remainMails; curMail != nil; curMail = remainMails.next {
		c.onLogMessage(curMail)
	}
}

func (c *CosmosSelf) logging(lm *LogMail) {
	var msg string
	if lm.Id != nil {
		msg = fmt.Sprintf("%s::%s::%s => %s", lm.Id.Node, lm.Id.Element, lm.Id.Name, lm.Message)
	} else {
		msg = fmt.Sprintf("%s", lm.Message)
	}
	switch lm.Level {
	case LogLevel_Debug:
		logDebug(lm.Time.AsTime(), msg)
	case LogLevel_Info:
		logInfo(lm.Time.AsTime(), msg)
	case LogLevel_Warn:
		logWarn(lm.Time.AsTime(), msg)
	case LogLevel_Error:
		logErr(lm.Time.AsTime(), msg)
	case LogLevel_Fatal:
		logFatal(lm.Time.AsTime(), msg)
	default:
		logWarn(lm.Time.AsTime(), msg)
	}
}
