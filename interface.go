package atomos

import (
	"time"

	"google.golang.org/protobuf/proto"
)

// This file defines the interfaces that developers implement to customize actor behavior.
// Interfaces are organized by category:
//
//   Required — Must be implemented for the framework to function.
//     CosmosMainScript, ElementDeveloper, ElementLoader
//
//   Data Persistence — Automatic state save/load on spawn/halt.
//     AutoData, AtomAutoData, ElementAutoData
//
//   Lifecycle — Hooks into actor start, stop, and runtime behavior.
//     ElementStartRunning, ElementAtomExit, ElementAuthorization
//
//   Metadata — Version, capacity, and log level configuration.
//     ElementVersion, ElementAtomInitNum, ElementLogLevel
//
//   Communication — Wormhole messaging and panic recovery.
//     AtomosAcceptWormhole, AtomosRecover

// ━━━ Required Interfaces ━━━

// CosmosMainScript defines the process-level lifecycle hooks.
// Every application must provide one implementation, set via CosmosRunnable.SetMainScript.
//
// Lifecycle order:
//  1. OnBoot    — called once before any elements are spawned. Use for
//                 database connections, config validation, etc.
//  2. OnStartUp — called after all elements are spawned and the cluster is ready.
//                 Use for initial Atoms, scheduled jobs, etc.
//  3. OnShutdown — called when the process is stopping. Use for graceful cleanup.
type CosmosMainScript interface {
	OnBoot(local *CosmosProcess) *Error
	OnStartUp(local *CosmosProcess) *Error
	OnShutdown() *Error
}

// ElementDeveloper is the factory interface for creating actor instances.
// Every Element type must provide one implementation, registered via
// CosmosRunnable.AddElementImplementation.
//
// ElementConstructor is called once when the Element spawns.
// AtomConstructor is called each time an Atom of this Element is spawned.
type ElementDeveloper interface {
	// ElementConstructor returns the Atomos implementation for this Element.
	// Called once per Element at spawn time.
	ElementConstructor() Atomos

	// AtomConstructor returns the Atomos implementation for a new Atom.
	// Called each time an Atom of this Element is spawned. The name is
	// the caller-chosen unique name for this Atom instance.
	AtomConstructor(name string) Atomos
}

// ElementLoader provides database resource lifecycle hooks.
// Implement this on your ElementDeveloper when you need to load/unload
// external resources (database connections, file handles, caches) that
// outlive individual Atoms.
//
// Load is called before the Element's data is fetched via AutoData.
// Unload is called after the Element halts and its data has been saved.
type ElementLoader interface {
	Load(self ElementSelfID, config map[string][]byte, args ...ArgsForBaseAtomos) *Error
	Unload() *Error
}

// ━━━ Data Persistence Interfaces ━━━

// AutoData enables automatic state persistence for Elements and Atoms.
// Implement this on your ElementDeveloper to have the framework automatically:
//   - Load Element data before ElementSpawner runs
//   - Save Element data after Halt returns save=true
//   - Load Atom data before AtomSpawner runs (auto-spawn on GetAtomID miss)
//   - Save Atom data after Halt returns save=true
//
// Return nil from AtomAutoData or ElementAutoData to opt out of persistence
// for that level while keeping it for the other.
type AutoData interface {
	AtomAutoData() AtomAutoData
	ElementAutoData() ElementAutoData
}

// AtomAutoData reads and writes individual Atom state.
// GetAtomData is called before AtomSpawner. Return nil,nil if there is no
// previously saved state (first spawn).
// SetAtomData is called after a stateful Halt.
type AtomAutoData interface {
	GetAtomData(name string) (proto.Message, *Error)
	SetAtomData(name string, data proto.Message) *Error
}

// ElementAutoData reads and writes Element-level state.
// GetElementData is called before ElementSpawner. Return nil,nil if there
// is no previously saved state.
// SetElementData is called after a stateful Element Halt.
type ElementAutoData interface {
	GetElementData() (proto.Message, *Error)
	SetElementData(data proto.Message) *Error
}

// ━━━ Lifecycle Interfaces ━━━

// ElementStartRunning is called in a new goroutine after the Element spawns.
// Use it for background work that should run for the lifetime of the Element.
// The goroutine is protected by panic recovery.
type ElementStartRunning interface {
	StartRunning()
}

// ElementAtomExit controls how Atoms within this Element are killed during
// Element shutdown. Atoms are killed concurrently with a bounded goroutine pool.
//
// StopTimeout is the per-Atom deadline for graceful shutdown.
// StopGap is the pause inserted between starting each Atom's kill sequence,
// which can reduce load spikes on external systems during mass shutdown.
type ElementAtomExit interface {
	StopTimeout() time.Duration
	StopGap() time.Duration
}

// ElementAuthorization gates cross-actor kill operations.
// When implemented, Atom.Kill will call AtomCanKill before proceeding.
// Return nil to allow the kill, or an *Error to deny it.
type ElementAuthorization interface {
	AtomCanKill(ID) *Error
}

// ━━━ Metadata Interfaces ━━━

// ElementVersion declares the version of this Element implementation.
// This is a single uint64 used for hot-upgrade compatibility detection in
// the etcd cluster layer: nodes with the same version number are treated
// as compatible and can replace each other. It is NOT a semver.
type ElementVersion interface {
	GetElementVersion() uint64
}

// ElementAtomInitNum pre-sizes the Atom container map to the expected number
// of Atoms, reducing allocation during startup. If not implemented, the map
// starts with Go's default capacity.
type ElementAtomInitNum interface {
	GetElementAtomsInitNum() int
}

// ElementLogLevel overrides the process-wide log level for this Element.
// All Atoms within the Element inherit this level.
type ElementLogLevel interface {
	GetElementLogLevel() LogLevel
}

// ━━━ Communication Interfaces ━━━

// AtomosAcceptWormhole enables an actor to receive wormhole messages.
// Wormholes carry arbitrary typed data (implementing BaseAtomosWormhole)
// between actors. If not implemented, SendWormhole to this actor returns
// ErrAtomosNotSupportWormhole.
type AtomosAcceptWormhole interface {
	AcceptWormhole(fromID ID, wormhole BaseAtomosWormhole) *Error
}

// AtomosRecover gives an actor visibility into panics that occur during
// its lifecycle. When implemented, the corresponding method is called
// instead of the default Fatal-log-and-continue behavior.
//
// Each method receives the *Error that captures the panic stack and reason.
// Use this to implement custom alerting, metrics, or graceful degradation.
type AtomosRecover interface {
	ParallelRecover(err *Error)
	SpawnRecover(arg proto.Message, err *Error)
	MessageRecover(name string, arg proto.Message, err *Error)
	TaskRecover(taskID uint64, name string, arg proto.Message, err *Error)
	StopRecover(err *Error)
}
