package go_atomos

// CHECKED!

import (
	"errors"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
)

// Atom状态
// AtomState

type AtomState int

const (
	// 停止
	// Atom is stopped.
	AtomHalt AtomState = 0

	// 启动中
	// Atom is starting up.
	AtomSpawning AtomState = 1

	// 启动成功，等待消息
	// Atom is started and waiting for message.
	AtomWaiting AtomState = 2

	// 启动成功，正在处理消息
	// Atom is started and busy processing message.
	AtomBusy AtomState = 3

	// 停止中
	// Atom is stopping.
	AtomStopping AtomState = 4
)

// Atom Error

var (
	ErrFromNotFound     = errors.New("atom fromId not found")
	ErrAtomNotFound     = errors.New("atom not found")
	ErrAtomExists       = errors.New("atom exists")
	ErrAtomCannotSpawn  = errors.New("atom cannot spawn")
	ErrAtomIsNotRunning = errors.New("atom is not running")
	ErrAtomCannotKill   = errors.New("atom cannot be killed")

	ErrAtomType             = errors.New("atom type error")
	ErrAtomMessageAtomType  = errors.New("atom message atom type error")
	ErrAtomMessageArgType   = errors.New("atom message arg type error")
	ErrAtomMessageReplyType = errors.New("atom message reply type error")
)

// Actual Atom
// The implementations of a local Atom type.

type AtomCore struct {
	// 对目前ElementLocal实例的引用。
	// 具体指向的ElementLocal对象对AtomCore是只读的，因此对其所有的操作都需要加上读锁。
	// 指向ElementLocal引用只可以在Atom加载和Cosmos被更新时改变。
	//
	// Reference to current ElementLocal instance.
	// The concrete ElementLocal instance should be read-only, so read-lock is required when access to it.
	// The reference only be set when atom load and cosmos upgrade.
	element *ElementLocal

	// ElementInterface的版本
	// Version of ElementInterface.
	version uint64

	// ElementLocal中的唯一Name。
	// Unique Name of atom in the ElementLocal.
	name string

	// 开发者实现的Atom类型的实例，通过ElementLocal中的AtomCreator方法创建。
	//
	// Atom-type instance that has been created by AtomConstructor method of ElementLocal,
	// which is implemented by developer.
	instance Atom

	// 状态
	// State
	state AtomState

	// 邮箱，也是实现Atom无锁队列的关键。
	// Mailbox, the key of lockless queue of Atom.
	mailbox *MailBox

	// 任务管理器，用于处理来自Atom内部的任务调派。
	// Task Manager, uses to handle Task from inner Atom.
	task atomTasksManager

	// 日志管理器，用于处理来自Atom内部的日志。
	// Logs Manager, uses to handle Log from inner Atom.
	log atomLogsManager

	// Atom信息的protobuf对象，以便于Atom信息的序列化。
	// Protobuf instance of Atom information, for a convenience serialization of Atom information.
	atomId *AtomId
}

// Atom对象的内存池
// Atom instance pools.
var atomsPool = sync.Pool{
	New: func() interface{} {
		return &AtomCore{}
	},
}

//
// Implementation of Id
//
// Id，相当于Atom的句柄的概念。
// 通过Id，可以访问到Atom所在的Cosmos、Element、Name，以及发送Kill信息，但是否能成功Kill，还需要AtomCanKill函数的认证。
// 直接用AtomCore继承Id，因此本地的Id直接使用AtomCore的引用即可。
//
// Id, a concept similar to file descriptor of an atom.
// With Id, we can access the Cosmos, Element and Name of the Atom. We can also send Kill signal to the Atom,
// then the AtomCanKill method judge kill it or not.
// AtomCore implements Id interface directly, so local Id is able to use AtomCore reference directly.

func (a *AtomCore) Cosmos() CosmosNode {
	return a.element.cosmos.local
}

func (a *AtomCore) Element() Element {
	return a.element
}

func (a *AtomCore) Name() string {
	return a.name
}

func (a *AtomCore) Version() uint64 {
	return a.version
}

// 从另一个AtomCore，或者从Main Script发送Kill消息给Atom。
// write Kill signal from other AtomCore or from Main Script.
func (a *AtomCore) Kill(from Id) error {
	if ok := a.element.implements[a.version].Developer.AtomCanKill(from); !ok {
		return ErrAtomCannotKill
	}
	return a.pushKillMail(from)
}

func (a *AtomCore) getLocalAtom() *AtomCore {
	return a
}

//
// Implementation of AtomSelf
//
// AtomSelf，是Atom内部可以访问的Atom资源的概念。
// 通过AtomSelf，Atom内部可以访问到自己的Cosmos（CosmosSelf）、可以杀掉自己（KillSelf），以及提供Log和Task的相关功能。
//
// AtomSelf, a concept that provide Atom resource access to inner Atom.
// With AtomSelf, Atom can access its self-cosmos with "CosmosSelf", can kill itself use "KillSelf" from inner.
// It also provide Log and Tasks method to inner Atom.

func (a *AtomCore) CosmosSelf() *CosmosSelf {
	return a.element.cosmos
}

// Atom kill itself from inner
func (a *AtomCore) KillSelf() {
	id, elem := a.atomId, a.element
	if err := a.pushKillMail(a); err != nil {
		elem.cosmos.logInfo("AtomCore: Kill self error, id=%+v,err=%s", id, err)
		return
	}
	elem.cosmos.logInfo("AtomCore: Kill self, id=%+v", id)
}

func (a *AtomCore) Log() *atomLogsManager {
	return &a.log
}

func (a *AtomCore) Task() *atomTasksManager {
	return &a.task
}

// 内部实现
// INTERNAL

// 生命周期相关
// Life Cycle
// Objective-C likes coding style: Alloc/Init/Release/Dealloc

func allocAtom() *AtomCore {
	return atomsPool.Get().(*AtomCore)
}

func initAtom(a *AtomCore, es *ElementLocal, name string, inst Atom) {
	a.element = es
	a.version = es.current.Interface.Config.Version
	a.name = name
	a.instance = inst
	a.state = AtomHalt
	a.atomId = &AtomId{
		Node:    a.CosmosSelf().GetName(),
		Element: a.Element().GetName(),
		Name:    a.name,
	}
	initAtomLog(&a.log, a)
	initAtomTasksManager(&a.task, a)
}

func releaseAtom(a *AtomCore) {
	releaseAtomTask(&a.task)
	releaseAtomLog(&a.log)
}

func deallocAtom(a *AtomCore) {
	atomsPool.Put(a)
}

// 邮箱控制器相关
// Mailbox Handler
// TODO: Performance tracer.

// 处理邮箱消息。
// Handle mailbox messages.
func (a *AtomCore) onReceive(mail *Mail) {
	am := mail.Content.(*atomMail)
	switch am.mailType {
	case AtomMailMessage:
		resp, err := a.handleMessage(am.from, am.name, am.arg)
		am.sendReply(resp, err)
		// Mail dealloc in AtomCore.pushMessageMail.
	case AtomMailTask:
		a.task.handleTask(am)
		// Mail dealloc in atomTasksManager.handleTask and cancels.
	case AtomMailReload:
		err := a.handleReload(am)
		am.sendReply(nil, err)
		// Mail dealloc in AtomCore.pushReloadMail.
	default:
		a.element.cosmos.logFatal("AtomCore.onReceive: Unknown message type, type=%v,mail=%+v", am.mailType, am)
	}
}

// 处理邮箱消息时发生的异常。
// Handle mailbox panic while it is processing Mail.
func (a *AtomCore) onPanic(mail *Mail, trace string) {
	am := mail.Content.(*atomMail)
	// Try to reply here, to prevent mail non-reply, and stub.
	errMsg := fmt.Errorf("AtomCore.onPanic: PANIC, name=%s,trace=%s", a.name, trace)
	switch am.mailType {
	case AtomMailMessage:
		am.sendReply(nil, errMsg)
		// Mail then will be dealloc in AtomCore.pushMessageMail.
	case AtomMailHalt:
		am.sendReply(nil, errMsg)
		// Mail then will be dealloc in AtomCore.pushKillMail.
	case AtomMailReload:
		am.sendReply(nil, errMsg)
		// Mail then will be dealloc in AtomCore.pushReloadMail.
	}
}

// 处理邮箱退出。
// Handle mailbox stops.
func (a *AtomCore) onStop(killMail, remainMails *Mail, num uint32) {
	a.task.stopLock()
	defer a.task.stopUnlock()

	a.setStopping()
	defer a.setHalt()

	killAtomMail := killMail.Content.(*atomMail)
	cancels := a.task.cancelAllSchedulingTasks()
	for ; remainMails != nil; remainMails = remainMails.next {
		remainAtomMail := remainMails.Content.(*atomMail)
		switch remainAtomMail.mailType {
		case AtomMailHalt:
			remainAtomMail.sendReply(nil, ErrMailBoxClosed)
			// Mail dealloc in AtomCore.pushKillMail.
		case AtomMailMessage:
			remainAtomMail.sendReply(nil, ErrMailBoxClosed)
			// Mail dealloc in AtomCore.pushMessageMail.
		case AtomMailTask:
			// Is it needed? It just for preventing new mails receive after cancelAllSchedulingTasks,
			// but it's impossible to add task after locking.
			a.element.cosmos.logFatal("AtomCore.onStop: FRAMEWORK ERROR, some task mails have not been deleted yet.")
			t, err := a.task.cancelTask(remainMails.id, nil)
			if err != nil {
				cancels[remainMails.id] = t
			}
			// Mail dealloc in atomTasksManager.cancelTask.
		case AtomMailReload:
			remainAtomMail.sendReply(nil, ErrMailBoxClosed)
			// Mail dealloc in AtomCore.pushReloadMail.
		default:
			a.element.cosmos.logFatal("AtomCore.onStop: Unknown message type, type=%v,mail=%+v",
				remainAtomMail.mailType, remainAtomMail)
		}
	}

	// Handle Kill and Reply Kill.
	err := a.handleKill(killAtomMail, cancels)
	killAtomMail.sendReply(nil, err)
	// Mail dealloc in Element.killingAtom
	a.element.mutex.Lock()
	delete(a.element.atoms, a.name)
	a.element.mutex.Unlock()
	releaseAtom(a)
	deallocAtom(a)
}

// 推送邮件，并管理邮件对象的生命周期。
// 处理邮件，并设置Atom的运行状态。
//
// Push Mail, and manage life cycle of Mail instance.
// Handle Mail, and set the state of Atom.

// Message Mail

// TODO: dead-lock loop checking
func (a *AtomCore) pushMessageMail(from Id, message string, args proto.Message) (reply proto.Message, err error) {
	am := allocAtomMail()
	initMessageMail(am, from, message, args)
	if ok := a.mailbox.PushTail(am.Mail); !ok {
		return reply, ErrAtomIsNotRunning
	}
	replyInterface, err := am.waitReply()
	deallocAtomMail(am)
	reply, ok := replyInterface.(proto.Message)
	if !ok {
		return reply, fmt.Errorf("AtomCore.pushMessageMail: Reply type error, message=%s,args=%+v,reply=%+v",
			message, args, replyInterface)
	}
	return reply, err
}

func (a *AtomCore) handleMessage(from Id, name string, in proto.Message) (out proto.Message, err error) {
	a.setBusy()
	defer a.setWaiting()
	handler := a.element.getMessageHandler(name, a.version)
	if handler == nil {
		return nil, fmt.Errorf("AtomCore.handleMessage: Handler not found, name=%s", name)
	}
	return handler(from, a.instance, in)
}

// Kill Mail

func (a *AtomCore) pushKillMail(from Id) error {
	am := allocAtomMail()
	initKillMail(am, from)
	if ok := a.mailbox.PushHead(am.Mail); !ok {
		return ErrAtomIsNotRunning
	}
	_, err := am.waitReply()
	deallocAtomMail(am)
	return err
}

// 有状态的Atom会在Halt被调用之后调用AtomSaver函数保存状态，期间Atom状态为Stopping。
// Stateful Atom will save data after Halt method has been called, while is doing this, Atom is set to Stopping.
func (a *AtomCore) handleKill(killAtomMail *atomMail, cancels map[uint64]CancelledTask) error {
	a.setStopping()
	a.instance.Halt(killAtomMail.from, cancels)
	if inst, ok := a.instance.(AtomStateful); ok {
		if err := a.element.implements[a.version].Developer.AtomSaver(a, inst); err != nil {
			a.element.cosmos.logInfo("AtomCore.handleKill: Save atom failed, id=%+v,version=%d,inst=%+v",
				a.atomId, a.version, a.instance)
			return err
		}
	}
	return nil
}

// Reload Mail

// 重载邮件，指定Atom的版本。
// Reload Mail with specific version.
func (a *AtomCore) pushReloadMail(version uint64) error {
	am := allocAtomMail()
	initReloadMail(am, version)
	if ok := a.mailbox.PushHead(am.Mail); !ok {
		return ErrAtomIsNotRunning
	}
	_, err := am.waitReply()
	deallocAtomMail(am)
	return err
}

// TODO: Test.
// 注意：Reload伴随着整个Element的Reload，而且即使reload失败，但也不停止Atom的运行。
// Notice: Atom reload goes with the Element reload, even an Atom reload failed, it doesn't stop Atom from running.
func (a *AtomCore) handleReload(am *atomMail) error {
	a.setBusy()
	defer a.setWaiting()

	// 检查Atom是否可以被重载。
	// Check if the Atom can be reloaded.
	reloadableAtom, ok := a.instance.(AtomReloadable)
	if !ok {
		return errors.New("atom cannot be reloaded")
	}

	// 如果没有新的Element，就用旧的Element。
	// Use old Element if there is no new Element.
	reloadElementImplement, has := a.element.implements[am.upgradeVersion]
	if !has {
		return errors.New("atom upgrade version invalid")
	}
	// 释放邮件。
	// Dealloc Atom Mail.
	deallocAtomMail(am)

	// Save old data.
	var err error
	var oldBuf []byte
	switch oldInst := a.instance.(type) {
	case AtomStateful:
		oldBuf, err = proto.Marshal(oldInst)
		if err != nil {
			return err
		}
	}
	// Notice reloading.
	reloadableAtom.WillReload()
	// Restoring data and replace instance.
	a.version = am.upgradeVersion
	a.instance = reloadElementImplement.Developer.AtomConstructor()
	switch inst := a.instance.(type) {
	case AtomStateful:
		err = proto.Unmarshal(oldBuf, inst)
		if err != nil {
			return err
		}
	}
	// 通知新的Atom已经Reload。（但如果新的Atom不支持Reload呢？）
	// Notifying the new Atom reloaded. But how about the new Atom not supports Reloadable?
	newAtom, ok := a.instance.(AtomReloadable)
	if ok {
		newAtom.DoReload()
	}
	return nil
}

// 各种状态
// State
// State of Atom

func (a *AtomCore) getState() AtomState {
	return a.state
}

func (a *AtomCore) setBusy() {
	a.state = AtomBusy
}

func (a *AtomCore) setWaiting() {
	a.state = AtomWaiting
}

func (a *AtomCore) setStopping() {
	a.state = AtomStopping
}

func (a *AtomCore) setHalt() {
	a.state = AtomHalt
}
