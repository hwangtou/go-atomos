package atomos

// P1 排空等待的框架级模拟:验证"空闲到期但在途回调非空时推迟自杀"的设计语义
// (生产实现在 server/shared/scale_timer.go 的 AtomCleanupTimer.check(),判定门逻辑相同):
//   阶段一:推迟窗口内回复到达 → 回调被正常消费,无 handler not found,无需清账;
//   阶段二:连续推迟至额度用尽 → KillSelf,残余回调被清账以 ErrAsyncCallbackCancelledByHalt 终结。
//
// 移植自生产副本 async_drain_window_test.go,适配当前仓库测试基建。

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// drainGate 与生产 AtomCleanupTimer.check() 的判定门一致(推迟至多 2 次,活跃期重置)
type drainGate struct {
	atom       *AtomLocal
	process    *CosmosProcess
	drainTried int
}

const maxDrainDefers = 2

// expire 模拟一次空闲到期检查:返回 true=推迟(未自杀),false=已 KillSelf
func (g *drainGate) expire(t *testing.T) bool {
	t.Helper()
	if g.drainTried < maxDrainDefers && g.atom.atomos.AsyncCallbackPending() > 0 {
		g.drainTried++
		t.Logf("空闲到期但在途回调 %d 个,推迟自杀(%d/%d)", g.atom.atomos.AsyncCallbackPending(), g.drainTried, maxDrainDefers)
		return true
	}
	if err := g.atom.Kill(g.process.local, []ArgsForBaseAtomos{ArgBaseAtomosWaitKilled()}); err != nil {
		t.Fatalf("KillSelf failed: %v", err)
	}
	return false
}

func TestAsyncDrainWindow(t *testing.T) {
	logger := &reproLogger{}
	process, elem := startClearCosmos(t, logger)
	svc := &atomosRemoteService{process: process}

	// ---- 阶段一:推迟窗口内回复到达,回调被正常消费 ----
	atomID, _, err := elem.SpawnAtom(process.local, "drain1", &Config{Cosmos: "arg"}, nil, false)
	if err != nil {
		t.Fatalf("spawn atom failed: %v", err)
	}
	atom := atomID.(*AtomLocal)
	gate := &drainGate{atom: atom, process: process}

	var consumed reproCounter
	startupID, asyncID := atom.atomos.asyncSet(func(out proto.Message, err error) {
		consumed.add()
	})

	// 第一次空闲到期:有在途回调 → 推迟
	if !gate.expire(t) {
		t.Fatalf("有在途回调时应推迟,不应自杀")
	}
	// 推迟窗口内回复到达 → 回调被正常消费(生产:结算回复赶在排空窗口内回来)
	callbackID := svc.getLocalIDFromIDInfo(&IDInfo{
		Type: IDType_Atom, Cosmos: "clearCosmos", Node: "clearNode",
		Element: "ClearElem", Atom: "drain1",
	})
	if callbackID == nil {
		t.Fatalf("按名字解析 callbackID 失败")
	}
	callbackID.(idInternal).asyncCallback(callbackID, "DrainAsync", startupID, asyncID, nil, nil)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && consumed.get() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if consumed.get() != 1 {
		t.Fatalf("排空窗口内的回复应被正常消费,实际 %d 次", consumed.get())
	}
	if logger.hasError("handler not found") {
		t.Errorf("排空窗口内消费不应出现 handler not found")
	}
	// 第二次空闲到期:已无在途回调 → 照常自杀(零回归)
	if gate.expire(t) {
		t.Fatal("无在途回调时应自杀")
	}
	if atom.State() != BaseAtomosHalt {
		t.Fatalf("atom 应已停机,state=%d", atom.State())
	}
	t.Log("✅ 阶段一通过: 推迟窗口内回复被消费,窗口关闭后正常回收,无 handler not found")

	// ---- 阶段二:连续推迟至额度用尽,残余回调被清账终结 ----
	atomID2, _, err := elem.SpawnAtom(process.local, "drain2", &Config{Cosmos: "arg"}, nil, false)
	if err != nil {
		t.Fatalf("spawn atom failed: %v", err)
	}
	atom2 := atomID2.(*AtomLocal)
	gate2 := &drainGate{atom: atom2, process: process}

	var cancelled reproCounter
	var cancelledCode reproErrCode
	atom2.atomos.asyncSet(func(out proto.Message, err error) {
		cancelled.add()
		if err != nil {
			cancelledCode.set(err)
		}
	})

	for i := 1; i <= maxDrainDefers; i++ {
		if !gate2.expire(t) {
			t.Fatalf("第 %d 次到期应推迟(在途回调仍在)", i)
		}
	}
	// 额度用尽:第三次到期 → KillSelf → 清账
	if gate2.expire(t) {
		t.Fatal("额度用尽后应自杀")
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cancelled.get() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if got := cancelled.get(); got != 1 {
		t.Fatalf("清账应终结回调 1 次,实际 %d 次", got)
	}
	if code := cancelledCode.get(); code != ErrCodeAsyncCallbackCancelledByHalt {
		t.Fatalf("清账错误码不符: 期望 ErrAsyncCallbackCancelledByHalt,实际 %d", code)
	}
	t.Log("✅ 阶段二通过: 推迟额度用尽后照常回收,残余回调被清账以 ErrAsyncCallbackCancelledByHalt 终结")
}
