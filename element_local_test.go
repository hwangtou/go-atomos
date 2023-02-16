package go_atomos

import (
	"testing"
	"time"
)

var sharedTestElement1 *ElementLocal

func TestElementLocalBase(t *testing.T) {
	var messages int

	initTestFakeCosmosProcess(t)
	if err := SharedCosmosProcess().Start(newTestFakeRunnable(t, false)); err != nil {
		t.Errorf("CosmosMain: Start failed. err=(%v)", err)
		return
	}
	process := SharedCosmosProcess()
	elemName := "testElement"
	testElem := process.main.elements[elemName]

	// Check Spawn state.
	if err := checkElementLocalInElement(t, process, elemName, AtomosWaiting); err != nil {
		t.Errorf("TestElementLocalBase: State invalid. err=(%v)", err)
		return
	}

	// Push Message without fromID.
	reply, err := testElem.pushMessageMail(nil, "testMessage", 0, nil)
	if err == nil || reply != nil {
		t.Errorf("TestElementLocalBase: Push Message without fromID succeed.")
		return
	}

	// Push Message.
	reply, err = testElem.pushMessageMail(process.main, "testMessage", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestElementLocalBase: Push Message failed. err=(%v)", err)
		return
	}
	messages += 1 // testMessage

	// Push Panic Message.
	reply, err = testElem.pushMessageMail(process.main, "testPanic", 0, nil)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" {
		t.Errorf("TestElementLocalBase: Push Panic Message failed. err=(%v)", err)
		return
	}
	messages += 1 // testPanic

	// Push Timeout Message.
	reply, err = testElem.pushMessageMail(process.main, "testMessageTimeout", 1*time.Millisecond, nil)
	if err == nil || err.Code != ErrAtomosPushTimeoutHandling {
		t.Errorf("TestElementLocalBase: Push Message Timeout failed. err=(%v)", err)
		return
	}
	messages += 1 // testMessageTimeout
	if err = checkElementLocalInElement(t, process, elemName, AtomosBusy); err != nil {
		t.Errorf("TestElementLocalBase: Push Message Timeout state invalid. err=(%v)", err)
		return
	}
	time.Sleep(2 * time.Millisecond)
	if err = checkElementLocalInElement(t, process, elemName, AtomosWaiting); err != nil {
		t.Errorf("TestElementLocalBase: Push Message Timeout state invalid. err=(%v)", err)
		return
	}

	// Push Deadlock Message.
	testAtomName := "testAtom"
	atom, tracker, err := testElem.SpawnAtom(testAtomName, &String{S: testAtomName}, 1)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Spawn failed. err=(%v)", err)
		return
	}
	sharedTestElement1 = testElem
	sharedTestAtom1 = atom
	reply, err = testElem.pushMessageMail(process.main, "testMessageDeadlock", 0, &String{S: testAtomName})
	if err == nil || err.Code != ErrAtomosCallDeadLock {
		t.Errorf("TestElementLocalBase: Push Message Deadlock failed. err=(%v)", err)
		return
	}
	if err = atom.pushKillMail(testElem, true, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Kill failed. err=(%v)", err)
		return
	}
	atom.Release(tracker)
	if err = checkAtomLocalInElement(t, testElem, testAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestElementLocalBase: Push Message Deadlock state invalid. err=(%v)", err)
		return
	}
	sharedTestElement1 = nil
	sharedTestAtom1 = nil
	messages += 1 // testMessageDeadlock

	// Push Tasking.
	reply, err = testElem.pushMessageMail(process.main, "testTask", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestElementLocalBase: Task Failed. state=(%d),err=(%v)", testElem.atomos.GetState(), err)
		return
	}
	messages += 1 // testTask
	messages += 2 // Task-Tasking
	if err = checkElementLocalInElement(t, process, elemName, AtomosBusy); err != nil {
		t.Errorf("TestElementLocalBase: Task Failed state invalid. err=(%v)", err)
		return
	}
	time.Sleep(10 * time.Millisecond)
	if err = checkElementLocalInElement(t, process, elemName, AtomosWaiting); err != nil {
		t.Errorf("TestElementLocalBase: Task Failed state invalid. err=(%v)", err)
		return
	}

	// Push Tasking Panic.
	reply, err = testElem.pushMessageMail(process.main, "testTaskPanic", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestElementLocalBase: Task Panic Failed. state=(%d),err=(%v)", testElem.atomos.GetState(), err)
		return
	}
	messages += 1 // testTaskPanic
	messages += 1 // Task-TaskingPanic
	time.Sleep(1 * time.Millisecond)
	if err = checkElementLocalInElement(t, process, elemName, AtomosWaiting); err != nil {
		t.Errorf("TestElementLocalBase: Task Panic Failed state invalid. err=(%v)", err)
		return
	}

	// Push Parallel.
	sharedTestElement1 = testElem
	reply, err = testElem.pushMessageMail(process.main, "testParallel", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestElementLocalBase: Parallel Failed. state=(%d),err=(%v)", testElem.atomos.GetState(), err)
		return
	}
	messages += 1 // testParallel
	sharedTestElement1 = nil
	if err = checkElementLocalInElement(t, process, elemName, AtomosWaiting); err != nil {
		t.Errorf("TestElementLocalBase: Parallel Failed state invalid. err=(%v)", err)
		return
	}
	// Message Tracker.
	messageCount := 0
	for _, info := range testElem.messageTracker.messages {
		messageCount += info.Count
	}
	if messages != messageCount {
		t.Errorf("TestAtomLocalBase: Message Tracker state invalid.")
		return
	}
	// TODO: 重新Spawn之后的Spawn统计时间不准确。
	t.Logf("TestAtomLocalBase: Meesage Tracker. spawn=(%v),run=(%v),stop=(%v),dump=(%v)",
		atom.messageTracker.spawnAt.Sub(atom.messageTracker.spawningAt),
		atom.messageTracker.stoppingAt.Sub(atom.messageTracker.spawnAt),
		atom.messageTracker.stoppedAt.Sub(atom.messageTracker.stoppingAt),
		atom.messageTracker.dump())
	//t.Logf("TestAtomLocalBase: Meesage Tracker. spawn=(%v),run=(%v),stop=(%v)", spawn, run, stop)

	if err = process.Stop(); err != nil {
		t.Errorf("TestAtomLocalBase: Process exit state invalid.")
		return
	}
	time.Sleep(10 * time.Millisecond)
}

func TestElementLocalScaleID(t *testing.T) {
	initTestFakeCosmosProcess(t)
	if err := SharedCosmosProcess().Start(newTestFakeRunnable(t, false)); err != nil {
		t.Errorf("CosmosMain: Start failed. err=(%v)", err)
		return
	}
	process := SharedCosmosProcess()
	elemName := "testElement"
	testElem := process.main.elements[elemName]
	testAtomName := "testAtom"

	// Check Spawn state.
	if err := checkElementLocalInElement(t, process, elemName, AtomosWaiting); err != nil {
		t.Errorf("TestElementLocalBase: State invalid. err=(%v)", err)
		return
	}

	atom, tracker, err := testElem.SpawnAtom(testAtomName, &String{S: testAtomName}, 1)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Spawn failed. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn waiting state invalid. err=(%v)", err)
		return
	}
	sharedTestAtom1 = atom

	scaleID, scaleTracker, err := testElem.pushScaleMail(process.main, "ScaleTestMessage", 0, nil, 1)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Get ScaleID failed. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 2); err != nil {
		t.Errorf("TestAtomLocalBase: Get ScaleID failed. err=(%v)", err)
		return
	}
	if scaleID != atom {
		t.Errorf("TestAtomLocalBase: Get ScaleID failed. err=(%v)", err)
		return
	}
	if scaleTracker.id == tracker.id {
		t.Errorf("TestAtomLocalBase: Get ScaleID failed. err=(%v)", err)
		return
	}
	scaleTracker.Release()
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Get ScaleID failed. err=(%v)", err)
		return
	}

	// Test Return Error.
	scaleID, scaleTracker, err = testElem.pushScaleMail(process.main, "ScaleTestMessageError", 0, nil, 1)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack != "" {
		t.Errorf("TestAtomLocalBase: Get ScaleID failed. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 2); err != nil {
		t.Errorf("TestAtomLocalBase: Get ScaleID failed. err=(%v)", err)
		return
	}

	// Test Return Panic.
	scaleID, scaleTracker, err = testElem.pushScaleMail(process.main, "ScaleTestMessagePanic", 0, nil, 1)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" {
		t.Errorf("TestAtomLocalBase: Get ScaleID failed. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 2); err != nil {
		t.Errorf("TestAtomLocalBase: Get ScaleID failed. err=(%v)", err)
		return
	}
}

func TestElementLocalLifeCycle(t *testing.T) {
	//elemName := "testElement"
	initTestFakeCosmosProcess(t)

	// Element Load Panic.
	sharedCosmosProcess = &CosmosProcess{}
	initCosmosMain(sharedCosmosProcess)
	runnable := newTestFakeRunnable(t, false)
	testElementLoadPanic = true
	err := SharedCosmosProcess().Start(runnable)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" {
		t.Errorf("CosmosMain: Start Spawn state invalid. err=(%v)", err)
		return
	}
	testElementLoadPanic = false

	// Element Spawn Panic.
	sharedCosmosProcess = &CosmosProcess{}
	initCosmosMain(sharedCosmosProcess)
	runnable = newTestFakeRunnable(t, true)
	testElementGetDataPanic = true
	err = SharedCosmosProcess().Start(runnable)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" {
		t.Errorf("CosmosMain: Start Spawn state invalid. err=(%v)", err)
		return
	}
	testElementGetDataPanic = false

	// Element Spawn Error.
	sharedCosmosProcess = &CosmosProcess{}
	initCosmosMain(sharedCosmosProcess)
	runnable = newTestFakeRunnable(t, true)
	testElementGetDataError = true
	err = SharedCosmosProcess().Start(runnable)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack != "" {
		t.Errorf("CosmosMain: Start Spawn state invalid. err=(%v)", err)
		return
	}
	testElementGetDataError = false

	// Element Spawn OK and Halt OK.
	sharedCosmosProcess = &CosmosProcess{}
	initCosmosMain(sharedCosmosProcess)
	runnable = newTestFakeRunnable(t, false)
	err = SharedCosmosProcess().Start(runnable)
	if err != nil || SharedCosmosProcess().state != CosmosProcessStateRunning {
		t.Errorf("CosmosMain: Life Cycle state invalid. err=(%v)", err)
		return
	}
	err = SharedCosmosProcess().Stop()
	if err != nil || SharedCosmosProcess().state != CosmosProcessStateOff {
		t.Errorf("CosmosMain: Life Cycle state invalid. err=(%v)", err)
		return
	}

	// Element Spawn OK and Halt Panic.
	sharedCosmosProcess = &CosmosProcess{}
	initCosmosMain(sharedCosmosProcess)
	runnable = newTestFakeRunnable(t, false)
	testElementHaltPanic = true
	err = SharedCosmosProcess().Start(runnable)
	if err != nil || SharedCosmosProcess().state != CosmosProcessStateRunning {
		t.Errorf("CosmosMain: Life Cycle state invalid. err=(%v)", err)
		return
	}
	err = SharedCosmosProcess().Stop()
	if err != nil || SharedCosmosProcess().state != CosmosProcessStateOff {
		t.Errorf("CosmosMain: Life Cycle state invalid. err=(%v)", err)
		return
	}
	testElementHaltPanic = false

	// Element Spawn OK and Unload Panic.
	sharedCosmosProcess = &CosmosProcess{}
	initCosmosMain(sharedCosmosProcess)
	runnable = newTestFakeRunnable(t, false)
	testElementUnloadPanic = true
	err = SharedCosmosProcess().Start(runnable)
	if err != nil || SharedCosmosProcess().state != CosmosProcessStateRunning {
		t.Errorf("CosmosMain: Life Cycle state invalid. err=(%v)", err)
		return
	}
	err = SharedCosmosProcess().Stop()
	if err != nil || SharedCosmosProcess().state != CosmosProcessStateOff {
		t.Errorf("CosmosMain: Life Cycle state invalid. err=(%v)", err)
		return
	}
	testElementUnloadPanic = false

	// Element Spawn OK and Set Data Error.
	sharedCosmosProcess = &CosmosProcess{}
	initCosmosMain(sharedCosmosProcess)
	runnable = newTestFakeRunnable(t, false)
	testElementSetDataError = true
	err = SharedCosmosProcess().Start(runnable)
	if err != nil || SharedCosmosProcess().state != CosmosProcessStateRunning {
		t.Errorf("CosmosMain: Life Cycle state invalid. err=(%v)", err)
		return
	}
	err = SharedCosmosProcess().Stop()
	if err != nil || SharedCosmosProcess().state != CosmosProcessStateOff {
		t.Errorf("CosmosMain: Life Cycle state invalid. err=(%v)", err)
		return
	}
	testElementSetDataError = false

	// Element Spawn OK and Set Data Panic.
	sharedCosmosProcess = &CosmosProcess{}
	initCosmosMain(sharedCosmosProcess)
	runnable = newTestFakeRunnable(t, false)
	testElementSetDataPanic = true
	err = SharedCosmosProcess().Start(runnable)
	if err != nil || SharedCosmosProcess().state != CosmosProcessStateRunning {
		t.Errorf("CosmosMain: Life Cycle state invalid. err=(%v)", err)
		return
	}
	err = SharedCosmosProcess().Stop()
	if err != nil || SharedCosmosProcess().state != CosmosProcessStateOff {
		t.Errorf("CosmosMain: Life Cycle state invalid. err=(%v)", err)
		return
	}
	testElementSetDataPanic = false
}

func checkElementLocalInElement(t *testing.T, process *CosmosProcess, name string, isState AtomosState) *Error {
	process.main.mutex.Lock()
	elem, has := process.main.elements[name]
	process.main.mutex.Unlock()
	if !has {
		t.Errorf("Element should exist.")
		return NewError(ErrFrameworkPanic, "Element should exist.").AddStack(nil)
	}
	// 需要在测试的atom的spawn预留一点时间才会成功。
	switch isState {
	case AtomosSpawning:
		if !elem.atomos.IsInState(AtomosSpawning) {
			return NewError(ErrFrameworkPanic, "Element should be spawning.").AddStack(nil)
		}
	case AtomosWaiting:
		if !elem.atomos.IsInState(AtomosWaiting) {
			return NewError(ErrFrameworkPanic, "Element should be waiting").AddStack(nil)
		}
	case AtomosBusy:
		if !elem.atomos.IsInState(AtomosBusy) {
			return NewError(ErrFrameworkPanic, "Element should be busy").AddStack(nil)
		}
	case AtomosStopping:
		if !elem.atomos.IsInState(AtomosStopping) {
			return NewError(ErrFrameworkPanic, "Element should be stopping").AddStack(nil)
		}
	}
	return nil
}
