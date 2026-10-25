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
	am := mail.data.(*baseAtomosMail)
	helper := am.atomosTask.helper
	m.handleAtomosMail(mail)
	if m.serial {
		helper.manager.checkCloseTaskSerialMailbox(helper, m)
	} else {
		helper.manager.checkCloseTaskMailbox(helper, m)
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
	// 串行/并发队列邮箱与主邮箱走同一套任务状态机:
	// 已取消的任务跳过执行、Mailing→Executing→Done 转换、完成后从 manager.tasks 清理。
	am.atomosTask.helper.manager.handleTaskQueue(am)
}
