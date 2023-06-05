package go_atomos

import (
	"bytes"
	"google.golang.org/protobuf/proto"
	"reflect"
	"runtime"
	"strconv"
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
	return n
}
