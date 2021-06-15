package go_atomos

import (
	"fmt"
	"github.com/golang/protobuf/ptypes"
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
	lm.Time = ptypes.TimestampNow()
	lm.Level = level
	lm.Message = msg
	m := NewMail(defaultLogMailId, lm)
	if ok := l.AtomCore.element.log.PushTail(m); !ok {
		// todo
		log.Println("Add Atom Log Mail failed", id, level, msg)
	}
}

var logMailsPool = sync.Pool{
	New: func() interface{} {
		return &LogMail{}
	},
}

func (l *atomLogsManager) Debug(format string, args ...interface{}) {
	if l.element.define.LogLevel > LogLevel_Debug {
		return
	}
	l.pushAtomLog(l.AtomCore.atomId, LogLevel_Debug, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Info(format string, args ...interface{}) {
	if l.element.define.LogLevel > LogLevel_Info {
		return
	}
	l.pushAtomLog(l.AtomCore.atomId, LogLevel_Info, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Warn(format string, args ...interface{}) {
	if l.element.define.LogLevel > LogLevel_Warn {
		return
	}
	l.pushAtomLog(l.AtomCore.atomId, LogLevel_Warn, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Error(format string, args ...interface{}) {
	if l.element.define.LogLevel > LogLevel_Error {
		return
	}
	l.pushAtomLog(l.AtomCore.atomId, LogLevel_Error, fmt.Sprintf(format, args...))
}

func (l *atomLogsManager) Fatal(format string, args ...interface{}) {
	if l.element.define.LogLevel > LogLevel_Fatal {
		return
	}
	l.pushAtomLog(l.AtomCore.atomId, LogLevel_Fatal, fmt.Sprintf(format, args...))
}

// Element defines MailBoxHandler to support logging.

func (e *ElementLocal) onLogMessage(mail *Mail) {
	lm := mail.Content.(*LogMail)
	e.logging(lm)
	logMailsPool.Put(lm)
	DelMail(mail)
}

func (e *ElementLocal) onLogPanic(mail *Mail, trace string) {
	lm := mail.Content.(*LogMail)
	e.logging(&LogMail{
		Id:      lm.Id,
		Time:    lm.Time,
		Level:   LogLevel_Fatal,
		Message: trace,
	})
}

func (e *ElementLocal) onLogStop(killMail, remainMails *Mail, num uint32) {
	for curMail := remainMails; curMail != nil; curMail = remainMails.next {
		e.onLogMessage(curMail)
	}
}

func (e *ElementLocal) logging(lm *LogMail) {
	var t, l string
	t = lm.Time.AsTime().Format("2006-01-02 15:04:05.999")
	switch lm.Level {
	case LogLevel_Debug:
		l = "[DEBUG]"
	case LogLevel_Info:
		l = "[INFO ]"
	case LogLevel_Warn:
		l = "[WARN ]"
	case LogLevel_Error:
		l = "[ERROR]"
	case LogLevel_Fatal:
		l = "[FATAL]"
	}
	// todo: write buffer to where
	fmt.Printf("%s %s %s::%s::%s\t%s\n",
		t, l, lm.Id.Node, lm.Id.Element, lm.Id.Name, lm.Message)
}
