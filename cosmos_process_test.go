package go_atomos

import (
	"testing"
)

type testMainScript struct {
	t *testing.T
}

func (s *testMainScript) OnStartup() *Error {
	s.t.Log("testMainScript: StartUp Begin")
	//panic("startup panic")
	s.t.Log("testMainScript: StartUp End")
	return nil
}

func (s *testMainScript) OnShutdown() *Error {
	s.t.Log("testMainScript: Shutdown Begin")
	//panic("shutdown panic")
	s.t.Log("testMainScript: Shutdown End")
	return nil
}
