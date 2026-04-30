package atomos

import "google.golang.org/protobuf/proto"

// Atomos is the interface that every actor (both Element and Atom) must implement.
// It defines the core lifecycle contract: an actor can be halted and must provide
// a human-readable identity via String.
//
// Implementing Atomos
//
// Both Element and Atom developers must provide an implementation. The framework
// calls String for logging/debugging and Halt when the actor is being stopped.
//
// Halt Semantics
//
// When an actor is halted, it receives the ID of the caller that initiated the halt
// and a list of task IDs that were cancelled during shutdown. The actor should:
//   - Return save=true and its serializable state if it wants data persisted.
//   - Return save=false if there is nothing to persist.
//   - The returned proto.Message is passed to AtomAutoData.SetAtomData (if configured).
type Atomos interface {
	// String returns a human-readable identifier for this actor.
	String() string

	// Halt is called when the actor is being stopped. The from parameter identifies
	// who requested the halt. cancelled lists all task IDs that were cancelled during
	// the shutdown process.
	//
	// Return save=true and your state as data to persist it via AutoData.
	// Return save=false to skip persistence.
	Halt(from ID, cancelled []uint64) (save bool, data proto.Message)
}

// AtomosUtilities provides access to framework services from within an actor.
// Every actor receives these utilities via its SelfID during spawn.
type AtomosUtilities interface {
	// Log returns the structured logger for this actor. All log messages are
	// automatically tagged with the actor's ID and routed through the logging
	// mailbox for serialized output.
	Log() Logging

	// Task returns the task scheduler for this actor. Tasks are executed
	// serially on the actor's mailbox goroutine — no concurrency concerns
	// within a single actor. Use Task for deferred work, recurring jobs,
	// and named serial/concurrent queues.
	Task() Task
}
