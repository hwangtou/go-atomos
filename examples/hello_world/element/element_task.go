package element

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"google.golang.org/protobuf/proto"
	"time"
)

// Firstly, create a struct to implement ElementDeveloper interface.

type TaskBoothElement struct {
	mainId atomos.MainId
}

func (t TaskBoothElement) Load(mainId atomos.MainId) error {
	t.mainId = mainId
	t.mainId.Log().Info("TaskBoothElement is loading")
	return nil
}

func (t TaskBoothElement) Unload() {
	t.mainId.Log().Info("TaskBoothElement is loading")
}

func (t TaskBoothElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return "TaskBooth", 1, atomos.LogLevel_Debug, 1
}

func (t TaskBoothElement) AtomConstructor() atomos.Atom {
	return &TaskBoothAtom{}
}

func (t TaskBoothElement) Persistence() atomos.ElementPersistence {
	return nil
}

func (t TaskBoothElement) AtomCanKill(id atomos.Id) bool {
	return true
}

// Secondly, create a struct to implement TaskBoothAtom.

type TaskBoothAtom struct {
	self atomos.AtomSelf
}

func (t *TaskBoothAtom) Spawn(self atomos.AtomSelf, arg *api.TaskBoothSpawnArg, data *api.TaskBoothData) error {
	self.Log().Info("Spawn")
	// Keep the AtomSelf pointer to use its service later.
	t.self = self
	tid, err := t.self.Task().AddAfter(5 * time.Second, t.TimerKiller1, nil)
	t.self.Log().Info("Spawn Kill, tid=%d,err=%v", tid, err)
	return nil
}

func (t *TaskBoothAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	t.self.Log().Info("AtomHalt: from=%+v", from)
	for tid, cancel := range cancels {
		t.self.Log().Info("AtomHalt: cancelId=%d,cancel=%+v", tid, cancel)
	}
	return nil
}

func (t *TaskBoothAtom) StartTask(from atomos.Id, in *api.StartTaskReq) (*api.StartTaskResp, error) {
	t.self.Log().Info("SayHello")

	// Append Immediately task.
	t.self.Log().Info("")
	t.self.Log().Info("Append Immediately task.")
	// Append Task with no argument.
	tid, err := t.self.Task().Append(t.TimerTest2, nil)
	t.self.Log().Info("Task.Append TimerTest, tid=%d,err=%v", tid, err)
	// Append Task with task id argument.
	tid, err = t.self.Task().Append(t.TimerTestTaskId3, nil)
	t.self.Log().Info("Task.Append TimerTestTaskId, tid=%d,err=%v", tid, err)
	// Append Task with task id and message argument.
	tid, err = t.self.Task().Append(t.TimerTestTaskIdAndArg4, &api.HelloTimer{Info: "Hi:4"})
	t.self.Log().Info("Task.Append TimerTestTaskIdAndArg, tid=%d,err=%v", tid, err)

	// Add Timer task.
	t.self.Log().Info("")
	t.self.Log().Info("Add Timer task.")
	// Add Timer Task with no argument.
	tid, err = t.self.Task().AddAfter(0 * time.Second, t.TimerTest5, nil)
	t.self.Log().Info("AddAfter TimerTest, tid=%d,err=%v", tid, err)
	// Add Timer Task with task id argument.
	tid, err = t.self.Task().AddAfter(1 * time.Second, t.TimerTestTaskId6, nil)
	t.self.Log().Info("AddAfter TimerTestTaskId, tid=%d,err=%v", tid, err)
	// Add Timer Task with task id and message argument.
	tid, err = t.self.Task().AddAfter(2 * time.Second, t.TimerTestTaskIdAndArg7, &api.HelloTimer{Info: "Hi:7"})
	t.self.Log().Info("AddAfter TimerTestTaskIdAndArg, tid=%d,err=%v", tid, err)

	// Append Immediately task then cancel.
	t.self.Log().Info("")
	t.self.Log().Info("Append Immediately task then cancel.")
	// Append Task with no argument.
	tid, err = t.self.Task().Append(t.TimerTest8, nil)
	t.self.Log().Info("Task.Append TimerTest1, tid=%d,err=%v", tid, err)
	cancel, err := t.self.Task().Cancel(tid)
	t.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)
	// Append Task with task id argument.
	tid, err = t.self.Task().Append(t.TimerTestTaskId9, nil)
	t.self.Log().Info("Task.Append TimerTestTaskId, tid=%d,err=%v", tid, err)
	cancel, err = t.self.Task().Cancel(tid)
	t.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)
	// Append Task with task id and message argument.
	tid, err = t.self.Task().Append(t.TimerTestTaskIdAndArg10, &api.HelloTimer{Info: "Hi:10"})
	t.self.Log().Info("Task.Append TimerTestTaskIdAndArg, tid=%d,err=%v", tid, err)
	cancel, err = t.self.Task().Cancel(tid)
	t.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)

	// Add Timer task then cancel.
	t.self.Log().Info("")
	t.self.Log().Info("Add Timer task then cancel.")
	// Add Timer Task with no argument.
	tid, err = t.self.Task().AddAfter(0 * time.Second, t.TimerTest11, nil)
	t.self.Log().Info("AddAfter TimerTest, tid=%d,err=%v", tid, err)
	cancel, err = t.self.Task().Cancel(tid)
	t.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)
	// Add Timer Task with task id argument.
	tid, err = t.self.Task().AddAfter(1 * time.Second, t.TimerTestTaskId12, nil)
	t.self.Log().Info("AddAfter TimerTestTaskId, tid=%d,err=%v", tid, err)
	cancel, err = t.self.Task().Cancel(tid)
	t.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)
	// Add Timer Task with task id and message argument.
	tid, err = t.self.Task().AddAfter(2 * time.Second, t.TimerTestTaskIdAndArg13, &api.HelloTimer{Info: "Hi:13"})
	t.self.Log().Info("AddAfter TimerTestTaskIdAndArg, tid=%d,err=%v", tid, err)
	cancel, err = t.self.Task().Cancel(tid)
	t.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)

	// Add Timer task after kill.
	t.self.Log().Info("")
	t.self.Log().Info("Add Timer task.")
	// Add Timer Task with no argument.
	tid, err = t.self.Task().AddAfter(6 * time.Second, t.TimerTest14, nil)
	t.self.Log().Info("AddAfter TimerTest, tid=%d,err=%v", tid, err)
	// Add Timer Task with task id argument.
	tid, err = t.self.Task().AddAfter(6 * time.Second, t.TimerTestTaskId15, nil)
	t.self.Log().Info("AddAfter TimerTestTaskId, tid=%d,err=%v", tid, err)
	// Add Timer Task with task id and message argument.
	tid, err = t.self.Task().AddAfter(6 * time.Second, t.TimerTestTaskIdAndArg16, &api.HelloTimer{Info: "Hi:16"})
	t.self.Log().Info("AddAfter TimerTestTaskIdAndArg, tid=%d,err=%v", tid, err)

	return &api.StartTaskResp{}, nil
}

func (t *TaskBoothAtom) TimerKiller1(taskId uint64) {
	t.self.Log().Info("TimerKiller start tid=%d", taskId)
	t.self.KillSelf()
	t.self.Log().Info("TimerKiller end tid=%d", taskId)
}

func (t *TaskBoothAtom) TimerTest2() {
	t.self.Log().Info("TimerTest 2 should execute")
}

func (t *TaskBoothAtom) TimerTestTaskId3(taskId uint64) {
	t.self.Log().Info("TimerTestTaskId 3 should execute: taskId=%d", taskId)
}

func (t *TaskBoothAtom) TimerTestTaskIdAndArg4(taskId uint64, arg *api.HelloTimer) {
	t.self.Log().Info("TimerTestTaskIdAndArg 4 should execute: taskId=%d,arg=%+v", taskId, arg)
}

func (t *TaskBoothAtom) TimerTest5() {
	t.self.Log().Info("TimerTest 5 should execute before TimerTest 1 because AddTimer adds to head of mailbox")
}

func (t *TaskBoothAtom) TimerTestTaskId6(taskId uint64) {
	t.self.Log().Info("TimerTestTaskId 6 should execute: taskId=%d", taskId)
}

func (t *TaskBoothAtom) TimerTestTaskIdAndArg7(taskId uint64, arg *api.HelloTimer) {
	t.self.Log().Info("TimerTestTaskIdAndArg 7 should execute: taskId=%d,arg=%+v", taskId, arg)
}

func (t *TaskBoothAtom) TimerTest8() {
	t.self.Log().Info("TimerTest 8 should not execute")
}

func (t *TaskBoothAtom) TimerTestTaskId9(taskId uint64) {
	t.self.Log().Info("TimerTestTaskId 9 should not execute: taskId=%d", taskId)
}

func (t *TaskBoothAtom) TimerTestTaskIdAndArg10(taskId uint64, arg *api.HelloTimer) {
	t.self.Log().Info("TimerTestTaskIdAndArg 10 should not execute: taskId=%d,arg=%+v", taskId, arg)
}

func (t *TaskBoothAtom) TimerTest11() {
	t.self.Log().Info("TimerTest 11 should not execute")
}

func (t *TaskBoothAtom) TimerTestTaskId12(taskId uint64) {
	t.self.Log().Info("TimerTestTaskId 12 should not execute: taskId=%d", taskId)
}

func (t *TaskBoothAtom) TimerTestTaskIdAndArg13(taskId uint64, arg *api.HelloTimer) {
	t.self.Log().Info("TimerTestTaskIdAndArg 13 should not execute: taskId=%d,arg=%+v", taskId, arg)
}

func (t *TaskBoothAtom) TimerTest14() {
	t.self.Log().Info("TimerTest 14 should not execute")
}

func (t *TaskBoothAtom) TimerTestTaskId15(taskId uint64) {
	t.self.Log().Info("TimerTestTaskId 15 should not execute: taskId=%d", taskId)
}

func (t *TaskBoothAtom) TimerTestTaskIdAndArg16(taskId uint64, arg *api.HelloTimer) {
	t.self.Log().Info("TimerTestTaskIdAndArg 16 should not execute: taskId=%d,arg=%+v", taskId, arg)
}
