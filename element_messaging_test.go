package atomos

// Revival of the sync-messaging coverage that lived commented-out in
// element_local_test.go / atom_local_test.go (written against the removed
// v2 harness: SharedCosmosProcess singleton, *Error returns, fake element).
// Ported to the v3 harness (testElementLocalAtomSpawnHelper +
// testRunnableAtom) it pins: handler panic hands the sync caller a panic
// stack, caller timeouts distinguish handling (mail executing) from reject
// (mail still queued), unknown message names are rejected, and the atom
// survives all of the above.

import (
	"testing"
	"time"
)

// messagingHarness boots a process whose logs go to a capturing logger
// instead of newTestLogging: a handler panic is logged through WriteErrorLog
// as FATAL, and newTestLogging turns that into t.Error, which would fail the
// very test exercising the panic path.
func messagingHarness(t *testing.T, exe func(p *CosmosProcess)) {
	t.Helper()
	logger := &reproLogger{}
	process, cerr := newCosmosProcess("msgTestCosmos", "msgTestNode", logger)
	if cerr != nil {
		t.Fatalf("newCosmosProcess failed: %v", cerr.Message)
	}
	r := newTestCosmosRunnable(&IDInfo{Type: IDType_Cosmos, Cosmos: "msgTestCosmos", Node: "msgTestNode"})
	if err := process.Start(r); err != nil {
		t.Fatalf("process start failed: %v", err)
	}
	t.Cleanup(func() { _ = process.Stop() })
	exe(process)
}

func TestElement_SyncPanicReturnsPanicStackAndAtomSurvives(t *testing.T) {
	messagingHarness(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const name = "panic_atom"
		spawnTestAtom(t, elem, name)
		id, tracker, err := elem.GetAtomID(name, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}
		defer tracker.Release()

		// Greeting panics on an unknown mode; the mailbox must recover it and
		// hand the sync caller an error carrying the panic stack.
		_, err = id.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 99}, ArgBaseAtomosTimeout(2*time.Second))
		if err == nil {
			t.Fatal("expected panic error from handler, got nil")
		}
		e := asError(err)
		if e == nil || len(e.CallStacks) == 0 || e.CallStacks[0].PanicStack == "" {
			t.Fatalf("expected CallStacks[0].PanicStack to be populated, got %+v", e)
		}
		// The FATAL panic-recovery log lands asynchronously via the logging
		// mailbox, so poll briefly for it.
		deadline := time.Now().Add(2 * time.Second)
		for !loggerHasError(p, "recovers from panic") && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if !loggerHasError(p, "recovers from panic") {
			t.Errorf("expected a FATAL panic-recovery log entry")
		}

		// The atom survives the panic and serves the next mail.
		out, err := id.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 1}, ArgBaseAtomosTimeout(2*time.Second))
		if err != nil {
			t.Fatalf("atom did not survive handler panic: %v", err)
		}
		if out == nil {
			t.Fatal("expected non-nil reply after panic recovery")
		}
	})
}

// loggerHasError reports whether the process logging captured an error-log
// line containing substr. Goes through the logger's locked accessor: the
// logging mailbox goroutine appends concurrently.
func loggerHasError(p *CosmosProcess, substr string) bool {
	l, ok := p.logging.logging.(*reproLogger)
	if !ok {
		return false
	}
	return l.hasError(substr)
}

func TestElement_SyncTimeoutHandling(t *testing.T) {
	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const name = "timeout_handling_atom"
		impl := spawnTestAtom(t, elem, name)
		impl.greetingWait = 300 * time.Millisecond
		id, tracker, err := elem.GetAtomID(name, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}
		defer tracker.Release()

		// The handler outlives the caller's timeout: the mail was already
		// dequeued for handling, so the caller sees TimeoutHandling.
		_, err = id.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 1}, ArgBaseAtomosTimeout(50*time.Millisecond))
		if CodeOf(err) != ErrCodeAtomosPushTimeoutHandling {
			t.Fatalf("want ErrCodeAtomosPushTimeoutHandling, got %v", err)
		}
		// The atom is still busy executing the timed-out mail.
		if s := id.State(); s != BaseAtomosBusy {
			t.Fatalf("atom state after caller timeout = %v, want Busy", s)
		}

		// Once the handler finishes the atom idles again and serves calls.
		deadline := time.Now().Add(2 * time.Second)
		for id.State() != BaseAtomosWaiting && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if s := id.State(); s != BaseAtomosWaiting {
			t.Fatalf("atom state after handler finished = %v, want Waiting", s)
		}
		if _, err = id.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 1}, ArgBaseAtomosTimeout(2*time.Second)); err != nil {
			t.Fatalf("follow-up call after handling timeout: %v", err)
		}
	})
}

func TestElement_SyncTimeoutRejectWhileHandlerBusy(t *testing.T) {
	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		const name = "timeout_reject_atom"
		impl := spawnTestAtom(t, elem, name)
		impl.greetingNotify = make(chan struct{}, 1)
		impl.greetingGate = make(chan struct{})
		id, tracker, err := elem.GetAtomID(name, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}
		defer tracker.Release()

		// First call parks inside the handler on the gate.
		firstDone := make(chan error, 1)
		go func() {
			_, e := id.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 1}, ArgBaseAtomosTimeout(5*time.Second))
			firstDone <- e
		}()
		<-impl.greetingNotify

		// The second caller's mail queues behind the busy handler; when its
		// timeout fires the mail is still queued (never dequeued), so the
		// caller sees TimeoutReject and the mail is removed, not executed.
		_, err = id.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 1}, ArgBaseAtomosTimeout(50*time.Millisecond))
		if CodeOf(err) != ErrCodeAtomosPushTimeoutReject {
			t.Fatalf("want ErrCodeAtomosPushTimeoutReject, got %v", err)
		}

		close(impl.greetingGate)
		if e := <-firstDone; e != nil {
			t.Fatalf("first call failed: %v", e)
		}
		if _, err = id.SyncMessagingByName(p.local, "Greeting", &ForTestGreetingI{Mode: 1}, ArgBaseAtomosTimeout(2*time.Second)); err != nil {
			t.Fatalf("follow-up call after reject timeout: %v", err)
		}
	})
}

func TestElement_UnknownMessageNameRejected(t *testing.T) {
	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		// Element-level: no handler registered under this name.
		_, err := elem.SyncMessagingByName(p.local, "NoSuchElementMessage", nil, ArgBaseAtomosTimeout(2*time.Second))
		if CodeOf(err) != ErrCodeMessageHandlerNotFound {
			t.Fatalf("element: want ErrCodeMessageHandlerNotFound, got %v", err)
		}

		// Atom-level: same miss against an atom's handlers.
		const name = "unknown_msg_atom"
		spawnTestAtom(t, elem, name)
		id, tracker, err := elem.GetAtomID(name, NewIDTrackerInfoFromLocalGoroutine(3), ArgBaseAtomosFromLocalOnly())
		if err != nil {
			t.Fatalf("GetAtomID: %v", err)
		}
		defer tracker.Release()
		_, err = id.SyncMessagingByName(p.local, "NoSuchAtomMessage", nil, ArgBaseAtomosTimeout(2*time.Second))
		if CodeOf(err) != ErrCodeMessageHandlerNotFound {
			t.Fatalf("atom: want ErrCodeMessageHandlerNotFound, got %v", err)
		}
	})
}
