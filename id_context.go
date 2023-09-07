package go_atomos

import (
	"strings"
	"sync"
)

// 调用链
// 处理同步调用的调用链，用于检测死锁。

// atomosIDContextLocal
// 本地实现的IDContext
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

func (f *atomosIDContextLocal) isLoop(fromChain []string, callerID SelfID) *Error {
	f.atomos.mailbox.mutex.Lock()
	defer f.atomos.mailbox.mutex.Unlock()

	if callerID.GetIDInfo().IsEqual(f.atomos.id) {
		return NewErrorf(ErrAtomosIDCallLoop, "AtomosIDContext: Loop call to self detected. target=(%s),chain=(%s)", f.atomos.id.Info(), strings.Join(fromChain, "->")).AddStack(nil)
	}
	self := f.atomos.id.Info()
	for _, chain := range fromChain {
		if chain == self {
			return NewErrorf(ErrAtomosIDCallLoop, "AtomosIDContext: Loop call detected. target=(%s),chain=(%s)", self, strings.Join(fromChain, "->")).AddStack(nil)
		}
	}
	return nil
}

// atomosIDContextRemote
// 远程实现的IDContext
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
