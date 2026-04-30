package atomos

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type LoggingService interface {
	PushLogging(id *IDInfo, level LogLevel, msg string)
	pushFrameworkInfoLog(format string, args ...any)
	pushFrameworkErrorLog(format string, args ...any)
	pushFrameworkFatalLog(format string, args ...any)
}

const (
	LoggingServiceDefaultMailboxName = "logging"
)

// 整個進程的Logging服務。
// Logging service of the whole process.

type loggingAtomos struct {
	logging appLoggingIntf
	// Logging service is thread-safe due to the mailbox.
	logBox *mailBox

	buf bytes.Buffer
}

func (c *loggingAtomos) init(logging appLoggingIntf) *Error {
	c.logging = logging
	c.logBox = newMailBox(LoggingServiceDefaultMailboxName, c, c)
	return c.logBox.start(func() *Error { return nil })
}

func (c *loggingAtomos) stop() {
	wait := make(chan struct{})
	m := allocLoggingKillMail(func() {
		wait <- struct{}{}
	})
	if ok := c.logBox.pushTail(m); !ok {
		c.pushFrameworkFatalLog("loggingAtomos: Has already stopped.")
	}
	<-wait
}

func (c *loggingAtomos) PushLogging(id *IDInfo, level LogLevel, msg string) {
	lm, m := allocLoggingMail()
	initLoggingMail(lm, m, id, level, msg)

	if ok := c.logBox.pushTail(m); !ok {
		c.mailboxWriteLog(lm, false)
		releaseMail(m)
	}
}

func (c *loggingAtomos) pushFrameworkInfoLog(format string, args ...any) {
	c.PushLogging(nil, LogLevel_CoreInfo, fmt.Sprintf(format, args...))
}

func (c *loggingAtomos) pushFrameworkErrorLog(format string, args ...any) {
	c.PushLogging(nil, LogLevel_CoreErr, fmt.Sprintf(format, args...))
}

func (c *loggingAtomos) pushFrameworkFatalLog(format string, args ...any) {
	c.PushLogging(nil, LogLevel_CoreFatal, fmt.Sprintf(format, args...))
}

// Logging Atomos的实现。
// Implementation of Logging Atomos.

func (c *loggingAtomos) mailboxOnStartUp(func() *Error) *Error {
	return nil
}

func (c *loggingAtomos) mailboxOnReceive(mail *mail) {
	l := unwrapLoggingMail(mail)
	c.mailboxWriteLog(l, true)
}

func (c *loggingAtomos) mailboxOnStop(killMail, remainMails *mail, num uint32) *Error {
	for curMail := remainMails; curMail != nil; curMail = curMail.next {
		l := unwrapLoggingMail(curMail)
		c.mailboxWriteLog(l, true)
	}
	// No need to release killMail, as it is allocated without pool.
	return nil
}

func (c *loggingAtomos) mailboxWriteLog(lm *LogMail, fromMailboxGoroutine bool) {
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

	releaseLoggingMail(lm)
}

func (c *loggingAtomos) Writer(buf []byte) (n int, err error) {
	c.logging.WriteAccessLog(string(buf))
	return len(buf), nil
}

// LogMail object pool for logging service.
var loggingLogMailPool = sync.Pool{
	New: func() any {
		return &LogMail{}
	},
}

// allocLoggingMail allocates a LogMail and a mail for logging service.
// mail.data is *LogMail
func allocLoggingMail() (*LogMail, *mail) {
	lm := loggingLogMailPool.Get().(*LogMail)
	m := allocMail()
	initMail(m, DefaultMailID, lm)
	return lm, m
}

// allocLoggingKillMail allocates a mailExitCommand and a mail for logging service.
// mail.data is *mailExitCommand
// mail.data.data is nil
// mail.data.onDone is set
func allocLoggingKillMail(onDone func()) *mail {
	m := allocMail()
	initKillMail(m, DefaultMailID, nil, onDone)
	return m
}

func unwrapLoggingMail(m *mail) *LogMail {
	return m.data.(*LogMail)
}

func unwrapLoggingKillMail(m *mail) *mailExitCommand {
	return m.data.(*mailExitCommand)
}

func initLoggingMail(lm *LogMail, m *mail, id *IDInfo, level LogLevel, msg string) {
	lm.Id = id
	lm.Time = timestamppb.Now()
	lm.Level = level
	lm.Message = msg

	m.next = nil
}

func releaseLoggingMail(lm *LogMail) {
	lm.Id = nil
	lm.Time = nil
	lm.Level = LogLevel_Debug
	lm.Message = ""

	loggingLogMailPool.Put(lm)
}
