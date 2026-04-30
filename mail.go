package atomos

import (
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
)

// Mail

const DefaultMailID = 0

type mail struct {
	next *mail
	id   uint64
	data any
}

type mailExitCommand struct {
	data   any
	onDone func()
}

var mailPool = sync.Pool{
	New: func() any { return &mail{} },
}

var allocMailUsingPool = false
var allocMailInitCheck = false
var allocMailDebug = false
var allocMailDebugMap = sync.Map{}

// allocMail 分配邮件对象
// Allocate mail object
func allocMail() *mail {
	if allocMailUsingPool {
		return mailPool.Get().(*mail)
	} else if allocMailDebug {
		m := &mail{}
		_, file, line, _ := runtime.Caller(1)
		allocMailDebugMap.Store(m, file+":"+strconv.Itoa(line))
		return m
	} else {
		return &mail{}
	}
}

// initMail 初始化邮件对象
// Initialize mail object
func initMail(m *mail, mailID uint64, data any) {
	if allocMailInitCheck {
		if m.next != nil {
			panic("initMail: mail already allocated")
		}
		if m.id != 0 {
			panic("initMail: mail already allocated")
		}
		if m.data != nil {
			panic("initMail: mail already allocated")
		}
	}
	m.next = nil
	m.id = mailID
	m.data = data
}

// initKillMail 初始化退出邮件对象
// Initialize kill mail object
func initKillMail(m *mail, mailID uint64, data any, onDone func()) *mailExitCommand {
	if allocMailInitCheck {
		if m.next != nil {
			panic("initKillMail: mail already allocated")
		}
		if m.id != 0 {
			panic("initKillMail: mail already allocated")
		}
		if m.data != nil {
			panic("initKillMail: mail already allocated")
		}
	}
	em := &mailExitCommand{
		data:   data,
		onDone: onDone,
	}
	m.next = nil
	m.id = mailID
	m.data = em
	return em
}

// releaseMail 释放邮件对象
// Release mail object
func releaseMail(m *mail) {
	if allocMailUsingPool {
		m.next = nil
		m.id = 0
		m.data = nil
		mailPool.Put(m)
	} else {
		if allocMailDebug {
			_, has := allocMailDebugMap.LoadAndDelete(m)
			if !has {
				panic("releaseMail: mail not in debug map")
			}
		}
	}
}

// Mailbox

type MailboxHandler interface {
	mailboxOnStartUp(fn func() *Error) *Error
	mailboxOnReceive(mail *mail)
	mailboxOnStop(stopMail, remainMails *mail, num uint32) *Error
}

// mailBox 邮箱
// Some thoughts about mailbox design:
//  1. Mailbox has a goroutine to process mails.
//  2. Mailbox has a queue to store incoming mails.
//  3. Mailbox has a mutex to protect the queue.
//     3.1 Even though the mailbox is running in a single goroutine, multiple goroutines may push mails into the mailbox concurrently.
//     So, the push of mail is from other goroutines, so the push won't be blocked by the mailbox goroutine.
//  4. Mailbox has a condition variable to signal the goroutine when new mail arrives.
//  5. Mailbox has a handler to process mails.
type mailBox struct {
	name    string
	mutex   sync.Mutex
	cond    *sync.Cond
	running bool
	handler MailboxHandler
	head    *mail
	tail    *mail
	num     uint32

	logging *loggingAtomos

	goID uint64
}

func newMailBox(name string, handler MailboxHandler, logging *loggingAtomos) *mailBox {
	mb := &mailBox{
		name:    name,
		mutex:   sync.Mutex{},
		cond:    nil,
		running: false,
		handler: handler,
		head:    nil,
		tail:    nil,
		num:     0,
		logging: logging,
		goID:    0,
	}
	mb.cond = sync.NewCond(&mb.mutex)
	return mb
}

func (mb *mailBox) isRunning() bool {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()
	return mb.running
}

func (mb *mailBox) getNum() uint32 {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()
	return mb.num
}

func (mb *mailBox) start(fn func() *Error) *Error {
	mb.mutex.Lock()
	if mb.running {
		mb.mutex.Unlock()
		return NewError(ErrFrameworkRecoverFromPanic, "Mailbox: Has already run.").AddStack(nil)
	}
	mb.running = true
	mb.mutex.Unlock()
	if err := mb.startLoop(fn); err != nil {
		return err.AddStack(nil)
	}
	return nil
}

func (mb *mailBox) waitPop() *mail {
	mb.mutex.Lock()
	if mb.num == 0 {
		mb.cond.Wait()
	}
	mb.num -= 1
	if mb.head == mb.tail {
		m := mb.head
		mb.head = nil
		mb.tail = nil
		if m != nil {
			m.next = nil
		}
		mb.mutex.Unlock()
		return m
	} else {
		m := mb.head
		mb.head = m.next
		m.next = nil
		mb.mutex.Unlock()
		return m
	}
}

func (mb *mailBox) getByID(id uint64) *mail {
	mb.mutex.Lock()
	m := mb.head
	if m == nil {
		mb.mutex.Unlock()
		return nil
	}
	for ; m != nil; m = m.next {
		if m.id == id {
			mb.mutex.Unlock()
			return m
		}
	}
	mb.mutex.Unlock()
	return nil
}

func (mb *mailBox) pushHead(m *mail) bool {
	mb.mutex.Lock()
	if !mb.running {
		mb.mutex.Unlock()
		return false
	}
	mb.num += 1
	if mb.head == nil {
		mb.head = m
		mb.tail = m
	} else {
		m.next = mb.head
		mb.head = m
	}
	mb.cond.Signal()
	mb.mutex.Unlock()
	return true
}

func (mb *mailBox) Push(m *mail) bool {
	return mb.pushTail(m)
}

func (mb *mailBox) pushTail(m *mail) bool {
	mb.mutex.Lock()
	if !mb.running {
		mb.mutex.Unlock()
		return false
	}
	mb.num += 1
	if mb.head == nil {
		mb.head = m
		mb.tail = m
	} else {
		mb.tail.next = m
		mb.tail = m
	}
	mb.cond.Signal()
	mb.mutex.Unlock()
	return true
}

// popAll 弹出所有邮件，返回邮件链表头指针和邮件数量
// Pop all mails, return the head pointer of mail linked list and the number of mails
// remember to release mails after use
func (mb *mailBox) popAll() (head *mail, num uint32) {
	mb.mutex.Lock()
	// There is no Mail in box
	if mb.head == nil {
		mb.mutex.Unlock()
		return nil, 0
	}
	head = mb.head
	num = mb.num
	mb.num = 0
	mb.head = nil
	mb.tail = nil
	mb.mutex.Unlock()
	return head, num
}

// popByID 根据邮件ID弹出邮件
// Pop mail by mail ID
// remember to release mail after use
func (mb *mailBox) popByID(id uint64) *mail {
	mb.mutex.Lock()
	var pM, m *mail = nil, mb.head
	if m == nil {
		mb.mutex.Unlock()
		return nil
	}
	for m != nil {
		if m.id == id {
			mb.num -= 1
			if pM == nil {
				mb.head = m.next
				if m == mb.tail {
					mb.tail = nil
				}
			} else {
				pM.next = m.next
				if m == mb.tail {
					mb.tail = pM
					mb.tail.next = nil
				}
			}
			m.next = nil
			mb.mutex.Unlock()
			return m
		}
		pM = m
		m = m.next
	}
	mb.mutex.Unlock()
	return nil
}

// removeMail 从邮箱中移除指定邮件
// Remove specified mail from mailbox
// remember to release mail after use
func (mb *mailBox) removeMail(dm *mail) (ok bool) {
	mb.mutex.Lock()
	var pM, m *mail = nil, mb.head
	if m == nil {
		mb.mutex.Unlock()
		return false
	}
	for m != nil {
		if m == dm {
			mb.num -= 1
			if pM == nil {
				mb.head = m.next
				if m == mb.tail {
					mb.tail = nil
				}
			} else {
				pM.next = m.next
				if m == mb.tail {
					mb.tail = pM
					mb.tail.next = nil
				}
			}
			m.next = nil
			mb.mutex.Unlock()
			return true
		}
		pM = m
		m = m.next
	}
	mb.mutex.Unlock()
	return false
}

//func (mb *mailBox) stop(onDone func()) {
//	mb.mutex.Lock()
//	if !mb.running {
//		mb.mutex.Unlock()
//		return
//	}
//	mb.running = false
//
//	m := allocMail()
//	initKillMail(m, DefaultMailID, nil, onDone)
//
//	mb.num += 1
//	if mb.head == nil {
//		mb.head = m
//		mb.tail = m
//	} else {
//		m.next = mb.head
//		mb.head = m
//	}
//	mb.cond.Signal()
//	mb.mutex.Unlock()
//}

// stopIfNoMail 如果邮箱中没有邮件，则停止邮箱运行，返回是否成功停止
// Stop mailbox if there is no mail, return whether it is successfully stopped
func (mb *mailBox) stopIfNoMail(onDone func()) (killed bool) {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()
	if !mb.running {
		return false
	}
	if mb.num > 0 {
		return false
	}
	mb.running = false

	m := allocMail()
	initKillMail(m, DefaultMailID, nil, onDone)

	mb.num += 1
	if mb.head == nil {
		mb.head = m
		mb.tail = m
	} else {
		m.next = mb.head
		mb.head = m
	}
	mb.cond.Signal()

	return true
}

func (mb *mailBox) startLoop(fn func() *Error) *Error {
	waitStart := make(chan *Error, 1)
	go mb.loop(waitStart, fn)
	err := <-waitStart
	if err != nil {
		mb.mutex.Lock()
		mb.running = false
		mb.mutex.Unlock()
		return err.AddStack(nil)
	}
	return nil
}

func (mb *mailBox) loop(wait chan *Error, fn func() *Error) {
	// 获取当前进程Goroutine ID。
	// Get current goroutine ID.
	mb.goID = func() uint64 {
		// Should not panic here, but just in case.
		// If panic happens, log it and return 0, which means failure and reject starting mailbox.
		defer func() {
			if r := recover(); r != nil {
				mb.logging.pushFrameworkErrorLog("Mailbox: Recover from panic. It's getting goID. reason=(%v),stack=(%s)",
					r, string(debug.Stack()))
			}
		}()
		return getGoID()
	}()
	if mb.goID == 0 {
		mb.logging.pushFrameworkFatalLog("Mailbox: Failed to get goID.")
		wait <- NewError(ErrFrameworkInternalError, "Failed to get goID.").AddStack(nil)
		return
	}

	mb.logging.pushFrameworkInfoLog("Mailbox: Start. name=(%s)", mb.name)
	defer func() {
		mb.logging.pushFrameworkInfoLog("Mailbox: Stop. name=(%s)", mb.name)
	}()

	if err := mb.handler.mailboxOnStartUp(fn); err != nil {
		wait <- err.AddStack(nil)
		return
	}
	wait <- nil

	for {
		if exit := func() (exit bool) {
			exit = false
			var curMail *mail
			defer func() {
				if r := recover(); r != nil {
					mb.logging.pushFrameworkErrorLog("Mailbox: Recover from panic. reason=(%v),stack=(%s)",
						r, string(debug.Stack()))
				}
			}()
			for {
				// If there is no more new message, just waiting.
				curMail = mb.waitPop()
				// If the mail has been deleted, continue to the next mail.
				if curMail == nil {
					continue
				}
				switch value := curMail.data.(type) {
				case *mailExitCommand:
					{
						if value == nil {
							mb.logging.pushFrameworkErrorLog("Mailbox: Invalid exit command.")
						}
						// Set stop running.
						// To refuse all incoming mails.
						mb.mutex.Lock()
						mb.running = false
						mb.mutex.Unlock()
						exit = true
						// Reject all mails backward.
						mails, num := mb.popAll()
						if err := mb.handler.mailboxOnStop(curMail, mails, num); err != nil {
							mb.logging.pushFrameworkErrorLog("Mailbox: Failed to execute stop. err=(%v)", err)
						}
						if value != nil && value.onDone != nil {
							value.onDone()
						}

						// release mails
						releaseMail(curMail)
						for ; mails != nil; mails = mails.next {
							releaseMail(mails)
						}
						return
					}
				default:
					{
						// When this line can be executed, it means there is mail in box.
						mb.handler.mailboxOnReceive(curMail)
						// release mail
						releaseMail(curMail)
					}
				}
			}
		}(); exit {
			break
		}
	}
}
