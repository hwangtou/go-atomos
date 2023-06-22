package go_atomos

import (
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

const defaultLogMailID = 0

//var processIDType = IDType_App

// Cosmos的Log接口。
// Interface of Cosmos Log.

type loggingAtomos struct {
	logBox    *mailBox
	accessLog loggingFn
	errorLog  loggingFn

	exitCh chan struct{}
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
		c.errorLog("loggingAtomos: Stop failed.")
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
		c.errorLog(fmt.Sprintf("loggingAtomos: Add log mail failed. id=(%+v),level=(%v),msg=(%s)", id, level, msg))
	}
}

func (c *loggingAtomos) pushFrameworkErrorLog(format string, args ...interface{}) {
	c.PushLogging(&IDInfo{
		//Type:    processIDType,
		Node:    "",
		Element: "",
		Atom:    "",
		GoId:    0,
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
	var msg string
	if id := lm.Id; id != nil {
		switch id.Type {
		case IDType_Atom:
			msg = fmt.Sprintf("%s::%s::%s => %s", id.Node, id.Element, id.Atom, lm.Message)
		case IDType_Element:
			msg = fmt.Sprintf("%s::%s => %s", id.Node, id.Element, lm.Message)
		case IDType_Cosmos:
			msg = fmt.Sprintf("%s => %s", id.Node, lm.Message)
		//case IDType_AppLoader:
		//	msg = fmt.Sprintf("AppLoader => %s", lm.Message)
		//case IDType_App:
		//	msg = fmt.Sprintf("App => %s", lm.Message)
		default:
			msg = fmt.Sprintf("Unknown => %s", lm.Message)
		}
	} else {
		msg = fmt.Sprintf("%s", lm.Message)
	}
	t := time.Unix(int64(lm.Time.GetSeconds()), int64(lm.Time.GetNanos())).Local().Format(logTimeFmt)
	switch lm.Level {
	case LogLevel_Debug:
		c.accessLog(fmt.Sprintf("%s [DEBUG] %s\n", t, msg))
	case LogLevel_Info:
		c.accessLog(fmt.Sprintf("%s [INFO]  %s\n", t, msg))
	case LogLevel_Warn:
		c.errorLog(fmt.Sprintf("%s [WARN]  %s\n", t, msg))
	case LogLevel_Err:
		c.errorLog(fmt.Sprintf("%s [ERROR] %s\n", t, msg))
	case LogLevel_Fatal:
		fallthrough
	default:
		c.errorLog(fmt.Sprintf("%s [FATAL] %s\n", t, msg))
	}
}
