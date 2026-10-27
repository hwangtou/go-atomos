package atomos

import "time"

type ArgType int32

const (
	ArgTypeInvalid ArgType = 0

	// ArgsForBaseAtomos

	ArgTypeBaseAtomosTimeout        ArgType = 2
	ArgTypeBaseAtomosAppendToHead   ArgType = 3
	ArgTypeBaseAtomosWaitKilled     ArgType = 4
	ArgTypeBaseAtomosWithAny        ArgType = 5
	ArgTypeBaseAtomosFromLocalOnly  ArgType = 6
	ArgTypeBaseAtomosFromRemoteOnly ArgType = 7
	// ArgTypeBaseAtomosWaitChain is internal-only: it carries the cross-node
	// wait chain from CosmosArgs.wait_chain into a remote-originated sync
	// mail. Not exposed as a public Arg constructor.
	ArgTypeBaseAtomosWaitChain   ArgType = 8
	ArgTypeBaseAtomosOperationID ArgType = 9

	// ArgsForTask

	ArgTypeTaskMark           ArgType = 10
	ArgTypeTaskDelay          ArgType = 11
	ArgTypeTaskLikeCrontab    ArgType = 12
	ArgTypeTaskAppendToHead   ArgType = 13
	ArgTypeTaskCancelCallback ArgType = 14
	ArgTypeTaskRecoverFunc    ArgType = 15

	// ArgsForSpawn
)

// argsFromLocalOrRemote reports whether args carry a user-originated routing
// mark (ArgBaseAtomosFromLocalOnly or ArgBaseAtomosFromRemoteOnly).
func argsFromLocalOrRemote(args []ArgsForBaseAtomos) bool {
	for _, a := range args {
		if a == nil {
			continue
		}
		switch a.getArgType() {
		case ArgTypeBaseAtomosFromLocalOnly, ArgTypeBaseAtomosFromRemoteOnly:
			return true
		}
	}
	return false
}

type ArgsInterfaces interface {
	getArgType() ArgType
}

// TODO: ArgsForSpawn and ArgsForGet are reserved for future use.

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

type argBaseAtomosWithAny struct {
	Any any
}

func (argBaseAtomosWithAny) getArgType() ArgType {
	return ArgTypeBaseAtomosWithAny
}
func (argBaseAtomosWithAny) argForBaseAtomos() {}

// BaseAtomosFromLocalOnly / BaseAtomosFromRemoteOnly

type argBaseAtomosFromLocalOnly struct{}

func (argBaseAtomosFromLocalOnly) getArgType() ArgType {
	return ArgTypeBaseAtomosFromLocalOnly
}
func (argBaseAtomosFromLocalOnly) argForBaseAtomos() {}

type argBaseAtomosFromRemoteOnly struct{}

func (argBaseAtomosFromRemoteOnly) getArgType() ArgType {
	return ArgTypeBaseAtomosFromRemoteOnly
}
func (argBaseAtomosFromRemoteOnly) argForBaseAtomos() {}

// argBaseAtomosWaitChain carries the cross-node sync-call wait chain into a
// remote-originated sync mail. Internal only.
type argBaseAtomosWaitChain struct {
	chain []string
}

func (argBaseAtomosWaitChain) getArgType() ArgType {
	return ArgTypeBaseAtomosWaitChain
}
func (argBaseAtomosWaitChain) argForBaseAtomos() {}

type argBaseAtomosOperationID struct {
	operationID string
}

func (argBaseAtomosOperationID) getArgType() ArgType {
	return ArgTypeBaseAtomosOperationID
}
func (argBaseAtomosOperationID) argForBaseAtomos() {}

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
