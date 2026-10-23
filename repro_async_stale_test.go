package atomos

// 复现正式服问题:同名 atom 换代后,过期的异步回复按名字路由落到新实例,
// 新实例回调表中无此 asyncID → "OnAsyncMessagingCallback handler not found"。
// 时序对应生产:业务结算方发出异步请求 → atom 空闲超时 KillSelf
// → 同名 atom 被再次 Spawn 出新实例 → 对端服务执行完毕按名字回复 → 新实例报 handler not found。
//
// 移植自生产副本 repro_async_stale_test.go,适配当前仓库测试基建:
// 使用 newCosmosProcess 直接建进程(不走 InitCosmosProcess 单例),
// AddElementImplementationWithSpawn 标记 spawn 入口,Spawner 闭包带 args 变参。

import (
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// ---- 测试日志收集 ----

type reproLogger struct {
	mu     sync.Mutex
	errs   []string
	access []string
}

func (l *reproLogger) WriteAccessLog(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.access = append(l.access, s)
}

func (l *reproLogger) WriteErrorLog(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, s)
}

func (l *reproLogger) Close() {}

func (l *reproLogger) hasError(substr string) bool {
	return l.hasAny(substr, true, false)
}

// hasAny 同时检查 error 与 access 两侧日志（Warn/Debug 走 access，Error 走 error）
func (l *reproLogger) hasAny(substr string, inErr, inAccess bool) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if inErr {
		for _, e := range l.errs {
			if strings.Contains(e, substr) {
				return true
			}
		}
	}
	if inAccess {
		for _, a := range l.access {
			if strings.Contains(a, substr) {
				return true
			}
		}
	}
	return false
}

// ---- 受互斥保护的计数器(回调在框架 goroutine 上执行,测试主协程轮询读取,须同步) ----

type reproCounter struct {
	mu sync.Mutex
	n  int
}

type reproErrCode struct {
	mu sync.Mutex
	v  int64
}

func (c *reproErrCode) set(err error) { c.mu.Lock(); c.v = CodeOf(err); c.mu.Unlock() }
func (c *reproErrCode) get() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.v
}

func (c *reproCounter) add() { c.mu.Lock(); c.n++; c.mu.Unlock() }
func (c *reproCounter) get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// ---- 最小 Element/Atom 实现 ----

const reproElemName = "ReproElem"
const reproAtomName = "player1"

type reproAtom struct {
	name string
}

func (a *reproAtom) String() string { return "reproAtom:" + a.name }
func (a *reproAtom) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

// 模拟对端业务的持久化存储:已有该名字的数据
var reproStore = map[string]proto.Message{}

// AtomAutoData:使 getAtomFromRemote 在 atom 不存在时自动重 Spawn(与生产对端业务行为一致)
func (a *reproAtom) GetAtomData(name string) (proto.Message, error) {
	if d, ok := reproStore[name]; ok {
		return d, nil
	}
	return &Config{Cosmos: "data:" + name}, nil
}
func (a *reproAtom) SetAtomData(name string, data proto.Message) error {
	reproStore[name] = data
	return nil
}

type reproDev struct{}

func (d *reproDev) ElementConstructor() Atomos         { return &reproAtom{name: "elem"} }
func (d *reproDev) AtomConstructor(name string) Atomos { return &reproAtom{name: name} }
func (d *reproDev) AtomAutoData() AtomAutoData         { return &reproAtom{name: "autodata"} }
func (d *reproDev) ElementAutoData() ElementAutoData   { return nil }

type reproMainScript struct{}

func (s *reproMainScript) OnBoot(local *CosmosProcess) error    { return nil }
func (s *reproMainScript) OnStartUp(local *CosmosProcess) error { return nil }
func (s *reproMainScript) OnShutdown() error                    { return nil }

// ---- 复现 ----

func TestReproStaleAsyncReplyAfterAtomRecycled(t *testing.T) {
	logger := &reproLogger{}
	process, cerr := newCosmosProcess("reproCosmos", "reproNode", logger)
	if cerr != nil {
		t.Fatalf("newCosmosProcess failed: %v", cerr.Message)
	}

	dev := &reproDev{}
	elemImpl := NewImplementationFromDeveloper(dev)
	elemImpl.Interface = NewInterfaceFromDeveloper(reproElemName, dev)
	elemImpl.Interface.ElementSpawner = func(s ElementSelfID, a Atomos, data proto.Message, args ...ArgsForBaseAtomos) error {
		return nil
	}
	elemImpl.Interface.AtomSpawner = func(s AtomSelfID, a Atomos, arg, data proto.Message, args ...ArgsForBaseAtomos) error {
		return nil
	}
	elemImpl.ElementHandlers = map[string]MessageHandler{}
	elemImpl.AtomHandlers = map[string]MessageHandler{}

	runnable := (&CosmosRunnable{}).
		SetConfig(&Config{
			Cosmos:         "reproCosmos",
			Node:           "reproNode",
			LogLevel:       LogLevel_Debug,
			EnableElements: []string{reproElemName},
		}).
		SetMainScript(&reproMainScript{}).
		AddElementImplementationWithSpawn(elemImpl)

	if err := process.Start(runnable); err != nil {
		for _, e := range logger.errs {
			t.Logf(" captured error log: %s", e)
		}
		t.Fatalf("process start failed: %v", err)
	}
	defer func() {
		_ = process.Stop()
	}()

	elem, err := process.local.getLocalElement(reproElemName)
	if err != nil {
		t.Fatalf("get element failed: %v", err.Message)
	}

	// T0: 首次 Spawn player1(生产对应玩家注册:SpawnAtom 带 arg,AutoData 元素必需)
	spawnArg := &Config{Cosmos: "spawnArg"}
	atomIDA, _, serr := elem.SpawnAtom(process.local, reproAtomName, spawnArg, nil)
	if serr != nil {
		t.Fatalf("spawn atom failed: %v", serr)
	}
	atomA, ok := atomIDA.(*AtomLocal)
	if !ok {
		t.Fatalf("expected *AtomLocal, got %T", atomIDA)
	}

	// T1: 实例 A 注册异步回调(等价于业务异步请求经 PushAsyncMessage 的 asyncSet)
	var callbackInvoked, callbackConsumed reproCounter // 成功消费(非清账终结)次数
	startupID, asyncID := atomA.atomos.asyncSet(func(out proto.Message, err error) {
		callbackInvoked.add()
		if err == nil || CodeOf(err) != ErrCodeAsyncCallbackCancelledByHalt {
			callbackConsumed.add()
		}
	})
	if startupID != process.startupID {
		t.Fatalf("startupID mismatch: %d != %d", startupID, process.startupID)
	}
	t.Logf("实例A注册回调: startupID=%d asyncID=%d", startupID, asyncID)

	// T1.5: 注册一个"在途"回调(不投递回复)——验证 Kill 时清账终结
	pendingInvoked := 0
	pendingCode := int64(0)
	atomA.atomos.asyncSet(func(out proto.Message, err error) {
		pendingInvoked++
		if err != nil {
			pendingCode = CodeOf(err)
		}
	})
	if n := atomA.atomos.AsyncCallbackPending(); n != 2 {
		t.Logf("注意: AsyncCallbackPending=%d (T1与T1.5共注册2个)", n)
	}

	// T3: Kill 实例 A(等价于空闲 1 分钟 KillSelf)
	if err := atomA.Kill(process.local, ArgBaseAtomosWaitKilled()); err != nil {
		t.Fatalf("kill atom failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// P3 清账断言: 在途回调在停机时以 ErrAsyncCallbackCancelledByHalt 终结,而非静默丢失
	if pendingInvoked != 1 {
		t.Errorf("清账未生效: 在途回调应被终结调用 1 次,实际 %d 次", pendingInvoked)
	} else if pendingCode != ErrCodeAsyncCallbackCancelledByHalt {
		t.Errorf("清账错误码不符: 期望 ErrAsyncCallbackCancelledByHalt,实际 %d", pendingCode)
	} else {
		t.Log("✅ 清账生效: 在途回调以 ErrAsyncCallbackCancelledByHalt 终结")
	}
	if n := atomA.atomos.AsyncCallbackPending(); n != 0 {
		t.Errorf("清账后 AsyncCallbackPending 应为 0,实际 %d", n)
	}

	// T3.5: 模拟长生命周期业务对象(如房间)持有已死实例的 self 继续发异步请求
	// (生产对应:业务对象离线保留时间较长,绑定的 atom 已被空闲清理)
	// 清账完成后到达的注册:不再入表静默丢失,而是同步以 ErrAsyncCallbackCancelledByHalt 终结
	var lostCallback reproCounter
	var lostErrCode reproErrCode
	atomA.atomos.asyncSet(func(out proto.Message, err error) {
		lostCallback.add()
		if err != nil {
			lostErrCode.set(err)
		}
	})
	// 日志管道异步落盘,轮询等待 Warn 出现
	waitDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(waitDeadline) && !logger.hasAny("asyncSet on halted atom", false, true) {
		time.Sleep(50 * time.Millisecond)
	}
	if !logger.hasAny("asyncSet on halted atom", false, true) {
		t.Errorf("未出现 'asyncSet on halted atom' Warn 日志(access)")
	} else {
		t.Log("✅ 死实例注册被 Warn 日志捕获: asyncSet on halted atom")
	}

	// T4: 模拟用户核心侧按名字再取(getAtomFromRemote)→ Halt 状态下按旧数据重生新实例 B
	// GetAtomData 返回上次保存的数据即可通过;这里让 GetAtomData 有数据:
	atomIDB, _, gerr := elem.GetAtomID(reproAtomName, nil)
	if gerr != nil {
		t.Fatalf("respawn atom failed: %v", gerr)
	}
	if atomIDB == atomIDA {
		t.Fatalf("expected new atom instance after recycle")
	}
	t.Logf("换代完成: A=%p -> B=%p (同名 %s)", atomIDA, atomIDB, reproAtomName)

	// T5: 模拟对端服务执行完毕,回复到达调用方进程侧的处理(生产:gRPC AsyncOnMessageCallback)。
	// 集群环境才有的 ToId 解析(需 etcd 注册 remoteCosmos)与本 bug 无关,
	// 从其后的等效点进入:按名字解析 callbackID(生产 getLocalIDFromIDInfo)→ asyncCallback 投递。
	svc := &atomosRemoteService{process: process}
	callbackIDInfo := &IDInfo{
		Type: IDType_Atom, Cosmos: "reproCosmos", Node: "reproNode",
		Element: reproElemName, Atom: reproAtomName,
	}
	callbackID := svc.getLocalIDFromIDInfo(callbackIDInfo)
	if callbackID == nil {
		t.Fatalf("按名字解析 callbackID 失败")
	}
	if callbackID == atomIDA {
		t.Fatalf("解析到旧实例 A,预期换代后的 B")
	}
	t.Logf("回复按名字解析到实例 B=%p (A=%p)", callbackID, atomIDA)
	callbackID.(idInternal).asyncCallback(callbackID, "ReproAsync", startupID, asyncID, nil, nil)
	time.Sleep(200 * time.Millisecond)

	// T6 断言: 报出 handler not found(带 instance 便于与注册实例比对),且回调未被消费
	if !logger.hasError("handler not found") || !logger.hasError("instance=") {
		t.Errorf("未复现: 期望出现带 instance= 的 'handler not found' 错误日志")
	} else {
		t.Log("✅ 复现成功: 新实例收到过期回复并报 handler not found (含 instance,可与注册日志比对实例)")
	}
	if got := callbackConsumed.get(); got != 0 {
		t.Errorf("回调不应被成功消费,实际消费 %d 次(仅允许清账终结)", got)
	}
	if got := lostCallback.get(); got != 1 {
		t.Errorf("停机后注册的回调应同步终结 1 次,实际 %d 次", got)
	} else if lostErrCode.get() != ErrCodeAsyncCallbackCancelledByHalt {
		t.Errorf("停机后注册的回调错误码不符: 期望 ErrAsyncCallbackCancelledByHalt,实际 %d", lostErrCode.get())
	} else {
		t.Log("✅ 清账后到达的注册被同步确定性终结(无泄漏、无静默丢失)")
	}

	// 对照组: 同一实例注册并投递 → 回调恰好一次,无报错
	var callbackInvoked2 reproCounter
	_, asyncID2 := atomIDB.(*AtomLocal).atomos.asyncSet(func(out proto.Message, err error) {
		callbackInvoked2.add()
	})
	callbackID.(idInternal).asyncCallback(callbackID, "ReproAsync", startupID, asyncID2, nil, nil)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && callbackInvoked2.get() == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if callbackInvoked2.get() != 1 {
		t.Errorf("对照组: 回调应恰好调用 1 次,实际 %d 次", callbackInvoked2.get())
	} else {
		t.Log("✅ 对照组通过: 未换代时回复正常消费")
	}
}
