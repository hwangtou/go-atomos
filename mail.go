package go_atomos

import (
	"errors"
	"log"
	"runtime/debug"
	"sync"
)

var (
	ErrMailBoxClosed = errors.New("MailBox: Closed")
	ErrMailBoxPanic  = errors.New("MailBox: Panic")
)

// Mail

type Mail struct {
	next    *Mail
	id      uint64
	action  MailAction
	Content interface{}
}

type MailAction int

const (
	MailActionRun  = 0
	MailActionExit = 1
)

var mailsPool = sync.Pool{
	New: func() interface{} {
		return &Mail{}
	},
}

func NewMail(id uint64, content interface{}) *Mail {
	m := mailsPool.Get().(*Mail)
	m.Reset()
	m.id = id
	m.Content = content
	return m
}

func NewExitMail() *Mail {
	m := NewMail(0, nil)
	m.action = MailActionExit
	return m
}

func DelMail(m *Mail) {
	mailsPool.Put(m)
}

func (m *Mail) Reset() {
	m.next = nil
	m.id = 0
	m.action = MailActionRun
	m.Content = nil
}

// Mail Box

type MailBoxOnReceiveFn func(mail *Mail)
type MailBoxOnPanicFn func(mail *Mail, trace string)
type MailBoxOnStopFn func(stopMail, remainMails *Mail, num uint32)

type MailBoxHandler struct {
	OnReceive MailBoxOnReceiveFn
	OnPanic   MailBoxOnPanicFn
	OnStop    MailBoxOnStopFn
}

type MailBox struct {
	mutex   sync.Mutex
	cond    *sync.Cond
	running bool
	handler MailBoxHandler
	head    *Mail
	tail    *Mail
	num     uint32
}

var mailBoxPool = sync.Pool{
	New: func() interface{} {
		b := &MailBox{}
		b.cond = sync.NewCond(&b.mutex)
		return b
	},
}

func NewMailBox(handler MailBoxHandler) *MailBox {
	mb := mailBoxPool.Get().(*MailBox)
	mb.handler = handler
	return mb
}

func initMailBox(a *AtomCore) {
	a.mailbox = NewMailBox(MailBoxHandler{
		OnReceive: a.onReceive,
		OnPanic:   a.onPanic,
		OnStop:    a.onStop,
	})
}

func DelMailBox(b *MailBox) {
	b.Reset()
	mailBoxPool.Put(b)
}

func (mb *MailBox) Start() {
	mb.mutex.Lock()
	defer mb.mutex.Unlock()
	if mb.running {
		return
	}
	mb.running = true
	go mb.loop()
}

func (mb *MailBox) Stop() {
	mb.PushHead(NewExitMail())
}

func (mb *MailBox) WaitPop() *Mail {
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

func (mb *MailBox) GetById(id uint64) *Mail {
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

func (mb *MailBox) PushHead(m *Mail) bool {
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

func (mb *MailBox) PushTail(m *Mail) bool {
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

func (mb *MailBox) PopAll() (head *Mail, num uint32) {
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

func (mb *MailBox) PopById(id uint64) *Mail {
	mb.mutex.Lock()
	var pM, m *Mail = nil, mb.head
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

func (mb *MailBox) loop() {
	for {
		if exit := func() (exit bool) {
			exit = false
			var curMail *Mail
			defer func() {
				if r := recover(); r != nil {
					// Only should AtomMailMessage and AtomMailTask 3rd-part logic
					// throws exception to here, otherwise it must be a bug of framework.
					traceMsg := string(debug.Stack())
					log.Printf("recovering from 3rd-part logic\nreason=%s\ntrace=%s", r, traceMsg)
					//curMail.sendReply(nil, errors.AddElement(traceMsg))
					// todo
					mb.handler.OnPanic(curMail, traceMsg)
				}
			}()
			for {
				// If there is no more new message, just waiting.
				curMail = mb.WaitPop()
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
					mails, num := mb.PopAll()
					mb.handler.OnStop(curMail, mails, num)
					break
				}
			}
		}(); exit {
			break
		}
	}
	DelMailBox(mb)
}

func (mb *MailBox) Reset() {
	mb.running = false
	mb.handler = MailBoxHandler{}
	mb.head = nil
	mb.tail = nil
	mb.num = 0
}

func (mb *MailBox) sharedLock() *sync.Mutex {
	return &mb.mutex
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
