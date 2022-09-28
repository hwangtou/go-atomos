package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

type TestAtomosHolder struct {
	T *testing.T
}

func (t *TestAtomosHolder) OnMessaging(from ID, name string, args proto.Message) (reply proto.Message, err *ErrorInfo) {
	t.T.Logf("OnMessage: from=(%v),state=(%v),name=(%s),args=(%v)", from, a.state, name, args)
	switch name {
	case "panic":
		_ = (*TestAtomosHolder)(nil).T
	}
	return
}

func (t *TestAtomosHolder) OnScaling(from ID, name string, args proto.Message) (id ID, err *ErrorInfo) {
	panic("not supported")
}

func (t *TestAtomosHolder) OnReloading(oldInstance Atomos, reloadObject AtomosReloadable) (newInstance Atomos) {
	o := oldInstance.(*TestAtomosInstance)
	n := reloadObject.(*TestAtomosInstance)
	t.T.Logf("OnReloading: state=(%v),oldInstanceReload=(%d),newInstanceReload=(%d),reloads=(%v)",
		a.state, o.reload, n.reload, a.reloads)
	return n
}

func (t *TestAtomosHolder) OnWormhole(from ID, wormhole AtomosWormhole) *ErrorInfo {
	t.T.Logf("OnWormhole: wormhole=(%v)", wormhole)
	return nil
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

func (t *TestAtomosInstance) TestTask(taskID uint64, data proto.Message) {
	t.T.Logf("TestTask: state=(%v),taskID=(%d),data=(%v),reload=(%v)", a.state, taskID, data, t.reload)
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
	holder := &TestAtomosHolder{T: t}
	atom := NewBaseAtomos(id, log, LogLevel_Debug, holder, instance, 1)
	a = atom
	// Push Message
	reply, err := a.PushMessageMailAndWaitReply(nil, "message", nil)
	t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%v),err=(%v)", reply, a.GetState(), err)
	// Push Task
	taskID, err := a.Task().AddAfter(0, instance.TestTask, nil)
	t.Logf("TaskAddAfter: taskID=(%v),state=(%v),err=(%v)", taskID, a.GetState(), err)
	// Push Task
	taskID, err = a.Task().AddAfter(1*time.Second, instance.TestTask, nil)
	t.Logf("TaskAddAfter: taskID=(%v),state=(%v),err=(%v)", taskID, a.GetState(), err)
	// Push Reload
	err = a.PushReloadMailAndWaitReply(nil, &TestAtomosInstance{T: t, reload: 2}, 2)
	t.Logf("PushReloadMailAndWaitReply: reply=(%v),state=(%v),err=(%v)", reply, a.GetState(), err)
	time.Sleep(100 * time.Millisecond)
	// Push Wormhole
	err = a.PushWormholeMailAndWaitReply(nil, "wormhole_message")
	t.Logf("PushWormholeMailAndWaitReply: err=(%v)", err)
	// Push Message
	reply, err = a.PushMessageMailAndWaitReply(nil, "message", nil)
	t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%v),err=(%v)", reply, a.GetState(), err)
	// Push Message Panic
	reply, err = a.PushMessageMailAndWaitReply(nil, "panic", nil)
	t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%v),err=(%v)", reply, a.GetState(), err)
	// Push Kill
	err = a.PushKillMailAndWaitReply(nil, true)
	t.Logf("PushKillMailAndWaitReply: state=(%v),err=(%v)", a.GetState(), err)
	// Push Message
	reply, err = a.PushMessageMailAndWaitReply(nil, "send_after_halt", nil)
	t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%v),err=(%v)", reply, a.GetState(), err)
	time.Sleep(100 * time.Millisecond)
}

// TODO
func TestBaseAtomosReferenceCount(t *testing.T) {
}

// TODO
func TestBaseAtomosTask(t *testing.T) {
}
