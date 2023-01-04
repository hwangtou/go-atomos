package go_atomos

import (
	"runtime/debug"
	"sync"
)

// Mail

type mail struct {
	next   *mail
	id     uint64
	action MailAction

	mail *atomosMail
	log  *LogMail
}

type MailAction int

const (
	MailActionRun  = 0
	MailActionExit = 1
)

// Mail Box

type MailBoxOnReceiveFn func(mail *mail)
type MailBoxOnStopFn func(stopMail, remainMails *mail, num uint32)

type MailBoxHandler struct {
	OnReceive MailBoxOnReceiveFn
	OnStop    MailBoxOnStopFn
}

type mailBox struct {
	name    string
	mutex   sync.Mutex
	cond    *sync.Cond
	running bool
	handler MailBoxHandler
	head    *mail
	tail    *mail
	num     uint32
}

func newMailBox(name string, handler MailBoxHandler) *mailBox {
	mb := &mailBox{
		name:    name,
		mutex:   sync.Mutex{},
		cond:    nil,
		running: false,
		handler: handler,
		head:    nil,
		tail:    nil,
		num:     0,
	}
	mb.cond = sync.NewCond(&mb.mutex)
	return mb
}

func (mb *mailBox) start() {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()
	if mb.running {
		return
	}
	mb.running = true
	go mb.loop()
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
		m.next = nil
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

func (mb *mailBox) removeMail(dm *mail) bool {
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

func (mb *mailBox) loop() {
	sharedLogging.pushProcessLog(LogLevel_Debug, "Mailbox: Start. name=(%s)", mb.name)
	defer func() {
		sharedLogging.pushProcessLog(LogLevel_Debug, "Mailbox: Stop. name=(%s)", mb.name)
	}()
	for {
		if exit := func() (exit bool) {
			exit = false
			var curMail *mail
			defer func() {
				if r := recover(); r != nil {
					sharedLogging.pushFrameworkErrorLog("Mailbox: Recover from panic, reason=(%v),stack=(%s)",
						r, string(debug.Stack()))
				}
			}()
			for {
				// If there is no more new message, just waiting.
				curMail = mb.waitPop()
				switch curMail.action {
				case MailActionRun:
					// When this line can be executed, it means there is mail in box.
					mb.handler.OnReceive(curMail)
				case MailActionExit:
					// Set stop running.
					// To refuse all incoming mails.
					mb.mutex.Lock()
					mb.running = false
					mb.mutex.Unlock()
					exit = true
					// Reject all mails backward.
					mails, num := mb.popAll()
					if curMail.mail.executeStop {
						mb.handler.OnStop(curMail, mails, num)
					}
					// Wait channel.
					curMail.mail.sendReply(nil, nil)
					return
				}
			}
		}(); exit {
			break
		}
	}
}

//goos: darwin
//goarch: amd64
//pkg: atomosPlayground/atomosClone/m
//BenchmarkMailbox1-12             3222063               380 ns/op
//BenchmarkMailbox2-12             2005591               574 ns/op
//BenchmarkMailbox4-12             1000000              1028 ns/op
//BenchmarkMailbox8-12              784436              1928 ns/op
//BenchmarkMailbox16-12             418808              3561 ns/op
//BenchmarkMailbox32-12             181590              6990 ns/op
//BenchmarkMailbox64-12             105127             14608 ns/op
//BenchmarkMailbox128-12             46101             27612 ns/op
//PASS
//ok      atomosPlayground/atomosClone/m  15.112s
