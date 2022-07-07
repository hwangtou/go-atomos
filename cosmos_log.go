package go_atomos

// CHECKED!

import (
	"fmt"
	"os"
	"time"
)

// Cosmos的Log接口。
// Interface of Cosmos Log.

type loggingMailBox struct {
	level LogLevel
	// Log
	log *mailBox
}

func newLoggingMailBox(lv LogLevel) *loggingMailBox {
	m := &loggingMailBox{
		level: lv,
		log:   nil,
	}
	m.log = newMailBox(MailBoxHandler{
		OnReceive: m.onLogMessage,
		OnPanic:   m.onLogPanic,
		OnStop:    m.onLogStop,
	})
	return m
}

//func (c *loggingMailBox) logDebug(format string, args ...interface{}) {
//	if c.level > LogLevel_Debug {
//		return
//	}
//	c.pushCosmosLog(LogLevel_Debug, fmt.Sprintf(format, args...))
//}
//
//func (c *loggingMailBox) logInfo(format string, args ...interface{}) {
//	if c == nil || c.config == nil || c.config.LogLevel > LogLevel_Info {
//		return
//	}
//	c.pushCosmosLog(LogLevel_Info, fmt.Sprintf(format, args...))
//}
//
//func (c *loggingMailBox) logWarn(format string, args ...interface{}) {
//	if c == nil || c.config == nil || c.config.LogLevel > LogLevel_Warn {
//		return
//	}
//	c.pushCosmosLog(LogLevel_Warn, fmt.Sprintf(format, args...))
//}
//
//func (c *loggingMailBox) logError(format string, args ...interface{}) {
//	if c == nil || c.config == nil || c.config.LogLevel > LogLevel_Error {
//		return
//	}
//	c.pushCosmosLog(LogLevel_Error, fmt.Sprintf(format, args...))
//}
//
//func (c *loggingMailBox) logFatal(format string, args ...interface{}) {
//	if c == nil || c.config == nil || c.config.LogLevel > LogLevel_Fatal {
//		return
//	}
//	c.pushCosmosLog(LogLevel_Fatal, fmt.Sprintf(format, args...))
//}

// Cosmos的Log实现。
// Implementation of Cosmos Log.

func (c *loggingMailBox) onLogMessage(mail *mail) {
	lm := mail.Content.(*LogMail)
	c.logging(lm)
	logMailsPool.Put(lm)
	delMail(mail)
}

func (c *loggingMailBox) onLogPanic(mail *mail, trace string) {
	lm := mail.Content.(*LogMail)
	c.logging(&LogMail{
		Id:      lm.Id,
		Time:    lm.Time,
		Level:   LogLevel_Fatal,
		Message: trace,
	})
}

func (c *loggingMailBox) onLogStop(killMail, remainMails *mail, num uint32) {
	for curMail := remainMails; curMail != nil; curMail = curMail.next {
		c.onLogMessage(curMail)
	}
}

func (c *loggingMailBox) logging(lm *LogMail) {
	var msg string
	if id := lm.Id; id != nil {
		switch id.Type {
		case IDType_Element:
			msg = fmt.Sprintf("%s::%s => %s", id.Cosmos, id.Element, lm.Message)
		case IDType_Cosmos:
			msg = fmt.Sprintf("%s => %s", id.Cosmos, lm.Message)
		case IDType_Atomos:
			fallthrough
		default:
			msg = fmt.Sprintf("%s::%s::%s => %s", id.Cosmos, id.Element, id.Atomos, lm.Message)
		}
	} else {
		msg = fmt.Sprintf("%s", lm.Message)
	}
	switch lm.Level {
	case LogLevel_Debug:
		logWrite(fmt.Sprintf("%s [DEBUG] %s\n", lm.Time.AsTime().Format(logTimeFmt), msg), false)
	case LogLevel_Info:
		logWrite(fmt.Sprintf("%s [INFO]  %s\n", lm.Time.AsTime().Format(logTimeFmt), msg), false)
	case LogLevel_Warn:
		logWrite(fmt.Sprintf("%s [WARN]  %s\n", lm.Time.AsTime().Format(logTimeFmt), msg), false)
	case LogLevel_Error:
		logWrite(fmt.Sprintf("%s [ERROR] %s\n", lm.Time.AsTime().Format(logTimeFmt), msg), true)
	case LogLevel_Fatal:
		logWrite(fmt.Sprintf("%s [FATAL] %s\n", lm.Time.AsTime().Format(logTimeFmt), msg), true)
	default:
		logWrite(fmt.Sprintf("%s [WARN]  %s\n", lm.Time.AsTime().Format(logTimeFmt), msg), true)
	}
}

func LogFormatter(t time.Time, level LogLevel, msg string) string {
	l := "ERROR"
	switch level {
	case LogLevel_Debug:
		l = "[DEBUG]"
	case LogLevel_Info:
		l = "[INFO] "
	case LogLevel_Warn:
		l = "[WARN] "
	case LogLevel_Error:
		l = "[ERROR]"
	case LogLevel_Fatal:
		l = "[FATAL]"
	default:
		l = "[WARN] "
	}
	return fmt.Sprintf("%s %s %s\n", t.Format(logTimeFmt), l, msg)
}

//func (c *loggingMailBox) pushCosmosLog(level LogLevel, msg string) {
//	lm := logMailsPool.Get().(*LogMail)
//	lm.Id = nil
//	lm.Time = timestamppb.Now()
//	lm.Level = level
//	lm.Message = msg
//	m := newMail(defaultLogMailId, lm)
//	if ok := c.log.pushTail(m); !ok {
//		log.Println("Cosmos Log Mail failed", level, msg)
//	}
//}

// Concrete log to file logic.

func logWrite(msg string, err bool) {
	if err {
		os.Stderr.WriteString(msg)
	} else {
		os.Stdout.WriteString(msg)
	}
}
