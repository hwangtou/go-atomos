package go_atomos

// CHECKED!

import (
	"google.golang.org/protobuf/proto"
)

//
// Atom
//

// 开发者需要实现的Atom的接口定义。
// Atom Interface Definition that developers have to implement.

// Atom类型
// Atom type.
type Atom interface {
	//Spawn(self AtomSelf, arg proto.Message) error
	Halt(from Id, cancels map[uint64]CancelledTask) (saveData proto.Message)
}

//// 可以被重载的Atom类型
//// Reloadable Atom type.
//type AtomReloadable interface {
//	// 旧的Atom实例在Reload前被调用，通知Atom准备重载。
//	// Calls the old Atom before it is going to be reloaded.
//	WillReload()
//
//	// 新的Atom实例在Reload后被调用，通知Atom已经重载。
//	// Calls the new Atom after it has already been reloaded.
//	DoReload()
//}
//
//// 有状态的Atom
//// 充分发挥protobuf的优势，在Atom非运行时，可以能够保存和恢复所有数据，所以千万不要把数据放在闭包中。
////
//// Stateful Atom
//// Take advantage of protobuf, Atom can save and recovery all data of itself, so never to take data
//// reference into closure.
//type AtomStateful interface {
//	Atom
//	proto.Message
//}
//
//// 无状态的Atom
//// 这种Atom没有需要保存和恢复的东西。
////
//// Stateless Atom
//// Such a type of Atom, is no need to save or recovery anything.
//type AtomStateless interface {
//	Atom
//}

// 暴露给Atom开发者使用的Atom接口。
// Some methods of Atom interface that expose Atom developers to use.

//
// Id
//

// Id，是Atom的类似句柄的对象。
// Id, an instance that similar to file descriptor of the Atom.
type Id interface {
	// Atom所在Cosmos节点。
	// Cosmos Node of the Atom.
	Cosmos() CosmosNode

	// Atom所属的Element类型。
	// Element type of the Atom.
	Element() Element

	// Atom的名称。
	// Name of the Atom.
	Name() string

	// ElementInterface的版本。
	// Version of ElementInterface.
	Version() uint64

	// 从其它Atom或者main发送Kill消息。
	// write Kill signal from other Atom or main.
	Kill(from Id) error

	// 内部使用，如果是本地Atom，会返回本地Atom的引用。
	// Inner use only, if Atom is local, it returns the local AtomCore reference.
	getLocalAtom() *AtomCore
}

//
// AtomSelf
//

// AtomSelf，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// AtomSelf, a concept that provide Atom resource access to inner Atom.
// With AtomSelf, Atom can access its self-cosmos with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provide Log and Tasks method to inner Atom.
type AtomSelf interface {
	Id

	// 获取Atom的CosmosSelf。
	// Access to the CosmosSelf of the Atom.
	CosmosSelf() *CosmosSelf

	// Atom从内部杀死自己。
	// Atom kills itself from inner.
	KillSelf()

	// Atom日志。
	// Atom Logs.
	Log() *atomLogsManager

	// Atom任务
	// Atom Tasks.
	Task() *atomTasksManager
}

//
// Wormhole
//

// WormholeAtom，支持WormholeAtom的Atom，可以得到Wormhole的支持。
// Implement WormholeAtom interface to gain wormhole support.
type WormholeAtom interface {
	Atom
	AcceptWorm(control WormholeControl) error
	CloseWorm(control WormholeControl)
}

// WormholeId，是Id接口的延伸，提供向WormholeAtom发送Wormhole的可能。
// Extend of Id, it lets send wormhole to WormholeAtom become possible.
type WormholeId interface {
	Id
	Accept(daemon WormholeDaemon) error
}

// WormholeDaemon，通常包装着wormhole（真实网络连接）。负责接受信息并处理，并提供操作接口。
// WormholeDaemon generally used to wrap the real connection. It handles message processing,
// and provides operating methods.
type WormholeDaemon interface {
	Daemon(AtomSelf) error
	WormholeControl
}

// WormholeControl，向WormholeAtom提供发送和关闭接口。
// WormholeControl provides Send and Close to WormholeAtom.
type WormholeControl interface {
	Send([]byte) error
	Close(isKickByNew bool) error
}
