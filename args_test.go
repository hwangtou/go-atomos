package atomos

import "testing"

func TestArgBaseAtomosAnyValue(t *testing.T) {
	if v, ok := ArgBaseAtomosAnyValue(nil); ok || v != nil {
		t.Fatalf("nil arg: got (%v, %v), want (nil, false)", v, ok)
	}
	if v, ok := ArgBaseAtomosAnyValue(ArgBaseAtomosTimeout(0)); ok {
		t.Fatalf("timeout arg: got (%v, true), want false", v)
	}
	arg := ArgBaseAtomosAny("payload")
	if _, ok := arg.(argBaseAtomosWithAny); !ok {
		t.Fatalf("ArgBaseAtomosAny returned %T, want argBaseAtomosWithAny", arg)
	}
	if v, ok := ArgBaseAtomosAnyValue(arg); !ok || v != "payload" {
		t.Fatalf("round trip: got (%v, %v), want (payload, true)", v, ok)
	}
	var ptr *argBaseAtomosWithAny
	if v, ok := ArgBaseAtomosAnyValue(ptr); ok || v != nil {
		t.Fatalf("typed nil: got (%v, %v), want (nil, false)", v, ok)
	}
}
