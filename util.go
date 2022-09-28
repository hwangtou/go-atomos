package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"reflect"
)

func IsNilProto(p proto.Message) bool {
	if p == nil {
		return true
	}
	return reflect.ValueOf(p).IsNil()
}
