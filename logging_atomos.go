package go_atomos

import (
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

const defaultLogMailID = 0

var processIDType = IDType_App

// Cosmos的Log接口。
// Interface of Cosmos Log.

type LoggingAtomos struct {
	logBox    *mailBox
	accessLog LoggingFn
	errorLog  LoggingFn
}

var sharedLogging LoggingAtomos

type LoggingFn func(string)

func SharedLogging() *LoggingAtomos {
	return &sharedLogging
}

func (c *LoggingAtomos) PushLogging(id *IDInfo, level LogLevel, msg string) {
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
		c.errorLog(fmt.Sprintf("LoggingAtomos: Add log mail failed. id=(%+v),level=(%v),msg=(%s)", id, level, msg))
	}
}

func initSharedLoggingAtomos(accessLog, errLog LoggingFn) {
	sharedLogging = LoggingAtomos{
		logBox:    nil,
		accessLog: accessLog,
		errorLog:  errLog,
	}
	sharedLogging.logBox = newMailBox("sharedLogging", &sharedLogging)
	_ = sharedLogging.logBox.start(func() *Error { return nil })
}

func (c *LoggingAtomos) pushFrameworkErrorLog(format string, args ...interface{}) {
	c.PushLogging(&IDInfo{
		Type:    processIDType,
		Cosmos:  "",
		Element: "",
		Atom:    "",
	}, LogLevel_Fatal, fmt.Sprintf(format, args...))
}

func (c *LoggingAtomos) PushProcessLog(level LogLevel, format string, args ...interface{}) {
	c.PushLogging(&IDInfo{
		Type:    processIDType,
		Cosmos:  "",
		Element: "",
		Atom:    "",
	}, level, fmt.Sprintf(format, args...))
}

// Logging Atomos的实现。
// Implementation of Logging Atomos.

func (c *LoggingAtomos) mailboxOnStartUp(func() *Error) *Error {
	return nil
}

func (c *LoggingAtomos) mailboxOnReceive(mail *mail) {
	c.logging(mail.log)
}

func (c *LoggingAtomos) mailboxOnStop(killMail, remainMails *mail, num uint32) {
	for curMail := remainMails; curMail != nil; curMail = curMail.next {
		c.logging(curMail.log)
	}
}

func (c *LoggingAtomos) logging(lm *LogMail) {
	var msg string
	if id := lm.Id; id != nil {
		switch id.Type {
		case IDType_Atom:
			msg = fmt.Sprintf("%s::%s::%s => %s", id.Cosmos, id.Element, id.Atom, lm.Message)
		case IDType_Element:
			msg = fmt.Sprintf("%s::%s => %s", id.Cosmos, id.Element, lm.Message)
		case IDType_Cosmos:
			msg = fmt.Sprintf("%s => %s", id.Cosmos, lm.Message)
		case IDType_AppLoader:
			msg = fmt.Sprintf("AppLoader => %s", lm.Message)
		case IDType_App:
			msg = fmt.Sprintf("App => %s", lm.Message)
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
		c.errorLog(fmt.Sprintf("%s [FATAL] %s\n", lm.Time.AsTime().Format(logTimeFmt), msg))
	}
}
