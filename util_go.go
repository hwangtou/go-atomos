package atomos

import (
	"bytes"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
)

func IsNilProto(p proto.Message) bool {
	if p == nil {
		return true
	}
	return reflect.ValueOf(p).IsNil()
}

// go:noinline
func getGoID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	if n == 0 {
		panic("cannot get goroutine id")
	}
	return n
}

func Recover(id SelfID) {
	if r := recover(); r != nil {
		err := NewErrorf(ErrFrameworkRecoverFromPanic, "Recovered from panic.").AddPanicStack(id, 3, r)
		id.Log().Error("Recover: %v", err)
	}
}

func dumpAllGoID() map[uint64]bool {
	m := map[uint64]bool{}
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	info := buf[:n]
	lines := strings.Split(string(info), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "goroutine ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				goIDStr := fields[1]
				goID, er := strconv.ParseUint(goIDStr, 10, 64)
				if er != nil {
					panic(er)
				}
				m[goID] = true
			}
		}
	}
	return m
}
