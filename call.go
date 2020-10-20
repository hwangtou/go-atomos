package atomos

import (
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
)

// Atomos and Atomos Call
// 原子和原子调用
// TODO Atom并发群发，通过组（分子）

// State
// 状态

type atomState uint8

const (
	Halt    atomState = 0
	Waiting atomState = 1
	Busy    atomState = 2
)

// Log
// 日志

type LogLevel string

const (
	Debug    LogLevel = "DEBUG"
	Info     LogLevel = "INFO"
	Warning  LogLevel = "WARNING"
	Critical LogLevel = "CRITICAL"
)

// Atom Controller
// 原子控制器

type AtomController interface {
	Log(level LogLevel, format string, args ...interface{})
	SetTimer(duration time.Duration, action AtomTimerFn) (uint64, error)
	CancelTimer(timerId uint64) error
	Close() error
}

func (a *atomWrap) Log(level LogLevel, format string, args ...interface{}) {
	a.data.LogId += 1
	a.cosmos.ds.logAtom(a.aType, a.aName, a.data.LogId, level, fmt.Sprintf(format, args...))
}

type AtomTimerFn func(exeOrCancel bool, delay time.Duration) uint64

// WARNING: Timer will not be saved.
func (a *atomWrap) SetTimer(duration time.Duration, action AtomTimerFn) (uint64, error) {
	a.mutex.Lock()
	if a.aState == Halt {
		a.mutex.Unlock()
		return 0, ErrAtomHalt
	}
	for {
		a.timer.curId += 1
		_, has := a.timer.timers[a.timer.curId]
		if !has {
			break
		}
		log.Println("timer")
	}
	timerId := a.timer.curId
	a.timer.timers[timerId] = &timerWrap{
		fn: action,
	}
	a.mutex.Unlock()
	time.AfterFunc(duration, func() {
		a.mutex.Lock()
		t, has := a.timer.timers[a.timer.curId]
		if has {
			delete(a.timer.timers, a.timer.curId)
		}
		a.mutex.Unlock()
		if has {
			mail := &atomMail{
				atom:  a.aIns,
				timer: t,
				sent:  time.Now(),
				next:  nil,
			}
			a.pushTimer(mail)
		}
	})
	return timerId, nil
}

func (a *atomWrap) CancelTimer(timerId uint64) error {
	a.mutex.Lock()
	if a.aState == Halt {
		a.mutex.Unlock()
		return ErrAtomHalt
	}
	t, has := a.timer.timers[timerId]
	if has {
		delete(a.timer.timers, timerId)
	}
	a.mutex.Unlock()
	if has {
		t.fn(false, 0)
	}
	return nil
}

func (a *atomWrap) Close() error {
	mail := &atomMail{
		atom:  a.aIns,
		reply: make(chan *replyWrap),
		halt:  true,
		sent:  time.Now(),
		next:  nil,
	}
	if ok := a.push(mail); !ok {
		return ErrAtomHalt
	}
	return nil
}

type atomWrap struct {
	cosmos *Cosmos    // Cosmos reference
	mutex  sync.Mutex // For atomic operation
	aType  string     // Type of atom
	aName  string     // Name of atom
	aIns   Atom       // Instance of atom
	aState atomState  // State(Halt/Waiting/Busy)
	// Mailbox, to receive messages
	mailsCond *sync.Cond
	mailsHead *atomMail
	mailsTail *atomMail
	mailsNum  uint
	// Timer
	timer *atomTimer
	// Data: AtomData & Log
	data *AtomData
	// Performance: Memory
	// Performance: Time
	elapses     map[string]*elapse
	procedureAt time.Time
	procedureFn string
	spawnAt     time.Time
}

// Looping atom runs
// 让原子循环运行起来
func (a *atomWrap) loop() {
	a.aState = Waiting
	a.spawnAt = time.Now()
	for {
		if a.mailsNum == 0 {
			a.mailsCond.L.Lock()
			a.mailsCond.Wait()
			a.mailsCond.L.Unlock()
		}
		m := a.pop()
		// Check exit
		if m.halt {
			a.mutex.Lock()
			a.data.Data = a.aIns.Save()
			err := a.cosmos.ds.saveAtom(a.aType, a.aName, a.data)
			m.reply <- &replyWrap{
				resp: nil,
				err:  err,
			}
			// Clean up remaining mails
			for m := a.pop(); m != nil; m = a.pop() {
				m.reply <- &replyWrap{
					resp: nil,
					err:  ErrAtomHalt,
				}
			}
			// Clean up remaining timers
			for id, t := range a.timer.timers {
				t.fn(false, 0)
				delete(a.timer.timers, id)
			}
			break
		}
		a.mutex.Lock()
		// Elapsed time: procedureAt
		c, has := a.elapses[m.fn.Name]
		if !has {
			c = &elapse{0, 0}
			a.elapses[m.fn.Name] = c
		}
		a.aState = Busy
		c.times += 1
		a.procedureAt, a.procedureFn = time.Now(), m.fn.Name
		a.mutex.Unlock()
		// Call or Timer
		if m.fn != nil {
			result, err := func() (proto.Message, error) {
				defer func() {
					if r := recover(); r != nil {
						a.Log(Critical,
							"call recovered from panic, stack traceback:\n%s",
							string(debug.Stack()))
					}
				}()
				return m.fn.Func(m.atom, m.arg)
			}()
			func() {
				defer func() {
					if r := recover(); r != nil {
						if r := recover(); r != nil {
							a.Log(Critical,
								"reply recovered from panic, stack traceback:\n%s",
								string(debug.Stack()))
						}
					}
				}()
				m.reply <- &replyWrap{
					resp: result,
					err:  err,
				}
			}()
		} else if m.timer != nil {
			m.timer.fn(true, time.Since(m.sent))
		}
		// Elapsed time: end
		a.mutex.Lock()
		c.duration += time.Since(a.procedureAt)
		a.procedureAt, a.procedureFn = time.Time{}, ""
		log.Printf("duration %v time %v", c.duration, c.times)
		a.aState = Waiting
		a.mutex.Unlock()
	}
	a.mutex.Unlock()
	a.aState = Halt
}

// Atom Mail Message
// 原子邮件信息

type atomMail struct {
	atom Atom
	// Call
	fn    *CallDesc
	arg   proto.Message
	reply chan *replyWrap
	// Timer
	timer *timerWrap
	// Action
	halt bool
	sent time.Time
	next *atomMail
}

func (a *atomWrap) push(m *atomMail) bool {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.aState == Halt {
		return false
	}
	a.mailsNum += 1
	if a.mailsHead == nil {
		a.mailsHead = m
		a.mailsTail = m
	} else {
		a.mailsTail.next = m
		a.mailsTail = m
	}
	a.mailsCond.Signal()
	return true
}

func (a *atomWrap) pushTimer(m *atomMail) bool {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.aState == Halt {
		return false
	}
	a.mailsNum += 1
	if a.mailsHead == nil {
		a.mailsHead = m
		a.mailsTail = m
	} else {
		m.next = a.mailsHead
		a.mailsHead = m
	}
	a.mailsCond.Signal()
	return true
}

func (a *atomWrap) pop() *atomMail {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.mailsHead == nil {
		return nil
	}
	a.mailsNum -= 1
	if a.mailsHead == a.mailsTail {
		m := a.mailsHead
		a.mailsHead = nil
		a.mailsTail = nil
		m.next = nil
		return m
	} else {
		m := a.mailsHead
		a.mailsHead = m.next
		m.next = nil
		return m
	}
}

// Performance Analyst
// 性能分析

type elapse struct {
	times    uint64
	duration time.Duration
}

type replyWrap struct {
	resp proto.Message
	err  error
}

// Timer
// 计时器

type atomTimer struct {
	curId  uint64
	timers map[uint64]*timerWrap
}

type timerWrap struct {
	fn AtomTimerFn
}

func (t *atomTimer) add() {
}

// Atom Type Container

type atomTypes map[string]*atomType

func (as atomTypes) getType(name string) *atomType {
	typeInfo, _ := as[name]
	return typeInfo
}

func (as atomTypes) addType(desc *AtomTypeDesc) {
	_, has := as[desc.Name]
	// todo remove old
	if !has {
		typeInfo := &atomType{
			name:  desc.Name,
			calls: make(map[string]*CallDesc),
			atoms: map[string]*atomWrap{},
		}
		for i := range desc.Calls {
			d := &desc.Calls[i]
			typeInfo.calls[d.Name] = &CallDesc{
				Name: d.Name,
				Func: d.Func,
			}
		}
		as[desc.Name] = typeInfo
	}
}

// Atom Type
// 原子类型

type atomType struct {
	name  string
	calls map[string]*CallDesc
	atoms map[string]*atomWrap
}

func (a *atomType) init(c *Cosmos, name string, atom Atom) (aw *atomWrap, err error) {
	aw = &atomWrap{
		cosmos:    c,
		mutex:     sync.Mutex{},
		aType:     a.name,
		aName:     name,
		aIns:      atom,
		aState:    Halt,
		mailsHead: nil,
		mailsTail: nil,
		mailsNum:  0,
		timer: &atomTimer{
			timers: map[uint64]*timerWrap{},
		},
		elapses: map[string]*elapse{},
		spawnAt: time.Now(),
	}
	aw.mailsCond = sync.NewCond(&aw.mutex)
	aw.data, err = c.ds.loadAtom(a.name, name)
	if err != nil {
		return nil, err
	}
	aw.aIns.Spawn(aw, aw.data.Data)
	a.atoms[name] = aw
	return aw, nil
}

func (a *atomType) has(name string) bool {
	_, has := a.atoms[name]
	return has
}

func (a *atomType) call(name, call string, arg proto.Message) (reply proto.Message, err error) {
	atom, has := a.atoms[name]
	if !has {
		return nil, ErrAtomNameNotExists
	}
	fnInfo, has := a.calls[call]
	if !has {
		return nil, ErrAtomCallNotExists
	}
	mail := &atomMail{
		atom:  atom.aIns,
		fn:    fnInfo,
		arg:   arg,
		reply: make(chan *replyWrap),
		sent:  time.Now(),
		next:  nil,
	}
	if ok := atom.push(mail); !ok {
		return nil, ErrAtomHalt
	}

	wait := <-mail.reply
	return wait.resp, wait.err
}

func (a *atomType) close(name string) error {
	atom, has := a.atoms[name]
	if !has {
		return ErrAtomNameNotExists
	}
	mail := &atomMail{
		atom:  atom.aIns,
		reply: make(chan *replyWrap),
		halt:  true,
		sent:  time.Now(),
		next:  nil,
	}
	if ok := atom.push(mail); !ok {
		return ErrAtomHalt
	}

	wait := <-mail.reply
	return wait.err
}

// Atom调用描述
// Atom Call Description

type CallFn func(atom Atom, in proto.Message) (proto.Message, error)
type CallableNewFn func(CosmosNode, string) Callable
type CallArgDecode func(buf []byte) (proto.Message, error)
type CallReplyDecode func(buf []byte) (proto.Message, error)

type CallDesc struct {
	Name     string
	Func     CallFn
	ArgDec   CallArgDecode
	ReplyDec CallReplyDecode
}

type AtomTypeDesc struct {
	Name        string
	NewCallable CallableNewFn
	Calls       []CallDesc
}
