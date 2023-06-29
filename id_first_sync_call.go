package go_atomos

import (
	"fmt"
	"sync"
)

// 调用链
// 处理同步调用的调用链，用于检测死锁。
//
// 原理：
// 我们可以把同步调用的调用链，称为同步调用链。
// 我们会想办法识别出第一个同步调用，并在此时生成一个"firstSyncCall（首个同步调用名）"，这个名称是当前进程唯一的。
// 在整个调用过程中，我们会把这个名称传递下去，在MailBox处理这个同步消息时，就把curFirstSyncCall设置成该值，处理完之后就设置成空。
// 那么如果我们在后续同步调用中，发现当前运行的curFirstSyncCall和caller的FirstSyncCall一样的话，则是循环调用死锁，此时返回错误。
// 要注意的是，获取到的curFirstSyncCall可能不为空，因为此时有可能在执行邮箱中其它的任务，反正不是相同就没问题。
//
// 如何识别第一个同步调用？我们要看看到底要有哪些调用情况，我们列举所有调用了pushMessageMail和pushScaleMail，因为它们是所有同步调用的入口：
// 1. 同步调用：这种情况，则需要判断是否caller的goroutine是否和当前调用push的goroutine是同一个，如果不是同一个异步调用，则参考下面2的异步调用逻辑（所以一定要保护好SelfID，只应该让当前atomos去持有）。
//    如果是同一个，则是同步调用。此时需要检查调用方是否有curFirstSyncCall，如果为空，证明是第一个同步调用（如Task中调用的），此时需要创建一个curFirstSyncCall，并设置到callerID和push对象（失败要取消callerID的设置）。
//    如果不为空，则检查是否和push向的ID的当前curFirstSyncCall一样，如果一样，则是循环调用死锁，返回错误。这些情况都检查过，则可以正常调用。
// 2. 异步调用：这种情况需要创建新的FirstSyncCall，因为这是一个新的调用链，调用的开端是push向的ID。
// 3. 远程调用：这种情况必须有FirstSyncCall，因为远程调用肯定有一个起源。

// idFirstSyncCall
type idFirstSyncCall interface {
	getCurFirstSyncCall() string
	setSyncMessageAndFirstCall(string) *Error
	unsetSyncMessageAndFirstCall()
	nextFirstSyncCall() string
	//checkLocalFirstSyncCall(callerID SelfID) (bool, string, *Error)
	//checkRemoteFirstSyncCall(callerID SelfID) (bool, string, *Error)
}

// idFirstSyncCallLocal
// 本地实现的idFirstSyncCall
type idFirstSyncCallLocal struct {
	info *IDInfo

	mutex sync.Mutex
	// 当前调用链的首个同步调用名。
	curFirstSyncCall string
	// 用于调用链的firstSyncCall生成的计数器。
	curCallCounter uint64
}

func initAtomosFirstSyncCall(fsc *idFirstSyncCallLocal, id *IDInfo) {
	fsc.info = id
}

// Implementation of idFirstSyncCall

func (f *idFirstSyncCallLocal) getCurFirstSyncCall() string {
	f.mutex.Lock()
	c := f.curFirstSyncCall
	f.mutex.Unlock()
	return c
}

func (f *idFirstSyncCallLocal) setSyncMessageAndFirstCall(firstSyncCall string) *Error {
	if firstSyncCall == "" {
		return NewError(ErrFrameworkRecoverFromPanic, "IDFirstSyncCall: Inputting firstSyncCall should not be empty.").AddStack(nil)
	}
	f.mutex.Lock()
	if f.curFirstSyncCall != "" {
		f.mutex.Unlock()
		return NewErrorf(ErrFrameworkRecoverFromPanic, "IDFirstSyncCall: Running firstSyncCall should be empty.").AddStack(nil)
	}
	f.curFirstSyncCall = firstSyncCall
	f.mutex.Unlock()
	return nil
}

func (f *idFirstSyncCallLocal) unsetSyncMessageAndFirstCall() {
	f.mutex.Lock()
	f.curFirstSyncCall = ""
	f.mutex.Unlock()
}

func (f *idFirstSyncCallLocal) nextFirstSyncCall() string {
	f.mutex.Lock()
	f.curCallCounter += 1
	callID := f.curCallCounter
	f.mutex.Unlock()
	return fmt.Sprintf("%s-%d", f.info.Info(), callID)
}
