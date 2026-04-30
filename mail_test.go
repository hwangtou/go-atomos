package atomos

import (
	"google.golang.org/protobuf/proto"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test

type testMailHandler struct {
	t    *testing.T
	mb   *mailBox
	done chan bool

	sendNum    int
	receiveNum int
	mailPool   sync.Pool
	hash       *UtilStringHashGenerator

	testType int
	testInt  int64
	testMap  map[int][]bool
}

type testMail struct {
	i        int
	str      string
	waitCh   chan struct{}
	duration time.Duration
}

func (m *testMail) sendReply(reply proto.Message, err *Error) {
	m.waitCh <- struct{}{}
}

const (
	testMailHandlerTestTypeSequence1Million = 1
	testMailHandlerTestTypeConcurrent100K   = 2
	testMailHandlerTestTypeClose            = 3
	testMailHandlerTestTypeRunningPanic     = 4
	testMailHandlerTestTypeCloseReturnsErr  = 5
	testMailHandlerTestTypeCloseRemainMails = 6
	testMailHandlerTestTypeMailsOrder       = 7

	testMailMillion = 1000000
	testMail100K    = 100000
)

var cpuNum = runtime.NumCPU()

func (h *testMailHandler) mailboxOnStartUp(fn func() *Error) *Error {
	if fn != nil {
		return fn()
	}
	return nil
}

func (h *testMailHandler) mailboxOnReceive(mail *mail) {
	validateRunning(h, true)
	switch h.testType {
	case testMailHandlerTestTypeSequence1Million:
		str := mail.content.(*testMail).str
		strList := strings.Split(str, ":")
		if len(strList) != 3 {
			h.t.Errorf("mailboxOnReceive: Invalid mail. str=(%s)", str)
			return
		}

		// id:randStr:hash
		id, er := strconv.ParseInt(strList[0], 10, 64)
		if er != nil {
			h.t.Errorf("mailboxOnReceive: Invalid mail. str=(%s)", str)
			return
		}
		if id != h.testInt+1 {
			h.t.Errorf("mailboxOnReceive: Invalid mail. id=(%d) testInt=(%d)", id, h.testInt)
			return
		}
		h.testInt = id

		hash, err := h.hash.Gen(strList[0] + ":" + strList[1])
		if err != nil {
			h.t.Errorf("mailboxOnReceive: Gen failed. err=(%v)", err.AddStack(nil))
			return
		}
		if hash != strList[2] {
			h.t.Errorf("mailboxOnReceive: Invalid mail. hash=(%s) strList[2]=(%s)", hash, strList[2])
			return
		}

		if id == testMailMillion-1 {
			h.testType = 0
			h.testInt = 0
			h.testMap = nil
			h.done <- true
		}

	case testMailHandlerTestTypeConcurrent100K:
		str := mail.content.(*testMail).str
		strList := strings.Split(str, ":")
		if len(strList) != 4 {
			h.t.Errorf("mailboxOnReceive: Invalid mail. str=(%s)", str)
			return
		}

		// cpu:id:randStr:hash
		cpu, er := strconv.ParseInt(strList[0], 10, 64)
		if er != nil {
			h.t.Errorf("mailboxOnReceive: Invalid mail. str=(%s)", str)
			return
		}
		id, er := strconv.ParseInt(strList[1], 10, 64)
		if er != nil {
			h.t.Errorf("mailboxOnReceive: Invalid mail. str=(%s)", str)
			return
		}
		if id < 0 || id >= testMail100K {
			h.t.Errorf("mailboxOnReceive: Invalid mail. id=(%d)", id)
			return
		}
		hash, err := h.hash.Gen(strList[0] + ":" + strList[1] + ":" + strList[2])
		if err != nil {
			h.t.Errorf("mailboxOnReceive: Gen failed. err=(%v)", err.AddStack(nil))
			return
		}
		if hash != strList[3] {
			h.t.Errorf("mailboxOnReceive: Invalid mail. hash=(%s) strList[3]=(%s)", hash, strList[3])
			return
		}
		if h.testMap[int(cpu)][id] {
			h.t.Errorf("mailboxOnReceive: Invalid mail. cpu=(%d) id=(%d)", cpu, id)
			return
		}
		h.testInt += 1
		h.testMap[int(cpu)][id] = true

		if h.testInt == testMail100K*int64(cpuNum) {
			for _, boolList := range h.testMap {
				for i, b := range boolList {
					if !b {
						h.t.Errorf("mailboxOnReceive: Invalid mail. cpu=(%d),i=(%v)", cpu, i)
						continue
					}
				}
			}
			h.testType = 0
			h.testInt = 0
			h.testMap = nil
			h.done <- true
		}

	case testMailHandlerTestTypeRunningPanic:
		h.testType = 0
		h.testInt = 0
		h.testMap = nil
		defer func() {
			h.done <- true
		}()
		panic("Test: Panic")

	case testMailHandlerTestTypeCloseRemainMails:
		if mail.content.(*testMail).i != 0 {
			h.t.Errorf("mailboxOnReceive: Invalid test. mail=(%v)", mail)
			return
		}
		if d := mail.content.(*testMail).duration; d > 0 {
			<-time.After(d)
		}

	case testMailHandlerTestTypeMailsOrder:
		<-time.After(time.Millisecond * 100)

	default:
		h.t.Errorf("mailboxOnReceive: Invalid test type. testType=(%d)", h.testType)
	}
	validateRunning(h, true)
}

func (h *testMailHandler) mailboxOnStop(killMail, remainMails *mail, num uint32) *Error {
	validateRunning(h, false)
	switch h.testType {
	case testMailHandlerTestTypeClose:
		if killMail == nil || killMail.action != MailActionExit {
			h.t.Errorf("mailboxOnStop: Invalid test. killMail=(%v)", killMail)
			return nil
		}
		if killMail.content.(*testMail).str != "exit" {
			h.t.Errorf("mailboxOnStop: Invalid test. killMail.content=(%v)", killMail.content)
			return nil
		}
		if remainMails != nil || num != 0 {
			h.t.Errorf("mailboxOnStop: Invalid test. remainMails=(%v) num=(%d)", remainMails, num)
			return nil
		}
		h.done <- true

	case testMailHandlerTestTypeCloseReturnsErr:
		h.done <- true
		return NewErrorf(ErrFrameworkIncorrectUsage, "Close error.").AddStack(nil)

	case testMailHandlerTestTypeCloseRemainMails:
		if killMail == nil || killMail.action != MailActionExit || killMail.content.(*testMail).i != -1 {
			h.t.Errorf("mailboxOnStop: Invalid test. killMail=(%v)", killMail)
			return nil
		}
		if remainMails == nil || num != 9 {
			h.t.Errorf("mailboxOnStop: Invalid test. remainMails=(%v) num=(%d)", remainMails, num)
			return nil
		}
		idx := 1
		for curMail := remainMails; curMail != nil; curMail = curMail.next {
			if curMail.content.(*testMail).i != idx {
				h.t.Errorf("mailboxOnStop: Invalid test. curMail=(%v)", curMail)
				return nil
			}
			idx += 1
		}
		if idx != 10 {
			h.t.Errorf("mailboxOnStop: Invalid test. idx=(%d)", idx)
			return nil
		}
		h.t.Logf("mailboxOnStop: Remain mails Done")
		h.done <- true

	case testMailHandlerTestTypeMailsOrder:

	default:
		h.t.Errorf("mailboxOnStop: Invalid test type. testType=(%d)", h.testType)
	}
	validateRunning(h, false)
	return nil
}

func TestMailboxSpawnAndExit(t *testing.T) {
	h := testMailHandler{t: t, done: make(chan bool, 1), mailPool: sync.Pool{New: func() any { return &mail{} }}, hash: NewUtilStringHashSHA256Generator()}
	ls := &loggingService{}
	tt := &appLoggingForTest{t: t}
	if err := ls.init(tt); err != nil {
		t.Fatalf("Test: Init failed. err=(%v)", err.AddStack(nil))
		return
	}
	mb := newMailBox("testMailbox", &h, ls)
	h.mb = mb
	validateRunning(&h, false)

	// Start once.
	if err := mb.start(func() *Error {
		validateRunning(&h, true)
		return nil
	}); err != nil {
		t.Fatalf("Test: Start failed. err=(%v)", err.AddStack(nil))
	}
	t.Log("Test: Start once Done")

	// Start twice.
	if err := mb.start(func() *Error {
		validateRunning(&h, true)
		return nil
	}); err == nil {
		t.Fatalf("Test: Start once should fail.")
	} else if err.Code != ErrMailboxIsRunning {
		t.Fatalf("Test: Start once failed. err=(%v)", err.AddStack(nil))
	}
	// Exit.
	h.testType = testMailHandlerTestTypeClose
	km := h.mailPool.Get().(*mail)
	km.action = MailActionExit
	km.content = &testMail{str: "exit", waitCh: make(chan struct{}, 1)}
	mb.pushTail(km)
	<-h.done
	<-km.content.(*testMail).waitCh
	// Check mailbox.
	if mb.name != "testMailbox" {
		t.Fatalf("Test: Invalid mailbox name. name=(%s)", mb.name)
	}
	validateRunning(&h, false)
	if mb.handler != &h {
		t.Fatalf("Test: Invalid mailbox handler.")
	}
	if mb.head != nil || mb.tail != nil || mb.num != 0 {
		t.Fatalf("Test: Invalid mailbox mail.")
	}
	if mb.logging != ls {
		t.Fatalf("Test: Invalid mailbox logging.")
	}
	t.Log("Test: Exit Done")

	// Start again.
	if err := mb.start(func() *Error {
		validateRunning(&h, true)
		return nil
	}); err != nil {
		t.Fatalf("Test: Start failed. err=(%v)", err.AddStack(nil))
	}
	// Exit.
	h.testType = testMailHandlerTestTypeClose
	km = h.mailPool.Get().(*mail)
	km.action = MailActionExit
	km.content = &testMail{str: "exit", waitCh: make(chan struct{}, 1)}
	mb.pushTail(km)
	<-h.done
	<-km.content.(*testMail).waitCh
	// Check mailbox.
	if mb.name != "testMailbox" {
		t.Fatalf("Test: Invalid mailbox name. name=(%s)", mb.name)
	}
	validateRunning(&h, false)
	if mb.handler != &h {
		t.Fatalf("Test: Invalid mailbox handler.")
	}
	if mb.head != nil || mb.tail != nil || mb.num != 0 {
		t.Fatalf("Test: Invalid mailbox mail.")
	}
	if mb.logging != ls {
		t.Fatalf("Test: Invalid mailbox logging.")
	}
	t.Log("Test: Start again Done")

	// Start returns error.
	tt.ignoreError = true
	if err := mb.start(func() *Error {
		return NewErrorf(ErrFrameworkIncorrectUsage, "Spawn error.").AddStack(nil)
	}); err == nil {
		t.Fatalf("Test: Start should fail.")
	} else if err.Code != ErrFrameworkIncorrectUsage {
		t.Fatalf("Test: Start failed. err=(%v)", err.AddStack(nil))
	} else if err.Message != "Spawn error." {
		t.Fatalf("Test: Start failed. err=(%v)", err.AddStack(nil))
	}
	<-time.After(time.Millisecond * 1) // Wait for error log. May be not accurate.
	tt.ignoreError = false
	validateRunning(&h, false)
	// Check mailbox.
	if mb.name != "testMailbox" {
		t.Fatalf("Test: Invalid mailbox name. name=(%s)", mb.name)
	}
	validateRunning(&h, false)
	if mb.handler != &h {
		t.Fatalf("Test: Invalid mailbox handler.")
	}
	if mb.head != nil || mb.tail != nil || mb.num != 0 {
		t.Fatalf("Test: Invalid mailbox mail.")
	}
	if mb.logging != ls {
		t.Fatalf("Test: Invalid mailbox logging.")
	}
	t.Log("Test: Start returns error Done")

	// Running panic.
	if err := mb.start(func() *Error {
		validateRunning(&h, true)
		return nil
	}); err != nil {
		t.Fatalf("Test: Start failed. err=(%v)", err.AddStack(nil))
	}
	h.testType = testMailHandlerTestTypeRunningPanic
	km = h.mailPool.Get().(*mail)
	km.action = MailActionRun
	km.content = &testMail{}
	tt.ignoreError = true
	mb.pushTail(km)
	<-h.done
	<-time.After(time.Millisecond * 1) // Wait for error log. May be not accurate.
	tt.ignoreError = false
	validateRunning(&h, true)
	t.Log("Test: Running panic Done")

	// Exit with errors.
	h.testType = testMailHandlerTestTypeCloseReturnsErr
	km = h.mailPool.Get().(*mail)
	km.action = MailActionExit
	km.content = &testMail{str: "exit", waitCh: make(chan struct{}, 1)}
	tt.ignoreError = true
	mb.pushTail(km)
	<-h.done
	<-km.content.(*testMail).waitCh
	<-time.After(time.Millisecond * 1) // Wait for error log. May be not accurate.
	tt.ignoreError = false
	// Check mailbox.
	if mb.name != "testMailbox" {
		t.Fatalf("Test: Invalid mailbox name. name=(%s)", mb.name)
	}
	validateRunning(&h, false)
	if mb.handler != &h {
		t.Fatalf("Test: Invalid mailbox handler.")
	}
	if mb.head != nil || mb.tail != nil || mb.num != 0 {
		t.Fatalf("Test: Invalid mailbox mail.")
	}
	if mb.logging != ls {
		t.Fatalf("Test: Invalid mailbox logging.")
	}
	t.Log("Test: Start again Done")
}

func TestMailboxExitWithRemainMails(t *testing.T) {
	h := testMailHandler{t: t, done: make(chan bool, 1), mailPool: sync.Pool{New: func() any { return &mail{} }}, hash: NewUtilStringHashSHA256Generator()}
	ls := &loggingService{}
	tt := &appLoggingForTest{t: t}
	if err := ls.init(tt); err != nil {
		t.Fatalf("Test: Init failed. err=(%v)", err.AddStack(nil))
		return
	}
	mb := newMailBox("testMailbox", &h, ls)
	h.mb = mb
	validateRunning(&h, false)

	if err := mb.start(func() *Error {
		validateRunning(&h, true)
		return nil
	}); err != nil {
		t.Fatalf("Test: Start failed. err=(%v)", err.AddStack(nil))
		return
	}
	validateRunning(&h, true)

	// Test close with remain mails.
	h.testType = testMailHandlerTestTypeCloseRemainMails
	for i := 0; i < 10; i++ {
		m := h.mailPool.Get().(*mail)
		m.next = nil
		m.id = DefaultMailID
		m.action = MailActionRun
		if i == 0 {
			m.content = &testMail{i: i, duration: time.Millisecond * 10}
		} else {
			m.content = &testMail{i: i}
		}
		mb.pushTail(m)
	}
	<-time.After(time.Millisecond * 1) // Wait for mails.
	km := h.mailPool.Get().(*mail)
	km.next = nil
	km.id = DefaultMailID
	km.action = MailActionExit
	km.content = &testMail{i: -1}
	mb.pushHead(km)
	<-h.done
	validateRunning(&h, false)
	t.Log("Test: Close with remain mails Done")
}

func TestMailboxMailsPop(t *testing.T) {
	h := testMailHandler{t: t, done: make(chan bool, 1), mailPool: sync.Pool{New: func() any { return &mail{} }}, hash: NewUtilStringHashSHA256Generator()}
	ls := &loggingService{}
	if err := ls.init(&appLoggingForTest{t: t}); err != nil {
		t.Fatalf("Test: Init failed. err=(%v)", err.AddStack(nil))
		return
	}
	mb := newMailBox("testMailbox", &h, ls)
	h.mb = mb
	validateRunning(&h, false)

	if err := mb.start(func() *Error {
		validateRunning(&h, true)
		return nil
	}); err != nil {
		t.Fatalf("Test: Start failed. err=(%v)", err.AddStack(nil))
		return
	}
	validateRunning(&h, true)

	// Insert 10 mails.
	h.testType = testMailHandlerTestTypeMailsOrder
	for i := 0; i < 10; i++ {
		m := h.mailPool.Get().(*mail)
		m.next = nil
		m.id = uint64(i)
		m.action = MailActionRun
		m.content = &testMail{i: i}
		mb.pushTail(m)
	}
	<-time.After(time.Millisecond * 1) // Wait for mails. 0 has been popped.

	// Check mails. Ignore thread-safe.
	check := func(list []int) bool {
		if mb.num != uint32(len(list)) {
			return false
		}
		num := 0
		for m := mb.head; m != nil; m = m.next {
			if m.content.(*testMail).i != list[num] {
				return false
			}
			if getByID := mb.getByID(uint64(list[num])); m != getByID {
				return false
			}
			num += 1
		}
		return num == len(list)
	}

	// Order: 1, 2, 3, 4, 5, 6, 7, 8, 9
	if !check([]int{1, 2, 3, 4, 5, 6, 7, 8, 9}) {
		t.Fatalf("Test: Invalid mails.")
	}

	// Pop #5.
	m5 := mb.popByID(5)
	if m5 == nil || m5.content.(*testMail).i != 5 {
		t.Fatalf("Test: Invalid pop mail.")
	}
	if !check([]int{1, 2, 3, 4, 6, 7, 8, 9}) {
		t.Fatalf("Test: Invalid mails.")
	}
	// Pop #5 again.
	if mb.removeMail(m5) {
		t.Fatalf("Test: Invalid remove mail.")
	}
	if !check([]int{1, 2, 3, 4, 6, 7, 8, 9}) {
		t.Fatalf("Test: Invalid mails.")
	}

	// Get #6 and pop.
	m6 := mb.getByID(6)
	if m6 == nil || m6.content.(*testMail).i != 6 {
		t.Fatalf("Test: Invalid get mail.")
	}
	if !mb.removeMail(m6) {
		t.Fatalf("Test: Invalid remove mail.")
	}
	if !check([]int{1, 2, 3, 4, 7, 8, 9}) {
		t.Fatalf("Test: Invalid mails.")
	}

	// Push #5 to head.
	mb.pushHead(m5)
	if !check([]int{5, 1, 2, 3, 4, 7, 8, 9}) {
		t.Fatalf("Test: Invalid mails.")
	}

	// Push #6 to tail.
	mb.pushTail(m6)
	if !check([]int{5, 1, 2, 3, 4, 7, 8, 9, 6}) {
		t.Fatalf("Test: Invalid mails.")
	}

	// Pop all mails.
	mailHead, num := mb.popAll()
	if mailHead == nil || num != 9 {
		t.Fatalf("Test: Invalid pop all mails.")
	}
	if !check([]int{}) {
		t.Fatalf("Test: Invalid mails.")
	}
	idx := 0
	should := []int{5, 1, 2, 3, 4, 7, 8, 9, 6}
	for m := mailHead; m != nil; m = m.next {
		if m.content.(*testMail).i != should[idx] {
			t.Fatalf("Test: Invalid mails.")
		}
		idx += 1
	}
	if idx != 9 {
		t.Fatalf("Test: Invalid mails.")
	}

	for m := mailHead; m != nil; m = m.next {
		h.mailPool.Put(m)
	}
	t.Log("Test: Mails pop Done")
}

func TestMailboxCommon(t *testing.T) {
	h := testMailHandler{t: t, done: make(chan bool, 1), mailPool: sync.Pool{New: func() any { return &mail{} }}, hash: NewUtilStringHashSHA256Generator()}
	ls := &loggingService{}
	if err := ls.init(&appLoggingForTest{t: t}); err != nil {
		t.Fatalf("Test: Init failed. err=(%v)", err.AddStack(nil))
		return
	}
	mb := newMailBox("testMailbox", &h, ls)
	h.mb = mb
	validateRunning(&h, false)

	if err := mb.start(func() *Error {
		validateRunning(&h, true)
		return nil
	}); err != nil {
		t.Fatalf("Test: Start failed. err=(%v)", err.AddStack(nil))
		return
	}
	validateRunning(&h, true)

	// Test sequence 1M.
	h.testType = testMailHandlerTestTypeSequence1Million
	h.testInt = -1
	randStrGen := NewUtilStringRandomStringGenerator()
	hashGen := NewUtilStringHashSHA256Generator()
	for i := 0; i < testMailMillion; i++ {
		str := strconv.FormatInt(int64(i), 10) + ":" + randStrGen.RandomString(100)
		hash, err := hashGen.Gen(str)
		if err != nil {
			t.Fatalf("Test: Gen failed. err=(%v)", err.AddStack(nil))
			return
		}
		str = str + ":" + hash
		testPushMail(&h, mb, str)
	}
	<-h.done
	validateRunning(&h, true)
	h.t.Logf("TestMailboxCommon: testMailHandlerTestTypeSequence1Million Done")

	// Test concurrent 100K.
	// Test correctness of concurrent processing, especially the mail pool.
	h.testType = testMailHandlerTestTypeConcurrent100K
	h.testInt = 0
	h.testMap = make(map[int][]bool, cpuNum)
	for c := 0; c < cpuNum; c++ {
		h.testMap[c] = make([]bool, testMail100K)
	}
	for c := 0; c < cpuNum; c++ {
		go func(c int) {
			randStrGen := NewUtilStringRandomStringGenerator()
			hashGen := NewUtilStringHashSHA256Generator()
			for i := 0; i < testMail100K; i++ {
				str := strconv.FormatInt(int64(c), 10) + ":" +
					strconv.FormatInt(int64(i), 10) + ":" +
					randStrGen.RandomString(100)
				hash, err := hashGen.Gen(str)
				if err != nil {
					t.Fatalf("Test: Gen failed. err=(%v)", err.AddStack(nil))
					return
				}
				str = str + ":" + hash
				testPushMail(&h, mb, str)
			}
		}(c)
	}
	<-h.done
	validateRunning(&h, true)
	h.t.Logf("TestMailboxCommon: testMailHandlerTestTypeConcurrent100K Done")

	// Test close.
	h.testType = testMailHandlerTestTypeClose
	km := h.mailPool.Get().(*mail)
	km.next = nil
	km.id = DefaultMailID
	km.action = MailActionExit
	km.content = &testMail{str: "exit", waitCh: make(chan struct{}, 1)}
	mb.pushHead(km)
	<-h.done
	<-km.content.(*testMail).waitCh
	validateRunning(&h, false)
	h.t.Logf("TestMailboxCommon: testMailHandlerTestTypeClose Done")
}

func testPushMail(h *testMailHandler, mb *mailBox, str string) {
	m := h.mailPool.Get().(*mail)
	m.next = nil
	m.id = DefaultMailID
	m.action = MailActionRun
	m.content = &testMail{str: str}
	mb.pushTail(m)
}

func validateRunning(h *testMailHandler, shouldRun bool) {
	if h.mb.isRunning() != shouldRun {
		h.t.Fatalf("validateRunning: Invalid running. shouldRun=(%v)", shouldRun)
	}
}

// Benchmark

type benchmarkMailHandler struct {
	b    *testing.B
	done chan bool

	sendNum    int
	receiveNum int
	mailPool   sync.Pool
}

func (h *benchmarkMailHandler) mailboxOnStartUp(fn func() *Error) *Error {
	return nil
}

func (h *benchmarkMailHandler) mailboxOnReceive(mail *mail) {
	h.receiveNum += 1
	if h.sendNum-h.receiveNum == 0 {
		h.done <- true
		h.b.Logf("Send=%d Recv=%d\n", h.sendNum, h.receiveNum)
	}
	h.mailPool.Put(mail)
}

func (h *benchmarkMailHandler) mailboxOnStop(killMail, remainMails *mail, num uint32) *Error {
	if killMail == nil || killMail.action != MailActionExit {
		h.b.Fatal("Benchmark mailbox has received invalid stop-mail.")
	}
	h.mailPool.Put(killMail)
	if remainMails != nil {
		h.b.Fatal("Benchmark mailbox has received invalid remain-mails.")
	}
	return nil
}

func benchmarkMailBox(b *testing.B, waiters int) {
	h := benchmarkMailHandler{b: b, done: make(chan bool, 1), mailPool: sync.Pool{New: func() any { return &mail{} }}}
	ls := &loggingService{}
	if err := ls.init(&appLoggingForBenchmark{b: b}); err != nil {
		b.Fatalf("Benchmark: Init failed. err=(%v)", err.AddStack(nil))
		return
	}
	mb := newMailBox("benchmarkMailbox", &h, ls)
	if err := mb.start(nil); err != nil {
		b.Fatalf("Benchmark: Start failed. err=(%v)", err.AddStack(nil))
		return
	}

	km := h.mailPool.Get().(*mail)
	km.next = nil
	km.id = DefaultMailID
	km.action = MailActionExit
	km.content = nil
	defer mb.pushHead(km)

	for routine := 0; routine < waiters+1; routine++ {
		h.sendNum += b.N
		go func() {
			for i := 0; i < b.N; i++ {
				m := h.mailPool.Get().(*mail)
				m.next = nil
				m.id = DefaultMailID
				m.action = MailActionRun
				m.content = nil
				mb.pushTail(m)
			}
		}()
	}
	<-h.done
}

func BenchmarkMailbox1(b *testing.B) {
	benchmarkMailBox(b, 1)
}

func BenchmarkMailbox2(b *testing.B) {
	initTestFakeCosmosProcessBenchmark(b)
	benchmarkMailBox(b, 2)
}

func BenchmarkMailbox4(b *testing.B) {
	initTestFakeCosmosProcessBenchmark(b)
	benchmarkMailBox(b, 4)
}

func BenchmarkMailbox8(b *testing.B) {
	initTestFakeCosmosProcessBenchmark(b)
	benchmarkMailBox(b, 8)
}

func BenchmarkMailbox16(b *testing.B) {
	initTestFakeCosmosProcessBenchmark(b)
	benchmarkMailBox(b, 16)
}

func BenchmarkMailbox32(b *testing.B) {
	initTestFakeCosmosProcessBenchmark(b)
	benchmarkMailBox(b, 32)
}

func BenchmarkMailbox64(b *testing.B) {
	initTestFakeCosmosProcessBenchmark(b)
	benchmarkMailBox(b, 64)
}

func BenchmarkMailbox128(b *testing.B) {
	initTestFakeCosmosProcessBenchmark(b)
	benchmarkMailBox(b, 128)
}
