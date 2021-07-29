package go_atomos

import (
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	"time"
)

// Cosmos的Log接口。
// Interface of Cosmos Log.

func (c *CosmosSelf) Debug(format string, args ...interface{}) {
	if c.config.LogLevel > LogLevel_Debug {
		return
	}
	c.pushCosmosLog(LogLevel_Debug, fmt.Sprintf(format, args...))
}

func (c *CosmosSelf) logInfo(format string, args ...interface{}) {
	if c.config.LogLevel > LogLevel_Info {
		return
	}
	c.pushCosmosLog(LogLevel_Info, fmt.Sprintf(format, args...))
}

func (c *CosmosSelf) logWarn(format string, args ...interface{}) {
	if c.config.LogLevel > LogLevel_Warn {
		return
	}
	c.pushCosmosLog(LogLevel_Warn, fmt.Sprintf(format, args...))
}

func (c *CosmosSelf) logError(format string, args ...interface{}) {
	if c.config.LogLevel > LogLevel_Error {
		return
	}
	c.pushCosmosLog(LogLevel_Error, fmt.Sprintf(format, args...))
}

func (c *CosmosSelf) logFatal(format string, args ...interface{}) {
	if c.config.LogLevel > LogLevel_Fatal {
		return
	}
	c.pushCosmosLog(LogLevel_Fatal, fmt.Sprintf(format, args...))
}

// Cosmos的Log实现。
// Implementation of Cosmos Log.

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
	for curMail := remainMails; curMail != nil; curMail = curMail.next {
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

// TODO: Log optimize.
func (c *CosmosSelf) pushCosmosLog(level LogLevel, msg string) {
	lm := logMailsPool.Get().(*LogMail)
	lm.Id = nil
	lm.Time = timestamppb.Now()
	lm.Level = level
	lm.Message = msg
	m := NewMail(defaultLogMailId, lm)
	if ok := c.log.PushTail(m); !ok {
		log.Println("Cosmos Log Mail failed", level, msg)
	}
}

// Concrete log to file logic.

func logDebug(time time.Time, msg string) {
	logWrite(fmt.Sprintf("%s [DEBUG] %s", logTime(time), msg))
}

func logInfo(time time.Time, msg string) {
	logWrite(fmt.Sprintf("%s [INFO]  %s", logTime(time), msg))
}

func logWarn(time time.Time, msg string) {
	logWrite(fmt.Sprintf("%s [WARN]  %s", logTime(time), msg))
}

func logErr(time time.Time, msg string) {
	logWrite(fmt.Sprintf("%s [ERROR] %s", logTime(time), msg))
}

func logFatal(time time.Time, msg string) {
	logWrite(fmt.Sprintf("%s [FATAL] %s", logTime(time), msg))
}

func logTime(time time.Time) string {
	return time.Format("2006-01-02 15:04:05.000000")
}

func logWrite(msg string) {
	fmt.Println(msg)
}
