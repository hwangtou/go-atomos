package atomos

import "time"

type ArgType int32

const (
	ArgTypeInvalid ArgType = 0

	// ArgsForBaseAtomos

	ArgTypeBaseAtomosTimeout      ArgType = 2
	ArgTypeBaseAtomosAppendToHead ArgType = 3
	ArgTypeBaseAtomosWaitKilled   ArgType = 4
	ArgTypeBaseAtomosWithAny      ArgType = 5

	// ArgsForTask

	ArgTypeTaskMark           ArgType = 10
	ArgTypeTaskDelay          ArgType = 11
	ArgTypeTaskLikeCrontab    ArgType = 12
	ArgTypeTaskAppendToHead   ArgType = 13
	ArgTypeTaskCancelCallback ArgType = 14
	ArgTypeTaskRecoverFunc    ArgType = 15

	// ArgsForSpawn
)

type ArgsInterfaces interface {
	getArgType() ArgType
}

//// GetOrSpawnAtomIfNotFound
//
//type argGetOrSpawnAtomIfNotFound struct{}
//
//func (argGetOrSpawnAtomIfNotFound) getArgType() ArgType {
//	return ArgTypeGetOrSpawnAtomIfNotFound
//}

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

// BaseAtomosWithAny

type ArgBaseAtomosWithAny struct {
	Any any
}

func (ArgBaseAtomosWithAny) getArgType() ArgType {
	return ArgTypeBaseAtomosWithAny
}
func (ArgBaseAtomosWithAny) argForBaseAtomos() {}

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

//// ArgsForSpawn
//
//type ArgsForSpawn interface {
//	ArgsInterfaces
//	argForSpawn()
//}
//
//// ArgsForGet
//
//type ArgsForGet interface {
//	ArgsInterfaces
//	argForGet()
//}
