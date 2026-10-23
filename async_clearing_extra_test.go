package atomos

// P1 修复的补充本地模拟场景:
// 1) 清账循环对 panic 回调的 recover——单个回调 panic 不得中断清账、不得击穿进程;
// 2) 回调注册与停机(Halt)的并发竞争——无 panic、无计数泄漏,终态 AsyncCallbackPending()==0。
//
// 移植自生产副本 async_clearing_extra_test.go,适配当前仓库测试基建:
// 使用 newCosmosProcess 直接建进程(不走 InitCosmosProcess 单例),
// AddElementImplementationWithSpawn 标记 spawn 入口,Spawner 闭包带 args 变参。

import (
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

type clearAtom struct{ name string }

func (a *clearAtom) String() string { return "clearAtom:" + a.name }
func (a *clearAtom) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}
func (a *clearAtom) GetAtomData(name string) (proto.Message, error) {
	return &Config{Cosmos: "data:" + name}, nil
}
func (a *clearAtom) SetAtomData(name string, data proto.Message) error { return nil }

type clearDev struct{}

func (d *clearDev) ElementConstructor() Atomos         { return &clearAtom{"elem"} }
func (d *clearDev) AtomConstructor(name string) Atomos { return &clearAtom{name} }
func (d *clearDev) AtomAutoData() AtomAutoData         { return &clearAtom{"autodata"} }
func (d *clearDev) ElementAutoData() ElementAutoData   { return nil }

type clearScript struct{}

func (s *clearScript) OnBoot(*CosmosProcess) error    { return nil }
func (s *clearScript) OnStartUp(*CosmosProcess) error { return nil }
func (s *clearScript) OnShutdown() error              { return nil }

func startClearCosmos(t *testing.T, logger *reproLogger) (*CosmosProcess, *ElementLocal) {
	t.Helper()
	process, cerr := newCosmosProcess("clearCosmos", "clearNode", logger)
	if cerr != nil {
		t.Fatalf("newCosmosProcess failed: %v", cerr.Message)
	}

	dev := &clearDev{}
	elemImpl := NewImplementationFromDeveloper(dev)
	elemImpl.Interface = NewInterfaceFromDeveloper("ClearElem", dev)
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
			Cosmos:         "clearCosmos",
			Node:           "clearNode",
			LogLevel:       LogLevel_Debug,
			EnableElements: []string{"ClearElem"},
		}).
		SetMainScript(&clearScript{}).
		AddElementImplementationWithSpawn(elemImpl)

	if err := process.Start(runnable); err != nil {
		t.Fatalf("process start failed: %v", err)
	}
	elem, err := process.local.getLocalElement("ClearElem")
	if err != nil {
		t.Fatalf("get element failed: %v", err.Message)
	}
	t.Cleanup(func() {
		_ = process.Stop()
	})
	return process, elem
}

// 场景 1:清账循环对 panic 回调的 recover。
func TestAsyncClearingRecoversPanickingCallback(t *testing.T) {
	logger := &reproLogger{}
	process, elem := startClearCosmos(t, logger)

	atomID, _, err := elem.SpawnAtom(process.local, "p1", &Config{Cosmos: "arg"}, nil)
	if err != nil {
		t.Fatalf("spawn atom failed: %v", err)
	}
	atom, ok := atomID.(*AtomLocal)
	if !ok {
		t.Fatalf("expected *AtomLocal, got %T", atomID)
	}

	panicCalled, normalCalled := 0, 0
	atom.atomos.asyncSet(func(out proto.Message, err error) {
		panicCalled++
		panic("boom: 清账回调 panic")
	})
	atom.atomos.asyncSet(func(out proto.Message, err error) {
		normalCalled++
	})

	if err := atom.Kill(process.local, ArgBaseAtomosWaitKilled()); err != nil {
		t.Fatalf("kill atom failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if panicCalled != 1 {
		t.Errorf("panic 回调应被终结调用 1 次,实际 %d 次", panicCalled)
	}
	if normalCalled != 1 {
		t.Errorf("清账 recover 失效: panic 之后其余在途回调未被继续终结,实际 %d 次", normalCalled)
	}
	if n := atom.atomos.AsyncCallbackPending(); n != 0 {
		t.Errorf("清账后 AsyncCallbackPending 应为 0,实际 %d", n)
	}
	if t.Failed() {
		return
	}
	t.Log("✅ 清账 recover 生效: 单个回调 panic 不中断清账循环,进程存活,计数归零")
}

// 场景 2:回调注册与停机的并发竞争。
func TestAsyncSetRaceWithHalt(t *testing.T) {
	logger := &reproLogger{}
	process, elem := startClearCosmos(t, logger)

	atomID, _, err := elem.SpawnAtom(process.local, "p2", &Config{Cosmos: "arg"}, nil)
	if err != nil {
		t.Fatalf("spawn atom failed: %v", err)
	}
	atom, ok := atomID.(*AtomLocal)
	if !ok {
		t.Fatalf("expected *AtomLocal, got %T", atomID)
	}

	const n = 64
	var mu sync.Mutex
	invoked := 0
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atom.atomos.asyncSet(func(out proto.Message, err error) {
				mu.Lock()
				invoked++
				mu.Unlock()
			})
		}()
	}
	time.Sleep(2 * time.Millisecond) // 让部分注册先落地,制造注册/清账交错

	_ = atom.Kill(process.local, ArgBaseAtomosWaitKilled())
	wg.Wait()
	time.Sleep(300 * time.Millisecond)

	if remain := atom.atomos.AsyncCallbackPending(); remain != 0 {
		t.Errorf("竞争终态 AsyncCallbackPending 应为 0,实际 %d", remain)
	}
	if invoked > n {
		t.Errorf("回调被调用 %d 次超过注册数 %d", invoked, n)
	}
	t.Logf("✅ 注册与停机竞争无 panic/无泄漏: %d 个并发注册中 %d 个被清账终结,其余注册落在停机后被拒(Warn)", n, invoked)
}
