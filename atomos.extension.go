package go_atomos

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
)

func (x *Config) Check() *Error {
	if x == nil {
		return NewError(ErrRunnableConfigInvalid, "Config is nil").AddStack(nil)
	}
	if x.Cosmos == "" {
		return NewError(ErrRunnableConfigInvalid, "Cosmos is empty").AddStack(nil)
	}
	if x.Node == "" {
		return NewError(ErrRunnableConfigInvalid, "Node is empty").AddStack(nil)
	}
	return nil
}

func (x *IDInfo) Info() string {
	if x == nil {
		return "NilID"
	}
	switch x.Type {
	case IDType_Atom:
		return fmt.Sprintf("%s::%s::%s", x.Node, x.Element, x.Atom)
	case IDType_Element:
		return fmt.Sprintf("%s::%s", x.Node, x.Element)
	case IDType_Cosmos:
		return x.Node
	default:
		return x.Node
	}
}

func (x *IDInfo) IsEqual(r *IDInfo) bool {
	if x.Type != r.Type {
		return false
	}
	return x.Atom == r.Atom && x.Element == r.Element && x.Node == r.Node
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
	return x.Code == ErrAtomIsRunning
}

func (x *Error) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	var str string
	var ok bool
	switch v := value.(type) {
	case string:
		str = v
		ok = true
	case []uint8:
		str = string(v)
		ok = true
	}
	if !ok {
		return fmt.Errorf("value is not bytes. type=(%T),value=(%v)", value, value)
	}
	er := json.Unmarshal([]byte(str), x)
	if er != nil {
		return er
	}
	return nil
}

func (x *Error) Value() (driver.Value, error) {
	if x == nil {
		return nil, nil
	}
	return json.Marshal(x)
}
