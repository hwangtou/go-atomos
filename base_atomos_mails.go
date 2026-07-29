package atomos

import (
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

//
// AtomosMail
//
// 关于Atomos的并发，我不采用Go语言常用的CSP模型，因为CSP模型有些问题，是比较难解决的：
// #1 Go的Channel在队列内容超出了容量之后，插入Channel的内容顺序不能确定，而且会阻塞发送方。
// #2 Go的Channel无法对内容进行"插队"或"取消"。
// #3 Go的Channel是单向的，没有系统的办法去处理回调问题。
//

// 邮件类型

type BaseAtomosMailType int

const (
	// BaseAtomosMailKill
	// 终止邮件，用于停止Atomos的运行。
	// Stopping Mail, for stopping an atomos from running.
	BaseAtomosMailKill BaseAtomosMailType = 0

	// BaseAtomosMailSync
	// 信息邮件，用于外部给运行中的Atomos传递信息。
	// Message Mail, for messaging to a running atomos from outer.
	BaseAtomosMailSync BaseAtomosMailType = 1

	// BaseAtomosMailAsync
	// 异步信息邮件
	// Async Message Mail without callback.
	BaseAtomosMailAsync BaseAtomosMailType = 2

	// BaseAtomosMailOnAsyncCallback
	// 异步信息回调邮件。
	BaseAtomosMailOnAsyncCallback BaseAtomosMailType = 3

	// BaseAtomosMailWormhole
	// 虫洞邮件，用于传递不属于"Atomos宇宙"概念的对象。
	// Wormhole Mail, for transporting non-"Atomos Cosmos" object.
	BaseAtomosMailWormhole BaseAtomosMailType = 4

	// BaseAtomosMailTask
	// 任务邮件，用于内部给运行中的Atomos新增任务。
	// Task Mail, for adding task to a running atomos from inner.
	BaseAtomosMailTask BaseAtomosMailType = 5

	// BaseAtomosMailTaskCallback
	// 回调邮件。
	BaseAtomosMailTaskCallback BaseAtomosMailType = 6
)

// Atomos邮件
// Atomos Mail

type baseAtomosMail struct {
	// 具体的Mail实例
	// Concrete Mail instance.
	mail *mail

	// Atomos邮件类型
	// Atomos mail type.
	//
	// Stopping, Message, Task, Reload
	mailType BaseAtomosMailType

	// 从哪个ID发来的邮件。
	// Mail send from which ID.
	from ID

	// Message和Task邮件会使用到的，调用的目标对象的名称。
	// Mail target name, used by Message mail and Task mail.
	name string

	// Message和Task邮件的参数。
	// Argument that pass to target, used by Message mail and Task mail.
	arg proto.Message
	err *Error

	tracker *IDTracker

	wormhole BaseAtomosWormhole

	taskClosure func(uint64)

	atomosTask     *atomosTask
	atomosCallback *atomosCallback

	startupID uint64
	asyncID   uint64

	// 用于发邮件时阻塞调用go程，以及返回结果用的channel。
	// A channel used to block messaging goroutine, and return the result.
	mailReply mailReply
	waitCh    chan *mailReply

	mutex sync.Mutex
}

type atomosCallback struct {
	callback  func()
	recoverFn func(any)
}

// Construct and destruct of Mail may be in different part of code.

func allocBaseAtomosMail() *baseAtomosMail {
	am := &baseAtomosMail{}
	am.mail = allocMail()
	initMail(am.mail, DefaultMailID, am)
	return am
}

func allocBaseAtomosKillMail() (*mailExitCommand, *baseAtomosMail, *mail) {
	am := &baseAtomosMail{}
	am.mail = allocMail()
	em := initKillMail(am.mail, DefaultMailID, am, nil)
	return em, am, am.mail
}

// releaseBaseAtomosMail is intentionally a no-op.
//
// baseAtomosMail holds a waitCh (buffered chan), a proto.Message arg, and
// closures; correctly pooling it would require strict init/release pairing
// across every mail type and careful reuse-ordering of waitCh (a reused chan
// still holding a stale reply would corrupt the next caller). The GC already
// reclaims these short-lived allocations, and no benchmark has shown this to
// be an allocation bottleneck. Implementing a pool here would trade a
// theoretical GC saving for a real use-after-reuse risk, so we deliberately
// rely on GC until profiling proves otherwise.
func releaseBaseAtomosMail(am *baseAtomosMail) {
}

func unwrapBaseAtomosMail(m *mail) *baseAtomosMail {
	return m.data.(*baseAtomosMail)
}

func unwrapBaseAtomosKillMail(m *mail) (*mailExitCommand, *baseAtomosMail) {
	em := m.data.(*mailExitCommand)
	return em, em.data.(*baseAtomosMail)
}

func unwrapBaseAtomosFromKillMail(em *mailExitCommand) *baseAtomosMail {
	return em.data.(*baseAtomosMail)
}

// 初始化同步消息邮件
// Init Synchronous Message Mail
func initBaseAtomosMailSync(am *baseAtomosMail, from ID, name string, arg proto.Message) {
	am.mailType = BaseAtomosMailSync
	am.from = from
	am.name = name
	// Sync messaging blocks the caller until the reply is received, so the
	// argument is owned by the caller for the whole call. Still, the argument
	// crosses goroutine boundaries (caller -> mailbox), so we clone it by
	// default to prevent the handler from mutating the caller's message.
	if arg != nil {
		if ShouldArgumentClone {
			am.arg = proto.Clone(arg)
		} else {
			am.arg = arg
		}
	} else {
		am.arg = nil
	}
	am.waitCh = make(chan *mailReply, 1)
}

func initBaseAtomosMailAsync(am *baseAtomosMail, from ID, name string, startupID, asyncID uint64, arg proto.Message) {
	am.mailType = BaseAtomosMailAsync
	am.from = from
	am.name = name
	if arg != nil {
		if ShouldArgumentClone {
			am.arg = proto.Clone(arg)
		} else {
			am.arg = arg
		}
	} else {
		am.arg = nil
	}
	am.startupID = startupID
	am.asyncID = asyncID
}

// AsyncMessageCallback邮件
// Async Message Callback Mail
func initAsyncMessageCallbackMail(am *baseAtomosMail, from ID, name string, startupID, asyncID uint64, arg proto.Message, err *Error) {
	am.mailType = BaseAtomosMailOnAsyncCallback
	am.from = from
	am.name = name
	am.arg = arg
	am.err = err
	am.startupID = startupID
	am.asyncID = asyncID
}

// 任务闭包邮件
// Task Closure Mail
// name中记录调用的闭包代码定位信息。
func initTaskClosureMail(am *baseAtomosMail, name string, taskID uint64, closure func(uint64)) {
	am.mail.id = taskID
	am.mailType = BaseAtomosMailTask

	am.name = name
	am.taskClosure = closure
	am.waitCh = make(chan *mailReply, 1)
}

func initTaskQueueMail(am *baseAtomosMail, name string, helper *taskHelper) {
	am.mail.id = helper.atomosTask.id
	am.mailType = BaseAtomosMailTask

	am.name = name
	am.atomosTask = helper.atomosTask
	am.atomosTask.atomosMail = am
	am.atomosTask.helper = helper
	am.waitCh = make(chan *mailReply, 1)
}

func initCallbackMail(am *baseAtomosMail, name string, callback func(), recoverFn func(any)) {
	am.mail.id = DefaultMailID
	am.mailType = BaseAtomosMailTaskCallback

	am.name = name
	am.atomosCallback = &atomosCallback{
		callback:  callback,
		recoverFn: recoverFn,
	}
	am.waitCh = make(chan *mailReply, 1)
}

// 虫洞邮件
// Reload Mail
func initWormholeMail(am *baseAtomosMail, from ID, wormhole BaseAtomosWormhole) {
	am.mail.id = DefaultMailID
	am.mailType = BaseAtomosMailWormhole
	am.from = from
	am.name = ""
	am.arg = nil
	am.tracker = nil
	am.wormhole = wormhole
	am.mailReply = mailReply{}
	am.waitCh = make(chan *mailReply, 1)
}

// 终止邮件
// Stopping Mail
func initAtomosKillMail(am *baseAtomosMail, from ID) {
	am.mail.id = DefaultMailID
	am.mailType = BaseAtomosMailKill
	am.from = from
	am.name = ""
	am.tracker = nil
	am.wormhole = nil
	am.mailReply = mailReply{}
	am.waitCh = make(chan *mailReply, 1)
}

// Mail返回
// Mail Reply
type mailReply struct {
	resp proto.Message
	id   ID
	err  *Error
}

// Method sendReply() will only be called in for-loop of MailBox, it's safe to do so, because while an atomos is
// waiting for replying, the atomos must still be running. Or if the atomos is not waiting for replying, after mailReply
// has been sent to waitCh, there will has no reference to the waitCh, waitCh will be collected.
func (m *baseAtomosMail) sendReply(resp proto.Message, err *Error) {
	m.mutex.Lock()
	waitCh := m.waitCh
	//m.waitCh = nil
	m.mutex.Unlock()
	if waitCh == nil {
		return
	}

	m.mailReply.resp = resp
	m.mailReply.err = err
	select {
	case waitCh <- &m.mailReply:
	default:
	}
	//waitCh = nil
}

func (m *baseAtomosMail) sendReplyID(id ID, err *Error) {
	m.mutex.Lock()
	waitCh := m.waitCh
	//m.waitCh = nil
	m.mutex.Unlock()
	if waitCh == nil {
		return
	}

	m.mailReply.id = id
	m.mailReply.err = err
	waitCh <- &m.mailReply
}

// TODO: Think about waitReply() is still waiting when cosmos runnable is exiting.
func (m *baseAtomosMail) waitReply(a *BaseAtomos, helper *baseAtomosHelper) (resp proto.Message, err *Error) {
	timeout := helper.timeout

	m.mutex.Lock()
	waitCh := m.waitCh
	m.mutex.Unlock()
	// An empty channel here means the receiver has received. It must be framework problem otherwise it won't happen.
	if waitCh == nil {
		return nil, NewErrorf(ErrFrameworkRecoverFromPanic, "Atomos Message wait invalid.").AddStack(nil)
	}
	var reply *mailReply
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	select {
	case reply = <-waitCh:
	case <-time.After(timeout):
		if a.mailbox.removeMail(m.mail) {
			releaseMail(m.mail)
			return nil, NewErrorf(ErrAtomosPushTimeoutReject, "Atomos: Message is timeout and rejected. id=(%s),name=(%s),timeout=(%v)", a.id.Info(), m.name, timeout).AddStack(nil)
		} else {
			return nil, NewErrorf(ErrAtomosPushTimeoutHandling, "Atomos: Message is handling timeout. id=(%s),name=(%s),timeout=(%v),current=(%s)", a.id.Info(), m.name, timeout, a.mt.current).AddStack(nil)
		}
	}
	// Wait channel must be empty before delete a mail.
	if reply == nil {
		return nil, NewErrorf(ErrFrameworkRecoverFromPanic, "Atomos: Message reply is invalid.").AddStack(nil)
	}
	resp = reply.resp
	err = reply.err
	return resp, err
}

// TODO: Think about waitReplyID() is still waiting when cosmos runnable is exiting.
func (m *baseAtomosMail) waitReplyID(a *BaseAtomos, timeout time.Duration) (id ID, err *Error) {
	m.mutex.Lock()
	waitCh := m.waitCh
	m.mutex.Unlock()
	// An empty channel here means the receiver has received. It must be framework problem otherwise it won't happen.
	if waitCh == nil {
		return nil, NewErrorf(ErrFrameworkRecoverFromPanic, "Atomos: Message wait invalid.").AddStack(nil)
	}

	// An empty channel here means the receiver has received. It must be framework problem otherwise it won't happen.
	var reply *mailReply
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	select {
	case reply = <-waitCh:
	case <-time.After(timeout):
		if a.mailbox.removeMail(m.mail) {
			releaseMail(m.mail)
			return nil, NewErrorf(ErrAtomosPushTimeoutReject, "Atomos: Message is timeout and rejected. id=(%s),name=(%s),timeout=(%v)", a.id.Info(), m.name, timeout).AddStack(nil)
		} else {
			return nil, NewErrorf(ErrAtomosPushTimeoutHandling, "Atomos: Message is handling timeout. id=(%s),name=(%s),timeout=(%v)", a.id.Info(), m.name, timeout).AddStack(nil)
		}
	}
	// Wait channel must be empty before delete a mail.
	id = reply.id
	err = reply.err
	return id, err
}
