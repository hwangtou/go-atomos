package atomos

import (
	"google.golang.org/protobuf/proto"
)

// Element is the type and container of Atoms — conceptually similar to a
// class plus the manager of its instances in object-oriented terms. There
// are two kinds of Element: local and remote.
// Element 类型：Atom的类型和管理器容器。两种Element类型，本地Element和远程Element。
type Element interface {
	ID

	// GetAtomID returns the ID of the Atom with the given name.
	// 通过Atom名称获取指定的Atom的ID。
	//
	// Pass ArgBaseAtomosFromLocalOnly/ArgBaseAtomosFromRemoteOnly in args for
	// user-originated calls that need IDTracker reference tracking; with no
	// routing mark the call is treated as framework-internal (untracked).
	GetAtomID(name string, tracker *IDTrackerInfo, args ...ArgsForBaseAtomos) (ID, *IDTracker, error)

	// Atom count information of this Element.
	// 该Element的Atom数量信息。

	GetAtomsNum() int
	GetActiveAtomsNum() int
	GetAllInactiveAtomsIDTrackerInfo() map[string]string

	// SpawnAtom spawns an Atom with the given name and argument.
	// 启动（自旋）一个Atom。
	//
	// Pass ArgBaseAtomosFromLocalOnly/ArgBaseAtomosFromRemoteOnly in args for
	// user-originated calls that need IDTracker reference tracking; with no
	// routing mark the call is treated as framework-internal (untracked).
	SpawnAtom(callerID ID, name string, arg proto.Message, tracker *IDTrackerInfo, args ...ArgsForBaseAtomos) (ID, *IDTracker, error)
}
