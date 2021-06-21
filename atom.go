package go_atomos

import (
	"google.golang.org/protobuf/proto"
)

// AtomState

type AtomState int

const (
	Halt     AtomState = 0
	Spawning AtomState = 1
	Waiting  AtomState = 2
	Busy     AtomState = 3
	Stopping AtomState = 4
)

// TaskState

type TaskState int

const (
	TaskScheduling TaskState = 0
	TaskCancelled  TaskState = 1
	TaskMailing    TaskState = 2
)

// Atom

type Atom interface {
	Spawn(self AtomSelf, arg proto.Message) error
	Halt(from Id, cancels map[uint64]CancelledTask)
}

// 有状态的Atom
// 充分发挥protobuf的优势
// Sleep和Awake时，需要能够保存和恢复所有数据
// 所以千万不要把数据放在闭包中
type AtomStateful interface {
	Atom
	proto.Message
}

// 无状态的Atom
type AtomStateless interface {
	Atom
	Reload() proto.Message
}

// AtomSelf
// Like Pointer to Atom

type AtomSelf interface {
	Id
	CosmosSelf() *CosmosSelf
	KillSelf()
	Log() *atomLogsManager
	Task() *atomTasksManager
}
