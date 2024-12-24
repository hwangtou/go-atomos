package go_atomos

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLoggingService(t *testing.T) {
	// Create a new logging service.
	ls := loggingService{}
	tt := &appLoggingForTest{t: t, ignoreError: true}
	if err := ls.init(tt); err != nil {
		t.Errorf("loggingService.init() = %v, want nil", err)
		return
	}

	ls.pushFrameworkInfoLog("Test: Framework info")
	ls.pushFrameworkErrorLog("Test: Framework error")
	ls.PushLogging(nil, LogLevel_Debug, "Test: No ID")
	ls.PushLogging(&IDInfo{Type: IDType_Cosmos, Cosmos: "test_cosmos", Node: "test_node"}, LogLevel_Debug, "Test: Cosmos")
	ls.PushLogging(&IDInfo{Type: IDType_Element, Cosmos: "test_cosmos", Node: "test_node", Element: "test_element"}, LogLevel_Debug, "Test: Element")
	ls.PushLogging(&IDInfo{Type: IDType_Atom, Cosmos: "test_cosmos", Node: "test_node", Element: "test_element", Atom: "test_atom"}, LogLevel_Debug, "Test: Atom")
	ls.PushLogging(&IDInfo{Type: IDType_Atom, Cosmos: "test_cosmos", Node: "test_node", Element: "test_element", Atom: "test_atom"}, LogLevel_Info, "Test: Atom")
	ls.PushLogging(&IDInfo{Type: IDType_Atom, Cosmos: "test_cosmos", Node: "test_node", Element: "test_element", Atom: "test_atom"}, LogLevel_Warn, "Test: Atom")
	ls.PushLogging(&IDInfo{Type: IDType_Atom, Cosmos: "test_cosmos", Node: "test_node", Element: "test_element", Atom: "test_atom"}, LogLevel_CoreInfo, "Test: Atom")
	ls.PushLogging(&IDInfo{Type: IDType_Atom, Cosmos: "test_cosmos", Node: "test_node", Element: "test_element", Atom: "test_atom"}, LogLevel_Err, "Test: Atom")
	ls.PushLogging(&IDInfo{Type: IDType_Atom, Cosmos: "test_cosmos", Node: "test_node", Element: "test_element", Atom: "test_atom"}, LogLevel_CoreErr, "Test: Atom")
	ls.PushLogging(&IDInfo{Type: IDType_Atom, Cosmos: "test_cosmos", Node: "test_node", Element: "test_element", Atom: "test_atom"}, LogLevel_Fatal, "Test: Atom")
	ls.PushLogging(&IDInfo{Type: IDType_Atom, Cosmos: "test_cosmos", Node: "test_node", Element: "test_element", Atom: "test_atom"}, LogLevel_CoreFatal, "Test: Atom")
	<-time.After(1 * time.Millisecond)
}

func TestLoggingServiceLifeCycle(t *testing.T) {
	// Create a new logging service.
	ls := loggingService{}
	tt := &appLoggingForTestToString{}
	if err := ls.init(tt); err != nil {
		//if err := ls.init(&appLoggingForTest{t: t}); err != nil {
		t.Errorf("loggingService.init() = %v, want nil", err)
		return
	}

	id := &IDInfo{
		Type:    IDType_Atom,
		Cosmos:  "test_cosmos",
		Node:    "test_node",
		Element: "test_element",
		Atom:    "test_atom",
		Version: 0,
	}
	randStrGen := NewUtilStringRandomStringGenerator()
	accessList := make([]*validateLog, 0, 100)
	info := ""
	for i := 0; i < 100; i++ {
		dt := time.Now()
		lv := LogLevel(i % 3)
		info = randStrGen.RandomString(i)
		l := &validateLog{
			run:  true,
			dt:   dt,
			lv:   lv,
			id:   id,
			info: info,
		}
		accessList = append(accessList, l)
		ls.PushLogging(id, lv, info)
	}
	// Wait for the logging service to process the logs.
	for {
		ss := tt.access.String()
		if strings.Index(ss, info) != -1 {
			break
		}
		<-time.After(1 * time.Millisecond)
	}
	// Check the content of the log file.
	ttAccessList := strings.Split(tt.access.String(), "\n")
	ttAccessList = ttAccessList[1 : len(ttAccessList)-1]
	for i, l := range ttAccessList {
		if er := validateLogString(l, accessList[i]); er != nil {
			t.Errorf("validateLogString() = %v", er)
			return
		}
	}
	if tt.error.Len() != 0 {
		t.Errorf("tt.error = %v", tt.error)
		return
	}

	tt.access.Reset()
	tt.error.Reset()
	accessList = make([]*validateLog, 0, 100)
	errorList := make([]*validateLog, 0, 100)
	for i := 0; i < 100; i++ {
		dt := time.Now()
		lv := LogLevel(4 + i%2)
		info = randStrGen.RandomString(i)
		l := &validateLog{
			run:  true,
			dt:   dt,
			lv:   lv,
			id:   id,
			info: info,
		}
		accessList = append(accessList, l)
		errorList = append(errorList, l)
		ls.PushLogging(id, lv, info)
	}
	// Wait for the logging service to process the logs.
	for {
		ss := tt.error.String()
		if strings.Index(ss, info) != -1 {
			break
		}
		<-time.After(1 * time.Millisecond)
	}
	// Check the content of the log file.
	ttErrorList := strings.Split(tt.error.String(), "\n")
	ttErrorList = ttErrorList[0 : len(ttErrorList)-1]
	for i, l := range ttErrorList {
		if er := validateLogString(l, errorList[i]); er != nil {
			t.Errorf("validateLogString() = %v", er)
			return
		}
	}
	if tt.access.Len() != 0 {
		t.Errorf("tt.access = %v", tt.access)
		return
	}

	// Stop the logging service, and then wait for the logging service to stop.
	ls.stop()
	for {
		if ls.logBox.running {
			<-time.After(1 * time.Millisecond)
		} else {
			break
		}
	}

	// After logging service stopped.
	tt.access.Reset()
	tt.error.Reset()
	accessList = make([]*validateLog, 0, 100)
	for i := 0; i < 100; i++ {
		dt := time.Now()
		lv := LogLevel(i % 3)
		info = randStrGen.RandomString(i)
		l := &validateLog{
			run:  false,
			dt:   dt,
			lv:   lv,
			id:   id,
			info: info,
		}
		accessList = append(accessList, l)
		ls.PushLogging(id, lv, info)
	}
	// Check the content of the log file.
	ttAccessList = strings.Split(tt.access.String(), "\n")
	ttAccessList = ttAccessList[:len(ttAccessList)-1]
	for i, l := range ttAccessList {
		if er := validateLogString(l, accessList[i]); er != nil {
			t.Errorf("validateLogString() = %v", er)
			return
		}
	}
	if tt.error.Len() != 0 {
		t.Errorf("tt.error = %v", tt.error)
		return
	}

	tt.access.Reset()
	tt.error.Reset()
	accessList = make([]*validateLog, 0, 100)
	errorList = make([]*validateLog, 0, 100)
	for i := 0; i < 100; i++ {
		dt := time.Now()
		lv := LogLevel(4 + i%2)
		info = randStrGen.RandomString(i)
		l := &validateLog{
			run:  false,
			dt:   dt,
			lv:   lv,
			id:   id,
			info: info,
		}
		accessList = append(accessList, l)
		errorList = append(errorList, l)
		ls.PushLogging(id, lv, info)
		<-time.After(1 * time.Millisecond)
	}
	// Wait for the logging service to process the logs.
	for {
		ss := tt.error.String()
		if strings.Index(ss, info) != -1 {
			break
		}
		<-time.After(1 * time.Millisecond)
	}
	// Check the content of the log file.
	ttErrorList = strings.Split(tt.error.String(), "\n")
	ttErrorList = ttErrorList[0 : len(ttErrorList)-1]
	for i, l := range ttErrorList {
		if er := validateLogString(l, errorList[i]); er != nil {
			t.Errorf("validateLogString() = %v", er)
			return
		}
	}
	if tt.access.Len() != 0 {
		t.Errorf("tt.access = %v", tt.access)
		return
	}
}

func TestLoggingServiceValidateLog(t *testing.T) {
	src := "2006-01-02 15:04:05.999999 [DEBUG] test_node::test_element::test_atom => Test"
	l := &validateLog{
		run: true,
		dt:  time.Date(2006, 1, 2, 15, 4, 5, 999999000, time.UTC),
		lv:  LogLevel_Debug,
		id: &IDInfo{
			Type:    IDType_Atom,
			Cosmos:  "test_cosmos",
			Node:    "test_node",
			Element: "test_element",
			Atom:    "test_atom",
			Version: 0,
		},
		info: "Test",
	}
	if er := validateLogString(src, l); er != nil {
		t.Errorf("validateLogString() = %v, want nil", er)
	}
}

func TestLoggingService_PushLogging(t *testing.T) {
	// Create a new logging service.
	ls := loggingService{}
	tt := &appLoggingForTestToString{}
	if err := ls.init(tt); err != nil {
		t.Errorf("loggingService.init() = %v, want nil", err)
		return
	}

	id := &IDInfo{
		Type:    IDType_Atom,
		Cosmos:  "test_cosmos",
		Node:    "test_node",
		Element: "test_element",
		Atom:    "test_atom",
		Version: 0,
	}
	tt.access.Reset()
	tt.error.Reset()
	cpu := runtime.NumCPU()
	times := 100000
	length := 100
	for c := 0; c < cpu; c++ {
		go func() {
			randStrGen := NewUtilStringRandomStringGenerator()
			hashGen := NewUtilStringHashSHA256Generator()
			for i := 0; i < times; i++ {
				lv := LogLevel(i % 3)
				info := randStrGen.RandomString(length)
				hash, err := hashGen.Gen(info)
				if err != nil {
					t.Errorf("UtilStringHashSHA256() = %v", err)
					return
				}
				info += ":" + hash
				ls.PushLogging(id, lv, info)
			}
		}()
	}
	// Wait for the logging service to process the logs.
	infoLen := len(datetimeFormat) + 9 + len(id.Info()) + 4
	for {
		if l := tt.access.Len(); l >= cpu*times*(infoLen+length+1+64+1) {
			break
		}
		<-time.After(1 * time.Microsecond)
	}
	// Check the content of the log file.
	ttAccessList := strings.Split(tt.access.String(), "\n")
	ttAccessList = ttAccessList[:len(ttAccessList)-1]
	hashGen := NewUtilStringHashSHA256Generator()
	for _, access := range ttAccessList {
		info := access[infoLen : infoLen+length]
		hash := access[infoLen+length+1:]
		if h, err := hashGen.Gen(info); err != nil || h != hash {
			t.Errorf("UtilStringHashSHA256() = %v, %v", h, err)
			return
		}
	}
}

func BenchmarkLoggingService_PushLogging(b *testing.B) {
	// Create a new logging service.
	ls := loggingService{}
	tt := &appLoggingForBenchmarkToString{}
	if err := ls.init(tt); err != nil {
		b.Errorf("loggingService.init() = %v, want nil", err)
		return
	}

	id := &IDInfo{
		Type:    IDType_Atom,
		Cosmos:  "test_cosmos",
		Node:    "test_node",
		Element: "test_element",
		Atom:    "test_atom",
		Version: 0,
	}
	tt.access.Reset()
	tt.error.Reset()
	cpu := b.N
	length := 100
	for c := 0; c < cpu; c++ {
		go func() {
			randStrGen := NewUtilStringRandomStringGenerator()
			hashGen := NewUtilStringHashSHA256Generator()
			lv := LogLevel_Debug
			info := randStrGen.RandomString(length)
			hash, err := hashGen.Gen(info)
			if err != nil {
				b.Errorf("UtilStringHashSHA256() = %v", err)
				return
			}
			info += ":" + hash
			ls.PushLogging(id, lv, info)
		}()
	}
	// Wait for the logging service to process the logs.
	infoLen := len(datetimeFormat) + 9 + len(id.Info()) + 4
	for {
		if l := tt.access.Len(); l >= cpu*(infoLen+length+1+64+1) {
			break
		}
		<-time.After(1 * time.Microsecond)
	}
}

type validateLog struct {
	run  bool
	dt   time.Time
	lv   LogLevel
	id   *IDInfo
	info string
}

func validateLogString(src string, l *validateLog) error {
	// 2006-01-02 15:04:05.999999 [DEBUG] test_node::test_element::test_atom => Test
	// Check datetime.
	datetimeStr := src[:len(datetimeFormat)]
	datetime, er := time.Parse(datetimeFormat, datetimeStr)
	if er != nil {
		return er
	}
	if gap := l.dt.Sub(datetime); gap > time.Microsecond {
		return errors.New("datetime not match")
	}

	// Check level.
	levelStr := src[len(datetimeFormat) : len(datetimeFormat)+9]
	switch true {
	case l.lv == LogLevel_Debug && levelStr != " [DEBUG] ":
		return errors.New("level not match")
	case l.lv == LogLevel_Info && levelStr != " [INFO]  ":
		return errors.New("level not match")
	case l.lv == LogLevel_Warn && levelStr != " [WARN]  ":
		return errors.New("level not match")
	case l.lv == LogLevel_CoreInfo && levelStr != " [COSMO] ":
		return errors.New("level not match")
	case l.lv == LogLevel_Err && levelStr != " [ERROR] ":
		return errors.New("level not match")
	case l.lv == LogLevel_Fatal && levelStr != " [FATAL] ":
		return errors.New("level not match")
	}

	// Check ID.
	idInfo := l.id.Info()
	idStr := src[len(datetimeFormat)+9 : len(datetimeFormat)+9+len(idInfo)]
	if idStr != idInfo {
		return errors.New("id not match")
	}

	// Check info.
	infoStr := src[len(datetimeFormat)+9+len(idInfo):]
	if !l.run {
		if infoStr != " -> "+l.info {
			return errors.New("info not match")
		}
	} else {
		if infoStr != " => "+l.info {
			return errors.New("info not match")
		}
	}

	return nil
}

const datetimeFormat = "2006-01-02 15:04:05.999999"
