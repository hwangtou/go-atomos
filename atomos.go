package go_atomos

import "google.golang.org/protobuf/proto"

// 开发者需要实现的Atom的接口定义。
// Atom Interface Definition that developers have to implement.

// Atomos
// Atomos类型
type Atomos interface {
	String() string

	// Halt 关闭
	// Halt
	Halt(from ID, cancelled []uint64) (save bool, data proto.Message)
}

// AtomosUtilities Atomos的实用工具集。
// Atomos Utilities.
type AtomosUtilities interface {
	// Log 日志。
	// Logging.
	Log() Logging

	// Task 任务，用于把任务放到Atomos的任务队列中。这是同步串行的执行逻辑。
	// Task is used to put tasks into the Atomos task queue. This is a synchronous serial execution logic.
	Task() Task
}
