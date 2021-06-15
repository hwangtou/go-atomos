package go_atomos

import (
	"github.com/golang/protobuf/proto"
	"log"
	"sync"
)

const DefaultMailId = 0

type MailType int

const (
	MailTypeHalt    MailType = 0
	MailTypeMessage MailType = 1
	MailTypeTask    MailType = 2
	MailTypeReload  MailType = 3
)

type atomMail struct {
	*Mail
	typ MailType
	// Message/Task/Halt
	from Id
	name string
	arg  proto.Message
	// Handler
	upgrade *ElementDefine
	// Reply
	mailReply mailReply
	waitCh    chan *mailReply
}

var atomMailsPool = sync.Pool{
	New: func() interface{} {
		return &atomMail{
			typ:       0,
			from:      nil,
			name:      "",
			arg:       nil,
			mailReply: mailReply{},
			waitCh:    nil,
		}
	},
}

// New and Delete of mail may be in different part of code.

func allocAtomMail() *atomMail {
	am := atomMailsPool.Get().(*atomMail)
	m := NewMail(DefaultMailId, am)
	am.Mail = m
	return am
}

func deallocAtomMail(am *atomMail) {
	DelMail(am.Mail)
	atomMailsPool.Put(am)
}

// Message Mail

func initMessageMail(am *atomMail, from Id, name string, arg proto.Message) {
	am.Mail.id = DefaultMailId
	am.Mail.action = MailActionRun
	am.typ = MailTypeMessage
	am.from = from
	am.name = name
	// TODO: Think about proto.Clone or not
	am.arg = proto.Clone(arg)
	am.mailReply = mailReply{}
	am.waitCh = make(chan *mailReply, 1)
}

// Task Mail

func initTaskMail(am *atomMail, taskId uint64, name string, arg proto.Message) {
	am.Mail.id = taskId
	am.Mail.action = MailActionRun
	am.typ = MailTypeTask
	am.name = name
	// TODO: Think about proto.Clone or not
	am.arg = proto.Clone(arg)
}

// Upgrade Mail

func initUpgradeMail(am *atomMail, define *ElementDefine) {
	am.Mail.id = DefaultMailId
	am.Mail.action = MailActionRestart
	am.typ = MailTypeReload
	am.upgrade = define
}

// Halt Mail

func initKillMail(am *atomMail, from Id) {
	am.Mail.id = DefaultMailId
	am.Mail.action = MailActionExit
	am.typ = MailTypeHalt
	am.from = from
	am.mailReply = mailReply{}
	am.waitCh = make(chan *mailReply, 1)
}

// Mail Reply

type mailReply struct {
	resp proto.Message
	err  error
}

func (m *atomMail) sendReply(resp proto.Message, err error) {
	m.mailReply.resp = resp
	m.mailReply.err = err
	// Because sendReply method will only be called in loop of MailBox,
	// it's safe to do above.
	if m.waitCh != nil {
		m.waitCh <- &m.mailReply
		m.waitCh = nil
	} else {
		log.Println("Repeatedly reply")
	}
}

// THINKING: Wait channel must be empty before delete mail.
// An empty channel here means the receiver has received.
func (m *atomMail) waitReply() (resp proto.Message, err error) {
	reply := <-m.waitCh
	resp = reply.resp
	err = reply.err
	return resp, err
}
