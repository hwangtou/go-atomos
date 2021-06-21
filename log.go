package go_atomos

import (
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	"time"
)

// TODO: Log optimize.
func (c *CosmosSelf) pushCosmosLog(level LogLevel, msg string) {
	lm := logMailsPool.Get().(*LogMail)
	lm.Id = nil
	lm.Time = timestamppb.Now()
	lm.Level = level
	lm.Message = msg
	m := NewMail(defaultLogMailId, lm)
	if ok := c.log.PushTail(m); !ok {
		// todo
		log.Println("Cosmos Log Mail failed", level, msg)
	}
}

func (c *CosmosSelf) Debug(format string, args ...interface{}) {
	if c.config.LogLevel > LogLevel_Debug {
		return
	}
	c.pushCosmosLog(LogLevel_Debug, fmt.Sprintf(format, args...))
}

func (c *CosmosSelf) Info(format string, args ...interface{}) {
	if c.config.LogLevel > LogLevel_Info {
		return
	}
	c.pushCosmosLog(LogLevel_Info, fmt.Sprintf(format, args...))
}

func (c *CosmosSelf) Warn(format string, args ...interface{}) {
	if c.config.LogLevel > LogLevel_Warn {
		return
	}
	c.pushCosmosLog(LogLevel_Warn, fmt.Sprintf(format, args...))
}

func (c *CosmosSelf) Error(format string, args ...interface{}) {
	if c.config.LogLevel > LogLevel_Error {
		return
	}
	c.pushCosmosLog(LogLevel_Error, fmt.Sprintf(format, args...))
}

func (c *CosmosSelf) Fatal(format string, args ...interface{}) {
	if c.config.LogLevel > LogLevel_Fatal {
		return
	}
	c.pushCosmosLog(LogLevel_Fatal, fmt.Sprintf(format, args...))
}

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
