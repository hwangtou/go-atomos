package atomos

// 死锁与生产场景模拟测试。
// 覆盖:
//  1. 互同步调用(A 的 handler 里 sync 调 B,B 的 handler 里 sync 调 A)的超时兜底与恢复
//  2. self-call(atom 在 handler 里 sync 调自己)的超时兜底与恢复
//  3. Kill 唤醒在途 sync waiter(排队中的邮件在停机时以 ErrAtomosIsStopping 回复,而非挂起)
//  4. AddToConcurrentQueue 立即任务在独立 goroutine 执行(验证立即任务队列路由修复)
//  5. 两个探测性测试,刻画"框架持锁执行用户 hook/回调"的已知自死锁限制
//
// 基建对齐 repro_async_stale_test.go:newCosmosProcess + runnable + AddElementImplementation,
// Spawner 闭包带 args ...ArgsForBaseAtomos 变参。

import (
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// ---- 共享注册表:handler 运行在 atom mailbox goroutine 上,与测试主 goroutine 并发,须加锁 ----

type dlRegistry struct {
	mu      sync.Mutex
	selfIDs map[string]AtomSelfID // atom name -> 自身 ID(spawner 时捕获)
	atomIDs map[string]ID         // atom name -> ID(spawn 后由测试填入)
	codes   map[string][]int64    // atom name -> handler 内层调用观察到的错误码(0 表示无错误)
	goIDs   map[string][]uint64   // atom name -> 并发任务实际执行的 goroutine id

	blockCh chan struct{} // "block" handler 的放行信号
	entered chan string   // "block" handler 进入通知
}

func newDLRegistry() *dlRegistry {
	return &dlRegistry{
		selfIDs: map[string]AtomSelfID{},
		atomIDs: map[string]ID{},
		codes:   map[string][]int64{},
		goIDs:   map[string][]uint64{},
		blockCh: make(chan struct{}),
		entered: make(chan string, 4),
	}
}

func (r *dlRegistry) setSelfID(name string, id AtomSelfID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.selfIDs[name] = id
}

func (r *dlRegistry) getSelfID(name string) AtomSelfID {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.selfIDs[name]
}

func (r *dlRegistry) setAtomID(name string, id ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.atomIDs[name] = id
}

func (r *dlRegistry) getAtomID(name string) ID {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.atomIDs[name]
}

func (r *dlRegistry) recordCode(name string, err *Error) {
	var code int64
	if err != nil {
		code = err.GetCode()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.codes[name] = append(r.codes[name], code)
}

func (r *dlRegistry) getCodes(name string) []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int64(nil), r.codes[name]...)
}

func (r *dlRegistry) recordGoID(name string, goID uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.goIDs[name] = append(r.goIDs[name], goID)
}

func (r *dlRegistry) getGoIDs(name string) []uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]uint64(nil), r.goIDs[name]...)
}

// ---- 最小 Element/Atom 实现 ----

const dlElemName = "DeadlockElem"

type dlAtom struct {
	name string
}

func (a *dlAtom) String() string { return "dlAtom:" + a.name }
func (a *dlAtom) Halt(from ID, cancelled []uint64) (save bool, data proto.Message) {
	return false, nil
}

// 模拟持久化存储,使元素满足 AutoData(Spawn 需要)。
var dlStore = map[string]proto.Message{}

func (a *dlAtom) GetAtomData(name string) (proto.Message, *Error) {
	return dlStore[name], nil
}
func (a *dlAtom) SetAtomData(name string, data proto.Message) *Error {
	dlStore[name] = data
	return nil
}

type dlDev struct{}

func (d *dlDev) ElementConstructor() Atomos         { return &dlAtom{name: "elem"} }
func (d *dlDev) AtomConstructor(name string) Atomos { return &dlAtom{name: name} }
func (d *dlDev) AtomAutoData() AtomAutoData         { return &dlAtom{name: "autodata"} }
func (d *dlDev) ElementAutoData() ElementAutoData   { return nil }

type dlMainScript struct{}

func (s *dlMainScript) OnBoot(local *CosmosProcess) *Error    { return nil }
func (s *dlMainScript) OnStartUp(local *CosmosProcess) *Error { return nil }
func (s *dlMainScript) OnShutdown() *Error                    { return nil }

// newDeadlockScenarioProcess 启动一个带 DeadlockElem 元素的进程,注册全部场景 handler。
func newDeadlockScenarioProcess(t *testing.T, reg *dlRegistry) (*CosmosProcess, *ElementLocal) {
	t.Helper()
	logger := &reproLogger{}
	process, cerr := newCosmosProcess("deadlockCosmos", "deadlockNode", logger)
	if cerr != nil {
		t.Fatalf("newCosmosProcess failed: %v", cerr.Message)
	}

	dev := &dlDev{}
	elemImpl := NewImplementationFromDeveloper(dev)
	elemImpl.Interface = NewInterfaceFromDeveloper(dlElemName, dev)
	elemImpl.Interface.ElementSpawner = func(s ElementSelfID, a Atomos, data proto.Message, args ...ArgsForBaseAtomos) *Error {
		return nil
	}
	elemImpl.Interface.AtomSpawner = func(s AtomSelfID, a Atomos, arg, data proto.Message, args ...ArgsForBaseAtomos) *Error {
		reg.setSelfID(a.(*dlAtom).name, s)
		return nil
	}
	elemImpl.ElementHandlers = map[string]MessageHandler{}
	elemImpl.AtomHandlers = map[string]MessageHandler{
		// 普通消息,用于验证超时/停机后 atom 状态恢复、邮箱仍可正常服务。
		"noop": func(from ID, to Atomos, in proto.Message) (proto.Message, *Error) {
			return &String{S: "ok"}, nil
		},
		// 互同步调用:handler 内 sync 调用 in 指定的目标 atom 的 noop。
		"mutual": func(from ID, to Atomos, in proto.Message) (proto.Message, *Error) {
			self := to.(*dlAtom).name
			target := in.(*String).S
			selfID := reg.getSelfID(self)
			targetID := reg.getAtomID(target)
			if selfID == nil || targetID == nil {
				return nil, NewErrorf(ErrFrameworkIncorrectUsage, "mutual: missing ids self=(%s),target=(%s)", self, target)
			}
			// 互同步调用:handler 内 sync 调用 in 指定的目标 atom 的 mutual(指回自己),
			// 形成 A→B→A 的互相占用。链条由超时截断,不会无限递归。
			// 首跳(来自 cosmos)给足超时,嵌套跳(来自 atom)用短超时——保证嵌套方
			// 必先超时(Reject),其错误经 handler 透传回首跳,时序确定。
			timeout := time.Second
			if from != nil && from.GetIDInfo().Type == IDType_Atom {
				timeout = 100 * time.Millisecond
			}
			_, err := targetID.SyncMessagingByName(selfID, "mutual", &String{S: self},
				[]ArgsForBaseAtomos{ArgBaseAtomosTimeout(timeout)})
			reg.recordCode(self, err)
			if err != nil {
				return nil, err
			}
			return &String{S: "ok"}, nil
		},
		// self-call:handler 内 sync 调用自己。记录内层错误码后对调用方返回 ok。
		"selfCall": func(from ID, to Atomos, in proto.Message) (proto.Message, *Error) {
			self := to.(*dlAtom).name
			selfID := reg.getSelfID(self)
			if selfID == nil {
				return nil, NewErrorf(ErrFrameworkIncorrectUsage, "selfCall: missing self id self=(%s)", self)
			}
			_, err := selfID.SyncMessagingByName(selfID, "noop", nil,
				[]ArgsForBaseAtomos{ArgBaseAtomosTimeout(100 * time.Millisecond)})
			reg.recordCode(self, err)
			return &String{S: "ok"}, nil
		},
		// block:进入后通知测试,阻塞直到测试放行。用于制造"执行中"的 atom。
		"block": func(from ID, to Atomos, in proto.Message) (proto.Message, *Error) {
			self := to.(*dlAtom).name
			reg.entered <- self
			<-reg.blockCh
			return &String{S: "ok"}, nil
		},
		// runConcurrent:handler 内连续提交 N 个立即并发任务,记录任务实际执行的 goroutine id。
		"runConcurrent": func(from ID, to Atomos, in proto.Message) (proto.Message, *Error) {
			self := to.(*dlAtom).name
			selfID := reg.getSelfID(self)
			if selfID == nil {
				return nil, NewErrorf(ErrFrameworkIncorrectUsage, "runConcurrent: missing self id self=(%s)", self)
			}
			const n = 8
			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				wg.Add(1)
				selfID.Task().AddToConcurrentQueue(func(taskID uint64) func() {
					reg.recordGoID(self, getGoID())
					wg.Done()
					return nil
				})
			}
			// handler 占着 atom mailbox goroutine;若立即并发任务被错误地投递回
			// 主邮箱(修复前行为),这里会自死锁直至外层超时。
			wg.Wait()
			return &String{S: "ok"}, nil
		},
	}

	runnable := (&CosmosRunnable{}).
		SetConfig(&Config{
			Cosmos:         "deadlockCosmos",
			Node:           "deadlockNode",
			LogLevel:       LogLevel_Debug,
			EnableElements: []string{dlElemName},
		}).
		SetMainScript(&dlMainScript{}).
		AddElementImplementation(elemImpl, true)

	if err := process.Start(runnable); err != nil {
		for _, e := range logger.errs {
			t.Logf(" captured error log: %s", e)
		}
		t.Fatalf("process start failed: %v", err.Message)
	}
	t.Cleanup(func() {
		_ = process.Stop()
	})

	elem, err := process.local.getLocalElement(dlElemName)
	if err != nil {
		t.Fatalf("get element failed: %v", err.Message)
	}
	return process, elem
}

// spawnDLAtom spawn 一个 atom 并把 ID 登记到注册表。
func spawnDLAtom(t *testing.T, process *CosmosProcess, elem *ElementLocal, reg *dlRegistry, name string) *AtomLocal {
	t.Helper()
	id, _, err := elem.SpawnAtom(process.local, name, &String{S: name}, nil, false)
	if err != nil {
		t.Fatalf("spawn atom %s failed: %v", name, err.Message)
	}
	atom, ok := id.(*AtomLocal)
	if !ok {
		t.Fatalf("expected *AtomLocal, got %T", id)
	}
	reg.setAtomID(name, atom)
	return atom
}

// waitDLAtomState 轮询等待 atom 进入指定状态。
func waitDLAtomState(t *testing.T, atom *AtomLocal, want BaseAtomosState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if got := atom.State(); got == want {
			return
		} else if time.Now().After(deadline) {
			t.Fatalf("atom %s state did not become %v in %v, current=(%v)", atom.GetIDInfo().Atom, want, timeout, got)
		}
		time.Sleep(time.Millisecond)
	}
}

// 场景 1: 互同步调用。
// alice 的 handler sync 调 bob,bob 的 handler sync 调 alice——双方 mailbox 都被占,
// 内层调用必然排队。期望:双方内层调用都以 ErrAtomosPushTimeoutReject 超时兜底(而非
// 永久死锁),双方状态恢复 Waiting,后续普通消息正常处理,无邮件泄漏。
func TestDeadlock_MutualSyncCallsTimeoutFallback(t *testing.T) {
	setAllocMailUsingPool(false)
	setAllocMailDebug(true)
	clearAllocMailDebugMap()

	reg := newDLRegistry()
	process, elem := newDeadlockScenarioProcess(t, reg)

	alice := spawnDLAtom(t, process, elem, reg, "alice")
	bob := spawnDLAtom(t, process, elem, reg, "bob")

	// 外层调用:alice 处理 mutual 时会 sync 调 bob,bob 处理时会 sync 调回 alice。
	_, err := alice.SyncMessagingByName(process.local, "mutual", &String{S: "bob"},
		[]ArgsForBaseAtomos{ArgBaseAtomosTimeout(3 * time.Second)})
	if err == nil {
		t.Fatal("Expected outer mutual call to return propagated timeout error, got nil error")
	} else if err.Code != ErrAtomosPushTimeoutReject {
		t.Fatalf("Expected outer error code ErrAtomosPushTimeoutReject, got=(%d) err=(%v)", err.Code, err)
	}

	// 双方 handler 的内层调用都应观察到超时拒绝:
	// bob 对 alice 的调用排队超时(Reject);alice 对 bob 的调用收到 bob 透传的同一错误。
	for _, name := range []string{"alice", "bob"} {
		codes := reg.getCodes(name)
		if len(codes) != 1 {
			t.Fatalf("Expected 1 inner call record for %s, got=(%v)", name, codes)
		}
		if codes[0] != ErrAtomosPushTimeoutReject {
			t.Fatalf("Expected inner call of %s to fail with ErrAtomosPushTimeoutReject, got=(%d)", name, codes[0])
		}
	}

	// 状态恢复:双方都回到 Waiting,且能正常处理后续消息。
	waitDLAtomState(t, alice, BaseAtomosWaiting, time.Second)
	waitDLAtomState(t, bob, BaseAtomosWaiting, time.Second)
	for _, atom := range []*AtomLocal{alice, bob} {
		out, err := atom.SyncMessagingByName(process.local, "noop", nil,
			[]ArgsForBaseAtomos{ArgBaseAtomosTimeout(time.Second)})
		if err != nil || out.(*String).S != "ok" {
			t.Fatalf("Expected noop to succeed after timeout recovery, out=(%v),err=(%v)", out, err)
		}
	}

	// 无邮件泄漏:超时的内层邮件应已被 removeMail+release,正常邮件处理完即释放。
	if n := waitAllocMailDebugZero(2 * time.Second); n != 0 {
		t.Fatal("Expected 0 allocated mails after mutual sync timeout, got", n, getAllAllocMailDebugInfo())
	}
}

// 场景 2: self-call。
// atom 在 handler 里 sync 调自己——邮件排进自己的 mailbox,而 mailbox 正被当前 handler
// 占用。期望:内层调用以 ErrAtomosPushTimeoutReject 超时兜底,atom 恢复 Waiting,
// 后续消息正常,无邮件泄漏。
func TestDeadlock_SelfSyncCallTimeoutFallback(t *testing.T) {
	setAllocMailUsingPool(false)
	setAllocMailDebug(true)
	clearAllocMailDebugMap()

	reg := newDLRegistry()
	process, elem := newDeadlockScenarioProcess(t, reg)

	solo := spawnDLAtom(t, process, elem, reg, "solo")

	out, err := solo.SyncMessagingByName(process.local, "selfCall", nil,
		[]ArgsForBaseAtomos{ArgBaseAtomosTimeout(3 * time.Second)})
	if err != nil || out.(*String).S != "ok" {
		t.Fatalf("Expected selfCall outer reply ok, out=(%v),err=(%v)", out, err)
	}

	codes := reg.getCodes("solo")
	if len(codes) != 1 {
		t.Fatalf("Expected 1 inner call record for solo, got=(%v)", codes)
	}
	if codes[0] != ErrAtomosPushTimeoutReject {
		t.Fatalf("Expected self inner call to fail with ErrAtomosPushTimeoutReject, got=(%d)", codes[0])
	}

	waitDLAtomState(t, solo, BaseAtomosWaiting, time.Second)
	out, err = solo.SyncMessagingByName(process.local, "noop", nil,
		[]ArgsForBaseAtomos{ArgBaseAtomosTimeout(time.Second)})
	if err != nil || out.(*String).S != "ok" {
		t.Fatalf("Expected noop to succeed after self-call timeout, out=(%v),err=(%v)", out, err)
	}

	if n := waitAllocMailDebugZero(2 * time.Second); n != 0 {
		t.Fatal("Expected 0 allocated mails after self-call timeout, got", n, getAllAllocMailDebugInfo())
	}
}

// 场景 3: Kill 唤醒在途 sync waiter。
// victim 的 block handler 占住 mailbox;随后到达的 noop 排队。此时 Kill(AppendToHead+
// WaitKilled)排在 noop 之前。放行 block 后:执行中的 sync 正常返回;kill 触发停机,
// 排队中的 noop waiter 应以 ErrAtomosIsStopping 被唤醒,而非永久挂起;WaitKilled 及时返回。
func TestDeadlock_KillWakesQueuedSyncWaiter(t *testing.T) {
	setAllocMailUsingPool(false)
	setAllocMailDebug(true)
	clearAllocMailDebugMap()

	reg := newDLRegistry()
	process, elem := newDeadlockScenarioProcess(t, reg)

	victim := spawnDLAtom(t, process, elem, reg, "victim")

	type syncResult struct {
		out proto.Message
		err *Error
	}
	blockResult := make(chan syncResult, 1)
	noopResult := make(chan syncResult, 1)
	killResult := make(chan *Error, 1)

	// G1: 执行中的长耗时 sync(占住 mailbox)。
	go func() {
		out, err := victim.SyncMessagingByName(process.local, "block", nil, nil)
		blockResult <- syncResult{out, err}
	}()
	// 等 block handler 真正进入(在 mailbox goroutine 上执行)。
	select {
	case <-reg.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("block handler did not enter in time")
	}

	// G2: 排队中的 sync(等 block 完成才会被处理——除非 kill 先触发停机)。
	go func() {
		out, err := victim.SyncMessagingByName(process.local, "noop", nil, nil)
		noopResult <- syncResult{out, err}
	}()
	// 给 G2 一点时间上送邮件进队列。
	time.Sleep(50 * time.Millisecond)

	// G3: kill 插到队首(在 noop 之前),并等待停机完成。
	go func() {
		killResult <- victim.Kill(process.local,
			[]ArgsForBaseAtomos{ArgBaseAtomosAppendToHead(), ArgBaseAtomosWaitKilled()})
	}()
	time.Sleep(50 * time.Millisecond)

	// 放行 block handler。
	close(reg.blockCh)

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
		} else if res.err.Code != ErrAtomosIsStopping {
			t.Fatalf("Expected queued noop error code ErrAtomosIsStopping, got=(%d) err=(%v)", res.err.Code, res.err)
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

	waitDLAtomState(t, victim, BaseAtomosHalt, time.Second)
	if n := waitAllocMailDebugZero(2 * time.Second); n != 0 {
		t.Fatal("Expected 0 allocated mails after kill, got", n, getAllAllocMailDebugInfo())
	}
}

// 场景 4: AddToConcurrentQueue 立即任务的执行位置(回归立即任务队列路由修复)。
// 修复前立即任务被投递到 atomos 主邮箱:任务在 atom 自己的 mailbox goroutine 上执行,
// 且 handler 内 wg.Wait() 会直接自死锁;修复后任务投递到独立的并发队列 goroutine。
func TestDeadlock_ConcurrentQueueImmediateTaskRunsOffAtomGoroutine(t *testing.T) {
	setAllocMailUsingPool(false)
	setAllocMailDebug(true)
	clearAllocMailDebugMap()

	reg := newDLRegistry()
	process, elem := newDeadlockScenarioProcess(t, reg)

	worker := spawnDLAtom(t, process, elem, reg, "worker")
	testGoID := getGoID()

	out, err := worker.SyncMessagingByName(process.local, "runConcurrent", nil,
		[]ArgsForBaseAtomos{ArgBaseAtomosTimeout(3 * time.Second)})
	if err != nil || out.(*String).S != "ok" {
		t.Fatalf("Expected runConcurrent to succeed, out=(%v),err=(%v)", out, err)
	}

	atomGoID := worker.atomos.GetGoID()
	goIDs := reg.getGoIDs("worker")
	if len(goIDs) != 8 {
		t.Fatalf("Expected 8 concurrent task records, got=(%d)", len(goIDs))
	}
	for _, id := range goIDs {
		if id == atomGoID {
			t.Fatalf("Immediate concurrent task ran on the atom mailbox goroutine goID=(%d); immediate tasks must run on their own queue goroutine", id)
		}
		if id == testGoID {
			t.Fatalf("Immediate concurrent task ran on the test goroutine goID=(%d)", id)
		}
	}

	// 并发任务完成后,其临时 mailbox goroutine 应由 stopIfNoMail 回收,
	// 常驻 goroutine 数不应随任务数线性增长。
	before := len(dumpAllGoID())
	deadline := time.Now().Add(2 * time.Second)
	after := before
	for {
		after = len(dumpAllGoID())
		if after <= before+2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Goroutine count grew after concurrent tasks: before=(%d) after=(%d); concurrent task mailboxes should be recycled", before, after)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if n := waitAllocMailDebugZero(2 * time.Second); n != 0 {
		t.Fatal("Expected 0 allocated mails after concurrent tasks, got", n, getAllAllocMailDebugInfo())
	}
}

// 场景 5a(探测性): 生命周期 hook 在 mailbox.mutex 临界区内执行。
// setSpawn/setSpawning/setStopping/setHalted 持 mailbox.mutex 调用 process.onID* →
// 用户 hook;hook 内若调用同一实例上需要 mailbox.mutex 的方法(如 State()/GetState()),
// 会在同一 goroutine 上自死锁。
//
// 评估结论:把 hook 移出临界区会改变"状态变更→hook"的可见性顺序(并影响 tracker 回调
// 时序),回归风险大于收益,生产代码暂不改动。本测试刻画当前行为——hook 内的 State()
// 会阻塞;用 channel+超时探测,不挂 CI。若未来修复(如 hook 异步化或移出临界区),
// 本测试会失败,届时请更新实现并反转断言。
func TestDeadlock_HookUnderMailboxLock_Probe(t *testing.T) {
	p := newTestCosmosProcessWithoutCluster(t, "probeCosmos", "probeNode")
	id := &IDInfo{Type: IDType_Atom, Cosmos: "probeCosmos", Node: "probeNode", Element: "probeElem", Atom: "probeAtom"}

	tba := &testBaseAtomos{t: t, process: p}
	ti := &testInstance{}
	ba := NewBaseAtomos(tba, id, LogLevel_Debug, tba, ti, p)
	tba.atomos = ba
	tba.instance = ti

	hookEntered := make(chan struct{})
	hookReturned := make(chan struct{})
	// spawnHook 在 setSpawn 的 mailbox.mutex 临界区内被调用;
	// hook 里再调 GetState()(需要同一把锁)即自死锁。
	p.local.runnable = &CosmosRunnable{
		spawnHook: func(id *IDInfo) {
			close(hookEntered)
			_ = ba.GetState()
			close(hookReturned)
		},
	}

	setSpawnDone := make(chan struct{})
	go func() {
		ba.setSpawn()
		close(setSpawnDone)
	}()

	select {
	case <-hookEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("spawn hook was not invoked")
	}

	select {
	case <-hookReturned:
		// 行为已改变:hook 不再在锁内执行(或 State() 不再抢同一把锁)。
		// 说明已知限制已修复,请更新本测试为正向断言。
		t.Fatal("KNOWN LIMITATION FIXED? hook calling State() did not block under mailbox.mutex; update this probe to assert the new behavior")
	case <-time.After(300 * time.Millisecond):
		t.Log("Confirmed known limitation: lifecycle hook runs under mailbox.mutex; calling State() inside the hook self-deadlocks")
	}

	select {
	case <-setSpawnDone:
		// 同上:若 hook 能返回,setSpawn 应很快完成。
		t.Fatal("KNOWN LIMITATION FIXED? setSpawn completed despite hook calling State(); update this probe")
	case <-time.After(100 * time.Millisecond):
	}
	// 泄漏说明:hook goroutine 与 setSpawn goroutine 永久阻塞在 mailbox.mutex 上,
	// 这是探测"阻塞"语义的代价;测试对象独立,不影响其他用例。
}

// 场景 5b(探测性): mailboxOnStop 持 task 互斥锁(stopLock)执行用户停机回调。
// AtomLocal.OnStopping → 用户 Halt() 在 task.mutex 临界区内运行;回调里若调用
// self.Task().Add/Cancel 等需要同一把锁的方法,会在同一 goroutine 上自死锁。
//
// 评估结论:停机流程依赖 stopLock 阻止"停机期间新任务入队",把用户回调移出临界区会
// 重新打开该窗口,生产代码暂不改动。本测试刻画当前行为,说明同场景 5a。
func TestDeadlock_StopHookUnderTaskLock_Probe(t *testing.T) {
	p := newTestCosmosProcessWithoutCluster(t, "probeCosmos2", "probeNode2")
	id := &IDInfo{Type: IDType_Atom, Cosmos: "probeCosmos2", Node: "probeNode2", Element: "probeElem", Atom: "probeAtom"}

	tba := &testBaseAtomos{t: t, process: p}
	ti := &testInstance{}
	holder := &probeStopHolder{}
	ba := NewBaseAtomos(tba, id, LogLevel_Debug, holder, ti, p)
	tba.atomos = ba
	tba.instance = ti
	holder.ba = ba
	if err := ba.start(func() *Error { return nil }); err != nil {
		t.Fatalf("Failed to start BaseAtomos: %v", err)
	}

	killDone := make(chan *Error, 1)
	go func() {
		killDone <- tba.Kill(p.local, []ArgsForBaseAtomos{ArgBaseAtomosWaitKilled()})
	}()

	select {
	case err := <-killDone:
		// 行为已改变:停机回调不再持 task 锁执行。请更新本测试为正向断言。
		t.Fatalf("KNOWN LIMITATION FIXED? kill returned (%v) despite Halt calling Task().Add under task lock; update this probe", err)
	case <-time.After(500 * time.Millisecond):
		t.Log("Confirmed known limitation: mailboxOnStop holds task lock while running user stop callback; calling Task().Add inside it self-deadlocks")
	}
}

// probeStopHolder 复用 testBaseAtomos 的全部实现,仅覆盖 OnStopping:
// 在停机回调里调用 Task().Add,触发"持 task 锁执行用户回调"的自死锁。
type probeStopHolder struct {
	*testBaseAtomos
	ba *BaseAtomos
}

func (h *probeStopHolder) OnStopping(from ID, cancelled []uint64) *Error {
	h.ba.Task().Add(func(taskID uint64) {})
	return nil
}
