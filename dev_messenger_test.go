package atomos

import "testing"

func TestMessengerAsyncNilCallerAndNilCallback(t *testing.T) {
	messenger := Messenger[ID, ID, Atomos, *ForTestGreetingI, *ForTestGreetingO]{}
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "element",
			call: func() error {
				return messenger.AsyncElement(nil, nil, nil, nil)
			},
		},
		{
			name: "atom",
			call: func() error {
				return messenger.AsyncAtom(nil, nil, nil, nil)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if got := CodeOf(err); got != ErrCodeAtomFromIDInvalid {
				t.Fatalf("code = %v, want ErrCodeAtomFromIDInvalid", got)
			}
		})
	}
}

func TestMessengerAsyncNilCallerUsesCallbackWhenProvided(t *testing.T) {
	messenger := Messenger[ID, ID, Atomos, *ForTestGreetingI, *ForTestGreetingO]{}
	called := 0
	err := messenger.AsyncAtom(nil, nil, nil, func(out *ForTestGreetingO, err error) {
		called++
		if out != nil {
			t.Fatalf("out = %v, want nil", out)
		}
		if got := CodeOf(err); got != ErrCodeAtomFromIDInvalid {
			t.Fatalf("callback code = %v, want ErrCodeAtomFromIDInvalid", got)
		}
	})
	if err != nil {
		t.Fatalf("error returned despite callback delivery: %v", err)
	}
	if called != 1 {
		t.Fatalf("callback calls = %d, want 1", called)
	}
}
