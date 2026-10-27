package atomos_test

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/atomostest"
)

// ExampleCodeOf shows how to inspect framework errors: errors.Is matches the
// package sentinels by code, and CodeOf extracts the numeric code (OK for
// nil, ErrCodeUnknown for non-framework errors).
func ExampleCodeOf() {
	err := atomos.NewErrorf(atomos.ErrCodeAtomNotExists, "atom %q does not exist", "foo")

	fmt.Println(atomos.CodeOf(err))
	fmt.Println(errors.Is(err, atomos.ErrAtomNotExists))
	fmt.Println(atomos.CodeOf(errors.New("not a framework error")))
	fmt.Println(atomos.CodeOf(nil))

	// Output:
	// 4015
	// true
	// -1
	// 0
}

// ExampleArgBaseAtomosAnyValue shows the wrap/extract round trip for carrying
// arbitrary data through a call's variadic args.
func ExampleArgBaseAtomosAnyValue() {
	arg := atomos.ArgBaseAtomosAny("payload")
	v, ok := atomos.ArgBaseAtomosAnyValue(arg)
	fmt.Println(v, ok)

	// Args created by other constructors carry no payload.
	_, ok = atomos.ArgBaseAtomosAnyValue(atomos.ArgBaseAtomosWaitKilled())
	fmt.Println(ok)

	// Output:
	// payload true
	// false
}

// ExampleWithRebind shows a call that automatically re-resolves its target
// ID and retries once when the first attempt fails with a stale-ID error
// (here ErrAtomNotExists, produced by the first fake).
func ExampleWithRebind() {
	attempts := 0
	resolve := func() (atomos.ID, error) {
		attempts++
		fake := atomostest.NewFakeID()
		if attempts == 1 {
			// The first resolution yields a stale target.
			fake.SetMessageReply("Ping", nil, atomos.ErrAtomNotExists)
		} else {
			fake.SetMessageReply("Ping", wrapperspb.String("pong"), nil)
		}
		return fake, nil
	}

	out, err := atomos.WithRebind(resolve, func(id atomos.ID) (proto.Message, error) {
		return id.SyncMessagingByName(nil, "Ping", nil)
	})
	fmt.Println(attempts, err)
	fmt.Println(out)

	// Output:
	// 2 <nil>
	// value:"pong"
}
