package go_atomos

import "google.golang.org/protobuf/proto"

// 开发者需要实现的Atom的接口定义。
// Atom Interface Definition that developers have to implement.

// Atomos
// Atomos类型
type Atomos interface {
	Description() string

	// Halt
	// 关闭
	Halt(from ID, cancelled map[uint64]CancelledTask) (save bool, data proto.Message)

	// Reload
	// 新的Atomos会收到调用，把旧的有用的东西转移到newInstance，旧的会被废弃。
	Reload(oldInstance Atomos)
}

// AtomosTransaction
// Atomos事务
type AtomosTransaction interface {
	// Begin
	// 开始事务
	// 传入事务名称，开发者可以根据不同的name来决定不同的事务处理方式，例如如何接受和拒绝请求。
	Begin(name string)

	// End
	// 结束事务
	// 成功的话，会放弃Clone出来的对象，End调用到本来的Atomos。
	// 失败的话，会使用Clone出来的对象，End调用到Clone出来的Atomos，本来的Atomos会被抛弃。
	End(succeed bool)

	// Clone
	// 事务开始时的复制备份
	Clone() Atomos
}

type AtomosUtilities interface {
	// Log
	// Atomos日志。
	// Atomos Logs.
	Log() Logging

	// Task
	// Atomos任务
	// Atomos Tasks.
	Task() Task
}
