package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"sync"
)

//
// AtomMail
//

const DefaultMailId = 0

// 邮件类型
type AtomMailType int

const (
	// 终止邮件，用于停止Atom的运行。
	// Halt Mail, for stopping an atom from running.
	AtomMailHalt AtomMailType = 0

	// 信息邮件，用于外部给运行中的Atom传递信息。
	// Message Mail, for messaging to a running atom from outer.
	AtomMailMessage AtomMailType = 1

	// 任务邮件，用于内部给运行中的Atom新增任务。
	// Task Mail, for adding task to a running atom from inner.
	AtomMailTask AtomMailType = 2

	// 重载邮件，用于升级Atom的ElementLocal引用，以实现热更。
	// Reload Mail, for upgrading ElementLocal reference of an atom, to support hot-reload feature.
	AtomMailReload AtomMailType = 3
)

// Atom邮件
// Atom Mail
type atomMail struct {
	// 具体的Mail实例
	// Concrete Mail instance.
	*Mail

	// Atom邮件类型
	// Atom mail type.
	//
	// Halt, Message, Task, Reload
	mailType AtomMailType

	// 从哪个Id发来的邮件。
	// Mail send from which Id.
	from Id

	// Message和Task邮件会使用到的，调用的目标对象的名称。
	// Mail target name, used by Message mail and Task mail.
	name string

	// Message和Task邮件的参数。
	// Argument that pass to target, used by Message mail and Task mail.
	arg proto.Message

	// 需要升级的Element。
	// Upgrade Element.
	upgradeVersion uint64

	// 用于发邮件时阻塞调用go程，以及返回结果用的channel。
	// A channel used to block messaging goroutine, and return the result.
	mailReply mailReply
	waitCh    chan *mailReply
}

// Atom邮件内存池
// Atom Mails Pool
var atomMailsPool = sync.Pool{
	New: func() interface{} {
		return &atomMail{
			mailType:  0,
			from:      nil,
			name:      "",
			arg:       nil,
			mailReply: mailReply{},
			waitCh:    nil,
		}
	},
}

// Construct and destruct of Mail may be in different part of code.

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

// 消息邮件
// Message Mail
func initMessageMail(am *atomMail, from Id, name string, arg proto.Message) {
	am.Mail.id = DefaultMailId
	am.Mail.action = MailActionRun
	am.mailType = AtomMailMessage
	am.from = from
	am.name = name
	// TODO: Think about proto.Clone or not. I think it has to because argument is pass through Atoms.
	am.arg = proto.Clone(arg)
	am.mailReply = mailReply{}
	am.waitCh = make(chan *mailReply, 1)
}

// 任务邮件
// Task Mail
func initTaskMail(am *atomMail, taskId uint64, name string, arg proto.Message) {
	am.Mail.id = taskId
	am.Mail.action = MailActionRun
	am.mailType = AtomMailTask
	am.name = name
	// TODO: Think about proto.Clone or not. I think it don't have to clone because Atom is thread-safe.
	am.arg = arg
}

// 重载邮件
// Reload Mail
func initReloadMail(am *atomMail, version uint64) {
	am.Mail.id = DefaultMailId
	am.Mail.action = MailActionRun
	am.mailType = AtomMailReload
	am.upgradeVersion = version
}

// 终止邮件
// Halt Mail
func initKillMail(am *atomMail, from Id) {
	am.Mail.id = DefaultMailId
	am.Mail.action = MailActionExit
	am.mailType = AtomMailHalt
	am.from = from
	am.mailReply = mailReply{}
	am.waitCh = make(chan *mailReply, 1)
}

// Mail返回
// Mail Reply
type mailReply struct {
	resp proto.Message
	err  error
}

func (m *atomMail) sendReply(resp proto.Message, err error) {
	m.mailReply.resp = resp
	m.mailReply.err = err
	// Because sendReply method will only be called in loop of MailBox, so it's safe to do below, because while
	// Atom is waiting for replying, the Atom must still be running.
	// TODO: Think about for exiting an Atom.
	if m.waitCh != nil {
		m.waitCh <- &m.mailReply
		m.waitCh = nil
	} else {
		panic("ErrDuplicatedReply")
	}
}

// TODO: Think about wait channel must be empty before delete a mail.
// An empty channel here means the receiver has received.
func (m *atomMail) waitReply() (resp proto.Message, err error) {
	reply := <-m.waitCh
	resp = reply.resp
	err = reply.err
	return resp, err
}
