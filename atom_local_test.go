package go_atomos

import (
	"testing"
	"time"
)

var sharedTestAtom1, sharedTestAtom2 *AtomLocal

func TestAtomLocalBase(t *testing.T) {
	var hasSpawning, hasSpawn, hasStopping, hasHalted bool
	var messages int
	var spawn, run, stop time.Duration

	initTestFakeCosmosProcess(t)
	runnable := newTestFakeRunnable(t, false)
	runnable.hookAtomSpawning = func(elem string, name string) {
		hasSpawning = true
	}
	runnable.hookAtomSpawn = func(elem string, name string) {
		hasSpawn = true
	}
	runnable.hookAtomStopping = func(elem string, name string) {
		hasStopping = true
	}
	runnable.hookAtomHalt = func(elem string, name string) {
		hasHalted = true
	}
	if err := SharedCosmosProcess().Start(runnable); err != nil {
		t.Errorf("TestAtomLocalBase: Start failed. err=(%v)", err)
		return
	}
	testElemName := "testElement"
	testAtomName := "testAtom"
	testDeadlockAtomName := "testDeadlockAtom"

	process := SharedCosmosProcess()
	testElem, err := process.main.getElement(testElemName)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Element not found. name=(%s)", testElemName)
		return
	}

	// Spawn an atom.
	atom, tracker, err := testElem.SpawnAtom(testAtomName, &String{S: testAtomName}, 1)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Spawn failed. err=(%v)", err)
		return
	}
	if !(hasSpawning && hasSpawn) && (hasStopping && hasHalted) {
		t.Errorf("TestAtomLocalBase: Spawn state invalid.")
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn waiting state invalid. err=(%v)", err)
		return
	}

	// Get an atom.
	checkAtom, checkTracker, err := testElem.GetAtomID(testAtomName, 1)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Get atom failed. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 2); err != nil {
		t.Errorf("TestAtomLocalBase: Get atom failed. err=(%v)", err)
		return
	}
	checkAtom.(*AtomLocal).Release(checkTracker)

	// Spawn an atom.
	deadlockAtom, deadlockTracker, err := testElem.SpawnAtom(testDeadlockAtomName, &String{S: testDeadlockAtomName}, 1)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Spawn deadlock atom failed. err=(%v)", err)
		return
	}
	deadlockAtom.Release(deadlockTracker)
	if err = checkAtomLocalInElement(t, testElem, testDeadlockAtomName, false, AtomosWaiting, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn waiting state invalid. err=(%v)", err)
		return
	}

	// Push Message without fromID.
	reply, err := atom.pushMessageMail(nil, "testMessage", 0, nil)
	if err == nil || reply != nil {
		t.Errorf("TestAtomLocalBase: Push Message without fromID succeed.")
		return
	}

	// Push Message.
	reply, err = atom.pushMessageMail(testElem, "testMessage", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Push Message failed. err=(%v)", err)
		return
	}
	messages += 1 // testMessage

	// Push Panic Message.
	reply, err = atom.pushMessageMail(testElem, "testPanic", 0, nil)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" {
		t.Errorf("TestAtomLocalBase: Push Panic Message failed. err=(%v)", err)
		return
	}
	messages += 1 // testPanic

	// Push Timeout Message.
	reply, err = atom.pushMessageMail(testElem, "testMessageTimeout", 1*time.Millisecond, nil)
	if err == nil || err.Code != ErrAtomosPushTimeoutHandling {
		t.Errorf("TestAtomLocalBase: Push Message Timeout failed. err=(%v)", err)
		return
	}
	messages += 1 // testMessageTimeout
	// Push Timeout Reject Mail.
	reply, err = atom.pushMessageMail(testElem, "testMessage", 1*time.Microsecond, nil)
	if err == nil || err.Code != ErrAtomosPushTimeoutReject {
		t.Errorf("TestAtomLocalBase: Push Message Timeout failed. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosBusy, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Push Message Timeout state invalid. err=(%v)", err)
		return
	}
	time.Sleep(5 * time.Millisecond)
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Push Message Timeout state invalid. err=(%v)", err)
		return
	}

	// Push Deadlock Message.
	sharedTestAtom1 = atom
	sharedTestAtom2 = deadlockAtom
	reply, err = atom.pushMessageMail(testElem, "testMessageDeadlock", 0, &String{S: testAtomName})
	if err == nil || err.Code != ErrAtomosCallDeadLock {
		t.Errorf("TestAtomLocalBase: Push Message Deadlock failed.")
		return
	}
	messages += 1 // testMessageDeadlock
	sharedTestAtom1 = nil
	sharedTestAtom2 = nil
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Push Message Deadlock state invalid. err=(%v)", err)
		return
	}

	// Push Tasking.
	reply, err = atom.pushMessageMail(testElem, "testTask", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Task Failed. state=(%d),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	messages += 1 // testTask
	messages += 2 // Task-Tasking
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosBusy, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Task Failed state invalid. err=(%v)", err)
		return
	}
	time.Sleep(10 * time.Millisecond)
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Task Failed state invalid. err=(%v)", err)
		return
	}

	// Push Tasking Panic.
	reply, err = atom.pushMessageMail(testElem, "testTaskPanic", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Task Panic Failed. state=(%d),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	messages += 1 // testTaskPanic
	messages += 1 // Task-TaskingPanic
	time.Sleep(1 * time.Millisecond)
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Task Panic Failed state invalid. err=(%v)", err)
		return
	}

	// Push Parallel.
	sharedTestAtom1 = atom
	reply, err = atom.pushMessageMail(testElem, "testParallel", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Parallel Failed. state=(%d),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	messages += 1 // testParallel
	sharedTestAtom1 = nil
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Parallel Failed state invalid. err=(%v)", err)
		return
	}

	// Try Spawning twice.
	atomTwice, trackerTwice, err := testElem.SpawnAtom(testAtomName, &String{S: testAtomName}, 1)
	if err == nil || err.Code != ErrAtomExists {
		t.Errorf("TestAtomLocalBase: Spawn twice state invalid. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 2); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn twice state invalid. err=(%v)", err)
		return
	}
	// Try kill self.
	sharedTestAtom1 = atom
	reply, err = atom.pushMessageMail(testElem, "testKillSelf", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: KillSelf Failed. state=(%d),err=(%v)", atom.atomos.GetState(), err)
		return
	}
	messages += 1 // testKillSelf
	sharedTestAtom1 = nil
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosHalt, 2); err != nil {
		t.Errorf("TestAtomLocalBase: KillSelf state invalid. err=(%v)", err)
		return
	}
	atomTwice.Release(trackerTwice)
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosHalt, 1); err != nil {
		t.Errorf("TestAtomLocalBase: KillSelf state invalid. err=(%v)", err)
		return
	}

	// Message Tracker.
	messageCount := 0
	for _, info := range atom.messageTracker.messages {
		messageCount += info.Count
	}
	if messages != messageCount {
		t.Errorf("TestAtomLocalBase: Message Tracker state invalid.")
		return
	}

	// Spawn again.
	start := time.Now()
	for {
		// Because the atom may be stopping, so we have to retry.
		atomTwice, trackerTwice, err = testElem.SpawnAtom(testAtomName, &String{S: testAtomName}, 1)
		if err != nil {
			if err.Code == ErrAtomIsStopping {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			t.Errorf("TestAtomLocalBase: Spawn twice state invalid. err=(%v)", err)
			return
		}
		break
	}
	spawn = time.Now().Sub(start)
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 2); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn twice state invalid. err=(%v)", err)
		return
	}
	// Try message.
	reply, err = atom.pushMessageMail(testElem, "testMessage", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Spawn twice Push Message failed. err=(%v)", err)
		return
	}
	messages = 1 // testMessage
	reply, err = atomTwice.pushMessageMail(testElem, "testMessage", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Spawn twice Push Message failed. err=(%v)", err)
		return
	}
	messages += 1 // testMessage
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 2); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn twice state invalid. err=(%v)", err)
		return
	}
	atomTwice.Release(trackerTwice)
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn twice release state invalid. err=(%v)", err)
		return
	}

	// Reference Count.
	// Test 0 reference count release.
	if err = checkAtomLocalInElement(t, testElem, testDeadlockAtomName, false, AtomosWaiting, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Reference count state invalid. err=(%v)", err)
		return
	}
	if err = deadlockAtom.pushKillMail(testElem, true, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Push Kill failed. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testDeadlockAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Reference count state invalid. err=(%v)", err)
		return
	}

	// Reference Count.
	// Test non-0 reference count release.
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Reference count state invalid. err=(%v)", err)
		return
	}
	run = time.Now().Sub(start) - spawn
	if err = atom.pushKillMail(testElem, true, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Reference count push kill failed. err=(%v)", err)
	}
	stop = time.Now().Sub(start) - spawn - run
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosHalt, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Reference count state invalid. err=(%v)", err)
		return
	}
	reply, err = atom.pushMessageMail(testElem, "testMessage", 0, nil)
	if err == nil || err.Code != ErrAtomosIsNotRunning {
		t.Errorf("TestAtomLocalBase: Reference count push message state invalid. err=(%v)", err)
		return
	}
	atom.Release(tracker)
	if err = checkAtomLocalInElement(t, testElem, testAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Reference count state invalid. err=(%v)", err)
		return
	}
	// Check ID Tracker.
	if atom.idTracker.RefCount() != 0 {
		t.Errorf("TestAtomLocalBase: ID Tracker state invalid.")
		return
	}
	// Message Tracker.
	messageCount = 0
	for _, info := range atom.messageTracker.messages {
		messageCount += info.Count
	}
	if messages != messageCount {
		t.Errorf("TestAtomLocalBase: Message Tracker state invalid.")
		return
	}
	t.Logf("TestAtomLocalBase: Meesage Tracker. spawn=(%v),run=(%v),stop=(%v),dump=(%v)",
		atom.messageTracker.spawnAt.Sub(atom.messageTracker.spawningAt),
		atom.messageTracker.stoppingAt.Sub(atom.messageTracker.spawnAt),
		atom.messageTracker.stoppedAt.Sub(atom.messageTracker.stoppingAt),
		atom.messageTracker.Dump())
	t.Logf("TestAtomLocalBase: Meesage Tracker. spawn=(%v),run=(%v),stop=(%v)", spawn, run, stop)

	// Spawn panic.
	spawnPanicAtomName := "spawn_panic"
	spawnPanicAtom, trackerSpawnPanicAtom, err := testElem.SpawnAtom(spawnPanicAtomName, &String{S: "panic"}, 1)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" {
		t.Errorf("TestAtomLocalBase: Spawn panic atom state invalid.")
		return
	}
	if spawnPanicAtom != nil || trackerSpawnPanicAtom != nil {
		t.Errorf("TestAtomLocalBase: Spawn panic atom state invalid.")
		return
	}
	if err = checkAtomLocalInElement(t, testElem, spawnPanicAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn panic atom state invalid. err=(%v)", err)
		return
	}

	// Stopping panic.
	stoppingPanicAtomName := "stopping_panic"
	stoppingPanicAtom, trackerStoppingPanicAtom, err := testElem.SpawnAtom(stoppingPanicAtomName, nil, 1)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Stopping panic atom failed. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, stoppingPanicAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Stopping panic atom state invalid. err=(%v)", err)
		return
	}
	err = stoppingPanicAtom.pushKillMail(testElem, true, 0)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" {
		t.Errorf("TestAtomLocalBase: Stopping panic atom push kill state invalid. err=(%v)", err)
		return
	}
	stoppingPanicAtom.Release(trackerStoppingPanicAtom)
	if err = checkAtomLocalInElement(t, testElem, stoppingPanicAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Stopping panic atom state invalid. err=(%v)", err)
		return
	}

	// Auto Data Persistence
	runnable.implements["testElement"].Developer = &testElementAutoDataDev{}

	// GetAtomData returns Error.
	dataAtomName := "get_data_error"
	dataAtom, trackerDataAtom, err := testElem.SpawnAtom(dataAtomName, nil, 1)
	if err == nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if dataAtom != nil || trackerDataAtom != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, dataAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}

	// GetAtomData returns Panic.
	dataAtomName = "get_data_panic"
	dataAtom, trackerDataAtom, err = testElem.SpawnAtom(dataAtomName, nil, 1)
	if err == nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if dataAtom != nil || trackerDataAtom != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, dataAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}

	// GetAtomData returns OK.
	dataAtomName = "data_ok"
	dataAtom, trackerDataAtom, err = testElem.SpawnAtom(dataAtomName, nil, 1)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, dataAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	inner := dataAtom.atomos.instance.(*testAtom)
	if inner.data.(*String).S != dataAtomName {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}

	// SetAtomData returns OK.
	err = dataAtom.pushKillMail(testElem, true, 0)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if setAtomCounter != 1 {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	dataAtom.Release(trackerDataAtom)

	// SetAtomData returns Error.
	dataAtomName = "set_data_error"
	dataAtom, trackerDataAtom, err = testElem.SpawnAtom(dataAtomName, nil, 1)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	err = dataAtom.pushKillMail(testElem, true, 0)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack != "" || len(err.CallStacks[1].Args) == 0 {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	dataAtom.Release(trackerDataAtom)
	if err = checkAtomLocalInElement(t, testElem, dataAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}

	// SetAtomData returns panic.
	dataAtomName = "set_data_panic"
	dataAtom, trackerDataAtom, err = testElem.SpawnAtom(dataAtomName, nil, 1)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	err = dataAtom.pushKillMail(testElem, true, 0)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" || len(err.CallStacks[0].Args) == 0 {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	dataAtom.Release(trackerDataAtom)
	if err = checkAtomLocalInElement(t, testElem, dataAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}

	if err := SharedCosmosProcess().Stop(); err != nil {
		t.Errorf("TestAtomLocalBase: Process state invalid. err=(%v)", err)
		return
	}
	time.Sleep(10 * time.Millisecond)
}

func checkAtomLocalInElement(t *testing.T, elem *ElementLocal, name string, isNotExist bool, isState AtomosState, refCount int) *Error {
	elem.lock.RLock()
	atom, hasAtom := elem.atoms[name]
	elem.lock.RUnlock()
	if isNotExist {
		if hasAtom {
			t.Errorf("Atom should not exist.")
			return NewError(ErrFrameworkPanic, "Atom should not exist.").AddStack(nil)
		}
		return nil
	}
	if !hasAtom {
		t.Errorf("Atom should exist.")
		return NewError(ErrFrameworkPanic, "Atom should exist.").AddStack(nil)
	}
	// 需要在测试的atom的spawn预留一点时间才会成功。
	switch isState {
	case AtomosSpawning:
		if !atom.atomos.IsInState(AtomosSpawning) {
			return NewError(ErrFrameworkPanic, "Atom should be spawning.").AddStack(nil)
		}
	case AtomosWaiting:
		if !atom.atomos.IsInState(AtomosWaiting) {
			return NewError(ErrFrameworkPanic, "Atom should be waiting").AddStack(nil)
		}
	case AtomosBusy:
		if !atom.atomos.IsInState(AtomosBusy) {
			return NewError(ErrFrameworkPanic, "Atom should be busy").AddStack(nil)
		}
	case AtomosStopping:
		if !atom.atomos.IsInState(AtomosStopping) {
			return NewError(ErrFrameworkPanic, "Atom should be stopping").AddStack(nil)
		}
	}
	if curCount := atom.idTracker.RefCount(); curCount != refCount {
		return NewErrorf(ErrFrameworkPanic, "Atom ref count not match, has=(%d),should=(%d)", curCount, refCount).AddStack(nil)
	}
	return nil
}
