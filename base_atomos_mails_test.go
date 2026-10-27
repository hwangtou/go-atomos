package atomos

import (
	"testing"
	"time"
)

func TestBaseAtomosMailWaitInternalErrors(t *testing.T) {
	a := &BaseAtomos{}
	helper := &baseAtomosHelper{}

	t.Run("nil wait channel", func(t *testing.T) {
		m := &baseAtomosMail{}
		resp, err := m.waitReply(a, helper)
		if resp != nil || CodeOf(err) != ErrCodeFrameworkInternalError {
			t.Fatalf("resp = %v, error = %v, want nil and ErrCodeFrameworkInternalError", resp, err)
		}
	})

	t.Run("nil reply", func(t *testing.T) {
		waitCh := make(chan *mailReply, 1)
		waitCh <- nil
		m := &baseAtomosMail{waitCh: waitCh}
		resp, err := m.waitReply(a, helper)
		if resp != nil || CodeOf(err) != ErrCodeFrameworkInternalError {
			t.Fatalf("resp = %v, error = %v, want nil and ErrCodeFrameworkInternalError", resp, err)
		}
	})

	t.Run("nil ID wait channel", func(t *testing.T) {
		m := &baseAtomosMail{}
		id, err := m.waitReplyID(a, time.Second)
		if id != nil || CodeOf(err) != ErrCodeFrameworkInternalError {
			t.Fatalf("id = %v, error = %v, want nil and ErrCodeFrameworkInternalError", id, err)
		}
	})
}
