package atomos

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
