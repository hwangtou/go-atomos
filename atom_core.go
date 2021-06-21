package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"log"
	"sync"
)

// Actual Atom

type AtomCore struct {
	element  *ElementLocal
	name     string
	instance Atom
	state    AtomState
	mailbox  *MailBox
	task     atomTasksManager
	log      atomLogsManager
	atomId   *AtomId
}

var atomsPool = sync.Pool{
	New: func() interface{} {
		return &AtomCore{}
	},
}

//
// Id
//

func (a *AtomCore) Cosmos() CosmosNode {
	return a.element.cosmos.local
}

func (a *AtomCore) Element() Element {
	return a.element
}

func (a *AtomCore) Name() string {
	return a.name
}

// Kill from other AtomCore
func (a *AtomCore) Kill(from Id) error {
	if ok := a.element.define.AtomCanKill(from); !ok {
		return ErrAtomCannotKill
	}
	return a.pushKillMail(from)
}

func (a *AtomCore) getLocalAtom() *AtomCore {
	return a
}

//
// AtomSelf
//

func (a *AtomCore) CosmosSelf() *CosmosSelf {
	return a.element.cosmos
}

// Atom kill itself from inner
func (a *AtomCore) KillSelf() {
	if err := a.pushKillMail(nil); err != nil {
		log.Println("SelfKill failed.", err) // todo
	}
}

func (a *AtomCore) Log() *atomLogsManager {
	return &a.log
}

func (a *AtomCore) Task() *atomTasksManager {
	return &a.task
}

// INTERNAL

// Life Cycle

// Objective-C likes coding style: Alloc/Init/Release/Dealloc
func allocAtom() *AtomCore {
	return atomsPool.Get().(*AtomCore)
}

func initAtom(a *AtomCore, es *ElementLocal, name string, inst Atom) {
	a.element = es
	a.name = name
	a.instance = inst
	a.state = Halt
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

// Mailbox Handler
// todo: performance tracker

func (a *AtomCore) onReceive(mail *Mail) {
	am := mail.Content.(*atomMail)
	switch am.typ {
	case MailTypeMessage:
		resp, err := a.handleMessage(am.from, am.name, am.arg)
		am.sendReply(resp, err)
		// Mail dealloc in Element.messagingAtom
	case MailTypeTask:
		a.task.handleTask(am)
		// Mail dealloc in atomTasksManager.handleTask and cancels
	default:
		panic("AtomCore receive unsupported mail type")
	}
}

func (a *AtomCore) onPanic(mail *Mail, trace string) {
	am := mail.Content.(*atomMail)
	// Try to reply here, to prevent mail non-reply, and stub.
	switch am.typ {
	case MailTypeMessage:
		am.sendReply(nil, ErrMailBoxPanic)
	case MailTypeHalt:
		am.sendReply(nil, ErrMailBoxPanic)
	}
}

// TODO: Think about Panic happens during stopping, the remaining mails might not be reply, the callers might stub.
func (a *AtomCore) onStop(killMail, remainMails *Mail, num uint32) {
	a.setBusy()
	defer a.setHalt()

	killAtomMail := killMail.Content.(*atomMail)
	cancels := a.task.cancelAllSchedulingTasks()
	for ; remainMails != nil; remainMails = remainMails.next {
		remainAtomMail := remainMails.Content.(*atomMail) //(*atomMail)(unsafe.Pointer(&remainMails))
		switch remainAtomMail.typ {
		case MailTypeHalt:
			remainAtomMail.sendReply(nil, ErrMailBoxClosed)
			// Mail dealloc in Element.killingAtom
		case MailTypeMessage:
			remainAtomMail.sendReply(nil, ErrMailBoxClosed)
			// Mail dealloc in Element.messagingAtom
		case MailTypeTask:
			// TODO: Is it needed? Just want to clean up those new mails receive after cancelAllSchedulingTasks
			t, err := a.task.cancelTask(remainMails.id, nil)
			if err != nil {
				cancels[remainMails.id] = t
			}
			// Mail dealloc in atomTasksManager.handleTask and cancels
		//case MailTypeReload:
		//	remainAtomMail.sendReply(nil, ErrMailBoxClosed)
		//	// Mail dealloc in Element.upgradingAtom
		default:
			panic("AtomCore receive unsupported mail type")
		}
	}
	// Reply kill
	err := a.handleKill(killAtomMail, cancels)
	killAtomMail.sendReply(nil, err)
	// Mail dealloc in Element.killingAtom
	releaseAtom(a)
	deallocAtom(a)
}

//func (a *AtomCore) onRestart(mail *Mail) {
//	am := mail.Content.(*atomMail)
//	if err := a.handleUpgrade(am); err != nil {
//		log.Println("ErrMailTypeUpgrade")
//	}
//	// Mail dealloc in Element.upgradingAtom
//}

// Push & Handle

// Message
func (a *AtomCore) pushMessageMail(from Id, message string, args proto.Message) (reply proto.Message, err error) {
	am := allocAtomMail()
	initMessageMail(am, from, message, args)
	if ok := a.mailbox.PushTail(am.Mail); !ok {
		return reply, ErrAtomIsNotSpawning
	}
	replyInterface, err := am.waitReply()
	deallocAtomMail(am)
	reply, ok := replyInterface.(proto.Message)
	if !ok {
		return reply, ErrAtomReplyType
	}
	return reply, err
}

func (a *AtomCore) handleMessage(from Id, name string, in proto.Message) (out proto.Message, err error) {
	a.setBusy()
	defer a.setWaiting()
	call := a.element.getCall(name)
	if call == nil {
		return nil, ErrAtomMessageNotFound
	}
	return call.Handler(from, a.instance, in)
}

// Kill
func (a *AtomCore) pushKillMail(from Id) error {
	am := allocAtomMail()
	initKillMail(am, from)
	if ok := a.mailbox.PushHead(am.Mail); !ok {
		return ErrAtomIsNotSpawning
	}
	_, err := am.waitReply()
	deallocAtomMail(am)
	return err
}

func (a *AtomCore) handleKill(killAtomMail *atomMail, cancels map[uint64]CancelledTask) error {
	a.setBusy()
	defer a.setHalt()

	a.instance.Halt(killAtomMail.from, cancels)
	if inst, ok := a.instance.(AtomStateful); ok {
		if err := a.element.define.AtomSaver(a, inst); err != nil {
			// todo
			return err
		}
	}
	return nil
}

//// Upgrade
//func (a *AtomCore) pushUpgradeMail(define *ElementDefine) error {
//	am := allocAtomMail()
//	initUpgradeMail(am, define)
//	if ok := a.mailbox.PushHead(am.Mail); !ok {
//		return ErrAtomIsNotSpawning
//	}
//	return nil
//}
//
//func (a *AtomCore) handleUpgrade(am *atomMail) error {
//	a.setBusy()
//	defer a.setWaiting()
//
//	newDefine := am.upgrade
//	deallocAtomMail(am)
//
//	switch inst := a.instance.(type) {
//	case AtomStateful:
//		newInst, ok := newDefine.AtomNew().(AtomStateful)
//		if !ok {
//			return ErrAtomReload
//		}
//		oldBuf, err := proto.Marshal(inst)
//		if err != nil {
//			return err
//		}
//		err = proto.Unmarshal(oldBuf, newInst)
//		if err != nil {
//			return err
//		}
//		a.instance = newInst
//	case AtomStateless:
//		newInst, ok := newDefine.AtomNew().(AtomStateless)
//		if !ok {
//			return ErrAtomReload
//		}
//		a.instance = newInst
//		arg := inst.Reload()
//		err := newInst.Spawn(a, arg)
//		if err != nil {
//			return err
//		}
//	}
//
//	return nil
//}

// State
// State of Atom

func (a *AtomCore) GetState() AtomState {
	return a.state
}

func (a *AtomCore) setBusy() {
	a.state = Busy
}

func (a *AtomCore) setWaiting() {
	a.state = Waiting
}

func (a *AtomCore) setHalt() {
	a.state = Halt
}
