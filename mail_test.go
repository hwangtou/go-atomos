package atomos

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test for mailbox

func clearAllocMailDebugMap() {
	if !allocMailDebug.Load() {
		return
	}
	// Delete entries instead of reassigning the sync.Map: mailbox goroutines of
	// other tests may be calling LoadAndDelete on the map variable concurrently,
	// and reassigning it would race with those reads.
	allocMailDebugMap.Range(func(key, value any) bool {
		allocMailDebugMap.Delete(key)
		return true
	})
}

func getAllocMailDebugNum() int {
	if !allocMailDebug.Load() {
		return 0
	}
	num := 0
	allocMailDebugMap.Range(func(key, value any) bool {
		num++
		return true
	})
	return num
}

func getAllAllocMailDebugInfo() map[*mail]string {
	if !allocMailDebug.Load() {
		return nil
	}
	info := make(map[*mail]string)
	allocMailDebugMap.Range(func(key, value any) bool {
		m := key.(*mail)
		s := value.(string)
		info[m] = s
		return true
	})
	return info
}

// waitAllocMailDebugZero polls until the debug allocation map drains (or the
// timeout elapses and fails the test). Background goroutines (logging mailboxes
// of test fixtures) hold transient debug mails, so an instantaneous zero check
// is flaky; polling waits out the transients while still catching real leaks.
func waitAllocMailDebugZero(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if getAllocMailDebugNum() == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Memory leak detected, some mails are not released. remaining=(%d),info=(%v)",
				getAllocMailDebugNum(), getAllAllocMailDebugInfo())
		}
		time.Sleep(time.Millisecond)
	}
}

// Smoke Test for Mailbox Life Cycle
// It is a simple test to verify the mailbox life cycle.
// Also test for:
// #1 isRunning
// #2 start / startLoop / loop
// #3 pushTail
// #4 pushHead
// #5 mailboxOnReceive
// #6 mailboxOnStop
func TestMailbox_LifeCycle(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	var wg sync.WaitGroup
	h := &testMailboxHandler{
		t:  t,
		mb: nil,
		recv: func(mail *mail) {
			t.Log("TestMailbox_Smoke: Mail received.")
			wg.Done()
		},
		stop: func(killMail, remainMails *mail, num uint32) *Error {
			wg.Done()
			t.Log("TestMailbox_Smoke: Mail box stopped.")
			return nil
		},
	}
	h.mb = newMailBox("testMailbox", h, newTestLoggingAtomos(t))
	if err := h.mb.start(func() *Error {
		return nil
	}); err != nil {
		t.Fatalf("TestMailbox_Smoke: Start failed. err=(%v)", err.AddStack(nil))
	}
	// Wait loggingAtomos logs popped
	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	// Send mails
	count := 10
	wg.Add(count)
	for i := 0; i < count; i++ {
		m := allocMail()
		// NOTE: no per-iteration debug-map count assertion — the mailbox
		// consumes (and releases) pushed mails concurrently, so any exact or
		// bounded count here is inherently nondeterministic. Allocation
		// tracking is verified by waitAllocMailDebugZero at the end.
		initMail(m, DefaultMailID, nil)
		h.mb.pushTail(m)
	}
	wg.Wait()

	// Kill
	wg.Add(1)
	m := allocMail()
	initKillMail(m, DefaultMailID, nil, nil)
	h.mb.pushHead(m)
	if !h.mb.isRunning() {
		t.Fatal("TestMailbox_Smoke: Mailbox is not running after sending exit mail.")
	}
	wg.Wait()
	if h.mb.isRunning() {
		t.Fatal("TestMailbox_Smoke: Mailbox is still running after stopped.")
	}

	// Check memory leak: background log mails are transient, so poll until the
	// debug map drains instead of asserting a single instantaneous zero.
	waitAllocMailDebugZero(t, time.Second)

	<-time.After(time.Millisecond)
}

// Push 100 mails into mailbox, use a sentWait to wait all mails sent.
// After each mail sent, check the mailbox num is correct.
// Push 100 more mails to test push kill mails after all mails sent.
// #1 getNum
func TestMailBox_GetNum_ConsumeSlow(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	var sendWait, wg sync.WaitGroup
	var h *testMailboxHandler
	count := uint64(100)
	// killPushed is closed by the main goroutine after the kill mail has been
	// pushed, so that mail 0 in phase 2 blocks the mailbox until the kill mail
	// is at the head of the queue. This makes the stop-time mail count
	// deterministic and avoids t.Fatal being called from the mailbox goroutine
	// (FailNow -> runtime.Goexit would kill the mailbox loop and deadlock wg).
	killPushed := make(chan struct{})
	var phase atomic.Int32
	var errMu sync.Mutex
	var errs []string
	recordErr := func(format string, args ...any) {
		errMu.Lock()
		errs = append(errs, fmt.Sprintf(format, args...))
		errMu.Unlock()
	}
	h = &testMailboxHandler{
		t:  t,
		mb: nil,
		recv: func(mail *mail) {
			t.Log("TestMailBox_GetNum: Mail received.", mail)
			if n := mail.data.(uint64); n == 0 {
				t.Log("TestMailBox_GetNum: A waiting mail received, wait for all mails sent.")
				sendWait.Wait()
				if phase.Load() == 2 {
					// Phase 2: hold the mailbox until the kill mail is at the
					// head, so the stop-time remaining count is deterministic.
					<-killPushed
				}
			} else if num := int32(n) + int32(h.mb.getNum()); num < int32(count-2) || num > int32(count-1) {
				recordErr("TestMailBox_GetNum: GetNum returned wrong value during receiving mails. expect=(%d or %d) got=(%d)", n-1, n, h.mb.getNum())
			}
			wg.Done()
		},
		stop: func(killMail, remainMails *mail, num uint32) *Error {
			t.Log("TestMailBox_GetNum: Mail stopping.")
			if killMail == nil {
				recordErr("TestMailBox_GetNum: Stop received nil killMail.")
				return nil
			}
			if _, ok := killMail.data.(*mailExitCommand); !ok {
				recordErr("TestMailBox_GetNum: Stop received wrong killMail action. expect=(*mailExitCommand) got=(%T)", killMail.data)
			}
			if num != uint32(count-1) { // one mail is being processed
				recordErr("TestMailBox_GetNum: Stop received wrong num. expect=(%d) got=(%d)", count-1, num)
			}
			cur := 1
			for curMail := remainMails; curMail != nil; curMail = curMail.next {
				if curMail.data.(uint64) != uint64(cur) {
					recordErr("TestMailBox_GetNum: Remaining mail has wrong data. expect=(%d) got=(%d)", cur, curMail.data)
				}
				cur++
				t.Log("TestMailBox_GetNum: Remaining mail:", curMail)
				wg.Done()
			}
			return nil
		},
	}
	h.mb = newMailBox("testMailboxGetNum", h, newTestLoggingAtomos(t))
	if err := h.mb.start(func() *Error {
		return nil
	}); err != nil {
		t.Fatalf("TestMailBox_GetNum: Start failed. err=(%v)", err.AddStack(nil))
	}
	// Wait loggingAtomos logs popped
	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	wg.Add(int(count))
	sendWait.Add(int(count))
	for i := uint64(0); i < count; i++ {
		t.Log("TestMailBox_GetNum: Mail send.", i)
		m := allocMail()
		if getAllocMailDebugNum() == 0 {
			t.Fatal("TestMailBox_GetNum: Mail allocation tracking failed.", getAllocMailDebugNum())
		}
		initMail(m, DefaultMailID, i)
		h.mb.pushTail(m)
		sendWait.Done()
		if gap := int32(i+1) - int32(h.mb.getNum()); gap < 0 || gap > 1 {
			t.Fatalf("TestMailBox_GetNum: GetNum returned wrong value. expect=(%d or %d) got=(%d)", i, i+1, h.mb.getNum())
		}
	}
	wg.Wait()
	if h.mb.getNum() != 0 {
		t.Fatalf("TestMailBox_GetNum: GetNum returned wrong value after all mails received. expect=(0) got=(%d)", h.mb.getNum())
	}

	// Kill
	count = uint64(100)
	phase.Store(2)
	wg.Add(int(count))
	sendWait.Add(int(count))
	for i := uint64(0); i < count; i++ {
		m := allocMail()
		if getAllocMailDebugNum() == 0 {
			t.Fatal("TestMailBox_GetNum: Mail allocation tracking failed.", getAllocMailDebugNum())
		}
		initMail(m, DefaultMailID, i)
		h.mb.pushTail(m)
		sendWait.Done()
	}

	// kill mail
	m := allocMail()
	if getAllocMailDebugNum() == 0 {
		t.Fatal("TestMailBox_GetNum: Mail allocation tracking failed.", getAllocMailDebugNum())
	}
	initKillMail(m, DefaultMailID, nil, nil)
	h.mb.pushHead(m)
	close(killPushed)
	wg.Wait()

	// Fail in the main goroutine if any callback recorded an error.
	errMu.Lock()
	for _, e := range errs {
		t.Error(e)
	}
	errMu.Unlock()

	// Check memory leak
	<-time.After(time.Millisecond)
	if getAllocMailDebugNum() > 0 {
		t.Fatal("TestMailBox_GetNum: Memory leak detected, some mails are not released.", getAllocMailDebugNum())
	}

	<-time.After(time.Millisecond)
}

// Test for list correction in mailbox
// #1 getByID
// #2 pushHead
// #3 pushTail / Push
// #4 popByID
func TestMailBox_ListCorrection(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	mb := newMailBox("testMailbox", &testMailboxHandler{}, newTestLoggingAtomos(t))
	mb.running = true
	// No mail
	if mb.num != 0 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong for no mail. expect=(0) got=(%d)", mb.num)
	}
	if mb.head != nil || mb.tail != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong for no mail. expect=(nil) got=(head:%v tail:%v)", mb.head, mb.tail)
	}
	// Wait loggingAtomos logs popped
	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	// 1 mail case

	// Push one mail to head
	mail1Head := allocMail()
	initMail(mail1Head, 1, uint64(1))
	if !mb.pushHead(mail1Head) {
		t.Fatalf("TestMailBox_ListCorrection: PushHead failed for one mail.")
	}
	if mb.num != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong for one mail. expect=(1) got=(%d)", mb.num)
	}
	if mb.head != mail1Head || mb.tail != mail1Head {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong for one mail. expect=(%v/%v) got=(head:%v tail:%v)", mail1Head, mail1Head, mb.head, mb.tail)
	}
	if mb.head.next != nil || mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail next is wrong for one mail. expect=(nil) got=(head.next:%v tail.next:%v)", mb.head.next, mb.tail.next)
	}
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for one mail.")
	}
	// Get not found mail
	notFoundMail := mb.getByID(0)
	if notFoundMail != nil {
		t.Fatalf("TestMailBox_ListCorrection: GetByID returned wrong mail for not found. expect=(nil) got=(%v)", notFoundMail)
	}
	notFoundMail = mb.popByID(0)
	if notFoundMail != nil {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for not found. expect=(nil) got=(%v)", notFoundMail)
	}
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed after get/pop not found mail.")
	}
	// Get found mail and pop
	foundMail := mb.getByID(mail1Head.id)
	if foundMail != mail1Head {
		t.Fatalf("TestMailBox_ListCorrection: GetByID returned wrong mail for found. expect=(%v) got=(%v)", mail1Head, foundMail)
	}
	foundMail = mb.popByID(mail1Head.id)
	if foundMail != mail1Head {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for found. expect=(%v) got=(%v)", mail1Head, foundMail)
	}
	releaseMail(foundMail)
	if getAllocMailDebugNum() != 0 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed after pop found mail.")
	}
	if foundMail.id != mail1Head.id {
		t.Fatalf("TestMailBox_ListCorrection: Popped mail has wrong id. expect=(%d) got=(%d)", mail1Head.id, foundMail.id)
	}
	if foundMail.data != uint64(1) {
		t.Fatalf("TestMailBox_ListCorrection: Popped mail has wrong data. expect=(%d) got=(%d)", 1, foundMail.data)
	}
	if mb.num != 0 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong after pop mail. expect=(0) got=(%d)", mb.num)
	}
	if mb.head != nil || mb.tail != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong after pop mail. expect=(nil) got=(head:%v tail:%v)", mb.head, mb.tail)
	}
	t.Log("TestMailBox_ListCorrection: One mail push head case passed.")

	// Push one mail to tail
	mail1Tail := allocMail()
	initMail(mail1Tail, 2, uint64(2))
	if !mb.pushTail(mail1Tail) {
		t.Fatalf("TestMailBox_ListCorrection: PushTail failed for one mail.")
	}
	if mb.num != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong for one mail. expect=(1) got=(%d)", mb.num)
	}
	if mb.head != mail1Tail || mb.tail != mail1Tail {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong for one mail. expect=(%v/%v) got=(head:%v tail:%v)", mail1Tail, mail1Tail, mb.head, mb.tail)
	}
	if mb.head.next != nil || mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail next is wrong for one mail. expect=(nil) got=(head.next:%v tail.next:%v)", mb.head.next, mb.tail.next)
	}
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for one mail.")
	}
	// Get not found mail
	notFoundMail = mb.getByID(0)
	if notFoundMail != nil {
		t.Fatalf("TestMailBox_ListCorrection: GetByID returned wrong mail for not found. expect=(nil) got=(%v)", notFoundMail)
	}
	notFoundMail = mb.popByID(0)
	if notFoundMail != nil {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for not found. expect=(nil) got=(%v)", notFoundMail)
	}
	// Get found mail and pop
	foundMail = mb.getByID(mail1Tail.id)
	if foundMail != mail1Tail {
		t.Fatalf("TestMailBox_ListCorrection: GetByID returned wrong mail for found. expect=(%v) got=(%v)", mail1Tail, foundMail)
	}
	foundMail = mb.popByID(mail1Tail.id)
	if foundMail != mail1Tail {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for found. expect=(%v) got=(%v)", mail1Tail, foundMail)
	}
	releaseMail(mail1Tail)
	if getAllocMailDebugNum() != 0 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed after pop found mail.")
	}
	if foundMail.id != mail1Tail.id {
		t.Fatalf("TestMailBox_ListCorrection: Popped mail has wrong id. expect=(%d) got=(%d)", mail1Tail.id, foundMail.id)
	}
	if foundMail.data != uint64(2) {
		t.Fatalf("TestMailBox_ListCorrection: Popped mail has wrong data. expect=(%d) got=(%d)", 2, foundMail.data)
	}
	if mb.num != 0 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong after pop mail. expect=(0) got=(%d)", mb.num)
	}
	if mb.head != nil || mb.tail != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong after pop mail. expect=(nil) got=(head:%v tail:%v)", mb.head, mb.tail)
	}
	t.Log("TestMailBox_ListCorrection: One mail push tail case passed.")

	// Check no mail
	if mb.num != 0 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong for no mail before three mails. expect=(0) got=(%d)", mb.num)
	}
	if mb.head != nil || mb.tail != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong for no mail before three mails. expect=(nil) got=(head:%v tail:%v)", mb.head, mb.tail)
	}

	// Multiple mails case
	mail3Head1 := allocMail()
	initMail(mail3Head1, 11, uint64(11))
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}

	mail3Head2 := allocMail()
	initMail(mail3Head2, 12, uint64(12))
	if getAllocMailDebugNum() != 2 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}

	mail3Head3 := allocMail()
	initMail(mail3Head3, 13, uint64(13))
	if getAllocMailDebugNum() != 3 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}

	// Push first mail to head
	if !mb.pushHead(mail3Head1) {
		t.Fatalf("TestMailBox_ListCorrection: PushHead failed for three mails.")
	}
	if mb.num != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong for one mail. expect=(1) got=(%d)", mb.num)
	}
	if mb.head != mail3Head1 || mb.tail != mail3Head1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong for one mail. expect=(%v/%v) got=(head:%v tail:%v)", mail3Head1, mail3Head1, mb.head, mb.tail)
	}
	if mb.head.next != nil || mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail next is wrong for one mail. expect=(nil) got=(head.next:%v tail.next:%v)", mb.head.next, mb.tail.next)
	}
	if getAllocMailDebugNum() != 3 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}
	// Push second mail to head
	// mail order: mail3Head2 -> mail3Head1
	if !mb.pushHead(mail3Head2) {
		t.Fatalf("TestMailBox_ListCorrection: PushHead failed for three mails.")
	}
	if mb.num != 2 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong for two mails. expect=(2) got=(%d)", mb.num)
	}
	if mb.head != mail3Head2 || mb.tail != mail3Head1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong for two mails. expect=(%v/%v) got=(head:%v tail:%v)", mail3Head2, mail3Head1, mb.head, mb.tail)
	}
	if mb.head.next != mail3Head1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next is wrong for two mails. expect=(%v) got=(%v)", mail3Head1, mb.head.next)
	}
	if mb.head.next.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next next next is wrong for two mails. expect=(nil) got=(%v)", mb.head.next.next.next)
	}
	if mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox tail next is wrong for two mails. expect=(nil) got=(%v)", mb.tail.next)
	}
	if getAllocMailDebugNum() != 3 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}
	// Push third mail to head
	// mail order: mail3Head3 -> mail3Head2 -> mail3Head1
	if !mb.pushHead(mail3Head3) {
		t.Fatalf("TestMailBox_ListCorrection: PushHead failed for three mails.")
	}
	if mb.num != 3 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong for three mails. expect=(3) got=(%d)", mb.num)
	}
	if mb.head != mail3Head3 || mb.tail != mail3Head1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong for three mails. expect=(%v/%v) got=(head:%v tail:%v)", mail3Head3, mail3Head1, mb.head, mb.tail)
	}
	if mb.head.next != mail3Head2 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next is wrong for three mails. expect=(%v) got=(%v)", mail3Head2, mb.head.next)
	}
	if mb.head.next.next != mail3Head1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox second mail next is wrong for three mails. expect=(%v) got=(%v)", mail3Head1, mb.head.next.next)
	}
	if mb.head.next.next.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next next next is wrong for three mails. expect=(nil) got=(%v)", mb.head.next.next.next)
	}
	if mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox tail next is wrong for three mails. expect=(nil) got=(%v)", mb.tail.next)
	}
	if getAllocMailDebugNum() != 3 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}
	t.Log("TestMailBox_ListCorrection: Multiple mails push head case passed.")

	// Pop mails one by one and check
	// Pop not found mail
	notFoundMail = mb.popByID(0)
	if notFoundMail != nil {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for not found. expect=(nil) got=(%v)", notFoundMail)
	}
	// Get and pop mail3Head2
	// mail order: mail3Head3 -> mail3Head1
	foundMail = mb.getByID(mail3Head2.id)
	if foundMail != mail3Head2 {
		t.Fatalf("TestMailBox_ListCorrection: GetByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Head2, foundMail)
	}
	foundMail = mb.popByID(mail3Head2.id)
	if foundMail != mail3Head2 {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Head2, foundMail)
	}
	releaseMail(foundMail)
	if getAllocMailDebugNum() != 2 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed after pop found mail.")
	}
	if mb.num != 2 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong after pop mail. expect=(2) got=(%d)", mb.num)
	}
	if mb.head != mail3Head3 || mb.tail != mail3Head1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong after pop mail. expect=(%v/%v) got=(head:%v tail:%v)", mail3Head3, mail3Head1, mb.head, mb.tail)
	}
	if mb.head.next != mail3Head1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next is wrong after pop mail. expect=(%v) got=(%v)", mail3Head1, mb.head.next)
	}
	if mb.head.next.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next next is wrong after pop mail. expect=(nil) got=(%v)", mb.head.next.next)
	}
	if mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox tail next is wrong after pop mail. expect=(nil) got=(%v)", mb.tail.next)
	}
	// Get and pop mail3Head1
	// mail order: mail3Head3
	foundMail = mb.getByID(mail3Head1.id)
	if foundMail != mail3Head1 {
		t.Fatalf("TestMailBox_ListCorrection: GetByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Head1, foundMail)
	}
	foundMail = mb.popByID(mail3Head1.id)
	if foundMail != mail3Head1 {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Head1, foundMail)
	}
	releaseMail(foundMail)
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed after pop found mail.")
	}
	if mb.num != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong after pop mail. expect=(1) got=(%d)", mb.num)
	}
	if mb.head != mail3Head3 || mb.tail != mail3Head3 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong after pop mail. expect=(%v/%v) got=(head:%v tail:%v)", mail3Head3, mail3Head3, mb.head, mb.tail)
	}
	if mb.head.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next is wrong after pop mail. expect=(nil) got=(%v)", mb.head.next)
	}
	if mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox tail next is wrong after pop mail. expect=(nil) got=(%v)", mb.tail.next)
	}
	// Get and pop mail3Head3
	foundMail = mb.getByID(mail3Head3.id)
	if foundMail != mail3Head3 {
		t.Fatalf("TestMailBox_ListCorrection: GetByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Head3, foundMail)
	}
	foundMail = mb.popByID(mail3Head3.id)
	if foundMail != mail3Head3 {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Head3, foundMail)
	}
	releaseMail(foundMail)
	if getAllocMailDebugNum() != 0 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed after pop found mail.")
	}
	if mb.num != 0 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong after pop mail. expect=(0) got=(%d)", mb.num)
	}
	if mb.head != nil || mb.tail != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong after pop mail. expect=(nil) got=(head:%v tail:%v)", mb.head, mb.tail)
	}
	t.Log("TestMailBox_ListCorrection: Multiple mails pop case passed.")

	// Push multiple mails to tail
	mail3Tail1 := allocMail()
	initMail(mail3Tail1, 21, uint64(21))
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}

	mail3Tail2 := allocMail()
	initMail(mail3Tail2, 22, uint64(22))
	if getAllocMailDebugNum() != 2 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}

	mail3Tail3 := allocMail()
	initMail(mail3Tail3, 23, uint64(23))
	if getAllocMailDebugNum() != 3 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}

	// Push first mail to tail
	if !mb.pushTail(mail3Tail1) {
		t.Fatalf("TestMailBox_ListCorrection: PushTail failed for three mails.")
	}
	if mb.num != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong for one mail. expect=(1) got=(%d)", mb.num)
	}
	if mb.head != mail3Tail1 || mb.tail != mail3Tail1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong for one mail. expect=(%v/%v) got=(head:%v tail:%v)", mail3Tail1, mail3Tail1, mb.head, mb.tail)
	}
	if mb.head.next != nil || mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail next is wrong for one mail. expect=(nil) got=(head.next:%v tail.next:%v)", mb.head.next, mb.tail.next)
	}
	if getAllocMailDebugNum() != 3 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}
	// Push second mail to tail
	if !mb.pushTail(mail3Tail2) {
		t.Fatalf("TestMailBox_ListCorrection: PushTail failed for three mails.")
	}
	if mb.num != 2 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong for two mails. expect=(2) got=(%d)", mb.num)
	}
	if mb.head != mail3Tail1 || mb.tail != mail3Tail2 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong for two mails. expect=(%v/%v) got=(head:%v tail:%v)", mail3Tail1, mail3Tail2, mb.head, mb.tail)
	}
	if mb.head.next != mail3Tail2 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next is wrong for two mails. expect=(%v) got=(%v)", mail3Tail2, mb.head.next)
	}
	if mb.head.next.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next next is wrong for two mails. expect=(nil) got=(%v)", mb.head.next.next)
	}
	if mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox tail next is wrong for two mails. expect=(nil) got=(%v)", mb.tail.next)
	}
	if getAllocMailDebugNum() != 3 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}
	// Push third mail to tail
	if !mb.pushTail(mail3Tail3) {
		t.Fatalf("TestMailBox_ListCorrection: PushTail failed for three mails.")
	}
	if mb.num != 3 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong for three mails. expect=(3) got=(%d)", mb.num)
	}
	if mb.head != mail3Tail1 || mb.tail != mail3Tail3 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong for three mails. expect=(%v/%v) got=(head:%v tail:%v)", mail3Tail1, mail3Tail3, mb.head, mb.tail)
	}
	if mb.head.next != mail3Tail2 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next is wrong for three mails. expect=(%v) got=(%v)", mail3Tail2, mb.head.next)
	}
	if mb.head.next.next != mail3Tail3 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox second mail next is wrong for three mails. expect=(%v) got=(%v)", mail3Tail3, mb.head.next.next)
	}
	if mb.head.next.next.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next next next is wrong for three mails. expect=(nil) got=(%v)", mb.head.next.next.next)
	}
	if mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox tail next is wrong for three mails. expect=(nil) got=(%v)", mb.tail.next)
	}
	if getAllocMailDebugNum() != 3 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed for three mails.")
	}
	t.Log("TestMailBox_ListCorrection: Multiple mails push tail case passed.")

	// Pop mails one by one and check
	// Pop not found mail
	notFoundMail = mb.popByID(0)
	if notFoundMail != nil {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for not found. expect=(nil) got=(%v)", notFoundMail)
	}
	// Get and pop mail3Tail2
	// mail order: mail3Tail1 -> mail3Tail3
	foundMail = mb.getByID(mail3Tail2.id)
	if foundMail != mail3Tail2 {
		t.Fatalf("TestMailBox_ListCorrection: GetByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Tail2, foundMail)
	}
	foundMail = mb.popByID(mail3Tail2.id)
	if foundMail != mail3Tail2 {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Tail2, foundMail)
	}
	releaseMail(foundMail)
	if getAllocMailDebugNum() != 2 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed after pop found mail.")
	}
	if mb.num != 2 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong after pop mail. expect=(2) got=(%d)", mb.num)
	}
	if mb.head != mail3Tail1 || mb.tail != mail3Tail3 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong after pop mail. expect=(%v/%v) got=(head:%v tail:%v)", mail3Tail1, mail3Tail3, mb.head, mb.tail)
	}
	if mb.head.next != mail3Tail3 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next is wrong after pop mail. expect=(%v) got=(%v)", mail3Tail3, mb.head.next)
	}
	if mb.head.next.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next next is wrong after pop mail. expect=(nil) got=(%v)", mb.head.next.next)
	}
	if mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox tail next is wrong after pop mail. expect=(nil) got=(%v)", mb.tail.next)
	}
	// Get and pop mail3Tail1
	// mail order: mail3Tail3
	foundMail = mb.getByID(mail3Tail1.id)
	if foundMail != mail3Tail1 {
		t.Fatalf("TestMailBox_ListCorrection: GetByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Tail1, foundMail)
	}
	foundMail = mb.popByID(mail3Tail1.id)
	if foundMail != mail3Tail1 {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Tail1, foundMail)
	}
	releaseMail(foundMail)
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed after pop found mail.")
	}
	if mb.num != 1 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong after pop mail. expect=(1) got=(%d)", mb.num)
	}
	if mb.head != mail3Tail3 || mb.tail != mail3Tail3 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong after pop mail. expect=(%v/%v) got=(head:%v tail:%v)", mail3Tail3, mail3Tail3, mb.head, mb.tail)
	}
	if mb.head.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head next is wrong after pop mail. expect=(nil) got=(%v)", mb.head.next)
	}
	if mb.tail.next != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox tail next is wrong after pop mail. expect=(nil) got=(%v)", mb.tail.next)
	}
	// Get and pop mail3Tail3
	foundMail = mb.getByID(mail3Tail3.id)
	if foundMail != mail3Tail3 {
		t.Fatalf("TestMailBox_ListCorrection: GetByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Tail3, foundMail)
	}
	foundMail = mb.popByID(mail3Tail3.id)
	if foundMail != mail3Tail3 {
		t.Fatalf("TestMailBox_ListCorrection: PopByID returned wrong mail for found. expect=(%v) got=(%v)", mail3Tail3, foundMail)
	}
	releaseMail(foundMail)
	if getAllocMailDebugNum() != 0 {
		t.Fatalf("TestMailBox_ListCorrection: Mail allocation tracking failed after pop found mail.")
	}
	if mb.num != 0 {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox num is wrong after pop mail. expect=(0) got=(%d)", mb.num)
	}
	if mb.head != nil || mb.tail != nil {
		t.Fatalf("TestMailBox_ListCorrection: Mailbox head/tail is wrong after pop mail. expect=(nil) got=(head:%v tail:%v)", mb.head, mb.tail)
	}
	t.Log("TestMailBox_ListCorrection: Multiple mails pop case passed.")

	<-time.After(time.Millisecond)
}

// Test for waitPop and popAll in mailbox
// #1 waitPop
// #2 popAll
func TestMailBox_WaitPopAndPopAll(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	mb := newMailBox("testMailboxWaitPop", &testMailboxHandler{}, newTestLoggingAtomos(t))
	mb.running = true
	if mb.num != 0 {
		t.Fatalf("TestMailBox_WaitPop: Mailbox num is wrong for no mail. expect=(0) got=(%d)", mb.num)
	}
	if mb.head != nil || mb.tail != nil {
		t.Fatalf("TestMailBox_WaitPop: Mailbox head/tail is wrong for no mail. expect=(nil) got=(head:%v tail:%v)", mb.head, mb.tail)
	}
	// Wait loggingAtomos logs popped
	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	// Push one mail and wait pop
	mail1 := allocMail()
	initMail(mail1, 1, uint64(1))
	if !mb.pushTail(mail1) {
		t.Fatalf("TestMailBox_WaitPop: PushTail failed for one mail.")
	}
	if mb.num != 1 {
		t.Fatalf("TestMailBox_WaitPop: Mailbox num is wrong after push mail. expect=(1) got=(%d)", mb.num)
	}
	if mb.head != mail1 {
		t.Fatalf("TestMailBox_WaitPop: Mailbox head is wrong after push mail. expect=(%v) got=(%v)", mail1, mb.head)
	}
	if mb.tail != mail1 {
		t.Fatalf("TestMailBox_WaitPop: Mailbox tail is wrong after push mail. expect=(%v) got=(%v)", mail1, mb.tail)
	}
	if mb.head.next != nil {
		t.Fatalf("TestMailBox_WaitPop: Mailbox head next is wrong after push mail. expect=(nil) got=(%v)", mb.head.next)
	}
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_WaitPop: Mail allocation tracking failed for one mail.")
	}

	popMail1 := mb.waitPop()
	if popMail1 != mail1 {
		t.Fatalf("TestMailBox_WaitPop: WaitPop returned wrong mail. expect=(%v) got=(%v)", mail1, popMail1)
	}
	releaseMail(popMail1)
	if popMail1.next != nil {
		t.Fatalf("TestMailBox_WaitPop: Popped mail next is wrong after wait pop mail. expect=(nil) got=(%v)", popMail1.next)
	}
	if mb.num != 0 {
		t.Fatalf("TestMailBox_WaitPop: Mailbox num is wrong after wait pop mail. expect=(0) got=(%d)", mb.num)
	}
	if mb.head != nil || mb.tail != nil {
		t.Fatalf("TestMailBox_WaitPop: Mailbox head/tail is wrong after wait pop mail. expect=(nil) got=(head:%v tail:%v)", mb.head, mb.tail)
	}
	if getAllocMailDebugNum() != 0 {
		t.Fatalf("TestMailBox_WaitPop: Mail allocation tracking failed after wait pop mail.")
	}
	t.Log("TestMailBox_WaitPop: WaitPop returned correct mail.", popMail1)

	// Push two mail and wait pop
	mail2 := allocMail()
	initMail(mail2, 2, uint64(2))
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_WaitPop: Mail allocation tracking failed for two mails.")
	}

	mail3 := allocMail()
	initMail(mail3, 3, uint64(3))
	if getAllocMailDebugNum() != 2 {
		t.Fatalf("TestMailBox_WaitPop: Mail allocation tracking failed for two mails.")
	}

	if !mb.pushTail(mail2) {
		t.Fatalf("TestMailBox_WaitPop: PushTail failed for mail2.")
	}
	if !mb.pushTail(mail3) {
		t.Fatalf("TestMailBox_WaitPop: PushTail failed for mail3.")
	}

	popMail2 := mb.waitPop()
	if popMail2 != mail2 {
		t.Fatalf("TestMailBox_WaitPop: WaitPop returned wrong mail for mail2. expect=(%v) got=(%v)", mail2, popMail2)
	}
	releaseMail(popMail2)
	if popMail2.next != nil {
		t.Fatalf("TestMailBox_WaitPop: Popped mail2 next is wrong after wait pop mail. expect=(nil) got=(%v)", popMail2.next)
	}
	if mb.num != 1 {
		t.Fatalf("TestMailBox_WaitPop: Mailbox num is wrong after wait pop mail2. expect=(1) got=(%d)", mb.num)
	}
	if mb.head != mail3 || mb.tail != mail3 {
		t.Fatalf("TestMailBox_WaitPop: Mailbox head/tail is wrong after wait pop mail2. expect=(%v/%v) got=(head:%v tail:%v)", mail3, mail3, mb.head, mb.tail)
	}
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_WaitPop: Mail allocation tracking failed after wait pop mail2.")
	}
	t.Log("TestMailBox_WaitPop: WaitPop returned correct mail for mail2.", popMail2)

	popMail3 := mb.waitPop()
	if popMail3 != mail3 {
		t.Fatalf("TestMailBox_WaitPop: WaitPop returned wrong mail for mail3. expect=(%v) got=(%v)", mail3, popMail3)
	}
	releaseMail(popMail3)
	if popMail3.next != nil {
		t.Fatalf("TestMailBox_WaitPop: Popped mail3 next is wrong after wait pop mail. expect=(nil) got=(%v)", popMail3.next)
	}
	if mb.num != 0 {
		t.Fatalf("TestMailBox_WaitPop: Mailbox num is wrong after wait pop mail3. expect=(0) got=(%d)", mb.num)
	}
	if mb.head != nil || mb.tail != nil {
		t.Fatalf("TestMailBox_WaitPop: Mailbox head/tail is wrong after wait pop mail3. expect=(nil) got=(head:%v tail:%v)", mb.head, mb.tail)
	}
	if getAllocMailDebugNum() != 0 {
		t.Fatalf("TestMailBox_WaitPop: Mail allocation tracking failed after wait pop mail3.")
	}
	t.Log("TestMailBox_WaitPop: WaitPop returned correct mail for mail3.", popMail3)

	if !mb.mutex.TryLock() {
		t.Fatal("TestMailBox_WaitPop: Mailbox mutex is not released after wait pop.")
	}
	mb.mutex.Unlock()

	// Push one mail and pop all
	mail4 := allocMail()
	initMail(mail4, 4, uint64(4))
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_PopAll: Mail allocation tracking failed for one mail.")
	}

	if !mb.pushTail(mail4) {
		t.Fatalf("TestMailBox_PopAll: PushTail failed for one mail.")
	}
	if mb.num != 1 {
		t.Fatalf("TestMailBox_PopAll: Mailbox num is wrong after push mail. expect=(1) got=(%d)", mb.num)
	}
	popMails, popNum := mb.popAll()
	if popNum != 1 {
		t.Fatalf("TestMailBox_PopAll: PopAll returned wrong num. expect=(1) got=(%d)", popNum)
	}
	if popMails != mail4 {
		t.Fatalf("TestMailBox_PopAll: PopAll returned wrong mail. expect=(%v) got=(%v)", mail4, popMails)
	}
	releaseMail(popMails)
	if popMails.next != nil {
		t.Fatalf("TestMailBox_PopAll: Popped mail next is wrong after pop all mail. expect=(nil) got=(%v)", popMails.next)
	}
	if mb.num != 0 {
		t.Fatalf("TestMailBox_PopAll: Mailbox num is wrong after pop all mail. expect=(0) got=(%d)", mb.num)
	}
	if mb.head != nil || mb.tail != nil {
		t.Fatalf("TestMailBox_PopAll: Mailbox head/tail is wrong after pop all mail. expect=(nil) got=(head:%v tail:%v)", mb.head, mb.tail)
	}
	if getAllocMailDebugNum() != 0 {
		t.Fatalf("TestMailBox_PopAll: Mail allocation tracking failed after pop all mail.")
	}
	t.Log("TestMailBox_PopAll: PopAll returned correct mail for one mail.", popMails)

	// Push multiple mails and pop all
	mail5 := allocMail()
	initMail(mail5, 5, uint64(5))
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_PopAll: Mail allocation tracking failed for two mails.")
	}

	mail6 := allocMail()
	initMail(mail6, 6, uint64(6))
	if getAllocMailDebugNum() != 2 {
		t.Fatalf("TestMailBox_PopAll: Mail allocation tracking failed for two mails.")
	}

	if !mb.pushTail(mail5) {
		t.Fatalf("TestMailBox_PopAll: PushTail failed for mail5.")
	}
	if !mb.pushTail(mail6) {
		t.Fatalf("TestMailBox_PopAll: PushTail failed for mail6.")
	}
	if mb.num != 2 {
		t.Fatalf("TestMailBox_PopAll: Mailbox num is wrong after push mails. expect=(2) got=(%d)", mb.num)
	}
	popMails, popNum = mb.popAll()
	if popNum != 2 {
		t.Fatalf("TestMailBox_PopAll: PopAll returned wrong num. expect=(2) got=(%d)", popNum)
	}
	if popMails != mail5 {
		t.Fatalf("TestMailBox_PopAll: PopAll returned wrong first mail. expect=(%v) got=(%v)", mail5, popMails)
	}
	if popMails.next != mail6 {
		t.Fatalf("TestMailBox_PopAll: PopAll returned wrong second mail. expect=(%v) got=(%v)", mail6, popMails.next)
	}
	if popMails.next.next != nil {
		t.Fatalf("TestMailBox_PopAll: Popped mails next next is wrong after pop all mails. expect=(nil) got=(%v)", popMails.next.next)
	}
	if mb.num != 0 {
		t.Fatalf("TestMailBox_PopAll: Mailbox num is wrong after pop all mails. expect=(0) got=(%d)", mb.num)
	}
	if mb.head != nil || mb.tail != nil {
		t.Fatalf("TestMailBox_PopAll: Mailbox head/tail is wrong after pop all mails. expect=(nil) got=(head:%v tail:%v)", mb.head, mb.tail)
	}
	for ; popMails != nil; popMails = popMails.next {
		releaseMail(popMails)
	}
	if getAllocMailDebugNum() != 0 {
		t.Fatalf("TestMailBox_PopAll: Mail allocation tracking failed after pop all mails.")
	}
	t.Log("TestMailBox_PopAll: PopAll returned correct mails for multiple mails.", popMails)

	if !mb.mutex.TryLock() {
		t.Fatal("TestMailBox_PopAll: Mailbox mutex is not released after pop all.")
	}
	mb.mutex.Unlock()
	t.Log("TestMailBox_WaitPopAndPopAll: All cases passed.")

	<-time.After(time.Millisecond)
}

func TestMailBox_RemoveMail(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	mb := newMailBox("testMailboxRemoveMail", &testMailboxHandler{}, newTestLoggingAtomos(t))
	mb.running = true

	// Wait loggingAtomos logs popped
	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	// Push multiple mails
	mail1 := allocMail()
	initMail(mail1, 1, uint64(1))
	if getAllocMailDebugNum() != 1 {
		t.Fatalf("TestMailBox_RemoveMail: Mail allocation tracking failed for three mails.")
	}

	mail2 := allocMail()
	initMail(mail2, 2, uint64(2))
	if getAllocMailDebugNum() != 2 {
		t.Fatalf("TestMailBox_RemoveMail: Mail allocation tracking failed for three mails.")
	}

	mail3 := allocMail()
	initMail(mail3, 3, uint64(3))
	if getAllocMailDebugNum() != 3 {
		t.Fatalf("TestMailBox_RemoveMail: Mail allocation tracking failed for three mails.")
	}

	if !mb.pushTail(mail1) {
		t.Fatalf("TestMailBox_RemoveMail: PushTail failed for mail1.")
	}
	if !mb.pushTail(mail2) {
		t.Fatalf("TestMailBox_RemoveMail: PushTail failed for mail2.")
	}
	if !mb.pushTail(mail3) {
		t.Fatalf("TestMailBox_RemoveMail: PushTail failed for mail3.")
	}
	if mb.num != 3 {
		t.Fatalf("TestMailBox_RemoveMail: Mailbox num is wrong after push mails. expect=(3) got=(%d)", mb.num)
	}
	if mb.head != mail1 {
		t.Fatalf("TestMailBox_RemoveMail: Mailbox head is wrong after push mails. expect=(%v) got=(%v)", mail1, mb.head)
	}
	if mb.head.next != mail2 {
		t.Fatalf("TestMailBox_RemoveMail: Mailbox head next is wrong after push mails. expect=(%v) got=(%v)", mail2, mb.head.next)
	}
	if mb.head.next.next != mail3 {
		t.Fatalf("TestMailBox_RemoveMail: Mailbox head next next is wrong after push mails. expect=(%v) got=(%v)", mail3, mb.head.next.next)
	}
	if mb.head.next.next.next != nil {
		t.Fatalf("TestMailBox_RemoveMail: Mailbox head next next next is wrong after push mails. expect=(nil) got=(%v)", mb.head.next.next.next)
	}
	if mb.tail != mail3 {
		t.Fatalf("TestMailBox_RemoveMail: Mailbox tail is wrong after push mails. expect=(%v) got=(%v)", mail3, mb.tail)
	}

	// Remove mail2
	if !mb.removeMail(mail2) {
		t.Fatalf("TestMailBox_RemoveMail: RemoveMail failed for mail2.")
	} else {
		releaseMail(mail2)
	}
	if getAllocMailDebugNum() != 2 {
		t.Fatalf("TestMailBox_RemoveMail: Mail allocation tracking failed after remove mail2.")
	}
	if mb.num != 2 {
		t.Fatalf("TestMailBox_RemoveMail: Mailbox num is wrong after remove mail2. expect=(2) got=(%d)", mb.num)
	}
	if mb.head != mail1 {
		t.Fatalf("TestMailBox_RemoveMail: Mailbox head is wrong after remove mail2. expect=(%v) got=(%v)", mail1, mb.head)
	}
	if mb.head.next != mail3 {
		t.Fatalf("TestMailBox_RemoveMail: Mailbox head next is wrong after remove mail2. expect=(%v) got=(%v)", mail3, mb.head.next)
	}
	if mb.head.next.next != nil {
		t.Fatalf("TestMailBox_RemoveMail: Mailbox head next next is wrong after remove mail2. expect=(nil) got=(%v)", mb.head.next.next)
	}
	if mb.tail != mail3 {
		t.Fatalf("TestMailBox_RemoveMail: Mailbox tail is wrong after remove mail2. expect=(%v) got=(%v)", mail3, mb.tail)
	}

	t.Log("TestMailBox_RemoveMail: Mail allocation tracking complete.")

	<-time.After(time.Millisecond)
}

func TestMailBox_StopIfNoMail(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	letItRun := make(chan struct{})
	stopped := make(chan struct{})
	h := &testMailboxHandler{
		t:  t,
		mb: nil,
		recv: func(mail *mail) {
			<-letItRun
		},
		stop: func(killMail, remainMails *mail, num uint32) *Error {
			<-stopped
			return nil
		},
	}
	h.mb = newMailBox("testMailbox", h, newTestLoggingAtomos(t))
	if err := h.mb.start(func() *Error {
		return nil
	}); err != nil {
		t.Fatalf("TestMailbox_Smoke: Start failed. err=(%v)", err.AddStack(nil))
	}

	// Wait loggingAtomos logs popped
	for {
		if num := getAllocMailDebugNum(); num != 0 {
			t.Log("TestMailBox_StopIfNoMail: Waiting for mail logs to be popped. allocMailDebugNum=", num)
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	// Push a mail to mailbox
	m := allocMail()
	initMail(m, DefaultMailID, nil)
	h.mb.pushTail(m)
	if killed := h.mb.stopIfNoMail(nil); killed {
		t.Fatalf("TestMailBox_StopIfNoMail: StopIfNoMail returned killed=true but there is a mail.")
	}
	letItRun <- struct{}{}
	time.After(time.Millisecond)
	if killed := h.mb.stopIfNoMail(nil); !killed {
		t.Fatalf("TestMailBox_StopIfNoMail: StopIfNoMail returned killed=false but there is no mail.")
	}
	stopped <- struct{}{}

	// Wait a moment to ensure mailbox is stopped
	time.After(time.Millisecond)
	if h.mb.running {
		t.Fatalf("TestMailBox_StopIfNoMail: Mailbox is still running after StopIfNoMail.")
	}

	if h.mb.num != 0 {
		t.Fatalf("TestMailBox_StopIfNoMail: Mailbox num is wrong after stopped. expect=(0) got=(%d)", h.mb.num)
	}
	<-time.After(time.Millisecond)
	if getAllocMailDebugNum() != 0 {
		t.Fatalf("TestMailBox_StopIfNoMail: Mail allocation tracking failed after mailbox stopped.")
	}
	t.Log("TestMailBox_StopIfNoMail: StopIfNoMail cases passed.")

	<-time.After(time.Millisecond)
}

func TestMailBox_StartLoop(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	h := &testMailboxHandler{
		t:    t,
		mb:   nil,
		recv: func(mail *mail) {},
		stop: func(killMail, remainMails *mail, num uint32) *Error { return nil },
	}
	h.mb = newMailBox("testMailboxStartLoop", h, newTestLoggingAtomos(t))

	// Wait loggingAtomos logs popped
	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	// Test for starting failed
	if err := h.mb.start(func() *Error {
		return NewErrorf(1, "test error in mailboxOnStartUp")
	}); err == nil {
		t.Fatalf("TestMailBox_StartLoop: Start did not return error for mailboxOnStartUp failure.")
	} else if err.Code == 1 && err.Message == "test error in mailboxOnStartUp" {
		t.Logf("TestMailBox_StartLoop: Start returned expected error for mailboxOnStartUp failure. err=(%v)", err.AddStack(nil))
	} else {
		t.Fatalf("TestMailBox_StartLoop: Start returned wrong error for mailboxOnStartUp failure. expect=(code:1 message:'test error in mailboxOnStartUp') got=(code:%d message:'%s')", err.Code, err.Message)
	}
	if h.mb.running {
		t.Fatalf("TestMailBox_StartLoop: Mailbox is running after Start failed.")
	}
	if h.mb.goID == 0 {
		t.Fatalf("TestMailBox_StartLoop: Mailbox goID is not zero after Start failed.")
	}
	t.Log("TestMailBox_StartLoop: Start failed cases passed.")

	// Test for starting again
	if err := h.mb.start(func() *Error {
		return nil
	}); err != nil {
		t.Fatalf("TestMailBox_StartLoop: Start failed. err=(%v)", err.AddStack(nil))
	}
	if !h.mb.running {
		t.Fatalf("TestMailBox_StartLoop: Mailbox is not running after Start.")
	}
	if h.mb.goID == 0 {
		t.Fatalf("TestMailBox_StartLoop: Mailbox goID is zero after Start.")
	} else if h.mb.goID == getGoID() {
		t.Fatalf("TestMailBox_StartLoop: Mailbox goID is current goroutine ID after Start.")
	}
	t.Log("TestMailBox_StartLoop: Start succeeded cases passed.")

	// Test for double Start
	if err := h.mb.start(func() *Error {
		return nil
	}); err == nil {
		t.Fatalf("TestMailBox_StartLoop: Double Start did not return error.")
	} else if err.Code == ErrFrameworkRecoverFromPanic && err.Message == "Mailbox: Has already run." {
		t.Logf("TestMailBox_StartLoop: Double Start returned expected error. err=(%v)", err.AddStack(nil))
	} else {
		t.Fatalf("TestMailBox_StartLoop: Double Start returned wrong error. expect=(code:%d message:'%s') got=(code:%d message:'%s')", ErrFrameworkRecoverFromPanic, "Mailbox: Has already run.", err.Code, err.Message)
	}
	t.Log("TestMailBox_StartLoop: Start double failed cases passed.")

	<-time.After(time.Millisecond)
}

func TestMailBox_Loop(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	h := &testMailboxHandler{
		t:    t,
		mb:   nil,
		recv: func(mail *mail) {},
		stop: func(killMail, remainMails *mail, num uint32) *Error { return nil },
	}
	h.mb = newMailBox("testMailboxLoop", h, newTestLoggingAtomos(t))
	h.mb.running = true
	h.mb.goID = getGoID()

	// Wait loggingAtomos logs popped
	for {
		if getAllocMailDebugNum() != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	// Test for loop with exit due to mailboxOnStartUp error
	wait := make(chan *Error, 1)
	h.mb.loop(wait, func() *Error {
		return NewErrorf(1, "test error in mailboxOnStartUp")
	})
	err := <-wait
	if err == nil {
		t.Fatalf("TestMailBox_Loop: Loop did not return error from mailboxOnStartUp.")
	}
	if err.Code != 1 || err.Message != "test error in mailboxOnStartUp" {
		t.Fatalf("TestMailBox_Loop: Loop returned wrong error from mailboxOnStartUp. expect=(code:1 message:'test error in mailboxOnStartUp') got=(code:%d message:'%s')", err.Code, err.Message)
	}

	<-time.After(time.Millisecond)
}

func TestMailBox_MailPool_ReleaseTwice(t *testing.T) {
	allocMailUsingPool.Store(true)
	allocMailInitCheck.Store(false)
	allocMailDebug.Store(false)

	m := allocMail()
	releaseMail(m)
	releaseMail(m)

	<-time.After(time.Millisecond)
}

func TestMailBox_MailPool_CheckRelease(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailInitCheck.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	n := 1000
	waiters := 128
	testMailBoxHelper(t, n, waiters)

	<-time.After(time.Millisecond)
}

func TestMailBox_MailPool_CheckPool(t *testing.T) {
	allocMailUsingPool.Store(true)
	allocMailInitCheck.Store(true)
	allocMailDebug.Store(false)

	n := 1000
	waiters := 128
	testMailBoxHelper(t, n, waiters)

	<-time.After(time.Millisecond)
}

func testMailBoxHelper(t *testing.T, n, waiters int) {
	var wg sync.WaitGroup
	h := &testMailboxHandler{
		t:  t,
		mb: nil,
		recv: func(mail *mail) {
			wg.Done()
		},
		stop: func(killMail, remainMails *mail, num uint32) *Error {
			wg.Done()
			return nil
		},
	}
	h.mb = newMailBox("testMailbox", h, newTestLoggingAtomos(t))
	if err := h.mb.start(func() *Error { return nil }); err != nil {
		t.Fatalf("TestMailBox_MailPool: Start failed. err=(%v)", err.AddStack(nil))
		return
	}

	// Wait loggingAtomos logs popped
	for {
		if num := getAllocMailDebugNum(); num != 0 {
			<-time.After(time.Millisecond)
		} else {
			break
		}
	}

	t.Log("TestMailBox_MailPool: Start sending mails.")
	for routine := 0; routine < waiters; routine++ {
		wg.Add(n)
		go func() {
			for i := 0; i < n; i++ {
				m := allocMail()
				initMail(m, DefaultMailID, nil)
				h.mb.pushTail(m)
			}
		}()
	}
	wg.Wait()

	// Send exit mail
	wg.Add(1)
	killMail := allocMail()
	initKillMail(killMail, DefaultMailID, nil, nil)
	killMail.data = &mailExitCommand{}
	h.mb.pushHead(killMail)
	wg.Wait()
	if h.mb.running {
		t.Fatalf("TestMailBox_MailPool: Mailbox is still running after sending exit mail.")
	}
	<-time.After(time.Millisecond)
	if getAllocMailDebugNum() != 0 {
		t.Fatalf("TestMailBox_MailPool: Mail allocation tracking failed after mailbox stopped.")
	}
}

// Handler

type testMailboxHandler struct {
	t    *testing.T
	mb   *mailBox
	recv func(mail *mail)
	stop func(killMail, remainMails *mail, num uint32) *Error
}

func (h *testMailboxHandler) mailboxOnStartUp(fn func() *Error) *Error {
	if !h.mb.running {
		h.t.Fatal("testMailboxHandler: mailboxOnStartUp called but mailbox is not running.")
	}
	if h.mb.goID == 0 {
		h.t.Fatal("testMailboxHandler: mailboxOnStartUp called but mailbox goID is zero.")
	} else if h.mb.goID != getGoID() {
		h.t.Fatal("testMailboxHandler: mailboxOnStartUp called but mailbox goID is current goroutine ID.")
	}
	if err := fn(); err != nil {
		return err.AddStack(nil)
	}
	return nil
}

func (h *testMailboxHandler) mailboxOnReceive(mail *mail) {
	if !h.mb.running {
		h.t.Fatal("testMailboxHandler: mailboxOnReceive called but mailbox is not running.")
	}
	if h.mb.goID == 0 {
		h.t.Fatal("testMailboxHandler: mailboxOnReceive called but mailbox goID is zero.")
	} else if h.mb.goID != getGoID() {
		h.t.Fatal("testMailboxHandler: mailboxOnReceive called but mailbox goID is current goroutine ID.")
	}
	if h.recv != nil {
		h.recv(mail)
	}
}

func (h *testMailboxHandler) mailboxOnStop(killMail, remainMails *mail, num uint32) *Error {
	if h.mb.running {
		h.t.Fatal("testMailboxHandler: mailboxOnStop called but mailbox is still running.")
	}
	if h.mb.goID == 0 {
		h.t.Fatal("testMailboxHandler: mailboxOnStop called but mailbox goID is not zero.")
	} else if h.mb.goID != getGoID() {
		h.t.Fatal("testMailboxHandler: mailboxOnStop called but mailbox goID is current goroutine ID.")
	}
	if h.stop != nil {
		return h.stop(killMail, remainMails, num)
	}
	return nil
}

// Benchmark

type benchmarkMailHandler struct {
	b       *testing.B
	wg      sync.WaitGroup
	sendNum int
	recvNum int
}

func (h *benchmarkMailHandler) mailboxOnStartUp(fn func() *Error) *Error {
	return nil
}

func (h *benchmarkMailHandler) mailboxOnReceive(mail *mail) {
	h.recvNum += 1
	if h.sendNum-h.recvNum == 0 {
		log.Printf("Send=%d Recv=%d\n", h.sendNum, h.recvNum)
	}
	h.wg.Done()
}

func (h *benchmarkMailHandler) mailboxOnStop(killMail, remainMails *mail, num uint32) *Error {
	//log.Println("Benchmark mailbox has received stop-mail.")
	//for ; remainMails != nil; remainMails = remainMails.next {
	//}
	h.wg.Done()
	return nil
}

func benchmarkMailBox(b *testing.B, waiters int) {
	allocMailUsingPool.Store(true)
	h := benchmarkMailHandler{b: b}
	mb := newMailBox("benchmarkMailbox", &h, newBenchLoggingAtomos(b))
	if err := mb.start(nil); err != nil {
		b.Errorf("Benchmark: Start failed. err=(%v)", err.AddStack(nil))
		return
	}
	b.Log("add", b.N, "waiters", waiters)
	for routine := 0; routine < waiters; routine++ {
		h.wg.Add(b.N)
		h.sendNum += b.N
		go func() {
			for i := 0; i < b.N; i++ {
				m := allocMail()
				initMail(m, DefaultMailID, nil)
				mb.pushTail(m)
			}
		}()
	}
	h.wg.Wait()
	h.wg.Add(1)
	killMail := allocMail()
	initKillMail(killMail, DefaultMailID, nil, nil)
	killMail.data = &mailExitCommand{}
	mb.pushHead(killMail)
	h.wg.Wait()
	if mb.running {
		b.Errorf("Benchmark: Mailbox is still running after sending exit mail.")
	}
	if h.recvNum != h.sendNum {
		b.Errorf("Benchmark: Mailbox received wrong number of mails. expect=(%d) got=(%d)", h.sendNum, h.recvNum)
	}
}

func BenchmarkMailbox1(b *testing.B) {
	benchmarkMailBox(b, 1)
}

func BenchmarkMailbox2(b *testing.B) {
	benchmarkMailBox(b, 2)
}

func BenchmarkMailbox4(b *testing.B) {
	benchmarkMailBox(b, 4)
}

func BenchmarkMailbox8(b *testing.B) {
	benchmarkMailBox(b, 8)
}

func BenchmarkMailbox16(b *testing.B) {
	benchmarkMailBox(b, 16)
}

func BenchmarkMailbox32(b *testing.B) {
	benchmarkMailBox(b, 32)
}

func BenchmarkMailbox64(b *testing.B) {
	benchmarkMailBox(b, 64)
}

func BenchmarkMailbox128(b *testing.B) {
	benchmarkMailBox(b, 128)
}
