package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

type TestAtomosHolder struct {
	T *testing.T
}

func (t *TestAtomosHolder) OnMessaging(from ID, name string, args proto.Message) (reply proto.Message, err *Error) {
	t.T.Logf("OnMessage: from=(%v),state=(%v),name=(%s),args=(%v)", from, a.state, name, args)
	func() {
		defer func() {
			if r := recover(); r != nil {
				if err == nil {
					err = NewErrorf(ErrFrameworkPanic, "Atom: Messaging recovers from panic.").AddPanicStack(nil, 1, r)
				}
			}
		}()
		switch name {
		case "panic":
			_ = (*TestAtomosHolder)(nil).T
		}
	}()
	return
}

func (t *TestAtomosHolder) OnScaling(from ID, name string, args proto.Message) (id ID, err *Error) {
	panic("not supported")
}

func (t *TestAtomosHolder) OnWormhole(from ID, wormhole AtomosWormhole) *Error {
	t.T.Logf("OnWormhole: wormhole=(%v)", wormhole)
	return nil
}

func (t *TestAtomosHolder) OnStopping(from ID, cancelled map[uint64]CancelledTask) *Error {
	t.T.Logf("OnStopping: from=(%v),state=(%v),cancelled=(%v)", from, a.state, cancelled)
	return nil
}

func (t *TestAtomosHolder) Spawn() {
	t.T.Logf("Spawn")
}

func (t *TestAtomosHolder) Set(message string) {
	t.T.Logf("Set %s", message)
}

func (t *TestAtomosHolder) Unset(message string) {
	t.T.Logf("Unset %s", message)
}

func (t *TestAtomosHolder) Stopping() {
	t.T.Logf("Stopping")
}

func (t *TestAtomosHolder) Halted() {
	t.T.Logf("Halted")
}

type TestAtomosInstance struct {
	T      *testing.T
	reload int
}

func (t *TestAtomosInstance) String() string {
	return "Description"
}

func (t *TestAtomosInstance) Halt(from ID, cancelled map[uint64]CancelledTask) (save bool, data proto.Message) {
	t.T.Logf("Stopping: from=(%v),cancelled=(%v)", from, cancelled)
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
	initTestFakeCosmosProcess(t)
	time.Sleep(10 * time.Millisecond)
	instance := &TestAtomosInstance{T: t, reload: 1}
	holder := &TestAtomosHolder{T: t}
	atom := NewBaseAtomos(id, LogLevel_Debug, holder, instance)
	a = atom
	// Push Message
	reply, err := a.PushMessageMailAndWaitReply(nil, "message", 0, nil)
	t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%d),err=(%v)", reply, a.GetState(), err)
	// Push Task
	taskID, err := a.Task().AddAfter(0, instance.TestTask, nil)
	t.Logf("TaskAddAfter: taskID=(%v),state=(%d),err=(%v)", taskID, a.GetState(), err)
	// Push Task
	taskID, err = a.Task().AddAfter(1*time.Second, instance.TestTask, nil)
	t.Logf("TaskAddAfter: taskID=(%v),state=(%d),err=(%v)", taskID, a.GetState(), err)
	// Push Wormhole
	err = a.PushWormholeMailAndWaitReply(nil, 0, "wormhole_message")
	t.Logf("PushWormholeMailAndWaitReply: err=(%v)", err)
	// Push Message
	reply, err = a.PushMessageMailAndWaitReply(nil, "message", 0, nil)
	t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%d),err=(%v)", reply, a.GetState(), err)
	// Push Message Panic
	reply, err = a.PushMessageMailAndWaitReply(nil, "panic", 0, nil)
	t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%d),err=(%v)", reply, a.GetState(), err)
	// Push Kill
	err = a.PushKillMailAndWaitReply(nil, true, 0)
	t.Logf("PushKillMailAndWaitReply: state=(%d),err=(%v)", a.GetState(), err)
	// Push Message
	reply, err = a.PushMessageMailAndWaitReply(nil, "send_after_halt", 0, nil)
	t.Logf("PushMessageMailAndWaitReply: reply=(%v),state=(%d),err=(%v)", reply, a.GetState(), err)
	time.Sleep(10 * time.Millisecond)
}

// TODO
func TestBaseAtomosReferenceCount(t *testing.T) {
}

// TODO
func TestBaseAtomosTask(t *testing.T) {
}
