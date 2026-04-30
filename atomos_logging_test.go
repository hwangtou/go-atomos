package atomos

import (
	"reflect"
	"testing"
)

func TestAtomosLogging(t *testing.T) {
	al := atomosLogging{}
	id := &IDInfo{Type: IDType_Atom, Cosmos: "test_cosmos", Node: "test_node", Element: "test_element", Atom: "test_atom"}
	ls := &testLoggingService{}

	initAtomosLog(&al, id, LogLevel_Debug, ls)

	ls.logList = nil
	al.Debug("test debug 0")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Debug, "test debug 0"}) {
		t.Errorf("TestAtomosLogging: Debug failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Info("test info 0")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Info, "test info 0"}) {
		t.Errorf("TestAtomosLogging: Info failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Warn("test warn 0")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Warn, "test warn 0"}) {
		t.Errorf("TestAtomosLogging: Warn failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreInfo("test core info 0")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreInfo, "test core info 0"}) {
		t.Errorf("TestAtomosLogging: CoreInfo failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Error("test error 0")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Err, "test error 0"}) {
		t.Errorf("TestAtomosLogging: Error failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Fatal("test fatal 0")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Fatal, "test fatal 0"}) {
		t.Errorf("TestAtomosLogging: Fatal failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreError("test core error 0")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreErr, "test core error 0"}) {
		t.Errorf("TestAtomosLogging: CoreError failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreFatal("test core fatal 0")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreFatal, "test core fatal 0"}) {
		t.Errorf("TestAtomosLogging: CoreFatal failed. logList=(%v)", ls.logList)
		return
	}

	al.SetLevel(LogLevel_Info)
	ls.logList = nil
	al.Debug("test debug 1")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Debug failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Info("test info 1")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Info, "test info 1"}) {
		t.Errorf("TestAtomosLogging: Info failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Warn("test warn 1")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Warn, "test warn 1"}) {
		t.Errorf("TestAtomosLogging: Warn failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreInfo("test core info 1")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreInfo, "test core info 1"}) {
		t.Errorf("TestAtomosLogging: CoreInfo failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Error("test error 1")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Err, "test error 1"}) {
		t.Errorf("TestAtomosLogging: Error failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Fatal("test fatal 1")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Fatal, "test fatal 1"}) {
		t.Errorf("TestAtomosLogging: Fatal failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreError("test core error 1")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreErr, "test core error 1"}) {
		t.Errorf("TestAtomosLogging: CoreError failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreFatal("test core fatal 1")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreFatal, "test core fatal 1"}) {
		t.Errorf("TestAtomosLogging: CoreFatal failed. logList=(%v)", ls.logList)
		return
	}

	al.SetLevel(LogLevel_CoreInfo)
	ls.logList = nil
	al.Debug("test debug 2")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Debug failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Info("test info 2")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Info failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Warn("test warn 2")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Warn failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreInfo("test core info 2")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreInfo, "test core info 2"}) {
		t.Errorf("TestAtomosLogging: CoreInfo failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Error("test error 2")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Err, "test error 2"}) {
		t.Errorf("TestAtomosLogging: Error failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Fatal("test fatal 2")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Fatal, "test fatal 2"}) {
		t.Errorf("TestAtomosLogging: Fatal failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreError("test core error 2")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreErr, "test core error 2"}) {
		t.Errorf("TestAtomosLogging: CoreError failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreFatal("test core fatal 2")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreFatal, "test core fatal 2"}) {
		t.Errorf("TestAtomosLogging: CoreFatal failed. logList=(%v)", ls.logList)
		return
	}

	al.SetLevel(LogLevel_Warn)
	ls.logList = nil
	al.Debug("test debug 3")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Debug failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Info("test info 3")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Info failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Warn("test warn 3")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Warn, "test warn 3"}) {
		t.Errorf("TestAtomosLogging: Warn failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreInfo("test core info 3")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreInfo, "test core info 3"}) {
		t.Errorf("TestAtomosLogging: CoreInfo failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Error("test error 3")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Err, "test error 3"}) {
		t.Errorf("TestAtomosLogging: Error failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Fatal("test fatal 3")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Fatal, "test fatal 3"}) {
		t.Errorf("TestAtomosLogging: Fatal failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreError("test core error 3")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreErr, "test core error 3"}) {
		t.Errorf("TestAtomosLogging: CoreError failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreFatal("test core fatal 3")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreFatal, "test core fatal 3"}) {
		t.Errorf("TestAtomosLogging: CoreFatal failed. logList=(%v)", ls.logList)
		return
	}

	al.SetLevel(LogLevel_Err)
	ls.logList = nil
	al.Debug("test debug 4")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Debug failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Info("test info 4")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Info failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Warn("test warn 4")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Warn failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreInfo("test core info 4")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: CoreInfo failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Error("test error 4")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Err, "test error 4"}) {
		t.Errorf("TestAtomosLogging: Error failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Fatal("test fatal 4")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Fatal, "test fatal 4"}) {
		t.Errorf("TestAtomosLogging: Fatal failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreError("test core error 4")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreErr, "test core error 4"}) {
		t.Errorf("TestAtomosLogging: CoreError failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreFatal("test core fatal 4")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreFatal, "test core fatal 4"}) {
		t.Errorf("TestAtomosLogging: CoreFatal failed. logList=(%v)", ls.logList)
		return
	}

	al.SetLevel(LogLevel_CoreErr)
	ls.logList = nil
	al.Debug("test debug 5")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Debug failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Info("test info 5")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Info failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Warn("test warn 5")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Warn failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreInfo("test core info 5")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: CoreInfo failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Error("test error 5")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Error failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Fatal("test fatal 5")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Fatal failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreError("test core error 5")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreErr, "test core error 5"}) {
		t.Errorf("TestAtomosLogging: CoreError failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreFatal("test core fatal 5")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreFatal, "test core fatal 5"}) {
		t.Errorf("TestAtomosLogging: CoreFatal failed. logList=(%v)", ls.logList)
		return
	}

	al.SetLevel(LogLevel_Fatal)
	ls.logList = nil
	al.Debug("test debug 6")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Debug failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Info("test info 6")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Info failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Warn("test warn 6")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Warn failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreInfo("test core info 6")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: CoreInfo failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Error("test error 6")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Error failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Fatal("test fatal 6")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_Fatal, "test fatal 6"}) {
		t.Errorf("TestAtomosLogging: Fatal failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreError("test core error 6")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreErr, "test core error 6"}) {
		t.Errorf("TestAtomosLogging: CoreError failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreFatal("test core fatal 6")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreFatal, "test core fatal 6"}) {
		t.Errorf("TestAtomosLogging: CoreFatal failed. logList=(%v)", ls.logList)
		return
	}

	al.SetLevel(LogLevel_CoreFatal)
	ls.logList = nil
	al.Debug("test debug 7")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Debug failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Info("test info 7")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Info failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Warn("test warn 7")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Warn failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreInfo("test core info 7")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: CoreInfo failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Error("test error 7")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Error failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.Fatal("test fatal 7")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: Fatal failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreError("test core error 7")
	if len(ls.logList) != 0 {
		t.Errorf("TestAtomosLogging: CoreError failed. logList=(%v)", ls.logList)
		return
	}
	ls.logList = nil
	al.coreFatal("test core fatal 7")
	if len(ls.logList) != 1 || !reflect.DeepEqual(ls.logList[0], &testLoggingServiceLog{id, LogLevel_CoreFatal, "test core fatal 7"}) {
		t.Errorf("TestAtomosLogging: CoreFatal failed. logList=(%v)", ls.logList)
		return
	}

}
