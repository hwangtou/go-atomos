package atomos

import "runtime/debug"

// SerialQueue

type taskMailbox struct {
	serial  bool
	mailbox *mailBox
}

func createTaskMailbox(name string, serial bool, logging *loggingAtomos) *taskMailbox {
	a := &taskMailbox{
		serial: serial,
	}
	a.mailbox = newMailBox(name, a, logging)
	if err := a.mailbox.start(nil); err != nil {
		logging.pushFrameworkFatalLog("createTaskMailbox: Failed to start mailbox %s: %v", name, err)
		panic("createTaskMailbox: Failed to start mailbox")
	}
	return a
}

func (m *taskMailbox) mailboxOnStartUp(_ func() *Error) *Error {
	return nil
}

func (m *taskMailbox) mailboxOnReceive(mail *mail) {
	m.handleAtomosMail(mail)
	am := mail.data.(*baseAtomosMail)
	if m.serial {
		am.atomosTask.helper.manager.checkCloseTaskSerialMailbox(am.atomosTask.helper, m)
	} else {
		am.atomosTask.helper.manager.checkCloseTaskMailbox(m)
	}
}

func (m *taskMailbox) mailboxOnStop(_, remainMails *mail, num uint32) *Error {
	if remainMails != nil {
		m.mailbox.logging.pushFrameworkFatalLog("taskMailbox: Stop with %d remain mails.", num)
	}
	return nil
}

func (m *taskMailbox) handleAtomosMail(mail *mail) {
	am := mail.data.(*baseAtomosMail)
	helper := am.atomosTask.helper
	at := helper.manager
	// Wrap the user closure in a recover so that a panic does not escape into
	// the mailbox loop (which would only log a generic error and drop the rest
	// of the queue). Mirrors atomosTaskManager.handleTaskQueue: invoke the
	// helper's recoverFn when provided, otherwise log the panic.
	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					at.atomos.log.Fatal("AtomosTask: Recover from panic in panic handler (taskMailbox). reason=(%v), stack=(%s)\n", r2, string(debug.Stack()))
				}
			}()
			if f := helper.recoverFn; f != nil {
				f(r)
			} else {
				at.atomos.log.Fatal("AtomosTask: Recover from panic when handling task (taskMailbox). reason=(%v), stack=(%s)\n", r, string(debug.Stack()))
			}
		}
	}()
	if fn := helper.task; fn != nil {
		fn(am.mail.id)
	} else if fnWithCallback := helper.taskWithCallback; fnWithCallback != nil {
		callback := fnWithCallback(am.mail.id)
		if callback != nil {
			at.atomos.PushTaskCallbackMail(am.name, callback, helper.recoverFn)
		}
	} else {
		at.atomos.log.Fatal("AtomosTask: Task closure is nil. taskHelper=(%+v)", helper)
	}
}
