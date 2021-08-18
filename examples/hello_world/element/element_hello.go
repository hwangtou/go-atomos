package element

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"google.golang.org/protobuf/proto"
	"time"
)

type GreeterElement struct {
}

func (h *GreeterElement) Check() error {
	return nil
}

func (h *GreeterElement) Info() (name string, version uint64, logLevel atomos.LogLevel, initNum int) {
	return "Greeter", 1, atomos.LogLevel_Debug, 100
}

func (h *GreeterElement) Loaded(mainId atomos.Id) {
}

func (h *GreeterElement) Unloaded() {
}

func (h *GreeterElement) AtomConstructor() atomos.Atom {
	return &helloAtom{}
}

func (h *GreeterElement) AtomDataLoader(name string) (proto.Message, error) {
	return nil, nil
}

func (h *GreeterElement) AtomDataSaver(name string, data proto.Message) error {
	return nil
}

func (h *GreeterElement) AtomCanKill(id atomos.Id) bool {
	return true
}

type helloAtom struct {
	self atomos.AtomSelf
	*api.HelloData
}

func (h *helloAtom) Spawn(self atomos.AtomSelf, arg *api.HelloSpawnArg, data *api.HelloData) error {
	self.Log().Info("Spawn")
	h.self = self
	if arg == nil {
		// If arg is nil, this is a reload spawn.
		h.self.Log().Info("Reload Spawn")
	}
	h.HelloData = data
	tid, err := h.self.Task().AddAfter(5 * time.Second, h.TimerKiller1, nil)
	//tid, err := h.self.Task().Add(h.TimerKiller, nil)
	h.self.Log().Info("Spawn Kill, tid=%d,err=%v", tid, err)
	return nil
}

func (h *helloAtom) Halt(from atomos.Id, cancels map[uint64]atomos.CancelledTask) (saveData proto.Message) {
	h.self.Log().Info("AtomHalt: from=%+v", from)
	for tid, cancel := range cancels {
		h.self.Log().Info("AtomHalt: cancelId=%d,cancel=%+v", tid, cancel)
	}
	return nil
}

func (h *helloAtom) SayHello(from atomos.Id, in *api.HelloRequest) (*api.HelloReply, error) {
	h.self.Log().Info("SayHello")

	// Add Immediately task.
	h.self.Log().Info("")
	h.self.Log().Info("Add Immediately task.")
	// Add Task with no argument.
	tid, err := h.self.Task().Add(h.TimerTest2, nil)
	h.self.Log().Info("Task.Add TimerTest, tid=%d,err=%v", tid, err)
	// Add Task with task id argument.
	tid, err = h.self.Task().Add(h.TimerTestTaskId3, nil)
	h.self.Log().Info("Task.Add TimerTestTaskId, tid=%d,err=%v", tid, err)
	// Add Task with task id and message argument.
	tid, err = h.self.Task().Add(h.TimerTestTaskIdAndArg4, &api.HelloTimer{Info: "Hi:4"})
	h.self.Log().Info("Task.Add TimerTestTaskIdAndArg, tid=%d,err=%v", tid, err)

	// Add Timer task.
	h.self.Log().Info("")
	h.self.Log().Info("Add Timer task.")
	// Add Timer Task with no argument.
	tid, err = h.self.Task().AddAfter(0 * time.Second, h.TimerTest5, nil)
	h.self.Log().Info("AddAfter TimerTest, tid=%d,err=%v", tid, err)
	// Add Timer Task with task id argument.
	tid, err = h.self.Task().AddAfter(1 * time.Second, h.TimerTestTaskId6, nil)
	h.self.Log().Info("AddAfter TimerTestTaskId, tid=%d,err=%v", tid, err)
	// Add Timer Task with task id and message argument.
	tid, err = h.self.Task().AddAfter(2 * time.Second, h.TimerTestTaskIdAndArg7, &api.HelloTimer{Info: "Hi:7"})
	h.self.Log().Info("AddAfter TimerTestTaskIdAndArg, tid=%d,err=%v", tid, err)

	// Add Immediately task then cancel.
	h.self.Log().Info("")
	h.self.Log().Info("Add Immediately task then cancel.")
	// Add Task with no argument.
	tid, err = h.self.Task().Add(h.TimerTest8, nil)
	h.self.Log().Info("Task.Add TimerTest1, tid=%d,err=%v", tid, err)
	cancel, err := h.self.Task().Cancel(tid)
	h.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)
	// Add Task with task id argument.
	tid, err = h.self.Task().Add(h.TimerTestTaskId9, nil)
	h.self.Log().Info("Task.Add TimerTestTaskId, tid=%d,err=%v", tid, err)
	cancel, err = h.self.Task().Cancel(tid)
	h.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)
	// Add Task with task id and message argument.
	tid, err = h.self.Task().Add(h.TimerTestTaskIdAndArg10, &api.HelloTimer{Info: "Hi:10"})
	h.self.Log().Info("Task.Add TimerTestTaskIdAndArg, tid=%d,err=%v", tid, err)
	cancel, err = h.self.Task().Cancel(tid)
	h.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)

	// Add Timer task then cancel.
	h.self.Log().Info("")
	h.self.Log().Info("Add Timer task then cancel.")
	// Add Timer Task with no argument.
	tid, err = h.self.Task().AddAfter(0 * time.Second, h.TimerTest11, nil)
	h.self.Log().Info("AddAfter TimerTest, tid=%d,err=%v", tid, err)
	cancel, err = h.self.Task().Cancel(tid)
	h.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)
	// Add Timer Task with task id argument.
	tid, err = h.self.Task().AddAfter(1 * time.Second, h.TimerTestTaskId12, nil)
	h.self.Log().Info("AddAfter TimerTestTaskId, tid=%d,err=%v", tid, err)
	cancel, err = h.self.Task().Cancel(tid)
	h.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)
	// Add Timer Task with task id and message argument.
	tid, err = h.self.Task().AddAfter(2 * time.Second, h.TimerTestTaskIdAndArg13, &api.HelloTimer{Info: "Hi:13"})
	h.self.Log().Info("AddAfter TimerTestTaskIdAndArg, tid=%d,err=%v", tid, err)
	cancel, err = h.self.Task().Cancel(tid)
	h.self.Log().Info("Cancel Task, cancel=%+v,err=%v", cancel, err)

	h.self.Log().Info("")

	// Add Timer task after kill.
	h.self.Log().Info("")
	h.self.Log().Info("Add Timer task.")
	// Add Timer Task with no argument.
	tid, err = h.self.Task().AddAfter(6 * time.Second, h.TimerTest14, nil)
	h.self.Log().Info("AddAfter TimerTest, tid=%d,err=%v", tid, err)
	// Add Timer Task with task id argument.
	tid, err = h.self.Task().AddAfter(6 * time.Second, h.TimerTestTaskId15, nil)
	h.self.Log().Info("AddAfter TimerTestTaskId, tid=%d,err=%v", tid, err)
	// Add Timer Task with task id and message argument.
	tid, err = h.self.Task().AddAfter(6 * time.Second, h.TimerTestTaskIdAndArg16, &api.HelloTimer{Info: "Hi:16"})
	h.self.Log().Info("AddAfter TimerTestTaskIdAndArg, tid=%d,err=%v", tid, err)

	return &api.HelloReply{ Message: "Ok" }, nil
}

func (h *helloAtom) TimerKiller1(taskId uint64) {
	h.self.Log().Info("TimerKiller start tid=%d", taskId)
	h.self.KillSelf()
	h.self.Log().Info("TimerKiller end tid=%d", taskId)
}

func (h *helloAtom) TimerTest2() {
	h.self.Log().Info("TimerTest 2 should execute")
}

func (h *helloAtom) TimerTestTaskId3(taskId uint64) {
	h.self.Log().Info("TimerTestTaskId 3 should execute: taskId=%d", taskId)
}

func (h *helloAtom) TimerTestTaskIdAndArg4(taskId uint64, arg *api.HelloTimer) {
	h.self.Log().Info("TimerTestTaskIdAndArg 4 should execute: taskId=%d,arg=%+v", taskId, arg)
}

func (h *helloAtom) TimerTest5() {
	h.self.Log().Info("TimerTest 5 should execute before TimerTest 1 because AddTimer adds to head of mailbox")
}

func (h *helloAtom) TimerTestTaskId6(taskId uint64) {
	h.self.Log().Info("TimerTestTaskId 6 should execute: taskId=%d", taskId)
}

func (h *helloAtom) TimerTestTaskIdAndArg7(taskId uint64, arg *api.HelloTimer) {
	h.self.Log().Info("TimerTestTaskIdAndArg 7 should execute: taskId=%d,arg=%+v", taskId, arg)
}

func (h *helloAtom) TimerTest8() {
	h.self.Log().Info("TimerTest 8 should not execute")
}

func (h *helloAtom) TimerTestTaskId9(taskId uint64) {
	h.self.Log().Info("TimerTestTaskId 9 should not execute: taskId=%d", taskId)
}

func (h *helloAtom) TimerTestTaskIdAndArg10(taskId uint64, arg *api.HelloTimer) {
	h.self.Log().Info("TimerTestTaskIdAndArg 10 should not execute: taskId=%d,arg=%+v", taskId, arg)
}

func (h *helloAtom) TimerTest11() {
	h.self.Log().Info("TimerTest 11 should not execute")
}

func (h *helloAtom) TimerTestTaskId12(taskId uint64) {
	h.self.Log().Info("TimerTestTaskId 12 should not execute: taskId=%d", taskId)
}

func (h *helloAtom) TimerTestTaskIdAndArg13(taskId uint64, arg *api.HelloTimer) {
	h.self.Log().Info("TimerTestTaskIdAndArg 13 should not execute: taskId=%d,arg=%+v", taskId, arg)
}

func (h *helloAtom) TimerTest14() {
	h.self.Log().Info("TimerTest 14 should execute")
}

func (h *helloAtom) TimerTestTaskId15(taskId uint64) {
	h.self.Log().Info("TimerTestTaskId 15 should execute: taskId=%d", taskId)
}

func (h *helloAtom) TimerTestTaskIdAndArg16(taskId uint64, arg *api.HelloTimer) {
	h.self.Log().Info("TimerTestTaskIdAndArg 16 should execute: taskId=%d,arg=%+v", taskId, arg)
}
