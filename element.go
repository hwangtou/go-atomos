package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"time"
)

// Element 类型
// Element type
//
// 我们可以把Element理解成Atom的类型和管理器容器，类似面向对象的class和对应实例的容器管理器的概念。
// 两种Element类型，本地Element和远程Element。
// We can think of Element as the type and manager container of Atom, similar to the concept of class and corresponding instance container manager in object-oriented.
// There are two kinds of Element, Local Element and Remote Element.
type Element interface {
	ID

	// GetAtomID
	// 通过Atom名称获取指定的Atom的ID。
	// Get AtomID by name of Atom.
	GetAtomID(name string, tracker *IDTrackerInfo, fromLocalOrRemote bool) (ID, *IDTracker, *Error)

	// 该Element的Atom数量信息。
	// Atom number information of this Element.

	GetAtomsNum() int
	GetActiveAtomsNum() int
	GetAllInactiveAtomsIDTrackerInfo() map[string]string

	// SpawnAtom
	// 启动（自旋）一个Atom。
	// Spawn (spin) an Atom.
	SpawnAtom(name string, arg proto.Message, tracker *IDTrackerInfo, fromLocalOrRemote bool) (ID, *IDTracker, *Error)

	// ScaleGetAtomID
	// 通过Atom名称获取指定的Atom的ID，这个ID由开发者提供的Element中的"Scale*GetID"方法返回。
	// Get AtomID by name of Atom, this ID is returned by the "Scale*GetID" method in the Element provided by the developer.
	ScaleGetAtomID(callerID SelfID, name string, timeout time.Duration, in proto.Message, tracker *IDTrackerInfo, fromLocalOrRemote bool) (ID, *IDTracker, *Error)
}
