package go_atomos

import (
	"testing"
	"time"
)

var sharedTestAtom1, sharedTestAtom2 *AtomLocal
var successCounter int

func TestAtomLocal_FirstSyncCall(t *testing.T) {
	initTestFakeCosmosProcess(t)
	if err := SharedCosmosProcess().Start(newTestFakeRunnable(t, sharedCosmosProcess, false)); err != nil {
		t.Errorf("CosmosLocal: Start failed. err=(%v)", err)
		return
	}
	process := SharedCosmosProcess()
	elemName := "testElement"
	atomName := "testAtom"
	atomOtherName := "testAtomOther"
	testElem := process.local.elements[elemName]

	a, tracker, err := testElem.SpawnAtom(process.local, atomName, &String{S: atomName}, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: Spawn failed. err=(%v)", err)
		return
	}
	tracker.Release()

	a2, tracker2, err := testElem.SpawnAtom(process.local, atomOtherName, &String{S: atomOtherName}, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: Spawn failed. err=(%v)", err)
		return
	}
	_ = a2
	tracker2.Release()

	// 测试本地同步调用到自己的情况，传入的from和to都是自己，in是自己的名字。会出现死锁，并返回错误。
	out, err := a.SyncMessagingByName(process.local, "testingLocalSyncSelfFirstSyncCallDeadlock", 0, &String{S: atomName})
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalSyncSelfFirstSyncCallDeadlock failed. err=(%v)", err)
		return
	}
	if out.(*String).S != "OK" {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalSyncSelfFirstSyncCallDeadlock out=(%v)", out)
		return
	}
	t.Logf("TestElementLocal_FirstSyncCall: testingLocalSyncSelfFirstSyncCallDeadlock tested")

	// 测试本地异步调用到自己的情况，传入的from和to都是自己，in是自己的名字。不会出现死锁。
	out, err = a.SyncMessagingByName(process.local, "testingLocalAsyncSelfFirstSyncCall", 0, &String{S: atomName})
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalAsyncSelfFirstSyncCall failed. err=(%v)", err)
		return
	}
	if out.(*String).S != "OK" {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalAsyncSelfFirstSyncCall out=(%v)", out)
		return
	}
	<-time.After(1 * time.Millisecond)
	if successCounter != 1 {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalAsyncSelfFirstSyncCall successCounter=(%v)", successCounter)
		return
	}
	t.Logf("TestElementLocal_FirstSyncCall: testingLocalAsyncSelfFirstSyncCall tested")

	// 测试本地同步和异步调用外部的情况，传入的from是自己，to是外部的名字，in是自己的名字。不会出现死锁。
	out, err = a.SyncMessagingByName(process.local, "testingLocalSyncAndAsyncOtherFirstSyncCall", 0, &Strings{Ss: []string{atomName, atomOtherName}})
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalSyncAndAsyncOtherFirstSyncCall failed. err=(%v)", err)
		return
	}
	if out.(*String).S != "OK" {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalSyncAndAsyncOtherFirstSyncCall out=(%v)", out)
		return
	}
	<-time.After(1 * time.Millisecond)
	if successCounter != 2 {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalAsyncSelfFirstSyncCall successCounter=(%v)", successCounter)
		return
	}
	t.Logf("TestElementLocal_FirstSyncCall: testingLocalSyncAndAsyncOtherFirstSyncCall tested")

	// 测试本地同步调用外部，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
	out, err = a.SyncMessagingByName(process.local, "testingLocalSyncChainSelfFirstSyncCallDeadlock", 0, &Strings{Ss: []string{atomName, atomOtherName}})
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalSyncChainSelfFirstSyncCallDeadlock failed. err=(%v)", err)
		return
	}
	if out.(*String).S != "OK" {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalSyncChainSelfFirstSyncCallDeadlock out=(%v)", out)
		return
	}
	t.Logf("TestElementLocal_FirstSyncCall: testingLocalSyncChainSelfFirstSyncCallDeadlock tested")

	// 测试本地异步调用外部，并回调到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。不会出现死锁。
	out, err = a.SyncMessagingByName(process.local, "testingLocalAsyncChainSelfFirstSyncCall", 0, &Strings{Ss: []string{atomName, atomOtherName}})
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalAsyncChainSelfFirstSyncCall failed. err=(%v)", err)
		return
	}
	if out.(*String).S != "OK" {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalAsyncChainSelfFirstSyncCall out=(%v)", out)
		return
	}
	<-time.After(1 * time.Millisecond)
	if successCounter != 3 {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalAsyncChainSelfFirstSyncCall successCounter=(%v)", successCounter)
		return
	}
	t.Logf("TestElementLocal_FirstSyncCall: testingLocalAsyncChainSelfFirstSyncCall tested")

	// 测试本地同步执行任务，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
	out, err = a.SyncMessagingByName(process.local, "testingLocalTaskChainSelfFirstSyncCall", 0, &Strings{Ss: []string{atomName, atomOtherName}})
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalTaskChainSelfFirstSyncCall failed. err=(%v)", err)
		return
	}
	if out.(*String).S != "OK" {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalTaskChainSelfFirstSyncCall out=(%v)", out)
		return
	}
	<-time.After(1 * time.Millisecond)
	if successCounter != 4 {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalAsyncSelfFirstSyncCall successCounter=(%v)", successCounter)
		return
	}
	t.Logf("TestElementLocal_FirstSyncCall: testingLocalTaskChainSelfFirstSyncCall tested")

	// 测试本地发送Wormhole，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
	out, err = a.SyncMessagingByName(process.local, "testingLocalWormholeChainSelfFirstSyncCall", 0, &Strings{Ss: []string{atomName, atomOtherName}})
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalWormholeChainSelfFirstSyncCall failed. err=(%v)", err)
		return
	}
	if out.(*String).S != "OK" {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalWormholeChainSelfFirstSyncCall out=(%v)", out)
		return
	}
	t.Logf("TestElementLocal_FirstSyncCall: testingLocalWormholeChainSelfFirstSyncCall tested")

	// 测试本地同步Scale调用，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
	out, err = a.SyncMessagingByName(process.local, "testingLocalScaleChainSelfFirstSyncCall", 0, &Strings{Ss: []string{atomName, atomOtherName}})
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalScaleChainSelfFirstSyncCall failed. err=(%v)", err)
		return
	}
	if out.(*String).S != "OK" {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalScaleChainSelfFirstSyncCall out=(%v)", out)
		return
	}
	t.Logf("TestElementLocal_FirstSyncCall: testingLocalScaleChainSelfFirstSyncCall tested")

	// 测试本地同步Kill调用，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
	out, err = a.SyncMessagingByName(process.local, "testingLocalKillChainSelfFirstSyncCall", 0, &Strings{Ss: []string{atomName, atomOtherName}})
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalKillChainSelfFirstSyncCall failed. err=(%v)", err)
		return
	}
	if out.(*String).S != "OK" {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalKillChainSelfFirstSyncCall out=(%v)", out)
		return
	}
	t.Logf("TestElementLocal_FirstSyncCall: testingLocalKillChainSelfFirstSyncCall tested")

	// 测试本地同步KillSelf调用，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
	out, err = a.SyncMessagingByName(process.local, "testingLocalKillSelfChainSelfFirstSyncCall", 0, &Strings{Ss: []string{atomName, atomOtherName}})
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalKillSelfChainSelfFirstSyncCall failed. err=(%v)", err)
		return
	}
	if out.(*String).S != "OK" {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalKillSelfChainSelfFirstSyncCall out=(%v)", out)
		return
	}
	t.Logf("TestElementLocal_FirstSyncCall: testingLocalKillSelfChainSelfFirstSyncCall tested")

	// 测试本地同步Spawn调用，并链式调用到自己的情况，传入的from是自己，to是外部的名字，in是自己的名字。会出现死锁。
	out, err = a.SyncMessagingByName(process.local, "testingLocalSpawnSelfChainSelfFirstSyncCall", 0, &Strings{Ss: []string{atomName, atomOtherName}})
	if err != nil {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalSpawnSelfChainSelfFirstSyncCall failed. err=(%v)", err)
		return
	}
	if out.(*String).S != "OK" {
		t.Errorf("TestElementLocal_FirstSyncCall: testingLocalSpawnSelfChainSelfFirstSyncCall out=(%v)", out)
		return
	}
	t.Logf("TestElementLocal_FirstSyncCall: testingLocalSpawnSelfChainSelfFirstSyncCall tested")
}

func TestAtomLocalBase(t *testing.T) {
	var hasSpawning, hasSpawn, hasStopping, hasHalted bool
	var messages int
	var spawn, run, stop time.Duration

	initTestFakeCosmosProcess(t)
	runnable := newTestFakeRunnable(t, sharedCosmosProcess, false)
	//runnable.hookAtomSpawning = func(elem string, name string) {
	//	hasSpawning = true
	//}
	//runnable.hookAtomSpawn = func(elem string, name string) {
	//	hasSpawn = true
	//}
	//runnable.hookAtomStopping = func(elem string, name string) {
	//	hasStopping = true
	//}
	//runnable.hookAtomHalt = func(elem string, name string) {
	//	hasHalted = true
	//}
	if err := SharedCosmosProcess().Start(runnable); err != nil {
		t.Errorf("TestAtomLocalBase: Start failed. err=(%v)", err)
		return
	}
	testElemName := "testElement"
	testAtomName := "testAtom"
	testDeadlockAtomName := "testDeadlockAtom"

	process := SharedCosmosProcess()
	testElem, err := process.local.getLocalElement(testElemName)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Element not found. name=(%s)", testElemName)
		return
	}

	// Spawn an atom.
	a, tracker, err := testElem.SpawnAtom(process.local, testAtomName, &String{S: testAtomName}, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Spawn failed. err=(%v)", err)
		return
	}
	atom := a.(*AtomLocal)
	if !(hasSpawning && hasSpawn) && (hasStopping && hasHalted) {
		t.Errorf("TestAtomLocalBase: Spawn state invalid.")
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn waiting state invalid. err=(%v)", err)
		return
	}

	// Get an atom.
	checkAtom, checkTracker, err := testElem.GetAtomID(testAtomName, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Get atom failed. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 2); err != nil {
		t.Errorf("TestAtomLocalBase: Get atom failed. err=(%v)", err)
		return
	}
	//checkAtom.(*AtomLocal).Release(checkTracker)
	_ = checkAtom
	checkTracker.Release()

	// Spawn an atom.
	deadlockAtom, deadlockTracker, err := testElem.SpawnAtom(process.local, testDeadlockAtomName, &String{S: testDeadlockAtomName}, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Spawn deadlock atom failed. err=(%v)", err)
		return
	}
	deadlockTracker.Release()
	if err = checkAtomLocalInElement(t, testElem, testDeadlockAtomName, false, AtomosWaiting, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn waiting state invalid. err=(%v)", err)
		return
	}

	// Push Message without fromID.
	reply, err := atom.SyncMessagingByName(nil, "testMessage", 0, nil)
	if err == nil || reply != nil {
		t.Errorf("TestAtomLocalBase: Push Message without fromID succeed.")
		return
	}

	// Push Message.
	reply, err = atom.SyncMessagingByName(testElem, "testMessage", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Push Message failed. err=(%v)", err)
		return
	}
	messages += 1 // testMessage

	// Push Panic Message.
	reply, err = atom.SyncMessagingByName(testElem, "testPanic", 0, nil)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" {
		t.Errorf("TestAtomLocalBase: Push Panic Message failed. err=(%v)", err)
		return
	}
	messages += 1 // testPanic

	// Push Timeout Message.
	reply, err = atom.SyncMessagingByName(testElem, "testMessageTimeout", 1*time.Millisecond, nil)
	if err == nil || err.Code != ErrAtomosPushTimeoutHandling {
		t.Errorf("TestAtomLocalBase: Push Message Timeout failed. err=(%v)", err)
		return
	}
	messages += 1 // testMessageTimeout
	// Push Timeout Reject Mail.
	reply, err = atom.SyncMessagingByName(testElem, "testMessage", 1*time.Microsecond, nil)
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
	sharedTestAtom2 = deadlockAtom.(*AtomLocal)
	reply, err = atom.SyncMessagingByName(testElem, "testMessageDeadlock", 0, &String{S: testAtomName})
	if err == nil || err.Code != ErrIDFirstSyncCallDeadlock {
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
	reply, err = atom.SyncMessagingByName(testElem, "testTask", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Task Failed. state=(%d),err=(%v)", atom.State(), err)
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
	reply, err = atom.SyncMessagingByName(testElem, "testTaskPanic", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Task Panic Failed. state=(%d),err=(%v)", atom.State(), err)
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
	reply, err = atom.SyncMessagingByName(testElem, "testParallel", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Parallel Failed. state=(%d),err=(%v)", atom.State(), err)
		return
	}
	messages += 1 // testParallel
	sharedTestAtom1 = nil
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Parallel Failed state invalid. err=(%v)", err)
		return
	}

	// Try Spawning twice.
	atomTwice, trackerTwice, err := testElem.SpawnAtom(process.local, testAtomName, &String{S: testAtomName}, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err == nil || err.Code != ErrAtomIsRunning {
		t.Errorf("TestAtomLocalBase: Spawn twice state invalid. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 2); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn twice state invalid. err=(%v)", err)
		return
	}
	// Try kill self.
	sharedTestAtom1 = atom
	reply, err = atom.SyncMessagingByName(testElem, "testKillSelf", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: KillSelf Failed. state=(%d),err=(%v)", atom.State(), err)
		return
	}
	messages += 1 // testKillSelf
	sharedTestAtom1 = nil
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosHalt, 2); err != nil {
		t.Errorf("TestAtomLocalBase: KillSelf state invalid. err=(%v)", err)
		return
	}
	trackerTwice.Release()
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosHalt, 1); err != nil {
		t.Errorf("TestAtomLocalBase: KillSelf state invalid. err=(%v)", err)
		return
	}

	// Message Tracker.
	messageCount := 0
	for _, info := range atom.atomos.mt.messages {
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
		atomTwice, trackerTwice, err = testElem.SpawnAtom(process.local, testAtomName, &String{S: testAtomName}, NewIDTrackerInfoFromLocalGoroutine(1), true)
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
	reply, err = atom.SyncMessagingByName(testElem, "testMessage", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Spawn twice Push Message failed. err=(%v)", err)
		return
	}
	messages = 1 // testMessage
	reply, err = atomTwice.SyncMessagingByName(testElem, "testMessage", 0, nil)
	if err != nil || reply.(*String).S != "OK" {
		t.Errorf("TestAtomLocalBase: Spawn twice Push Message failed. err=(%v)", err)
		return
	}
	messages += 1 // testMessage
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosWaiting, 2); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn twice state invalid. err=(%v)", err)
		return
	}
	trackerTwice.Release()
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
	if err = deadlockAtom.(*AtomLocal).atomos.PushKillMailAndWaitReply(testElem, true, 0); err != nil {
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
	if err = atom.atomos.PushKillMailAndWaitReply(testElem, true, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Reference count push kill failed. err=(%v)", err)
	}
	stop = time.Now().Sub(start) - spawn - run
	if err = checkAtomLocalInElement(t, testElem, testAtomName, false, AtomosHalt, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Reference count state invalid. err=(%v)", err)
		return
	}
	reply, err = atom.SyncMessagingByName(testElem, "testMessage", 0, nil)
	if err == nil || err.Code != ErrAtomosIsNotRunning {
		t.Errorf("TestAtomLocalBase: Reference count push message state invalid. err=(%v)", err)
		return
	}
	tracker.Release()
	//atom.Release(tracker)
	if err = checkAtomLocalInElement(t, testElem, testAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Reference count state invalid. err=(%v)", err)
		return
	}
	// Check ID Tracker.
	if atom.atomos.it.refCount() != 0 {
		t.Errorf("TestAtomLocalBase: ID Tracker state invalid.")
		return
	}
	// Message Tracker.
	messageCount = 0
	for _, info := range atom.atomos.mt.messages {
		messageCount += info.Count
	}
	if messages != messageCount {
		t.Errorf("TestAtomLocalBase: Message Tracker state invalid.")
		return
	}
	t.Logf("TestAtomLocalBase: Meesage Tracker. spawn=(%v),run=(%v),stop=(%v),dump=(%v)",
		atom.atomos.mt.spawnAt.Sub(atom.atomos.mt.spawningAt),
		atom.atomos.mt.stoppingAt.Sub(atom.atomos.mt.spawnAt),
		atom.atomos.mt.stoppedAt.Sub(atom.atomos.mt.stoppingAt),
		atom.atomos.mt.dump())
	t.Logf("TestAtomLocalBase: Meesage Tracker. spawn=(%v),run=(%v),stop=(%v)", spawn, run, stop)

	// Spawn panic.
	spawnPanicAtomName := "spawn_panic"
	spawnPanicAtom, trackerSpawnPanicAtom, err := testElem.SpawnAtom(process.local, spawnPanicAtomName, &String{S: "panic"}, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" {
		t.Errorf("TestAtomLocalBase: Spawn panic atom state invalid.")
		return
	}
	if spawnPanicAtom.(*AtomLocal) != nil || trackerSpawnPanicAtom != nil {
		t.Errorf("TestAtomLocalBase: Spawn panic atom state invalid.")
		return
	}
	if err = checkAtomLocalInElement(t, testElem, spawnPanicAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Spawn panic atom state invalid. err=(%v)", err)
		return
	}

	// Stopping panic.
	stoppingPanicAtomName := "stopping_panic"
	stoppingPanicAtom, trackerStoppingPanicAtom, err := testElem.SpawnAtom(process.local, stoppingPanicAtomName, nil, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Stopping panic atom failed. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, stoppingPanicAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Stopping panic atom state invalid. err=(%v)", err)
		return
	}
	err = stoppingPanicAtom.(*AtomLocal).atomos.PushKillMailAndWaitReply(testElem, true, 0)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" {
		t.Errorf("TestAtomLocalBase: Stopping panic atom push kill state invalid. err=(%v)", err)
		return
	}
	trackerStoppingPanicAtom.Release()
	if err = checkAtomLocalInElement(t, testElem, stoppingPanicAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Stopping panic atom state invalid. err=(%v)", err)
		return
	}

	// Auto Data Persistence
	runnable.implements["testElement"].Developer = &testElementAutoDataDev{}

	// GetAtomData returns Error.
	dataAtomName := "get_data_error"
	dataAtom, trackerDataAtom, err := testElem.SpawnAtom(process.local, dataAtomName, nil, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err == nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if dataAtom.(*AtomLocal) != nil || trackerDataAtom != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, dataAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}

	// GetAtomData returns Panic.
	dataAtomName = "get_data_panic"
	dataAtom, trackerDataAtom, err = testElem.SpawnAtom(process.local, dataAtomName, nil, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err == nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if dataAtom.(*AtomLocal) != nil || trackerDataAtom != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, dataAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}

	// GetAtomData returns OK.
	dataAtomName = "data_ok"
	dataAtom, trackerDataAtom, err = testElem.SpawnAtom(process.local, dataAtomName, nil, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if err = checkAtomLocalInElement(t, testElem, dataAtomName, false, AtomosWaiting, 1); err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	inner := dataAtom.(*AtomLocal).atomos.instance.(*testAtom)
	if inner.data.(*String).S != dataAtomName {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}

	// SetAtomData returns OK.
	err = dataAtom.(*AtomLocal).atomos.PushKillMailAndWaitReply(testElem, true, 0)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	if setAtomCounter != 1 {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	trackerDataAtom.Release()

	// SetAtomData returns Error.
	dataAtomName = "set_data_error"
	dataAtom, trackerDataAtom, err = testElem.SpawnAtom(process.local, dataAtomName, nil, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	err = dataAtom.(*AtomLocal).atomos.PushKillMailAndWaitReply(testElem, true, 0)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack != "" || len(err.CallStacks[1].Args) == 0 {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	trackerDataAtom.Release()
	if err = checkAtomLocalInElement(t, testElem, dataAtomName, true, AtomosHalt, 0); err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}

	// SetAtomData returns panic.
	dataAtomName = "set_data_panic"
	dataAtom, trackerDataAtom, err = testElem.SpawnAtom(process.local, dataAtomName, nil, NewIDTrackerInfoFromLocalGoroutine(1), true)
	if err != nil {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	err = dataAtom.(*AtomLocal).atomos.PushKillMailAndWaitReply(testElem, true, 0)
	if err == nil || len(err.CallStacks) == 0 || err.CallStacks[0].PanicStack == "" || len(err.CallStacks[0].Args) == 0 {
		t.Errorf("TestAtomLocalBase: Auto data state invalid. err=(%v)", err)
		return
	}
	trackerDataAtom.Release()
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
			return NewError(ErrFrameworkRecoverFromPanic, "Atom should not exist.").AddStack(nil)
		}
		return nil
	}
	if !hasAtom {
		t.Errorf("Atom should exist.")
		return NewError(ErrFrameworkRecoverFromPanic, "Atom should exist.").AddStack(nil)
	}
	// 需要在测试的atom的spawn预留一点时间才会成功。
	switch isState {
	case AtomosSpawning:
		if !atom.atomos.IsInState(AtomosSpawning) {
			return NewError(ErrFrameworkRecoverFromPanic, "Atom should be spawning.").AddStack(nil)
		}
	case AtomosWaiting:
		if !atom.atomos.IsInState(AtomosWaiting) {
			return NewError(ErrFrameworkRecoverFromPanic, "Atom should be waiting").AddStack(nil)
		}
	case AtomosBusy:
		if !atom.atomos.IsInState(AtomosBusy) {
			return NewError(ErrFrameworkRecoverFromPanic, "Atom should be busy").AddStack(nil)
		}
	case AtomosStopping:
		if !atom.atomos.IsInState(AtomosStopping) {
			return NewError(ErrFrameworkRecoverFromPanic, "Atom should be stopping").AddStack(nil)
		}
	}
	if curCount := atom.atomos.it.refCount(); curCount != refCount {
		return NewErrorf(ErrFrameworkRecoverFromPanic, "Atom ref count not match, has=(%d),should=(%d)", curCount, refCount).AddStack(nil)
	}
	return nil
}
