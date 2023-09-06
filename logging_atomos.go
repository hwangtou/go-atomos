package go_atomos

import (
	"bytes"
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

const defaultLogMailID = 0

// Cosmos的Log接口。
// Interface of Cosmos Log.

type loggingAtomos struct {
	logBox    *mailBox
	accessLog loggingFn
	errorLog  loggingFn

	exitCh chan struct{}

	buf bytes.Buffer
}

type loggingFn func(string)

type LoggingRaw interface {
	PushLogging(id *IDInfo, level LogLevel, msg string)
	pushFrameworkErrorLog(format string, args ...interface{})
}

func (c *loggingAtomos) init(accessLog, errLog loggingFn) *Error {
	c.accessLog = accessLog
	c.errorLog = errLog
	c.logBox = newMailBox("logging", c, accessLog, errLog)
	return c.logBox.start(func() *Error { return nil })
}

func (c *loggingAtomos) stop() {
	c.exitCh = make(chan struct{})
	if ok := c.logBox.pushTail(&mail{
		next:   nil,
		id:     0,
		action: MailActionExit,
		mail:   nil,
		log:    &LogMail{},
	}); !ok {
		c.errorLog("loggingAtomos: Stop failed.\n")
	}
	<-c.exitCh
}

func (c *loggingAtomos) PushLogging(id *IDInfo, level LogLevel, msg string) {
	lm := &LogMail{
		Id:      id,
		Time:    timestamppb.Now(),
		Level:   level,
		Message: msg,
	}
	m := &mail{
		next:   nil,
		id:     defaultLogMailID,
		action: MailActionRun,
		mail:   nil,
		log:    lm,
	}
	if ok := c.logBox.pushTail(m); !ok {
		c.errorLog(fmt.Sprintf("loggingAtomos: Add log mail failed. id=(%+v),level=(%v),msg=(%s)\n", id, level, msg))
	}
}

func (c *loggingAtomos) pushFrameworkErrorLog(format string, args ...interface{}) {
	c.PushLogging(&IDInfo{
		//Type:    processIDType,
		Node:    "",
		Element: "",
		Atom:    "",
		//GoId:    0,
	}, LogLevel_Fatal, fmt.Sprintf(format, args...))
}

// Logging Atomos的实现。
// Implementation of Logging Atomos.

func (c *loggingAtomos) mailboxOnStartUp(func() *Error) *Error {
	return nil
}

func (c *loggingAtomos) mailboxOnReceive(mail *mail) {
	c.logging(mail.log)
}

func (c *loggingAtomos) mailboxOnStop(killMail, remainMails *mail, num uint32) *Error {
	for curMail := remainMails; curMail != nil; curMail = curMail.next {
		c.logging(curMail.log)
	}
	if c.exitCh != nil {
		c.exitCh <- struct{}{}
	}
	return nil
}

func (c *loggingAtomos) logging(lm *LogMail) {
	c.buf.Reset()

	// Time
	c.buf.WriteString(time.Unix(lm.Time.GetSeconds(), int64(lm.Time.GetNanos())).Local().Format(logTimeFmt))
	// Level
	switch lm.Level {
	case LogLevel_Debug:
		c.buf.WriteString(" [DEBUG] ")
	case LogLevel_Info:
		c.buf.WriteString(" [INFO]  ")
	case LogLevel_Warn:
		c.buf.WriteString(" [WARN]  ")
	case LogLevel_CoreInfo:
		c.buf.WriteString(" [COSMO] ")
	case LogLevel_Err:
		c.buf.WriteString(" [ERROR] ")
	case LogLevel_CoreErr:
		c.buf.WriteString(" [COSMOS ERROR] ")
	case LogLevel_Fatal:
		c.buf.WriteString(" [FATAL] ")
	case LogLevel_CoreFatal:
		c.buf.WriteString(" [COSMOS FATAL] ")
	default:
		c.buf.WriteString(" [UNKNOWN ERROR] ")
	}
	// ID
	if id := lm.Id; id != nil {
		switch id.Type {
		case IDType_Atom:
			c.buf.WriteString(fmt.Sprintf("%s::%s::%s => %s\n", id.Node, id.Element, id.Atom, lm.Message))
		case IDType_Element:
			c.buf.WriteString(fmt.Sprintf("%s::%s => %s\n", id.Node, id.Element, lm.Message))
		case IDType_Cosmos:
			c.buf.WriteString(fmt.Sprintf("%s => %s\n", id.Node, lm.Message))
		default:
			c.buf.WriteString(fmt.Sprintf("Unknown => %s\n", lm.Message))
		}
	} else {
		c.buf.WriteString(fmt.Sprintf("%s", lm.Message))
	}
	switch lm.Level {
	case LogLevel_Debug, LogLevel_Info, LogLevel_Warn, LogLevel_CoreInfo:
		c.accessLog(c.buf.String())
	case LogLevel_Err, LogLevel_CoreErr, LogLevel_Fatal, LogLevel_CoreFatal:
		c.errorLog(c.buf.String())
	default:
		c.errorLog(c.buf.String())
	}
}

func (c *loggingAtomos) Writer(buf []byte) (n int, err error) {
	c.accessLog(string(buf))
	return len(buf), nil
}
