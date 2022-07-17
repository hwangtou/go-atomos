package go_atomos

import (
	"fmt"
)

func (x *IDInfo) str() string {
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
		Stack:   "",
	}
}

func NewErrorf(code int64, format string, args ...interface{}) *ErrorInfo {
	return &ErrorInfo{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Stack:   "",
	}
}

func NewErrorWithStack(code int64, stack, message string) *ErrorInfo {
	return &ErrorInfo{
		Code:    code,
		Message: message,
		Stack:   stack,
	}
}

func NewErrorfWithStack(code int64, stack []byte, format string, args ...interface{}) *ErrorInfo {
	return &ErrorInfo{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Stack:   string(stack),
	}
}

func (x *ErrorInfo) Error() string {
	if x == nil {
		return ""
	}
	if len(x.Stack) > 0 {
		return fmt.Sprintf("%s\n%s", x.Message, x.Stack)
	}
	return x.Message
}
