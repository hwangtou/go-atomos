package atomos

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

// isLoop
// 检测调用链是否存在死锁
// 1. 如果是spawn调用，直接返回
// 2. 如果是call调用，检查调用链是否存在死锁
// 2.1 交叉调用，检查调用链是否存在死锁
// 2.2 自调用，检查调用链是否存在死锁
func (f *atomosIDContextLocal) isLoop(fromChain []string, callerID SelfID, isSpawn bool) (gID uint64, err *Error) {
	f.atomos.mailbox.mutex.Lock()
	defer f.atomos.mailbox.mutex.Unlock()

	if !isSpawn && callerID.GetIDInfo().IsEqual(f.atomos.id) {
		gID = getGoID()
		if f.atomos.mailbox.goID == gID {
			return gID, NewErrorf(ErrAtomosIDCallLoop, "AtomosIDContext: Loop call to self detected. target=(%s),chain=(%s)", f.atomos.id.Info(), strings.Join(fromChain, "->")).AddStack(nil)
		}
	}
	self := f.atomos.id.Info()
	// 检查自调用
	selfChain := f.context.GetIdChain()
	for _, chain := range fromChain {
		if chain == self {
			return gID, NewErrorf(ErrAtomosIDCallLoop, "AtomosIDContext: Loop call detected. target=(%s),chain=(%s)", self, strings.Join(fromChain, "->")).AddStack(nil)
		}
		// 检查交叉调用
		for _, selfChainID := range selfChain {
			if chain == selfChainID {
				return gID, NewErrorf(ErrAtomosIDCallLoop, "AtomosIDContext: Loop call detected. target=(%s),chain=(%s)", self, strings.Join(fromChain, "->")).AddStack(nil)
			}
		}
	}
	return gID, nil
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
