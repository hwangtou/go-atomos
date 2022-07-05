package go_atomos

// CHECKED!

import (
	"sync"

	"google.golang.org/protobuf/proto"
)

//
// AtomMail
//
// 关于Atomos的并发，我不采用Go语言常用的CSP模型，因为CSP模型有些问题，是比较难解决的：
// #1 Go的Channel在队列内容超出了容量之后，插入Channel的内容顺序不能确定，而且会阻塞发送方。
// #2 Go的Channel无法对内容进行"插队"或"取消"。
// #3 Go的Channel是单向的，没有系统的办法去处理回调问题。
//

const DefaultMailId = 0

// 邮件类型

type AtomMailType int

const (
	// AtomMailHalt
	// 终止邮件，用于停止Atom的运行。
	// Halt Mail, for stopping an atom from running.
	AtomMailHalt AtomMailType = 0

	// AtomMailMessage
	// 信息邮件，用于外部给运行中的Atom传递信息。
	// Message Mail, for messaging to a running atom from outer.
	AtomMailMessage AtomMailType = 1

	// AtomMailTask
	// 任务邮件，用于内部给运行中的Atom新增任务。
	// Task Mail, for adding task to a running atom from inner.
	AtomMailTask AtomMailType = 2

	// AtomMailReload
	// 重载邮件，用于升级Atom的ElementLocal引用，以实现热更。
	// Reload Mail, for upgrading ElementLocal reference of an atom, to support hot-reload feature.
	AtomMailReload AtomMailType = 3

	AtomMailWormhole AtomMailType = 4
)

// Atom邮件
// Atom Mail

type atomMail struct {
	// 具体的Mail实例
	// Concrete Mail instance.
	*mail

	// Atom邮件类型
	// Atom mail type.
	//
	// Halt, Message, Task, Reload
	mailType AtomMailType

	// 从哪个Id发来的邮件。
	// Mail send from which Id.
	from ID

	// Message和Task邮件会使用到的，调用的目标对象的名称。
	// Mail target name, used by Message mail and Task mail.
	name string

	// Message和Task邮件的参数。
	// Argument that pass to target, used by Message mail and Task mail.
	arg proto.Message

	// 需要升级的Element。
	// Upgrade Element.
	upgrade      *ElementImplementation
	upgradeCount int

	wormholeAction int
	wormhole       WormholeDaemon

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
	m := newMail(DefaultMailId, am)
	am.mail = m
	return am
}

func deallocAtomMail(am *atomMail) {
	delMail(am.mail)
	atomMailsPool.Put(am)
}

// 消息邮件
// Message Mail
func initMessageMail(am *atomMail, from ID, name string, arg proto.Message) {
	am.mail.id = DefaultMailId
	am.mail.action = MailActionRun
	am.mailType = AtomMailMessage
	am.from = from
	am.name = name
	// I think it has to be cloned, because argument is passing between atoms.
	if arg != nil {
		am.arg = proto.Clone(arg)
	} else {
		am.arg = nil
	}
	am.upgrade = nil
	am.upgradeCount = 0
	am.mailReply = mailReply{}
	am.waitCh = make(chan *mailReply, 1)
}

// 任务邮件
// Task Mail
func initTaskMail(am *atomMail, taskId uint64, name string, arg proto.Message) {
	am.mail.id = taskId
	am.mail.action = MailActionRun
	am.mailType = AtomMailTask
	am.from = nil
	am.name = name
	// I think it doesn't have to clone, because Atom is thread-safe.
	am.arg = arg
	am.upgrade = nil
	am.upgradeCount = 0
	am.mailReply = mailReply{}
	am.waitCh = make(chan *mailReply, 1)
}

// 重载邮件
// Reload Mail
func initReloadMail(am *atomMail, elem *ElementImplementation, upgradeCount int) {
	am.mail.id = DefaultMailId
	am.mail.action = MailActionRun
	am.mailType = AtomMailReload
	am.from = nil
	am.name = ""
	am.upgrade = elem
	am.upgradeCount = upgradeCount
	am.mailReply = mailReply{}
	am.waitCh = make(chan *mailReply, 1)
}

func initWormholeMail(am *atomMail, action int, wormhole WormholeDaemon) {
	am.mail.id = DefaultMailId
	am.mail.action = MailActionRun
	am.mailType = AtomMailWormhole
	am.from = nil
	am.name = ""
	am.arg = nil
	am.wormholeAction = action
	am.wormhole = wormhole
	am.upgrade = nil
	am.upgradeCount = 0
	am.mailReply = mailReply{}
	am.waitCh = make(chan *mailReply, 1)
}

// 终止邮件
// Halt Mail
func initKillMail(am *atomMail, from ID) {
	am.mail.id = DefaultMailId
	am.mail.action = MailActionExit
	am.mailType = AtomMailHalt
	am.from = from
	am.name = ""
	am.upgrade = nil
	am.upgradeCount = 0
	am.mailReply = mailReply{}
	am.waitCh = make(chan *mailReply, 1)
}

// Mail返回
// Mail Reply
type mailReply struct {
	resp proto.Message
	err  error
}

// Method sendReply() will only be called in for-loop of MailBox, it's safe to do so, because while an atom is
// waiting for replying, the atom must still be running. Or if the atom is not waiting for replying, after mailReply
// has been sent to waitCh, there will has no reference to the waitCh, waitCh will be collected.
func (m *atomMail) sendReply(resp proto.Message, err error) {
	m.mailReply.resp = resp
	m.mailReply.err = err
	if m.waitCh != nil {
		m.waitCh <- &m.mailReply
		m.waitCh = nil
	} else {
		panic("atomMail.sendReply: waitCh has been replied")
	}
}

// TODO: Think about waitReply() is still waiting when cosmos runnable is exiting.
func (m *atomMail) waitReply() (resp proto.Message, err error) {
	// An empty channel here means the receiver has received. It must be framework problem otherwise it won't happen.
	reply := <-m.waitCh
	// Wait channel must be empty before delete a mail.
	resp = reply.resp
	err = reply.err
	return resp, err
}
