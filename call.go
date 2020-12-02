package atomos

import (
	"errors"
	"fmt"
	"log"
	"reflect"
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
	LogLevelDebug = "[DEBUG]"
	LogLevelInfo  = "[INFO] "
	LogLevelWarn  = "<WARN> "
	LogLevelError = "<ERROR>"
)

// Atom Controller
// 原子控制器

type AtomLog struct {
	cos   *Cosmos
	atom  *atomWrap
	logId uint64
}

func (l *AtomLog) init(c *Cosmos, a *atomWrap) {
	l.cos = c
	l.atom = a
}

func (l *AtomLog) log(level LogLevel, format string, args ...interface{}) {
	// Get key & set key
	l.logId += 1
	aType := l.atom.aType
	aName := l.atom.aName
	ts := time.Now().Format("2006-01-02 15:04:05.000000")
	msg := fmt.Sprintf(format, args...)
	key := []byte(fmt.Sprintf("logAtom:%s:%s:%v", aType, aName, l.logId))
	value := fmt.Sprintf("%s (%s:%s) %s %s", ts, aType, aName, level, msg)
	//print(key)
	fmt.Println(value)
	if err := l.cos.ds.db.Put(key, []byte(value), nil); err != nil {
		log.Println(err)
	}
}

func (l *AtomLog) Debug(format string, args ...interface{}) {
	l.log(LogLevelDebug, format, args...)
}

func (l *AtomLog) Info(format string, args ...interface{}) {
	l.log(LogLevelInfo, format, args...)
}

func (l *AtomLog) Warn(format string, args ...interface{}) {
	l.log(LogLevelWarn, format, args...)
}

func (l *AtomLog) Error(format string, args ...interface{}) {
	l.log(LogLevelError, format, args...)
}

type AtomTask struct {
	cos   *Cosmos
	atom  *atomWrap
	mu    sync.Mutex
	id    uint64
	tasks map[uint64]*taskWrapper
}

type taskWrapper struct {
	id   uint64
	mail *atomMail
}

func (t *AtomTask) init(c *Cosmos, a *atomWrap) {
	t.cos = c
	t.atom = a
	t.tasks = map[uint64]*taskWrapper{}
}

func (t *AtomTask) addTask(fn interface{}, msg proto.Message, d *time.Duration) (id uint64, err error) {
	id = 0
	// Type check
	fnV := reflect.ValueOf(fn)
	fnT := reflect.TypeOf(fn)
	if fnV.Kind() != reflect.Func {
		err = errors.New("")
		return
	}
	if fnT.NumIn() < 1 {
		err = errors.New("")
		return
	}
	argT := fnT.In(0)
	if argT.Kind() != reflect.Ptr {
		err = errors.New("")
		return
	}

	msgV := reflect.ValueOf(msg)
	msgT := reflect.TypeOf(msg)
	if msgV.Kind() != reflect.Ptr {
		err = errors.New("")
		return
	}

	if argT.String() != msgT.String() {
		err = errors.New("")
		return
	}

	// Schedule and execute
	// Gen next id
	t.mu.Lock()
	if t.atom.aState == Halt {
		t.mu.Unlock()
		err = ErrAtomHalt
		return
	}
	for {
		t.id += 1
		_, has := t.tasks[t.id]
		if !has {
			break
		}
		log.Println("timer")
	}
	id = t.id
	t.mu.Unlock()

	// Construct task
	mail := &atomMail{
		typ:    MailTypeTask,
		to:     t.atom.aIns,
		taskId: id,
		taskFn: fnV,
		arg:    proto.Clone(msg),
		sent:   time.Now(),
	}
	wrap := &taskWrapper{
		id:   id,
		mail: mail,
	}
	t.tasks[id] = wrap
	if d == nil {
		t.atom.pushTail(mail)
	} else {
		time.AfterFunc(*d, func() {
			t.mu.Lock()
			w, has := t.tasks[id]
			t.mu.Unlock()
			if has {
				t.atom.pushHead(w.mail)
			}
		})
	}
	return id, nil
}

func (t *AtomTask) delTask(id uint64) *taskWrapper {
	t.mu.Lock()
	w, has := t.tasks[id]
	if !has {
		t.mu.Unlock()
		return nil
	}
	exec := w.mail.executing
	if exec {
		t.mu.Unlock()
		return nil
	}
	delete(t.tasks, id)
	t.mu.Unlock()
	return w
}

func (t *AtomTask) Add(fn interface{}, msg proto.Message) (uint64, error) {
	return t.addTask(fn, msg, nil)
}

func (t *AtomTask) After(d time.Duration, fn interface{}, msg proto.Message) (uint64, error) {
	return t.addTask(fn, msg, &d)
}

func (t *AtomTask) Has(id uint64) (bool, proto.Message) {
	w, has := t.tasks[id]
	if !has {
		return false, nil
	}
	if w.mail.executing {
		return false, nil
	}
	return true, w.mail.arg
}

func (t *AtomTask) Del(id uint64) (bool, proto.Message) {
	w := t.delTask(id)
	if w == nil {
		return false, nil
	}
	return true, w.mail.arg
}

func (a *atomWrap) Kill(from Id) error {
	mail := &atomMail{
		typ:  MailTypeClose,
		to:   a.aIns,
		sent: time.Now(),
	}
	if ok := a.pushHead(mail); !ok {
		return ErrAtomHalt
	}
	return nil
}

type atomWrap struct {
	cosmos *Cosmos    // Cosmos reference
	mu     sync.Mutex // For atomic operation
	aType  string     // Type of to
	aName  string     // Name of to
	aIns   Atom       // Instance of to
	aState atomState  // State(Halt/Waiting/Busy)
	// Mailbox, to receive messages
	mailsCond *sync.Cond
	mailsHead *atomMail
	mailsTail *atomMail
	mailsNum  uint
	// Data: AtomData & Log
	data *AtomData
	// Performance: Memory
	// Performance: Time
	elapses     map[string]*elapse
	procedureAt time.Time
	procedureFn string
	spawnAt     time.Time
	// Log
	log  AtomLog
	task AtomTask
}

func (a *atomWrap) Cosmos() CosmosNode {
	return a.cosmos
}

func (a *atomWrap) Type() string {
	return a.aType
}

func (a *atomWrap) Name() string {
	return a.aName
}

func (a *atomWrap) SelfCosmos() *Cosmos {
	return a.cosmos
}

func (a *atomWrap) Halt() {
	mail := &atomMail{
		typ:  MailTypeClose,
		to:   a.aIns,
		sent: time.Now(),
	}
	if ok := a.pushHead(mail); !ok {
		log.Println("not ok") // todo
	}
}

func (a *atomWrap) Log() *AtomLog {
	return &a.log
}

func (a *atomWrap) Task() *AtomTask {
	return &a.task
}

// Looping to runs
// 让原子循环运行起来

// todo executing
func (a *atomWrap) loop() {
	a.aState = Waiting
	a.spawnAt = time.Now()
	for {
		if a.mailsNum == 0 {
			a.mailsCond.L.Lock()
			a.mailsCond.Wait()
			a.mailsCond.L.Unlock()
		}
		m := a.popHead(true)
		// Check exit
		if m.typ == MailTypeClose {
			a.mu.Lock()
			// Elapsed time: procedureAt
			a.aState = Busy
			// Clean up remaining mails
			for m := a.popHead(false); m != nil; m = a.popHead(false) {
				// todo check really has remaining mails
				if m.reply != nil {
					m.reply <- &replyWrap{
						resp: nil,
						err:  ErrAtomHalt,
					}
				}
			}
			// Clean up remaining timers
			remainTasks := map[uint64]proto.Message{}
			for id, t := range a.task.tasks {
				remainTasks[id] = t.mail.arg
				delete(a.task.tasks, id)
			}
			a.data.Data = a.aIns.Close(remainTasks)
			// Save Data
			err := a.cosmos.ds.saveAtom(a.aType, a.aName, a.data)
			m.reply <- &replyWrap{
				resp: nil,
				err:  err,
			}
			break
		}
		//a.task.mu.Lock()
		//a.task.mu.Unlock()
		a.mu.Lock()
		// Elapsed time: procedureAt
		a.aState = Busy
		a.mu.Unlock()
		// Call or Task
		if m.typ == MailTypeCall {
			m.executing = true
			name := m.callFn.Name
			a.procedureAt, a.procedureFn = time.Now(), name
			c, has := a.elapses[name]
			if !has {
				c = &elapse{0, 0}
				a.elapses[name] = c
			}
			c.times += 1
			result, err := func() (proto.Message, error) {
				defer func() {
					if r := recover(); r != nil {
						a.log.Error("call recovered from panic, stack traceback:\n%s",
							string(debug.Stack()))
					}
				}()
				return m.callFn.Func(m.from, m.to, m.arg)
			}()

			func() {
				defer func() {
					if r := recover(); r != nil {
						a.log.Error("reply recovered from panic, stack traceback:\n%s",
							string(debug.Stack()))
					}
				}()
				m.reply <- &replyWrap{
					resp: result,
					err:  err,
				}
			}()

			d := time.Since(a.procedureAt)
			c.duration += d
			log.Printf("duration %v time %v", d, c.times)
		} else if m.typ == MailTypeTask {
			w := a.task.delTask(m.taskId)
			m.executing = true
			name := w.mail.taskFn.String()
			a.procedureAt, a.procedureFn = time.Now(), name // todo: procedure name
			c, has := a.elapses[name]
			if !has {
				c = &elapse{0, 0}
				a.elapses[name] = c
			}
			c.times += 1
			if w != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							a.log.Error(
								"call recovered from panic, stack traceback:\n%s",
								string(debug.Stack()))
						}
					}()
					in := []reflect.Value{
						reflect.ValueOf(w.mail.arg),
					}
					w.mail.taskFn.Call(in)
				}()
			}
			d := time.Since(a.procedureAt)
			c.duration += d
			log.Printf("duration %v time %v", d, c.times)
		}
		// Elapsed time: end todo: right lock
		a.mu.Lock()
		a.procedureAt, a.procedureFn = time.Time{}, ""
		a.aState = Waiting
		a.mu.Unlock()
	}
	a.mu.Unlock()
	a.aState = Halt
}

// Atom Mail Message
// 原子邮件信息

type MailType int

const (
	MailTypeCall  MailType = 1
	MailTypeTask  MailType = 2
	MailTypeClose MailType = 3
)

type atomMail struct {
	typ  MailType
	from Id
	to   Atom
	// Call
	callFn *CallDesc
	// Task
	taskId uint64
	taskFn reflect.Value
	// Arg
	arg   proto.Message
	reply chan *replyWrap
	// State
	executing bool
	sent      time.Time
	next      *atomMail
}

func (a *atomWrap) pushTail(m *atomMail) bool {
	a.mu.Lock()
	if a.aState == Halt {
		a.mu.Unlock()
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
	a.mu.Unlock()
	return true
}

func (a *atomWrap) pushHead(m *atomMail) bool {
	a.mu.Lock()
	if a.aState == Halt {
		a.mu.Unlock()
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
	a.mu.Unlock()
	return true
}

func (a *atomWrap) popHead(lock bool) *atomMail {
	if lock {
		a.mu.Lock()
	}
	if a.mailsHead == nil {
		if lock {
			a.mu.Unlock()
		}
		return nil
	}
	a.mailsNum -= 1
	if a.mailsHead == a.mailsTail {
		m := a.mailsHead
		a.mailsHead = nil
		a.mailsTail = nil
		m.next = nil
		if lock {
			a.mu.Unlock()
		}
		return m
	} else {
		m := a.mailsHead
		a.mailsHead = m.next
		m.next = nil
		if lock {
			a.mu.Unlock()
		}
		return m
	}
}

func (a *atomWrap) popId(id uint64) *atomMail {
	a.mu.Lock()
	mail := a.mailsHead
	if mail == nil {
		a.mu.Unlock()
		return nil
	}
	for ; mail != nil; mail = mail.next {
		if mail.taskId == id {
			a.mu.Unlock()
			return mail
		}
	}
	a.mu.Unlock()
	return nil
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
		mu:        sync.Mutex{},
		aType:     a.name,
		aName:     name,
		aIns:      atom,
		aState:    Halt,
		mailsHead: nil,
		mailsTail: nil,
		mailsNum:  0,
		elapses:   map[string]*elapse{},
		spawnAt:   time.Now(),
	}
	aw.log.init(c, aw)
	aw.task.init(c, aw)
	aw.mailsCond = sync.NewCond(&aw.mu)
	aw.data, err = c.ds.loadAtom(a.name, name)
	if err != nil {
		return nil, err
	}
	err = aw.aIns.Spawn(aw, aw.data.Data)
	if err != nil {
		return nil, err
	}
	a.atoms[name] = aw
	return aw, nil
}

func (a *atomType) has(name string) bool {
	_, has := a.atoms[name]
	return has
}

func (a *atomType) call(from Id, name, call string, arg proto.Message) (reply proto.Message, err error) {
	atom, has := a.atoms[name]
	if !has {
		return nil, ErrAtomNameNotExists
	}
	fnInfo, has := a.calls[call]
	if !has {
		return nil, ErrAtomCallNotExists
	}
	mail := &atomMail{
		typ:    MailTypeCall,
		from:   from,
		to:     atom.aIns,
		callFn: fnInfo,
		arg:    proto.Clone(arg),
		reply:  make(chan *replyWrap),
		sent:   time.Now(),
	}
	if ok := atom.pushTail(mail); !ok {
		return nil, ErrAtomHalt
	}

	wait := <-mail.reply
	return wait.resp, wait.err
}

func (a *atomType) close(from Id, name string) error {
	atom, has := a.atoms[name]
	if !has {
		return ErrAtomNameNotExists
	}
	mail := &atomMail{
		typ:   MailTypeClose,
		from:  from,
		to:    atom.aIns,
		reply: make(chan *replyWrap),
		sent:  time.Now(),
	}
	if ok := atom.pushHead(mail); !ok {
		return ErrAtomHalt
	}

	wait := <-mail.reply
	return wait.err
}

// Atom调用描述
// Atom Call Description

type CallFn func(from Id, to Atom, in proto.Message) (proto.Message, error)
type IdConstructFn func(CosmosNode, string) Id
type CallArgDecode func(buf []byte) (proto.Message, error)
type CallReplyDecode func(buf []byte) (proto.Message, error)

type CallDesc struct {
	Name     string
	Func     CallFn
	ArgDec   CallArgDecode
	ReplyDec CallReplyDecode
}

type AtomTypeDesc struct {
	Name  string
	NewId IdConstructFn
	Calls []CallDesc
}
