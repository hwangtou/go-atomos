package core

import (
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"os"
	"sync"
)

const defaultLogMailId = 0

// Cosmos的Log接口。
// Interface of Cosmos Log.

type LoggingAtomos struct {
	log *mailBox
}

// Log内存池
// Log Mails Pool
var logMailsPool = sync.Pool{
	New: func() interface{} {
		return &LogMail{}
	},
}

func NewLoggingAtomos() *LoggingAtomos {
	m := &LoggingAtomos{}
	m.log = newMailBox(MailBoxHandler{
		OnReceive: m.onLogMessage,
		OnPanic:   m.onLogPanic,
		OnStop:    m.onLogStop,
	})
	m.log.start()
	return m
}

func (c *LoggingAtomos) PushProcessLog(level LogLevel, format string, args ...interface{}) {
	id := &IDInfo{
		Type:    IDType_Process,
		Cosmos:  "",
		Element: "",
		Atomos:  "",
	}
	c.pushLogging(id, level, fmt.Sprintf(format, args...))
}

func (c *LoggingAtomos) Close() {
	c.stop()
	c.log = nil
}

func (c *LoggingAtomos) stop() {
	c.log.waitStop()
}

func (c *LoggingAtomos) pushLogging(id *IDInfo, level LogLevel, msg string) {
	lm := logMailsPool.Get().(*LogMail)
	lm.Id = id
	lm.Time = timestamppb.Now()
	lm.Level = level
	lm.Message = msg
	m := newMail(defaultLogMailId, lm)
	if ok := c.log.pushTail(m); !ok {
		LogWrite(fmt.Sprintf("LoggingAtomos: Add log mail failed, id=(%+v),level=(%v),msg=(%s)", id, level, msg), true)
	}
}

// Logging Atomos的实现。
// Implementation of Logging Atomos.

func (c *LoggingAtomos) onLogMessage(mail *mail) {
	lm := mail.Content.(*LogMail)
	c.logging(lm)
	logMailsPool.Put(lm)
	delMail(mail)
}

func (c *LoggingAtomos) onLogPanic(mail *mail, trace []byte) {
	lm := mail.Content.(*LogMail)
	c.logging(&LogMail{
		Id:      lm.Id,
		Time:    lm.Time,
		Level:   LogLevel_Fatal,
		Message: string(trace),
	})
}

func (c *LoggingAtomos) onLogStop(killMail, remainMails *mail, num uint32) {
	for curMail := remainMails; curMail != nil; curMail = curMail.next {
		c.onLogMessage(curMail)
	}
}

func (c *LoggingAtomos) logging(lm *LogMail) {
	var msg string
	if id := lm.Id; id != nil {
		switch id.Type {
		case IDType_Atomos:
			msg = fmt.Sprintf("%s::%s::%s => %s", id.Cosmos, id.Element, id.Atomos, lm.Message)
		case IDType_Element:
			msg = fmt.Sprintf("%s::%s => %s", id.Cosmos, id.Element, lm.Message)
		case IDType_Cosmos:
			msg = fmt.Sprintf("%s => %s", id.Cosmos, lm.Message)
		case IDType_Main:
			msg = fmt.Sprintf("Main => %s", lm.Message)
		default:
			msg = fmt.Sprintf("%s", lm.Message)
		}
	} else {
		msg = fmt.Sprintf("%s", lm.Message)
	}
	switch lm.Level {
	case LogLevel_Debug:
		LogWrite(fmt.Sprintf("%s [DEBUG] %s\n", lm.Time.AsTime().Format(logTimeFmt), msg), false)
	case LogLevel_Info:
		LogWrite(fmt.Sprintf("%s [INFO]  %s\n", lm.Time.AsTime().Format(logTimeFmt), msg), false)
	case LogLevel_Warn:
		LogWrite(fmt.Sprintf("%s [WARN]  %s\n", lm.Time.AsTime().Format(logTimeFmt), msg), false)
	case LogLevel_Error:
		LogWrite(fmt.Sprintf("%s [ERROR] %s\n", lm.Time.AsTime().Format(logTimeFmt), msg), true)
	case LogLevel_Fatal:
		fallthrough
	default:
		LogWrite(fmt.Sprintf("%s [FATAL] %s\n", lm.Time.AsTime().Format(logTimeFmt), msg), true)
	}
}

// Concrete log to file logic.

func LogWrite(msg string, err bool) {
	if err {
		os.Stderr.WriteString(msg)
	} else {
		os.Stdout.WriteString(msg)
	}
}
