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
			if got := CodeOf(err); got != ErrCodeMessageFromIDInvalid {
				t.Fatalf("code = %v, want ErrCodeMessageFromIDInvalid", got)
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
		if got := CodeOf(err); got != ErrCodeMessageFromIDInvalid {
			t.Fatalf("callback code = %v, want ErrCodeMessageFromIDInvalid", got)
		}
	})
	if err != nil {
		t.Fatalf("error returned despite callback delivery: %v", err)
	}
	if called != 1 {
		t.Fatalf("callback calls = %d, want 1", called)
	}
}

func TestMessengerDecoderErrorCodes(t *testing.T) {
	messenger := Messenger[ID, ID, Atomos, *ForTestGreetingI, *ForTestGreetingO]{}
	decoder := messenger.Decoder(&ForTestGreetingI{}, &ForTestGreetingO{})
	tests := []struct {
		name        string
		decode      MessageDecoder
		protoOrJSON bool
		wantCode    int64
	}{
		{name: "input proto", decode: decoder.InDec, protoOrJSON: true, wantCode: ErrCodeMessageArgTypeInvalid},
		{name: "input json", decode: decoder.InDec, wantCode: ErrCodeMessageArgTypeInvalid},
		{name: "output proto", decode: decoder.OutDec, protoOrJSON: true, wantCode: ErrCodeMessageReplyTypeInvalid},
		{name: "output json", decode: decoder.OutDec, wantCode: ErrCodeMessageReplyTypeInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := tt.decode([]byte("{"), tt.protoOrJSON)
			if message != nil || CodeOf(err) != tt.wantCode {
				t.Fatalf("message = %v, error = %v, want nil and code %d", message, err, tt.wantCode)
			}
		})
	}
}

func TestMessengerReplyTypeErrorCode(t *testing.T) {
	messenger := Messenger[ID, ID, Atomos, *ForTestGreetingI, *ForTestGreetingO]{Name: "Greeting"}
	out, err := messenger.handleReply(&ForTestGreetingI{}, nil)
	if out != nil || CodeOf(err) != ErrCodeMessageReplyTypeInvalid {
		t.Fatalf("out = %v, error = %v, want nil and ErrCodeMessageReplyTypeInvalid", out, err)
	}
}
