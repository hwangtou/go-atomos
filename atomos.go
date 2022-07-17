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
