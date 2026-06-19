package atomos

import "time"

type ArgType int32

const (
	ArgTypeInvalid ArgType = 0

	// ArgsForBaseAtomos — Mail 选项,被 baseAtomosHelper 消费

	ArgTypeBaseAtomosTimeout      ArgType = 1
	ArgTypeBaseAtomosAppendToHead ArgType = 2
	ArgTypeBaseAtomosWaitKilled   ArgType = 3
	ArgTypeBaseAtomosWithAny      ArgType = 4 // 详见 ArgsForSpawn

	// ArgsForTask — 任务调度选项,被 taskHelper 消费

	ArgTypeTaskMark           ArgType = 10
	ArgTypeTaskDelay          ArgType = 11
	ArgTypeTaskLikeCrontab    ArgType = 12
	ArgTypeTaskAppendToHead   ArgType = 13
	ArgTypeTaskCancelCallback ArgType = 14
	ArgTypeTaskRecoverFunc    ArgType = 15

	// ArgsForSpawn — Spawn 参数,被用户 Load/AtomConstructor 消费
	// (ArgTypeBaseAtomosWithAny 历史上归在此语义,见问题2后归位)
)

type ArgsInterfaces interface {
	getArgType() ArgType
}

// ArgsForSpawn marks arguments intended for the spawn path (Load/Spawner),
// where they are consumed by USER code rather than the framework. The only
// current member is ArgBaseAtomosWithAny. These args still physically travel
// through the spawn chain as ArgsForBaseAtomos (for backward compatibility),
// but are NOT consumed by baseAtomosHelper — they are passed straight through
// to ElementLoader.Load / ElementSpawner / AtomSpawner.
//
// Design note: a full type-level split (a separate ArgsForSpawn variadic param
// on every spawn API) would be a breaking API change touching the code
// generator and all generated *_atomos.pb.go. We instead keep ArgsForBaseAtomos
// as the transport type and let the framework recognize WithAny explicitly so
// it is never misreported as an invalid mail option.
type ArgsForSpawn interface {
	ArgsForBaseAtomos
	argForSpawn()
}

// For Base Atomos

// BaseAtomosTimeout

type ArgsForBaseAtomos interface {
	ArgsInterfaces
	argForBaseAtomos()
}

type argBaseAtomosTimeout struct {
	timeout time.Duration
}

func (argBaseAtomosTimeout) getArgType() ArgType {
	return ArgTypeBaseAtomosTimeout
}
func (argBaseAtomosTimeout) argForBaseAtomos() {}

// BaseAtomosAppendToHead

type argBaseAtomosAppendToHead struct{}

func (argBaseAtomosAppendToHead) getArgType() ArgType {
	return ArgTypeBaseAtomosAppendToHead
}
func (argBaseAtomosAppendToHead) argForBaseAtomos() {}

// BaseAtomosWaitKilled

type argBaseAtomosWaitKilled struct{}

func (argBaseAtomosWaitKilled) getArgType() ArgType {
	return ArgTypeBaseAtomosWaitKilled
}
func (argBaseAtomosWaitKilled) argForBaseAtomos() {}

// BaseAtomosWithAny — a spawn-path argument that carries arbitrary context
// (e.g. *testing.T) to be consumed by user Load/Spawner code. It implements
// ArgsForSpawn to mark its intent, but is transported as ArgsForBaseAtomos.

type ArgBaseAtomosWithAny struct {
	Any any
}

func (ArgBaseAtomosWithAny) getArgType() ArgType {
	return ArgTypeBaseAtomosWithAny
}
func (ArgBaseAtomosWithAny) argForBaseAtomos() {}
func (ArgBaseAtomosWithAny) argForSpawn() {}

// For Task

type ArgsForTask interface {
	ArgsInterfaces
	argForTask()
}

// TaskMarking

type argTaskMark struct {
	mark string
}

func (argTaskMark) getArgType() ArgType {
	return ArgTypeTaskMark
}
func (argTaskMark) argForTask() {}

// TaskDelay

type argTaskDelay struct {
	duration time.Duration
}

func (argTaskDelay) getArgType() ArgType {
	return ArgTypeTaskDelay
}
func (argTaskDelay) argForTask() {}

// TaskLikeCrontab

type argTaskLikeCrontab struct {
	cron string
}

func (argTaskLikeCrontab) getArgType() ArgType {
	return ArgTypeTaskLikeCrontab
}
func (argTaskLikeCrontab) argForTask() {}

// TaskAppendToHead

type argTaskAppendToHead struct{}

func (argTaskAppendToHead) getArgType() ArgType {
	return ArgTypeTaskAppendToHead
}
func (argTaskAppendToHead) argForTask() {}

// TaskCancelCallback

type argTaskCancelCallback struct {
	callback func(reason string)
}

func (argTaskCancelCallback) getArgType() ArgType {
	return ArgTypeTaskCancelCallback
}
func (argTaskCancelCallback) argForTask() {}

// TaskRecoverFunc

type argTaskRecoverFunc struct {
	recoverFunc func(recoverInfo any)
}

func (argTaskRecoverFunc) getArgType() ArgType {
	return ArgTypeTaskRecoverFunc
}
func (argTaskRecoverFunc) argForTask() {}

