package go_atomos

import (
	"log"
	"testing"
)

// Benchmark

type benchmarkMailHandler struct {
	done    chan bool
	sendNum int
	recvNum int
}

func (h *benchmarkMailHandler) mailboxOnStartUp(fn func() *Error) *Error {
	return nil
}

func (h *benchmarkMailHandler) mailboxOnReceive(mail *mail) {
	h.recvNum += 1
	if h.sendNum-h.recvNum == 0 {
		h.done <- true
		log.Printf("Send=%d Recv=%d\n", h.sendNum, h.recvNum)
	}
}

func (h *benchmarkMailHandler) mailboxOnStop(killMail, remainMails *mail, num uint32) {
	//log.Println("Benchmark mailbox has received stop-mail.")
	//for ; remainMails != nil; remainMails = remainMails.next {
	//}
}

func benchmarkMailBox(b *testing.B, waiters int) {
	h := benchmarkMailHandler{
		done: make(chan bool),
	}
	mb := newMailBox("benchmarkMailbox", &h, func(s string) {
		b.Log(s)
	}, func(s string) {
		b.Error(s)
	})
	if err := mb.start(nil); err != nil {
		b.Errorf("Benchmark: Start failed. err=(%v)", err.AddStack(nil))
		return
	}
	defer mb.pushHead(&mail{
		next:   nil,
		id:     DefaultMailID,
		action: MailActionExit,
		mail:   nil,
		log:    nil,
	})
	for routine := 0; routine < waiters+1; routine++ {
		h.sendNum += b.N
		go func() {
			for i := 0; i < b.N; i++ {
				mb.pushTail(&mail{
					next:   nil,
					id:     DefaultMailID,
					action: MailActionRun,
					mail:   nil,
					log:    nil,
				})
			}
		}()
	}
	<-h.done
}

func BenchmarkMailbox1(b *testing.B) {
	initTestFakeCosmosProcessBenchmark(b)
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
