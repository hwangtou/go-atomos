package atomos

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"

	"google.golang.org/protobuf/proto"
)

func (x *Config) Check() error {
	if err := x.ValidateCosmosNodeConfig(); err != nil {
		return asError(err).AddStack(nil)
	}
	if err := x.ValidateSupervisorConfig(); err != nil {
		return asError(err).AddStack(nil)
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
	//case IDType_AppLoader:
	//	return "AppLoader"
	//case IDType_App:
	//	return "App"
	default:
		return x.Node
	}
}

func (x *IDInfo) IsEqual(r *IDInfo) bool {
	if x == nil || r == nil {
		return x == r
	}
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

func NewErrorf(code int64, format string, args ...any) *Error {
	return &Error{
		Code:       code,
		Message:    fmt.Sprintf(format, args...),
		CallStacks: nil,
	}
}

// errOrNil converts a nil *Error to a nil error interface. Returning a nil
// *Error through an error-typed API boundary would produce a non-nil
// interface holding a nil pointer (the typed-nil trap), breaking
// "err != nil" checks; every such boundary must go through errOrNil.
func errOrNil(err *Error) error {
	if err == nil {
		return nil
	}
	return err
}

// asError converts an error-typed value back to *Error for framework-internal
// handling (stacks, code inspection). The result is always fresh, so AddStack
// cannot mutate a shared sentinel or another owner's error. A wrapped *Error
// supplies the code while the complete original error remains the unwrap cause
// (see error_cause.go), preserving errors.Is/As and joined error chains.
func asError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return cloneError(e)
	}
	e := NewError(ErrCodeUnknown, err.Error())
	var frameworkErr *Error
	if errors.As(err, &frameworkErr) && frameworkErr != nil {
		e.Code = frameworkErr.Code
		e.CallStacks = append([]*ErrorCallerInfo(nil), frameworkErr.CallStacks...)
	}
	return setCause(e, err)
}

func cloneError(err *Error) *Error {
	if err == nil {
		return nil
	}
	clone := proto.Clone(err).(*Error)
	return setCause(clone, causeOf(err))
}

func (x *Error) AddPanicStack(id SelfID, skip int, reason any, args ...any) *Error {
	if x == nil {
		return nil
	}
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		file, line = "???", 0
	}
	x.CallStacks = append(x.CallStacks, &ErrorCallerInfo{
		Id:          SelfID2IDInfo(id),
		PanicStack:  string(debug.Stack()),
		PanicReason: fmt.Sprintf("%v", reason),
		File:        file,
		Line:        uint32(line),
		Args:        marshalCallArgs(args),
	})
	return x
}

func (x *Error) AddStack(id SelfID, args ...any) *Error {
	if x == nil {
		return nil
	}
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file, line = "???", 0
	}
	x.CallStacks = append(x.CallStacks, &ErrorCallerInfo{
		Id:          SelfID2IDInfo(id),
		PanicStack:  "",
		PanicReason: "",
		File:        file,
		Line:        uint32(line),
		Args:        marshalCallArgs(args),
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
			if stack.PanicReason != "" {
				builder.WriteString(fmt.Sprintf("\tRecover: %s\n%s\n", stack.PanicReason, stack.PanicStack))
			} else if stack.PanicStack != "" {
				// PanicStack doubles as the creation-site Go stack for
				// WithStack-captured errors (see error_cause.go).
				builder.WriteString(fmt.Sprintf("\tGoStack:\n%s\n", stack.PanicStack))
			}
		}
		return fmt.Sprintf("[%v] %s\n%s", x.Code, x.Message, builder.String())
	}
	return x.Message
}

// IsAtomExist reports whether x carries the ErrCodeAtomIsRunning code.
//
// Deprecated: use errors.Is(err, ErrAtomIsRunning) instead.
func (x *Error) IsAtomExist() bool {
	return errors.Is(x, ErrAtomIsRunning)
}

func (x *Error) Scan(value any) error {
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
