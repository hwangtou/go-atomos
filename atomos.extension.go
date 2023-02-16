package go_atomos

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
)

func (x *IDInfo) Info() string {
	if x == nil {
		return "NilID"
	}
	switch x.Type {
	case IDType_Atom:
		return fmt.Sprintf("%s::%s::%s", x.Cosmos, x.Element, x.Atomos)
	case IDType_Element:
		return fmt.Sprintf("%s::%s", x.Cosmos, x.Element)
	case IDType_Cosmos:
		return x.Cosmos
	case IDType_AppLoader:
		return "AppLoader"
	case IDType_App:
		return "App"
	default:
		return x.Cosmos
	}
}

func (x *IDInfo) IsEqual(r *IDInfo) bool {
	if x.Type != r.Type {
		return false
	}
	return x.Atomos == r.Atomos && x.Element == r.Element && x.Cosmos == r.Cosmos
}

func SelfID2IDInfo(id SelfID) *IDInfo {
	if id != nil && reflect.TypeOf(id).Kind() == reflect.Pointer && !reflect.ValueOf(id).IsNil() {
		return id.GetIDInfo()
	}
	return nil
}

func NewError(code int64, message string) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		CallStacks: nil,
	}
}

func NewErrorf(code int64, format string, args ...interface{}) *Error {
	return &Error{
		Code:       code,
		Message:    fmt.Sprintf(format, args...),
		CallStacks: nil,
	}
}

func (x *Error) AddPanicStack(id SelfID, skip int, reason interface{}, args ...interface{}) *Error {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		file, line = "???", 0
	}
	// Argument
	var argsBuf []string
	for _, arg := range args {
		if arg != nil && reflect.TypeOf(arg).Kind() == reflect.Pointer && !reflect.ValueOf(arg).IsNil() {
			buf, _ := json.Marshal(arg)
			argsBuf = append(argsBuf, string(buf))
		}
	}
	x.CallStacks = append(x.CallStacks, &ErrorCallerInfo{
		Id:          SelfID2IDInfo(id),
		PanicStack:  string(debug.Stack()),
		PanicReason: fmt.Sprintf("%v", reason),
		File:        file,
		Line:        uint32(line),
		Args:        argsBuf,
	})
	return x
}

func (x *Error) AddStack(id SelfID, args ...interface{}) *Error {
	if x == nil {
		return nil
	}
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file, line = "???", 0
	}
	// Argument
	var argsBuf []string
	for _, arg := range args {
		if arg != nil && reflect.TypeOf(arg).Kind() == reflect.Pointer && !reflect.ValueOf(arg).IsNil() {
			buf, _ := json.Marshal(arg)
			argsBuf = append(argsBuf, string(buf))
		}
	}
	x.CallStacks = append(x.CallStacks, &ErrorCallerInfo{
		Id:          SelfID2IDInfo(id),
		PanicStack:  "",
		PanicReason: "",
		File:        file,
		Line:        uint32(line),
		Args:        argsBuf,
	})
	return x
}

func (x *Error) Error() string {
	if x == nil {
		return ""
	}
	if len(x.CallStacks) > 0 {
		var builder strings.Builder
		n := len(x.CallStacks)
		for i, stack := range x.CallStacks {
			builder.WriteString(fmt.Sprintf("Stack[%d] %s -> %s:%d\n", n-i-1, stack.Id.Info(), stack.File, stack.Line))
			if len(stack.Args) > 0 {
				builder.WriteString(fmt.Sprintf("\tArguments: \n"))
				for _, arg := range stack.Args {
					builder.WriteString(fmt.Sprintf("\t\t%s\n", arg))
				}
			}
			if stack.PanicReason != "" || stack.PanicStack != "" {
				builder.WriteString(fmt.Sprintf("\tRecover: %s\n%s\n", stack.PanicReason, stack.PanicStack))
			}
		}
		return fmt.Sprintf("[%v] %s\n%s", x.Code, x.Message, builder.String())
	}
	return x.Message
}

func (x *Error) IsAtomExist() bool {
	return x.Code == ErrAtomExists
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
