package go_atomos

import (
	"fmt"
	"strings"
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

func (x *ErrorInfo) AddStack(id *IDInfo, stack []byte) *ErrorInfo {
	x.Stacks = append(x.Stacks, &ErrorStackInfo{
		Id:    id,
		Stack: stack,
	})
	return x
}

func (x *ErrorInfo) Error() string {
	if x == nil {
		return ""
	}
	if len(x.Stacks) > 0 {
		var stacks strings.Builder
		for i, stack := range x.Stacks {
			stacks.WriteString(fmt.Sprintf("Stack[%d]: %s\n", i, stack.Id.str()))
			stacks.Write(stack.Stack)
		}
		return fmt.Sprintf("%s\n%s", x.Message, stacks.String())
	}
	return x.Message
}
