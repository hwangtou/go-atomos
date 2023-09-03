package go_atomos

import (
	"strings"
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

// atomosIDContext
type atomosIDContext interface {
	isLoop(fromChain []string) *Error
	//setSyncMessageAndFirstCall(string) *Error
	//unsetSyncMessageAndFirstCall()
	//nextFirstSyncCall() string
}

// atomosIDContextLocal
// 本地实现的idFirstSyncCall
type atomosIDContextLocal struct {
	atomos *BaseAtomos

	context *IDContextInfo
}

func initAtomosIDContextLocal(ctx *atomosIDContextLocal, atomos *BaseAtomos) {
	ctx.atomos = atomos
	ctx.context = &IDContextInfo{}
}

// Implementation of atomosIDContext

func (f *atomosIDContextLocal) FromCallChain() []string {
	f.atomos.mailbox.mutex.Lock()
	defer f.atomos.mailbox.mutex.Unlock()
	return f.context.IdChain
}

func (f *atomosIDContextLocal) isLoop(fromChain []string) *Error {
	f.atomos.mailbox.mutex.Lock()
	defer f.atomos.mailbox.mutex.Unlock()

	self := f.atomos.id.Info()
	for _, chain := range fromChain {
		if chain == self {
			return NewErrorf(ErrAtomosIDCallLoop, "AtomosIDContext: Loop call detected. target=(%s),chain=(%s)", self, strings.Join(fromChain, "->")).AddStack(nil)
		}
	}
	return nil
}

// atomosIDContextRemote
// 远程实现的idFirstSyncCall
type atomosIDContextRemote struct {
	mutex   sync.RWMutex
	info    *IDInfo
	context *IDContextInfo
}

func initAtomosIDContextRemote(ctx *atomosIDContextRemote, info *IDInfo) {
	ctx.info = info
	ctx.context = &IDContextInfo{}
}

// Implementation of atomosIDContext

func (f *atomosIDContextRemote) FromCallChain() []string {
	f.mutex.RLock()
	defer f.mutex.RUnlock()
	return f.context.IdChain
}

func (f *atomosIDContextRemote) isLoop(fromChain []string) *Error {
	for _, chain := range fromChain {
		if chain == f.info.Info() {
			return NewErrorf(ErrAtomosIDCallLoop, "AtomosIDContext: Loop call detected. target=(%s),chain=(%s)", f.info, strings.Join(fromChain, "->")).AddStack(nil)
		}
	}
	return nil
}
