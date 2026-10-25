package atomos

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"weak"
)

// Cause preservation across kernel boundaries.
//
// *Error is a generated proto struct, so the original error a non-*Error
// value was converted from (by asError) cannot live in a struct field.
// The association lives in errCauses instead: keys are weak pointers, so an
// entry lasts exactly as long as the *Error it describes and is removed by
// an AddCleanup once the error is unreachable. The association is visible
// through (*Error).Unwrap.
//
// It is process-local by design: proto serialization (cross-node calls) and
// proto.Clone keep only Message, which asError already fills with the
// cause's text.
var errCauses sync.Map // weak.Pointer[Error] -> error

// setCause records cause as err's unwrap cause and schedules the entry's
// removal once err becomes unreachable.
func setCause(err *Error, cause error) *Error {
	if err == nil || cause == nil {
		return err
	}
	key := weak.Make(err)
	errCauses.Store(key, cause)
	runtime.AddCleanup(err, func(k weak.Pointer[Error]) {
		errCauses.Delete(k)
	}, key)
	return err
}

// causeOf returns the error err was converted from, or nil.
func causeOf(err *Error) error {
	if err == nil {
		return nil
	}
	// err is reachable while causeOf runs (Unwrap was invoked on it), so the
	// weak key cannot have been zeroed and the entry cannot be gone yet.
	if cause, ok := errCauses.Load(weak.Make(err)); ok {
		return cause.(error)
	}
	return nil
}

// Unwrap returns the original error this *Error was converted from at a
// framework boundary (asError / WithStack), or nil for errors constructed
// inside the framework. It lets errors.Is / errors.As keep reaching the %w
// chains and sentinel targets of errors returned by handlers after they
// cross a framework call, e.g.:
//
//	_, err := atomID.SyncMessagingByName(caller, "Get", in)
//	if errors.Is(err, myapp.ErrNotFound) { ... }
//
// The association is process-local: it does not survive proto
// serialization (remote calls) or proto.Clone.
func (x *Error) Unwrap() error {
	return causeOf(x)
}

// WithStack converts err into a framework error when needed and records the
// caller as a hop in the error's trace, so the error reads like the ones
// the kernel produces itself. On the first hop it additionally captures the
// caller's Go stack (capped at 32 frames) as the creation-site trace;
// later hops record file:line only, like the kernel's AddStack calls.
//
// A non-*Error err keeps its identity as the unwrap cause, so errors.Is /
// errors.As against the caller's own sentinels keep working:
//
//	return atomos.WithStack(fmt.Errorf("load config: %w", errConfigGone), self)
//
// WithStack adds its hop to a fresh *Error and never mutates err itself, so
// package-level sentinels and errors shared with other owners remain unchanged.
func WithStack(err error, id SelfID, args ...any) error {
	e := asError(err)
	if e == nil {
		return nil
	}
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file, line = "???", 0
	}
	entry := &ErrorCallerInfo{
		Id:   SelfID2IDInfo(id),
		File: file,
		Line: uint32(line),
		Args: marshalCallArgs(args),
	}
	if len(e.CallStacks) == 0 {
		// First frame on this error: attach the full Go stack. Reuses the
		// PanicStack field (displayed as "GoStack" by Error when no panic
		// reason is set) to avoid a wire-format change.
		entry.PanicStack = renderGoStack(1, 32)
	}
	e.CallStacks = append(e.CallStacks, entry)
	return e
}

// marshalCallArgs JSON-encodes the non-nil pointer arguments AddStack-style
// callers attach to a hop. Shared by AddStack, AddPanicStack and WithStack.
func marshalCallArgs(args []any) []string {
	var argsBuf []string
	for _, arg := range args {
		if arg != nil && reflect.TypeOf(arg).Kind() == reflect.Pointer && !reflect.ValueOf(arg).IsNil() {
			buf, _ := json.Marshal(arg)
			argsBuf = append(argsBuf, string(buf))
		}
	}
	return argsBuf
}

// renderGoStack renders up to maxFrames stack frames starting skip levels
// above renderGoStack's caller (skip=1 starts at the caller of the function
// that called renderGoStack).
func renderGoStack(skip, maxFrames int) string {
	pcs := make([]uintptr, maxFrames)
	n := runtime.Callers(skip+2, pcs)
	if n == 0 {
		return ""
	}
	var builder strings.Builder
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if frame.File != "" {
			fmt.Fprintf(&builder, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		} else {
			fmt.Fprintf(&builder, "%s\n", frame.Function)
		}
		if !more {
			break
		}
	}
	return builder.String()
}
