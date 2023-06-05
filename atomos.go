package go_atomos

import "google.golang.org/protobuf/proto"

// 开发者需要实现的Atom的接口定义。
// Atom Interface Definition that developers have to implement.

// Atomos
// Atomos类型
type Atomos interface {
	String() string

	// Halt
	// 关闭
	Halt(from ID, cancelled []uint64) (save bool, data proto.Message)
}

type AtomosRecover interface {
	ParallelRecover(err *Error)
	SpawnRecover(arg proto.Message, err *Error)
	MessageRecover(name string, arg proto.Message, err *Error)
	ScaleRecover(name string, arg proto.Message, err *Error)
	TaskRecover(taskID uint64, name string, arg proto.Message, err *Error)
	StopRecover(err *Error)
}

type AtomosUtilities interface {
	// Log
	// Atom日志。
	// Atom Logs.
	Log() Logging

	// Task
	// Atom任务
	// Atom Tasks.
	Task() Task
}
