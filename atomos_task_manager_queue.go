package atomos

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	concurrentMailboxSuffix = "::concurrent"
)

//func (at *atomosTaskManager) GetMarking(mark string) uint64 {
//
//}

// AddToAtomosQueue
// 添加任务到Atomos的任务队列，并返回一个取消函数。和Task.Add类似，但不返回任务ID。
// Append task to Atomos task queue, and return a cancel function.
func (at *atomosTaskManager) AddToAtomosQueue(workerFn TaskFn, ext ...ArgsForTask) (cancel func(reason string) *Error) {
	return at.addToAtomos(at.atomos.mailbox, workerFn, nil, ext...)
}

// AddToSerialQueue
// 添加任务到指定名称的串行任务队列，并返回一个取消函数。如果该队列不存在，则创建一个新的队列；当队列为空时，删除该队列。
// Append task to a named serial task queue, and return a cancel function. If the queue does not exist, create a new one; when the queue is empty, delete the queue.
func (at *atomosTaskManager) AddToSerialQueue(queueName string, workerFn TaskFnWithCallback, ext ...ArgsForTask) (cancel func(reason string) *Error) {
	mailbox := at.getTaskSerialMailbox(queueName)
	return at.addToAtomos(mailbox.mailbox, nil, workerFn, ext...)
}

// AddToConcurrentQueue
// 添加任务到并行任务队列，并返回一个取消函数。每一个并行任务都是独立的一个goroutine。
// Append task to a concurrent task queue, and return a cancel function. Each concurrent task is an independent goroutine.
func (at *atomosTaskManager) AddToConcurrentQueue(workerFn TaskFnWithCallback, ext ...ArgsForTask) (cancel func(reason string) *Error) {
	mailbox := at.newTaskSerialMailboxForConcurrent()
	return at.addToAtomos(mailbox.mailbox, nil, workerFn, ext...)
}

func (at *atomosTaskManager) HasMarking(mark string) bool {
	at.mutex.Lock()
	defer at.mutex.Unlock()
	for _, t := range at.tasks {
		if t.helper.mark == mark {
			return true
		}
	}
	return false
}

func (at *atomosTaskManager) addToAtomos(mailbox *mailBox, t TaskFn, tCb TaskFnWithCallback, ext ...ArgsForTask) (cancel func(reason string) *Error) {
	helper := createTaskHelper(at, mailbox, t, tCb, ext)
	if helper.hasErrors() {
		helper.errorHelper(at, helper.getErrorReason())
		return func(reason string) *Error { return nil }
	}
	helper.buildAtomosTask()

	am := allocBaseAtomosMail()
	initTaskQueueMail(am, at.closureInfo(1), helper)

	task := helper.atomosTask
	switch true {
	case helper.delay > 0:
		task.timer = time.AfterFunc(helper.delay, func() {
			at.delayAddToQueue(mailbox, task)
		})
		return func(reason string) *Error {
			if err := at.cancelTaskQueue(task, false, reason); err != nil {
				at.atomos.log.Error("atomosTaskManager: Cancel task %d failed because %s", task.id, err.Error())
				return err.AddStack(nil)
			}
			return nil
		}
	case helper.cronSchedule != nil:
		task.cronEID = at.getCron().Schedule(helper.cronSchedule, cron.FuncJob(func() {
			at.delayAddToQueue(mailbox, task)
		}))
		return func(reason string) *Error {
			if err := at.cancelTaskQueue(task, false, reason); err != nil {
				at.atomos.log.Error("atomosTaskManager: Cancel task %d failed because %s", task.id, err.Error())
				return err.AddStack(nil)
			}
			return nil
		}
	default:
		return at.addToQueue(at.atomos.mailbox, task)
	}
}

// Test cases:
// 1. If the task function panics, will it affect the running of Atomos.
// 2. Edge cases, e.g., cancel a task when the timer is just triggered.
// 3. If the parameters of Add and AddAfter are incorrect, will it affect the running of application.
// 4. The performance of Cancel.
// 5. The performance in various running states of Atomos, will it cause application crash because of user's misoperation.

func (at *atomosTaskManager) delayAddToQueue(mailbox *mailBox, t *atomosTask) {
	manager := t.helper.manager
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	it, has := at.tasks[t.id]
	if !has {
		// 当且仅当发生在边缘的情况，任务计时器还未成功被取消，但任务已经被移出容器的情况。
		// It should only happen at the edge of the timer has been triggered,
		// but the TimerTask has been cancelled.
		return
	}
	switch it.timerState {
	// 任务已被取消。
	// 正常来说这段代码不会触发到，除非框架逻辑有问题。
	// Task has been cancelled.
	// This switch-case is unreachable unless framework has a bug.
	case TaskCancelled:
		if ta, has := at.tasks[it.id]; has {
			delete(at.tasks, it.id)
			//deallocAtomosMail(ta.baseAtomosMail)
			_ = ta
			// FRAMEWORK LEVEL ERROR
			// Because it should not happen, once a TimerTask has been cancelled,
			// it will be removed, thread-safely, immediately.
			at.log.pushFrameworkErrorLog("AtomosTask: AddAfter, FRAMEWORK ERROR, timer cancel")
		}
		return

	// 任务正在排程。
	// 这段代码符合正常功能的期待。
	// Task is scheduling.
	// This code is expected.
	case TaskScheduling:
		it.timerState = TaskMailing
		//delete(at.tasks, it.id)
		//if ok := at.atomos.mailbox.pushHead(am.mail); !ok {
		//	at.atomos.log.Fatal("AtomosTask: AddAfter, atomos is not running")
		//}
		at.addToQueue(mailbox, t)

	// FRAMEWORK LEVEL ERROR
	// Because it should not happen, once a TimerTask has been executed,
	// it will be removed, thread-safely, immediately.
	default:
		at.log.pushFrameworkErrorLog("AtomosTask: AddAfter, FRAMEWORK ERROR, timer executing")
	}
}

func (at *atomosTaskManager) addToQueue(mailbox *mailBox, task *atomosTask) (cancel func(reason string) *Error) {
	ok := false
	am := task.atomosMail
	if task.helper.appendToHead {
		ok = mailbox.pushHead(am.mail)
	} else {
		ok = mailbox.pushTail(am.mail)
	}
	if !ok {
		reason := "Failed to add task to Atomos mailbox"
		task.helper.errorHelper(at, reason)
		releaseMail(am.mail)
		return func(reason string) *Error { return nil }
	}
	return func(reason string) *Error {
		if err := at.cancelTaskQueue(task, false, reason); err != nil {
			at.atomos.log.Error("atomosTaskManager: Cancel task %d failed because %s", task.id, err.Error())
			return err.AddStack(nil)
		}
		return nil
	}
}

func (at *atomosTaskManager) handleTaskQueue(am *baseAtomosMail) {
	defer func() {
		if r := recover(); r != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					at.atomos.log.Fatal("AtomosTask: Recover from panic in panic handler. reason=(%v), stack=(%s)\n", r2, string(debug.Stack()))
				}
			}()
			if f := am.atomosTask.helper.recoverFn; f != nil {
				f(r)
			} else {
				at.atomos.log.Fatal("AtomosTask: Recover from panic when handling task. reason=(%v), stack=(%s)\n", r, string(debug.Stack()))
			}
		}
	}()

	cancel := false
	task := am.atomosTask
	manager := am.atomosTask.helper.manager

	manager.mutex.Lock()
	state := task.timerState
	switch state {
	case TaskCancelled:
		cancel = true
	case TaskMailing:
		task.timerState = TaskExecuting
	default:
		manager.mutex.Unlock()
		at.atomos.log.Fatal("AtomosTask: Invalid task state when handling task. state=(%d), task=(%+v)", state, task)
		return
	}
	manager.mutex.Unlock()

	if cancel {
		// Task has been cancelled, do nothing.
		return
	}

	defer func() {
		manager.mutex.Lock()
		task.timerState = TaskDone
		if _, has := manager.tasks[task.id]; has {
			delete(manager.tasks, task.id)
		} else {
			// FRAMEWORK LEVEL ERROR
			at.atomos.log.Fatal("AtomosTask: Task done but not found in tasks holder. task=(%+v)", task)
		}
		manager.mutex.Unlock()
	}()

	if task.helper != nil {
		if task.helper.task != nil {
			task.helper.task(am.mail.id)
		} else if task.helper.taskWithCallback != nil {
			callback := task.helper.taskWithCallback(am.mail.id)
			if callback != nil {
				at.atomos.PushTaskCallbackMail(am.name, callback, task.helper.recoverFn)
			}
		} else {
			at.atomos.log.Fatal("AtomosTask: Task closure is nil. helper=(%+v)", task.helper)
		}
	} else {
		at.atomos.log.Fatal("AtomosTask: Task closure is nil. baseAtomosMail=(%+v)", am)
	}
}

func (at *atomosTaskManager) cancelTaskQueue(task *atomosTask, mailboxExit bool, reason string) (err *Error) {
	id := task.id
	isCanceled := false
	manager := task.helper.manager
	mailbox := task.helper.mailbox
	err = func() *Error {
		if !mailboxExit {
			manager.mutex.Lock()
			defer manager.mutex.Unlock()
		}

		switch task.timerState {
		case TaskCancelled:
			{
				//at.log.pushFrameworkErrorLog("AtomosTask: CancelTaskQueue, USAGE ERROR, task already cancelled. task=(%+v)", task)
				return NewErrorf(ErrAtomosTaskCannotCancelCancelledTask, "AtomosTask: CancelTaskQueue, USAGE ERROR, task already cancelled. task=(%+v)", task).AddStack(nil)
			}
		case TaskScheduling:
			{
				// 任务还未加入到Atomos mailbox中，停止定时器。
				// Task has not been added to Atomos mailbox, stop the timer.
				if task.timer != nil {
					ok := task.timer.Stop()
					if !ok {
						// Might only happen on the edge of scheduled time has reached, even though the timer has executed the function,
						// but the change of taskState should acquire the mutex lock, so the delayAddToQueue function is still not executed.
						// When the lock here is released, the delayAddToQueue function will be executed immediately, and failed to add to the mailbox
						// because the task has been cancelled.
						at.atomos.log.Info("AtomosTask: Cancel task on the edge of scheduled time has reached, but it is ok. task=(%+v)", task)
					}
				} else if task.cronEID != 0 {
					at.getCron().Remove(task.cronEID)
				} else {
					// FRAMEWORK LEVEL ERROR
					m := mailbox.popByID(id)
					at.log.pushFrameworkErrorLog("AtomosTask: CancelTaskQueue, FRAMEWORK ERROR. err=(%v)", err)
					if m != nil {
						releaseMail(m)
					}
					return NewErrorf(ErrFrameworkInternalError, "AtomosTask: CancelTaskQueue, FRAMEWORK ERROR, invalid timer. task=(%+v)", task).AddStack(nil)
				}

				// 删除任务容器中的任务。
				// Delete the task from tasks holder.
				released := false
				if t, has := manager.tasks[id]; has {
					delete(manager.tasks, id)
					//deallocAtomosMail(task.baseAtomosMail)
					released = true
					releaseMail(t.atomosMail.mail)
				} else {
					// FRAMEWORK LEVEL ERROR
					m := mailbox.popByID(id)
					at.log.pushFrameworkErrorLog("AtomosTask: CancelTaskQueue, FRAMEWORK ERROR. err=(%v)", err)
					if m != nil {
						releaseMail(m)
					}
					return NewErrorf(ErrFrameworkInternalError, "AtomosTask: CancelTaskQueue, FRAMEWORK ERROR, task not exists. task=(%+v)", task).AddStack(nil)
				}

				// 正确性检查。
				// Correctness check.
				if chkMail := mailbox.getByID(id); chkMail != nil {
					// FRAMEWORK LEVEL ERROR
					m := mailbox.popByID(id)
					at.log.pushFrameworkErrorLog("AtomosTask: CancelTaskQueue, FRAMEWORK ERROR. releases=(%v), err=(%v)", released, err)
					if m != nil && !released {
						releaseMail(m)
					}
					return NewErrorf(ErrFrameworkInternalError, "AtomosTask: CancelTaskQueue, FRAMEWORK ERROR, task mail still in mailbox. task=(%+v)", task).AddStack(nil)
				}

				// 设置任务状态为已取消。
				// Set task state to be cancelled.
				task.timerState = TaskCancelled
				isCanceled = true
			}

		case TaskMailing:
			{
				// “箭发弦上”的任务：不是由定时器触发的任务，或者已经被定时器加入到Atomos mailbox中。
				// "On the way" task: either not timer-triggered task, or timer task already added to Atomos mailbox.

				// 从Atomos mailbox中删除任务邮件。
				// Delete the task mail from Atomos mailbox.
				m := mailbox.popByID(id)
				at.atomos.Log().Debug("AtomosTask: CancelTaskQueue, try to pop mail by id=%d, got mail=%+v", id, m)
				if m == nil {
					// 如果遇到这种情况，说明任务已经执行到了handleTask，但还未到上锁的临界点。抢先把值设置成取消就可以阻止执行。
					// If this happens, it means the task has been executed to handleTask, but not yet to the lock point. Setting the value to cancelled first can prevent execution.
					at.atomos.log.Debug("AtomosTask: Cancel task on the edge of executing, but it is ok. task=(%+v)", task)
				} else {
					releaseMail(m)
				}

				// 设置任务状态为已取消。
				// Set task state to be cancelled.
				task.timerState = TaskCancelled
				isCanceled = true
			}
		case TaskExecuting:
			{
				return NewErrorf(ErrAtomosTaskCannotCancelRunningTask, "AtomosTask: Cannot cancel a running task. task=(%+v)", task).AddStack(nil)
			}
		case TaskDone:
			{
				return NewErrorf(ErrAtomosTaskCannotCancelDoneTask, "AtomosTask: Cannot cancel a done task. task=(%+v)", task).AddStack(nil)
			}
		}
		return nil
	}()
	if err != nil {
		return err.AddStack(nil)
	}
	if isCanceled {
		// 注意：这个回调函数是在调用者的goroutine下运行的。但它是线程安全的，因为取消函数只应该在Atomos goroutine的逻辑中使用。
		// NOTICE: This callback is run under the goroutine of the caller. But it is thread-safe because the cancel function should only be used in logic of Atomos goroutine.
		if cb := task.helper.cancelCallback; cb != nil {
			cb(reason)
		}
	}
	return nil
}

func (at *atomosTaskManager) getCron() *cron.Cron {
	at.mutex.Lock()
	if at.cron == nil {
		at.cron = cron.New()
		at.cron.Start()
	}
	c := at.cron
	at.mutex.Unlock()
	return c
}

// 在genTaskID生成返回之后，锁就已经解开，因此addToAtomos中有一个时间窗口是没有锁保护的。但不用担心竞态条件，因为atomosTaskManager是给atomos线程独占使用的，
// 不会有并发调用导致异常的问题，除非使用者故意暴露竞态条件。
// After genTaskID returns, the lock is released, so there is a time window in addToAtomos that is not protected by the lock.
// However, there is no need to worry about race conditions, because atomosTaskManager is exclusively used by the atomos thread,
// and there will be no concurrent calls that cause abnormal problems, unless the user deliberately
func (at *atomosTaskManager) genTaskID(schedule bool) (task *atomosTask) {
	at.mutex.Lock()
	at.curID += 1
	curID := at.curID
	task = &atomosTask{
		id:         curID,
		atomosMail: nil,
		timer:      nil,
		cronEID:    0,
		timerState: 0,
	}
	if schedule {
		task.timerState = TaskScheduling
	} else {
		task.timerState = TaskMailing
	}
	at.tasks[curID] = task
	at.mutex.Unlock()
	return task
}

func (at *atomosTaskManager) getTaskSerialMailbox(queueName string) *taskMailbox {
	queueName = at.atomos.id.Info() + "::" + queueName
	at.mutex.Lock()
	defer at.mutex.Unlock()
	mb, has := at.queueMap[queueName]
	if !has {
		mb = createTaskMailbox(queueName, true, at.atomos.log.logging)
		at.queueMap[queueName] = mb
	}
	return mb
}

func (at *atomosTaskManager) newTaskSerialMailboxForConcurrent() *taskMailbox {
	mb := createTaskMailbox(at.atomos.id.Info()+concurrentMailboxSuffix, false, at.atomos.log.logging)
	return mb
}

// 对于临界情况的思考：因为上锁之后才会做检查和删除操作，所以不会出现拿到正在退出的队列的情况。
// Consideration for edge case: since the check and delete operations are done after locking, there won't be a case of getting a mailbox that is exiting.
func (at *atomosTaskManager) checkCloseTaskSerialMailbox(helper *taskHelper, tmb *taskMailbox) {
	manager := helper.manager
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	mb, has := manager.queueMap[tmb.mailbox.name]
	if !has {
		return
	}
	if killed := mb.mailbox.stopIfNoMail(nil); killed {
		delete(manager.queueMap, tmb.mailbox.name)
	}
}

func (at *atomosTaskManager) checkCloseTaskMailbox(tmb *taskMailbox) {
	if killed := tmb.mailbox.stopIfNoMail(nil); killed {
		// Nothing to do.
	}
}

// taskHelper helps to parse and validate task arguments.

type taskHelper struct {
	task             TaskFn
	taskWithCallback TaskFnWithCallback

	invalidArgs  []taskExtInvalidArg
	conflictArgs []taskExtInvalidArg

	mark           string
	delay          time.Duration
	cronSchedule   cron.Schedule
	appendToHead   bool
	cancelCallback func(reason string)
	recoverFn      func(r any)

	manager *atomosTaskManager
	mailbox *mailBox

	// init after validation
	atomosTask *atomosTask
}
type taskExtInvalidArg struct {
	idx    int
	task   ArgsForTask
	reason string
}

// TODO
func createTaskHelper(manager *atomosTaskManager, mailbox *mailBox, t TaskFn, tCb TaskFnWithCallback, ext []ArgsForTask) *taskHelper {
	helper := &taskHelper{
		mailbox: mailbox,
		manager: manager,
	}
	if t == nil && tCb == nil {
		helper.addInvalidArg(-1, nil, "Task is nil")
	} else if t != nil && tCb != nil {
		helper.addInvalidArg(-1, nil, "Both TaskFn and TaskFnWithCallback are provided")
	} else {
		helper.task = t
		helper.taskWithCallback = tCb
	}
	for i, v := range ext {
		if v == nil {
			helper.addInvalidArg(i, nil, "Argument is nil")
			continue
		}
		switch v.getArgType() {
		case ArgTypeTaskMark:
			if helper.mark != "" {
				helper.addConflictArg(i, v, "Duplicate TaskMark argument")
			} else {
				helper.mark = v.(argTaskMark).mark
			}
		case ArgTypeTaskDelay:
			if helper.delay != 0 {
				if helper.delay < 0 {
					helper.addInvalidArg(i, v, "Invalid delay argument")
				} else {
					helper.addConflictArg(i, v, "Duplicate TaskDelay argument")
				}
			} else if helper.cronSchedule != nil {
				helper.addConflictArg(i, v, "Duplicate TaskCron argument")
			} else {
				if v.(argTaskDelay).duration < 0 {
					helper.addInvalidArg(i, v, "Invalid delay argument")
				} else {
					helper.delay = v.(argTaskDelay).duration
				}
			}
		case ArgTypeTaskLikeCrontab:
			if helper.cronSchedule != nil {
				helper.addConflictArg(i, v, "Duplicate TaskCron argument")
			} else if helper.delay != 0 {
				helper.addConflictArg(i, v, "Duplicate TaskDelay argument")
			} else {
				schedule, er := cron.ParseStandard(v.(argTaskLikeCrontab).cron) // validate cron format
				if er != nil {
					helper.addInvalidArg(i, v, fmt.Sprintf("Invalid crontab format: %s", er.Error()))
				} else {
					helper.cronSchedule = schedule
				}
			}
		case ArgTypeTaskAppendToHead:
			if helper.appendToHead {
				helper.addConflictArg(i, v, "Duplicate TaskAppendToHead argument")
			} else {
				helper.appendToHead = true
			}
		case ArgTypeTaskCancelCallback:
			if helper.cancelCallback != nil {
				helper.addConflictArg(i, v, "Duplicate TaskCancelCallback argument")
			} else {
				if v.(argTaskCancelCallback).callback == nil {
					helper.addInvalidArg(i, v, "Nil TaskCancelCallback argument")
				} else {
					helper.cancelCallback = v.(argTaskCancelCallback).callback
				}
			}
		case ArgTypeTaskRecoverFunc:
			if helper.recoverFn != nil {
				helper.addConflictArg(i, v, "Duplicate TaskRecoverFunc argument")
			} else {
				if v.(argTaskRecoverFunc).recoverFunc == nil {
					helper.addInvalidArg(i, v, "Nil TaskRecoverFunc argument")
				} else {
					helper.recoverFn = v.(argTaskRecoverFunc).recoverFunc
				}
			}
		default:
			helper.addInvalidArg(i, v, "Unknown argument type")
		}
	}
	return helper
}

func (h *taskHelper) addInvalidArg(index int, arg ArgsForTask, reason string) {
	h.invalidArgs = append(h.invalidArgs, taskExtInvalidArg{
		idx:    index,
		task:   arg,
		reason: reason,
	})
}

func (h *taskHelper) addConflictArg(index int, arg ArgsForTask, reason string) {
	h.conflictArgs = append(h.conflictArgs, taskExtInvalidArg{
		idx:    index,
		task:   arg,
		reason: reason,
	})
}

func (h *taskHelper) hasErrors() bool {
	return len(h.invalidArgs) > 0 || len(h.conflictArgs) > 0
}

func (h *taskHelper) getErrorReason() string {
	var builder strings.Builder
	if len(h.invalidArgs) > 0 {
		builder.WriteString("Invalid Arguments:")
		for _, ia := range h.invalidArgs {
			builder.WriteString(fmt.Sprintf("\nAt index %d is invalid: %s.", ia.idx, ia.reason))
		}
		builder.WriteString("\n")
	}
	if len(h.conflictArgs) > 0 {
		builder.WriteString("Conflicting Arguments:")
		for _, ca := range h.conflictArgs {
			builder.WriteString(fmt.Sprintf("\nAt index %d is conflicting: %s.", ca.idx, ca.reason))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func (h *taskHelper) getCancelCallback() func(reason string) {
	return h.cancelCallback
}

func (h *taskHelper) errorHelper(at *atomosTaskManager, callbackReason string) {
	if cancelCallback := h.cancelCallback; cancelCallback != nil {
		cancelCallback(callbackReason)
	} else {
		at.log.pushFrameworkErrorLog("atomosTaskManager: Proceed failed because %s", h.getErrorReason())
	}
}

func (h *taskHelper) buildAtomosTask() {
	var task *atomosTask
	if h.delay > 0 || h.cronSchedule != nil {
		task = h.manager.genTaskID(true)
	} else {
		task = h.manager.genTaskID(false)
	}

	task.helper = h
	h.atomosTask = task
}
