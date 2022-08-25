package go_atomos

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"reflect"
	"runtime"
	"strings"
)

func (x *IDInfo) Info() string {
	if x == nil {
		return "InvalidAtomId"
	}
	switch x.Type {
	case IDType_Atomos:
		return fmt.Sprintf("%s::%s::%s", x.Cosmos, x.Element, x.Atomos)
	case IDType_Element:
		return fmt.Sprintf("%s::%s", x.Cosmos, x.Element)
	case IDType_Cosmos:
		return fmt.Sprintf("%s", x.Cosmos)
	case IDType_Main:
		return fmt.Sprintf("Main")
	default:
		return fmt.Sprintf("%s", x.Cosmos)
	}
}

func (x *IDInfo) IsEqual(r *IDInfo) bool {
	if x.Type != r.Type {
		return false
	}
	return x.Atomos == r.Atomos && x.Element == r.Element && x.Cosmos == r.Cosmos
}

func NewError(code int64, message string) *ErrorInfo {
	return &ErrorInfo{
		Code:    code,
		Message: message,
		Stacks:  nil,
	}
}

func NewErrorf(code int64, format string, args ...interface{}) *ErrorInfo {
	return &ErrorInfo{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Stacks:  nil,
	}
}

func (x *ErrorInfo) AddStack(id SelfID, file, recoverInfo string, line int, args proto.Message) {
	// ID Info
	var idInfo *IDInfo
	if id != nil && reflect.TypeOf(id).Kind() == reflect.Pointer && !reflect.ValueOf(id).IsNil() {
		idInfo = id.GetIDInfo()
	}
	// Argument
	var buf, jsonBuf []byte
	if args != nil && reflect.TypeOf(args).Kind() == reflect.Pointer && !reflect.ValueOf(args).IsNil() {
		buf, _ = proto.Marshal(args)
		jsonBuf, _ = json.Marshal(args)
	}
	x.Stacks = append(x.Stacks, &ErrorCallerInfo{
		Id:       idInfo,
		Reason:   recoverInfo,
		File:     file,
		Line:     uint32(line),
		Args:     buf,
		ArgsRead: string(jsonBuf),
	})
}

func (x *ErrorInfo) AutoStack(id SelfID, args proto.Message) *ErrorInfo {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file, line = "???", 0
	}
	x.AddStack(id, file, "", line, args)
	return x
}

func (x *ErrorInfo) Error() string {
	if x == nil {
		return ""
	}
	if len(x.Stacks) > 0 {
		var stacks strings.Builder
		n := len(x.Stacks)
		for i, stack := range x.Stacks {
			stacks.WriteString(fmt.Sprintf("Chain[%d] %s -> %s:%d\n", n-i-1, stack.Id.Info(), stack.File, stack.Line))
			if stack.Reason != "" {
				stacks.WriteString(fmt.Sprintf("\tRecover: %s\n", stack.Reason))
			}
			if stack.ArgsRead != "" {
				stacks.WriteString(fmt.Sprintf("\tArguments: %s\n", stack.ArgsRead))
			}
		}
		return fmt.Sprintf("[%v] %s\n%s", x.Code, x.Message, stacks.String())
	}
	return x.Message
}

func (x *RemoteServerConfig) IsEqual(server *RemoteServerConfig) bool {
	if x.Host != server.Host {
		return false
	}
	if x.Port != server.Port {
		return false
	}
	return true
}
