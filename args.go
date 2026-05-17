package atomos

import "time"

// Atom

// Base Atomos

// ArgBaseAtomosTimeout creates an argument that specifies a timeout duration for BaseAtomos operations.
// 创建一个参数，指定BaseAtomos操作的超时时间。
// Example usage:
// toID.SyncMessagingByName(callerID, "message_name", inMessage, []atomos.ArgsForBaseAtomos{atomos.ArgBaseAtomosTimeout(5 * time.Second)})
func ArgBaseAtomosTimeout(timeout time.Duration) ArgsForBaseAtomos {
	return argBaseAtomosTimeout{timeout: timeout}
}

// ArgBaseAtomosAppendToHead creates an argument that indicates the message should be appended to the head of the mailbox queue.
// 创建一个参数，表示消息应添加到邮箱队列的头部。
// Example usage:
// toID.AsyncMessagingByName(callerID, "message_name", inMessage, callback, []atomos.ArgsForBaseAtomos{atomos.ArgBaseAtomosAppendToHead()})
func ArgBaseAtomosAppendToHead() ArgsForBaseAtomos {
	return argBaseAtomosAppendToHead{}
}

// ArgBaseAtomosWaitKilled creates an argument that indicates the operation should wait until the BaseAtomos is killed.
// 创建一个参数，表示操作应等待直到BaseAtomos被终止。
// Example usage:
// toID.Kill(callerID, []atomos.ArgsForBaseAtomos{atomos.ArgBaseAtomosWaitKilled()})
func ArgBaseAtomosWaitKilled() ArgsForBaseAtomos {
	return argBaseAtomosWaitKilled{}
}

// NewArgBaseAtomosWithAny creates an argument that holds any arbitrary data to be passed along with the BaseAtomos operation.
// 创建一个参数，包含任何任意数据，以便与BaseAtomos操作一起传递。
// Example usage:
// toID.SyncMessagingByName(callerID, "message_name", inMessage, []atomos.ArgsForBaseAtomos{atomos.NewArgBaseAtomosWithAny(myData)})
func NewArgBaseAtomosWithAny(any any) ArgsForBaseAtomos {
	return ArgBaseAtomosWithAny{Any: any}
}

// Task

// ArgTaskMark creates an argument that specifies a marking string for the task.
// 创建一个参数，指定任务的标记字符串。
// Example usage:
// self.Task().AddToAtomosQueue(func(taskID uint64) {}, atomos.ArgTaskMark("my_task_mark"))
func ArgTaskMark(mark string) ArgsForTask { return argTaskMark{mark: mark} }

// ArgTaskDelay creates an argument that specifies a delay duration before executing the task.
// 创建一个参数，指定在执行任务之前的延迟时间，即任务将在指定的延迟时间后执行。
// Example usage:
// self.Task().AddToAtomosQueue(func(taskID uint64) {}, atomos.ArgTaskDelay(2 * time.Second))
func ArgTaskDelay(duration time.Duration) ArgsForTask { return argTaskDelay{duration: duration} }

// ArgTaskLikeCrontab creates an argument that specifies a crontab-like schedule for the task.
// 创建一个参数，指定任务的类似crontab的调度计划。
// Example usage:
// self.Task().AddToAtomosQueue(func(taskID uint64) {}, atomos.ArgTaskLikeCrontab("0 0 * * *"))
func ArgTaskLikeCrontab(cron string) ArgsForTask { return argTaskLikeCrontab{cron: cron} }

// ArgTaskAppendToHead creates an argument that indicates the task should be appended to the head of the task queue.
// 创建一个参数，表示任务应添加到任务队列的头部。
// Example usage:
// self.Task().AddToAtomosQueue(func(taskID uint64) {}, atomos.ArgTaskAppendToHead())
func ArgTaskAppendToHead() ArgsForTask { return argTaskAppendToHead{} }

// ArgTaskCancelCallback creates an argument that holds a callback function to be called when a task is canceled.
// 创建一个参数，包含一个回调函数，当任务被取消时调用该函数。
// Example usage:
// self.Task().AddToAtomosQueue(func(taskID uint64) {}, atomos.ArgTaskCancelCallback(func() {}))
func ArgTaskCancelCallback(cb func(reason string)) ArgsForTask {
	return argTaskCancelCallback{callback: cb}
}

// ArgTaskRecoverFunc creates an argument that holds a recovery function to handle panics during task execution.
// 创建一个参数，包含一个恢复函数，用于处理任务执行过程中发生的panic。
// Example usage:
// self.Task().AddToAtomosQueue(func(taskID uint64) {}, atomos.ArgTaskRecoverFunc(func(r any) {}))
func ArgTaskRecoverFunc(recoverFunc func(r any)) ArgsForTask {
	return argTaskRecoverFunc{recoverFunc: recoverFunc}
}
