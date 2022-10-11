package go_atomos

import (
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"os"
	"sync"
)

const defaultLogMailID = 0

// Cosmos的Log接口。
// Interface of Cosmos Log.

type LoggingAtomos struct {
	file *os.File
	log  *mailBox
}

// Log内存池
// Log Mails Pool
var logMailsPool = sync.Pool{
	New: func() interface{} {
		return &LogMail{}
	},
}

func NewLoggingAtomos(logPath string) (*LoggingAtomos, *ErrorInfo) {
	f, er := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if er != nil {
		return nil, NewErrorf(ErrLogFileCannotOpen, "Logging filepath is not accessible, err=(%v)", er)
	}
	m := &LoggingAtomos{}
	m.file = f
	m.log = newMailBox(MailBoxHandler{
		OnReceive: m.onLogMessage,
		OnPanic:   m.onLogPanic,
		OnStop:    m.onLogStop,
	})
	m.log.start()
	return m, nil
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

func (c *LoggingAtomos) close() {
	c.log.waitTerminate()
	c.log = nil
}

func (c *LoggingAtomos) pushLogging(id *IDInfo, level LogLevel, msg string) {
	lm := logMailsPool.Get().(*LogMail)
	lm.Id = id
	lm.Time = timestamppb.Now()
	lm.Level = level
	lm.Message = msg
	m := newMail(defaultLogMailID, lm)
	if ok := c.log.pushTail(m); !ok {
		c.logWrite(fmt.Sprintf("LoggingAtomos: Add log mail failed, id=(%+v),level=(%v),msg=(%s)", id, level, msg))
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

func (c *LoggingAtomos) onLogPanic(mail *mail, info *ErrorInfo) {
	lm := mail.Content.(*LogMail)
	var message string
	if info != nil {
		message = info.Message
	}
	c.logging(&LogMail{
		Id:      lm.Id,
		Time:    lm.Time,
		Level:   LogLevel_Fatal,
		Message: message,
	})
}

func (c *LoggingAtomos) onLogStop(killMail, remainMails *mail, num uint32) {
	for curMail := remainMails; curMail != nil; curMail = curMail.next {
		c.onLogMessage(curMail)
	}
	if er := c.file.Close(); er != nil {
		c.stderrWrite(fmt.Sprintf("CosmosLogging: Closing the log file fails when logging is stopped, err=(%v)", er))
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
		c.logWrite(fmt.Sprintf("%s [DEBUG] %s\n", lm.Time.AsTime().Format(logTimeFmt), msg))
	case LogLevel_Info:
		c.logWrite(fmt.Sprintf("%s [INFO]  %s\n", lm.Time.AsTime().Format(logTimeFmt), msg))
	case LogLevel_Warn:
		c.logWrite(fmt.Sprintf("%s [WARN]  %s\n", lm.Time.AsTime().Format(logTimeFmt), msg))
	case LogLevel_Error:
		c.logWrite(fmt.Sprintf("%s [ERROR] %s\n", lm.Time.AsTime().Format(logTimeFmt), msg))
	case LogLevel_Fatal:
		fallthrough
	default:
		c.logWrite(fmt.Sprintf("%s [FATAL] %s\n", lm.Time.AsTime().Format(logTimeFmt), msg))
	}
}

// Concrete log to file logic.

func (c *LoggingAtomos) logWrite(msg string) {
	if _, er := c.file.WriteString(msg); er != nil {
		c.stderrWrite(msg)
	}
}

func (c *LoggingAtomos) stderrWrite(msg string) {
	os.Stderr.WriteString(msg)
}
