package atomos

// Kill 唤醒排队 sync waiter 的回归测试。
// 移植自 preview/20261022 线 4f3edec 的 TestDeadlock_KillWakesQueuedSyncWaiter
// (deadlock_scenario_test.go),基建适配当前线:waitAllocMailDebugZero 带 *testing.T、
// allocMailDebug 用 atomic 开关、复用 repro_async_stale_test.go 的 reproLogger。

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// ---- 最小 Element/Atom 实现 ----

const kwElemName = "KillWakeElem"

type kwAtom struct {
	name string
}

func (a *kwAtom) String() string { return "kwAtom:" + a.name }
func (a *kwAtom) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

func (a *kwAtom) GetAtomData(name string) (proto.Message, error) {
	return &Config{Cosmos: "data:" + name}, nil
}
func (a *kwAtom) SetAtomData(name string, data proto.Message) error { return nil }

type kwDev struct{}

func (d *kwDev) ElementConstructor() Atomos         { return &kwAtom{name: "elem"} }
func (d *kwDev) AtomConstructor(name string) Atomos { return &kwAtom{name: name} }
func (d *kwDev) AtomAutoData() AtomAutoData         { return &kwAtom{name: "autodata"} }
func (d *kwDev) ElementAutoData() ElementAutoData   { return nil }

type kwMainScript struct{}

func (s *kwMainScript) OnBoot(local *CosmosProcess) error    { return nil }
func (s *kwMainScript) OnStartUp(local *CosmosProcess) error { return nil }
func (s *kwMainScript) OnShutdown() error                    { return nil }

// Kill 唤醒在途 sync waiter。
// victim 的 block handler 占住 mailbox;随后到达的 noop 排队。此时 Kill(AppendToHead+
// WaitKilled)排在 noop 之前。放行 block 后:执行中的 sync 正常返回;kill 触发停机,
// 排队中的 noop waiter 应以 ErrAtomosIsStopping 被唤醒,而非永久挂起;WaitKilled 及时返回。
func TestDeadlock_KillWakesQueuedSyncWaiter(t *testing.T) {
	allocMailUsingPool.Store(false)
	allocMailDebug.Store(true)
	clearAllocMailDebugMap()

	logger := &reproLogger{}
	process, cerr := newCosmosProcess("killWakeCosmos", "killWakeNode", logger)
	if cerr != nil {
		t.Fatalf("newCosmosProcess failed: %v", cerr.Message)
	}

	dev := &kwDev{}
	elemImpl := NewImplementationFromDeveloper(dev)
	elemImpl.Interface = NewInterfaceFromDeveloper(kwElemName, dev)
	elemImpl.Interface.ElementSpawner = func(s ElementSelfID, a Atomos, data proto.Message, args ...ArgsForBaseAtomos) error {
		return nil
	}
	elemImpl.Interface.AtomSpawner = func(s AtomSelfID, a Atomos, arg, data proto.Message, args ...ArgsForBaseAtomos) error {
		return nil
	}

	// block handler 的放行信号与进入通知(handler 在 mailbox goroutine 上执行,须同步)。
	blockCh := make(chan struct{})
	entered := make(chan struct{}, 1)
	elemImpl.ElementHandlers = map[string]MessageHandler{}
	elemImpl.AtomHandlers = map[string]MessageHandler{
		// 普通消息,用于充当排队中的 sync 调用。
		"noop": func(from ID, to Atomos, in proto.Message) (proto.Message, error) {
			return &String{S: "ok"}, nil
		},
		// block:进入后通知测试,阻塞直到测试放行。用于制造"执行中"的 atom。
		"block": func(from ID, to Atomos, in proto.Message) (proto.Message, error) {
			entered <- struct{}{}
			<-blockCh
			return &String{S: "ok"}, nil
		},
	}

	runnable := (&CosmosRunnable{}).
		SetConfig(&Config{
			Cosmos:         "killWakeCosmos",
			Node:           "killWakeNode",
			LogLevel:       LogLevel_Debug,
			EnableElements: []string{kwElemName},
		}).
		SetMainScript(&kwMainScript{}).
		AddElementImplementation(elemImpl, true)

	if err := process.Start(runnable); err != nil {
		for _, e := range logger.errs {
			t.Logf(" captured error log: %s", e)
		}
		t.Fatalf("process start failed: %v", err)
	}
	t.Cleanup(func() {
		_ = process.Stop()
	})

	elem, err := process.local.getLocalElement(kwElemName)
	if err != nil {
		t.Fatalf("get element failed: %v", err.Message)
	}

	id, _, serr := elem.SpawnAtom(process.local, "victim", &Config{Cosmos: "arg"}, nil, false)
	if serr != nil {
		t.Fatalf("spawn atom failed: %v", serr)
	}
	victim, ok := id.(*AtomLocal)
	if !ok {
		t.Fatalf("expected *AtomLocal, got %T", id)
	}

	type syncResult struct {
		out proto.Message
		err error
	}
	blockResult := make(chan syncResult, 1)
	noopResult := make(chan syncResult, 1)
	killResult := make(chan error, 1)

	// G1: 执行中的长耗时 sync(占住 mailbox)。
	go func() {
		out, err := victim.SyncMessagingByName(process.local, "block", nil)
		blockResult <- syncResult{out, err}
	}()
	// 等 block handler 真正进入(在 mailbox goroutine 上执行)。
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("block handler did not enter in time")
	}

	// G2: 排队中的 sync(等 block 完成才会被处理——除非 kill 先触发停机)。
	go func() {
		out, err := victim.SyncMessagingByName(process.local, "noop", nil)
		noopResult <- syncResult{out, err}
	}()
	// 给 G2 一点时间上送邮件进队列。
	time.Sleep(50 * time.Millisecond)

	// G3: kill 插到队首(在 noop 之前),并等待停机完成。
	go func() {
		killResult <- victim.Kill(process.local,
			ArgBaseAtomosAppendToHead(), ArgBaseAtomosWaitKilled())
	}()
	time.Sleep(50 * time.Millisecond)

	// 放行 block handler。
	close(blockCh)

	// 执行中的 sync 正常完成。
	select {
	case res := <-blockResult:
		if res.err != nil || res.out.(*String).S != "ok" {
			t.Fatalf("Expected in-flight block call to succeed, out=(%v),err=(%v)", res.out, res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("In-flight block call did not return after handler released")
	}

	// 排队中的 sync waiter 被停机唤醒,收到 ErrAtomosIsStopping。
	select {
	case res := <-noopResult:
		if res.err == nil {
			t.Fatal("Expected queued noop call to be woken with error, got nil error")
		} else if CodeOf(res.err) != ErrCodeAtomosIsStopping {
			t.Fatalf("Expected queued noop error code ErrAtomosIsStopping, got=(%d) err=(%v)", CodeOf(res.err), res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Queued noop sync waiter was not woken by kill (deadlock)")
	}

	// WaitKilled 及时返回。
	select {
	case err := <-killResult:
		if err != nil {
			t.Fatalf("Kill with WaitKilled returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Kill with WaitKilled did not return in time")
	}

	deadline := time.Now().Add(time.Second)
	for victim.State() != BaseAtomosHalt {
		if time.Now().After(deadline) {
			t.Fatalf("atom did not halt in time, state=(%v)", victim.State())
		}
		time.Sleep(time.Millisecond)
	}
	waitAllocMailDebugZero(t, 2*time.Second)
}
