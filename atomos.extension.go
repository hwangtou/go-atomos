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
		fallthrough
	default:
		return fmt.Sprintf("%s", x.Cosmos)
	}
}

func NewError(code int64, message string) *ErrorInfo {
	return &ErrorInfo{
		Code:    code,
		Message: message,
		Stack:   "",
	}
}

func NewErrorWithStack(code int64, message, stack string) *ErrorInfo {
	return &ErrorInfo{
		Code:    code,
		Message: message,
		Stack:   stack,
	}
}

func (x *ErrorInfo) Error() string {
	return x.Message
}
