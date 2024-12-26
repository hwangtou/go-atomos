package go_atomos

import (
	"bytes"
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sync"
	"time"
)

type LoggingService interface {
	PushLogging(id *IDInfo, level LogLevel, msg string)
	pushFrameworkInfoLog(format string, args ...interface{})
	pushFrameworkErrorLog(format string, args ...interface{})
}

const (
	LoggingServiceDefaultLogMailID   = 0
	LoggingServiceDefaultMailboxName = "logging"
)

// 整個進程的Logging服務。
// Logging service of the whole process.

type loggingService struct {
	logging appLogging
	// Logging service is thread-safe due to the mailbox.
	logBox *mailBox

	buf bytes.Buffer
}

func (c *loggingService) init(logging appLogging) *Error {
	c.logging = logging
	c.logBox = newMailBox(LoggingServiceDefaultMailboxName, c, c)
	return c.logBox.start(func() *Error { return nil })
}

func (c *loggingService) stop() {
	m := loggingMailPool.Get().(*mail)
	m.next = nil
	m.id = 0
	m.action = MailActionExit
	m.mail = nil
	m.log = &LogMail{}

	if ok := c.logBox.pushTail(m); !ok {
		c.pushFrameworkErrorLog("loggingService: Has already stopped.")
	}
}

func (c *loggingService) PushLogging(id *IDInfo, level LogLevel, msg string) {
	lm := loggingLogMailPool.Get().(*LogMail)
	lm.Id = id
	lm.Time = timestamppb.Now()
	lm.Level = level
	lm.Message = msg

	m := loggingMailPool.Get().(*mail)
	m.next = nil
	m.id = LoggingServiceDefaultLogMailID
	m.action = MailActionRun
	m.mail = nil
	m.log = lm

	if ok := c.logBox.pushTail(m); !ok {
		c.mailboxWriteLog(lm, false)
	}
}

func (c *loggingService) pushFrameworkInfoLog(format string, args ...interface{}) {
	c.PushLogging(nil, LogLevel_CoreInfo, fmt.Sprintf(format, args...))
}

func (c *loggingService) pushFrameworkErrorLog(format string, args ...interface{}) {
	c.PushLogging(nil, LogLevel_CoreFatal, fmt.Sprintf(format, args...))
}

// Logging Atomos的实现。
// Implementation of Logging Atomos.

func (c *loggingService) mailboxOnStartUp(func() *Error) *Error {
	return nil
}

func (c *loggingService) mailboxOnReceive(mail *mail) {
	c.mailboxWriteLog(mail.log, true)
	loggingMailPool.Put(mail)
}

func (c *loggingService) mailboxOnStop(killMail, remainMails *mail, num uint32) *Error {
	for curMail := remainMails; curMail != nil; curMail = curMail.next {
		c.mailboxWriteLog(curMail.log, true)
		loggingMailPool.Put(curMail)
	}
	loggingMailPool.Put(killMail)
	return nil
}

func (c *loggingService) mailboxWriteLog(lm *LogMail, fromMailboxGoroutine bool) {
	buf := c.buf
	if fromMailboxGoroutine {
		buf.Reset()
	} else {
		buf = bytes.Buffer{}
	}

	// Time
	buf.WriteString(time.Unix(lm.Time.GetSeconds(), int64(lm.Time.GetNanos())).Local().Format(logTimeFmt))
	// Level
	switch lm.Level {
	case LogLevel_Debug:
		buf.WriteString(" [DEBUG] ")
	case LogLevel_Info:
		buf.WriteString(" [INFO]  ")
	case LogLevel_Warn:
		buf.WriteString(" [WARN]  ")
	case LogLevel_CoreInfo:
		buf.WriteString(" [COSMO] ")
	case LogLevel_Err:
		buf.WriteString(" [ERROR] ")
	case LogLevel_CoreErr:
		buf.WriteString(" [COSMOS ERROR] ")
	case LogLevel_Fatal:
		buf.WriteString(" [FATAL] ")
	case LogLevel_CoreFatal:
		buf.WriteString(" [COSMOS FATAL] ")
	default:
		buf.WriteString(" [UNKNOWN ERROR] ")
	}
	// ID
	sign := " => "
	if !fromMailboxGoroutine {
		sign = " -> "
	}
	if id := lm.Id; id != nil {
		switch id.Type {
		case IDType_Atom:
			buf.WriteString(id.Node + "::" + id.Element + "::" + id.Atom)
		case IDType_Element:
			buf.WriteString(id.Node + "::" + id.Element)
		case IDType_Cosmos:
			buf.WriteString(id.Node)
		default:
			buf.WriteString("Unknown")
		}
		buf.WriteString(sign + lm.Message + "\n")
	} else {
		buf.WriteString(lm.Message + "\n")
	}
	switch lm.Level {
	case LogLevel_Debug, LogLevel_Info, LogLevel_Warn, LogLevel_CoreInfo:
		c.logging.WriteAccessLog(buf.String())
	case LogLevel_Err, LogLevel_CoreErr, LogLevel_Fatal, LogLevel_CoreFatal:
		c.logging.WriteErrorLog(buf.String())
	default:
		c.logging.WriteErrorLog(buf.String())
	}

	loggingLogMailPool.Put(lm)
}

// LogMail object pool for logging service.
var loggingLogMailPool = sync.Pool{
	New: func() interface{} {
		return &LogMail{}
	},
}

// mail object pool for logging service.
var loggingMailPool = sync.Pool{
	New: func() interface{} {
		return &mail{}
	},
}

// TestLoggingService is the test object of logging service.

type testLoggingService struct {
	logList []*testLoggingServiceLog
}

type testLoggingServiceLog struct {
	id    *IDInfo
	level LogLevel
	msg   string
}

func (t *testLoggingService) PushLogging(id *IDInfo, level LogLevel, msg string) {
	t.logList = append(t.logList, &testLoggingServiceLog{id: id, level: level, msg: msg})
}

func (t *testLoggingService) pushFrameworkInfoLog(format string, args ...interface{}) {
	t.PushLogging(nil, LogLevel_CoreInfo, fmt.Sprintf(format, args...))
}

func (t *testLoggingService) pushFrameworkErrorLog(format string, args ...interface{}) {
	t.PushLogging(nil, LogLevel_CoreFatal, fmt.Sprintf(format, args...))
}
