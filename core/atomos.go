package core

import "google.golang.org/protobuf/proto"

// 开发者需要实现的Atom的接口定义。
// Atom Interface Definition that developers have to implement.

// Atom
// 类型
// Atom type.
type Atomos interface {
	OnMessaging(*IDInfo, string, proto.Message) (proto.Message, *ErrorInfo)
	OnReloading()
	OnTasking()
	OnStopping()
	////Spawn(self AtomSelf, arg proto.Message) error
	//Halt(from ID, cancels map[uint64]CancelledTask) (saveData proto.Message)
	//
	//onReceive(mail *mail)
	//handleMessage(from ID, name string, in proto.Message) (out proto.Message, err *ErrorInfo)
	//handleKill(killAtomMail *atomosMail, cancels map[uint64]CancelledTask) *ErrorInfo
	//handleReload(am *atomosMail) *ErrorInfo
	//handleWormhole(action int, wormhole WormholeDaemon) *ErrorInfo
}
