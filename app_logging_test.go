package go_atomos

import (
	"fmt"
	"log"
	"os"
	"path"
	"testing"
	"time"
)

func TestAppLoggingSplitFiles(t *testing.T) {
	dir := path.Join(os.TempDir(), "test_atomos_app_logging")
	l, err := NewAppLoggingToFile(dir, testLogMaxSize, AppLoggingAutoCleanupOff, func(e *Error) {
		t.Fatalf("Error: %v", e)
	})
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	randStrGen := NewUtilStringRandomStringGenerator()
	fileToContent := map[string]string{}
	for b := 0; b < 10; b += 1 {
		<-time.After(1 * time.Second)
		curAccessFilename := l.curAccessLog.Name()
		curErrorFilename := l.curErrorLog.Name()
		t.Logf("Access Log Filename: %s", curAccessFilename)
		for bc := 0; bc < testLogMaxSize; bc += 25 {
			digit25 := randStrGen.RandomString(24) + "\n"
			l.WriteAccessLog(digit25)
			fileToContent[curAccessFilename] += digit25
			fileToContent[curErrorFilename] = ""
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
	// Check the content of the log file.
	for filename, content := range fileToContent {
		f, er := os.OpenFile(filename, os.O_RDONLY, 0)
		if er != nil {
			t.Fatalf("Error: %v", er)
		}
		buf := make([]byte, len(content))
		n, er := f.Read(buf)
		_ = f.Close()
		if er != nil {
			t.Fatalf("Error: %v", er)
		}
		if n != len(content) {
			t.Fatalf("Error: read size not match, %d", len(content))
		}
		if string(buf) != content {
			t.Fatalf("Error: content not match")
		}
	}

	fileToContent = map[string]string{}
	for b := 0; b < 10; b += 1 {
		<-time.After(1 * time.Second)
		curAccessFilename := l.curAccessLog.Name()
		curErrorFilename := l.curErrorLog.Name()
		t.Logf("Access Log Filename: %s", curAccessFilename)
		t.Logf("Error Log Filename: %s", curErrorFilename)
		for bc := 0; bc < testLogMaxSize; bc += 25 {
			digit25 := randStrGen.RandomString(24) + "\n"
			l.WriteErrorLog(digit25)
			fileToContent[curAccessFilename] += digit25
			fileToContent[curErrorFilename] += digit25
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
	// Check the content of the log file.
	for filename, content := range fileToContent {
		f, er := os.OpenFile(filename, os.O_RDONLY, 0)
		if er != nil {
			t.Fatalf("Error: %v", er)
		}
		buf := make([]byte, len(content))
		n, er := f.Read(buf)
		_ = f.Close()
		if er != nil {
			t.Fatalf("Error: %v", er)
		}
		if n != len(content) {
			t.Fatalf("Error: read size not match, %d", len(content))
		}
		if string(buf) != content {
			t.Fatalf("Error: content not match")
		}
	}
}

func TestAppLoggingStdout(t *testing.T) {
	dir := path.Join(os.TempDir(), "test_atomos_app_logging")
	l, err := NewAppLoggingToFile(dir, testLogMaxSize, AppLoggingAutoCleanupOff, func(e *Error) {
		t.Fatalf("Error: %v", e)
	})
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	randStrGen := NewUtilStringRandomStringGenerator()
	fileToContent := map[string]string{}
	curAccessFilename := l.curAccessLog.Name()
	curErrorFilename := l.curErrorLog.Name()
	t.Logf("Access Log Filename: %s", curAccessFilename)
	for bc := 0; bc < 100; bc += 1 {
		digit25 := randStrGen.RandomString(24) + "\n"

		fmt.Print(digit25)
		fileToContent[curAccessFilename] += digit25
		log.Print(digit25)
		fileToContent[curAccessFilename] += time.Now().Format("2006/01/02 15:04:05") + " " + digit25
	}
	fileToContent[curErrorFilename] = ""

	// Check the content of the log file.
	for filename, content := range fileToContent {
		f, er := os.OpenFile(filename, os.O_RDONLY, 0)
		if er != nil {
			t.Fatalf("Error: %v", er)
		}
		buf := make([]byte, len(content))
		n, er := f.Read(buf)
		_ = f.Close()
		if er != nil {
			t.Fatalf("Error: %v", er)
		}
		if n != len(content) {
			t.Fatalf("Error: read size not match, %d", len(content))
		}
		if string(buf) != content {
			t.Fatalf("Error: content not match")
		}
	}
}

func TestAppLoggingCheckLogPathSize(t *testing.T) {
	dir := path.Join(os.TempDir(), "test_atomos_app_logging")
	l, err := NewAppLoggingToFile(dir, testLogMaxSize, 5, func(e *Error) {
		t.Fatalf("Error: %v", e)
	})
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	randStrGen := NewUtilStringRandomStringGenerator()
	fileToContent := map[string]string{}
	fileToDelete := map[string]bool{}
	for b := 0; b < 10; b += 1 {
		<-time.After(1 * time.Second)
		curAccessFilename := l.curAccessLog.Name()
		curErrorFilename := l.curErrorLog.Name()
		if b <= 5 {
			fileToDelete[curAccessFilename] = true
			fileToDelete[curErrorFilename] = true
		}
		t.Logf("Access Log Filename: %s", curAccessFilename)
		for bc := 0; bc < testLogMaxSize; bc += 25 {
			digit25 := randStrGen.RandomString(24) + "\n"
			l.WriteAccessLog(digit25)
			fileToContent[curAccessFilename] += digit25
			fileToContent[curErrorFilename] = ""
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
	// Check the content of the log file.
	for filename, content := range fileToContent {
		f, er := os.OpenFile(filename, os.O_RDONLY, 0)
		if er != nil {
			if !fileToDelete[filename] {
				t.Fatalf("Error: %v", er)
			}
			continue
		}
		buf := make([]byte, len(content))
		n, er := f.Read(buf)
		_ = f.Close()
		if er != nil {
			t.Fatalf("Error: %v", er)
		}
		if n != len(content) {
			t.Fatalf("Error: read size not match, %d", len(content))
		}
		if string(buf) != content {
			t.Fatalf("Error: content not match")
		}
	}
}
