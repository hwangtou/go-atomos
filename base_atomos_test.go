package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

type TestAtomosHolder struct {
	T *testing.T
	i *TestAtomosInstance
}

func (t *TestAtomosHolder) OnMessaging(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	t.T.Logf("OnMessage: from=(%v),state=(%v),name=(%s),args=(%v),instance=(%v)", from, a.state, name, args, t.i.reload)
	switch name {
	case "panic":
		_ = (*TestAtomosHolder)(nil).i
	}
	return
}

func (t *TestAtomosHolder) OnReloading(oldInstance, newInstance Atomos, reloads int) {
	o, n := oldInstance.(*TestAtomosInstance), newInstance.(*TestAtomosInstance)
	t.i = n
	t.T.Logf("OnReloading: state=(%v),oldInstanceReload=(%d),newInstanceReload=(%d),reloads=(%v)",
		a.state, o.reload, n.reload, reloads)
}

func (t *TestAtomosHolder) OnStopping(from ID, cancelled map[uint64]CancelledTask) *ErrorInfo {
	t.T.Logf("OnStopping: from=(%v),state=(%v),cancelled=(%v)", from, a.state, cancelled)
	return nil
}

type TestAtomosInstance struct {
	T      *testing.T
	reload int
}

func (t *TestAtomosInstance) Description() string {
	return "Description"
}

func (t *TestAtomosInstance) Halt(from ID, cancelled map[uint64]CancelledTask) (save bool, data proto.Message) {
	t.T.Logf("Halt: from=(%v),cancelled=(%v)", from, cancelled)
	return true, nil
}

func (t *TestAtomosInstance) Reload(oldInstance Atomos) {
	t.T.Logf("Reload: state=(%v),reload=(%v)", a.state, oldInstance)
}

func (t *TestAtomosInstance) TestTask(taskId uint64, data proto.Message) {
	t.T.Logf("TestTask: state=(%v),taskId=(%d),data=(%v),reload=(%v)", a.state, taskId, data, t.reload)
}

var a *BaseAtomos

func TestBaseAtomos(t *testing.T) {
	id := &IDInfo{
		Type:    IDType_Atomos,
		Cosmos:  "cosmos",
		Element: "element",
		Atomos:  "atomos",
	}
	log := NewLoggingAtomos()
	instance := &TestAtomosInstance{T: t, reload: 1}
	holder := &TestAtomosHolder{T: t, i: instance}
	atom := NewBaseAtomos(id, log, LogLevel_Debug, holder, instance)
	a = atom
	// Push Message
	reply, err := a.PushMessageMailAndWaitReply(nil, "message", nil)
	t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%v),err=(%v)", reply, a.GetState(), err)
	// Push Task
	taskId, err := a.Task().AddAfter(0, instance.TestTask, nil)
	t.Logf("TaskAddAfter: taskId=(%v),state=(%v),err=(%v)", taskId, a.GetState(), err)
	// Push Task
	taskId, err = a.Task().AddAfter(1*time.Second, instance.TestTask, nil)
	t.Logf("TaskAddAfter: taskId=(%v),state=(%v),err=(%v)", taskId, a.GetState(), err)
	// Push Reload
	err = a.PushReloadMailAndWaitReply(nil, &TestAtomosInstance{T: t, reload: 2}, 2)
	t.Logf("PushReloadMailAndWaitReply: reply=(%v),state=(%v),err=(%v)", reply, a.GetState(), err)
	time.Sleep(1 * time.Second)
	// Push Message
	reply, err = a.PushMessageMailAndWaitReply(nil, "message", nil)
	t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%v),err=(%v)", reply, a.GetState(), err)
	//// Push Message Panic
	//reply, err = a.PushMessageMailAndWaitReply(nil, "panic", nil)
	//t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%v),err=(%v)", reply, a.GetState(), err)
	// Push Kill
	err = a.PushKillMailAndWaitReply(nil, true)
	t.Logf("PushKillMailAndWaitReply: state=(%v),err=(%v)", a.GetState(), err)
	// Push Message
	reply, err = a.PushMessageMailAndWaitReply(nil, "send_after_halt", nil)
	t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%v),err=(%v)", reply, a.GetState(), err)
	time.Sleep(10 * time.Second)
}
