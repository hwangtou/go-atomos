package go_atomos

import (
	"testing"
	"time"
)

func TestAppLogging(t *testing.T) {
	l := newTestAppLogging(t)
	defer l.Close()

	digit25 := "abcdefghijklmnopqrstuvwxy"
	for b := 0; b < 10; b += 1 {
		time.Sleep(1 * time.Second)
		curAccessFilename := l.curAccessLog.Name()
		curErrorFilename := l.curErrorLog.Name()
		t.Logf("Access Log Filename: %s", curAccessFilename)
		//t.Logf("Error Log Filename: %s", l.curErrorLog.Name())
		for bc := 0; bc <= testLogMaxSize; bc += len(digit25) {
			l.WriteAccessLog(digit25 + "\n")
		}
		if curAccessFilename == l.curAccessLog.Name() {
			t.Errorf("same log file name, should change")
			return
		}
		if curErrorFilename != l.curErrorLog.Name() {
			t.Errorf("error log file should not change")
			return
		}
	}

	for b := 0; b < 10; b += 1 {
		time.Sleep(1 * time.Second)
		curAccessFilename := l.curAccessLog.Name()
		curErrorFilename := l.curErrorLog.Name()
		t.Logf("Access Log Filename: %s", curAccessFilename)
		t.Logf("Error Log Filename: %s", curErrorFilename)
		//t.Logf("Error Log Filename: %s", l.curErrorLog.Name())
		for bc := 0; bc <= testLogMaxSize; bc += len(digit25) {
			l.WriteErrorLog(digit25 + "\n")
		}
		if curAccessFilename == l.curAccessLog.Name() {
			t.Errorf("same log file name, should change")
			return
		}
		if curErrorFilename == l.curErrorLog.Name() {
			t.Errorf("same log file name, should change")
			return
		}
	}
}
