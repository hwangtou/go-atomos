package atomos

import (
	"strings"
	"testing"
)

type testLogging struct {
	t *testing.T
}

func newTestLogging(t *testing.T) *testLogging {
	return &testLogging{t: t}
}

func newTestLoggingAtomos(t *testing.T) *loggingAtomos {
	la := &loggingAtomos{}
	if err := la.init(&testLogging{t: t}); err != nil {
		t.Fatalf("newTestLogging: Init logging failed. err=(%v)", err.AddStack(nil))
	}
	return la
}

func (t testLogging) WriteAccessLog(msg string) {
	if strings.HasSuffix(msg, "\n") {
		msg = msg[:len(msg)-1]
	}
	t.t.Log(msg)
}

func (t testLogging) WriteErrorLog(msg string) {
	if strings.HasSuffix(msg, "\n") {
		msg = msg[:len(msg)-1]
	}
	t.t.Error(msg)
}

type benchLogging struct {
	b *testing.B
}

func newBenchLogging(b *testing.B) *benchLogging {
	return &benchLogging{b: b}
}

func newBenchLoggingAtomos(b *testing.B) *loggingAtomos {
	la := &loggingAtomos{}
	if err := la.init(&benchLogging{b: b}); err != nil {
		b.Fatalf("newBenchLogging: Init logging failed. err=(%v)", err.AddStack(nil))
	}
	return la
}

func (b benchLogging) WriteAccessLog(msg string) {
	if strings.HasSuffix(msg, "\n") {
		msg = msg[:len(msg)-1]
	}
	b.b.Log(msg)
}

func (b benchLogging) WriteErrorLog(msg string) {
	if strings.HasSuffix(msg, "\n") {
		msg = msg[:len(msg)-1]
	}
	b.b.Error(msg)
}
