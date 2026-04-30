package atomos

import (
	"testing"
	"time"
)

func TestBaseAtomosHelper_ValidArgs(t *testing.T) {
	tests := []struct {
		name     string
		mailType BaseAtomosMailType
		ext      []ArgsForBaseAtomos
		wantErr  bool
	}{
		{
			name:     "sync with timeout",
			mailType: BaseAtomosMailSync,
			ext:      []ArgsForBaseAtomos{ArgBaseAtomosTimeout(5 * time.Second)},
			wantErr:  false,
		},
		{
			name:     "sync with append to head",
			mailType: BaseAtomosMailSync,
			ext:      []ArgsForBaseAtomos{ArgBaseAtomosAppendToHead()},
			wantErr:  false,
		},
		{
			name:     "kill with wait killed and timeout",
			mailType: BaseAtomosMailKill,
			ext:      []ArgsForBaseAtomos{ArgBaseAtomosWaitKilled(), ArgBaseAtomosTimeout(3 * time.Second)},
			wantErr:  false,
		},
		{
			name:     "no args",
			mailType: BaseAtomosMailSync,
			ext:      nil,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := createBaseAtomosHelper(tt.mailType, tt.ext)
			if tt.wantErr && !helper.hasErrors() {
				t.Errorf("expected errors but got none")
			}
			if !tt.wantErr && helper.hasErrors() {
				t.Errorf("unexpected errors: %v", helper.getError())
			}
		})
	}
}

func TestBaseAtomosHelper_InvalidArgs(t *testing.T) {
	tests := []struct {
		name     string
		mailType BaseAtomosMailType
		ext      []ArgsForBaseAtomos
		errCount int
	}{
		{
			name:     "nil arg",
			mailType: BaseAtomosMailSync,
			ext:      []ArgsForBaseAtomos{nil},
			errCount: 1,
		},
		{
			name:     "timeout on async mail is invalid",
			mailType: BaseAtomosMailAsync,
			ext:      []ArgsForBaseAtomos{ArgBaseAtomosTimeout(5 * time.Second)},
			errCount: 1,
		},
		{
			name:     "wait killed on sync mail is invalid",
			mailType: BaseAtomosMailSync,
			ext:      []ArgsForBaseAtomos{ArgBaseAtomosWaitKilled()},
			errCount: 1,
		},
		{
			name:     "duplicate timeout",
			mailType: BaseAtomosMailSync,
			ext:      []ArgsForBaseAtomos{ArgBaseAtomosTimeout(1 * time.Second), ArgBaseAtomosTimeout(2 * time.Second)},
			errCount: 1,
		},
		{
			name:     "duplicate append to head",
			mailType: BaseAtomosMailSync,
			ext:      []ArgsForBaseAtomos{ArgBaseAtomosAppendToHead(), ArgBaseAtomosAppendToHead()},
			errCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := createBaseAtomosHelper(tt.mailType, tt.ext)
			if !helper.hasErrors() {
				t.Fatal("expected errors but got none")
			}
			if err := helper.getError(); err == nil {
				t.Fatal("getError returned nil despite hasErrors")
			} else if err.Code != ErrAtomosInvalidArguments {
				t.Errorf("unexpected error code: %v", err.Code)
			}
		})
	}
}

func TestBaseAtomosHelper_GetRemoteArg(t *testing.T) {
	ext := []ArgsForBaseAtomos{
		ArgBaseAtomosTimeout(3 * time.Second),
		ArgBaseAtomosAppendToHead(),
		ArgBaseAtomosWaitKilled(),
	}
	helper := createBaseAtomosHelper(BaseAtomosMailKill, ext)
	if helper.hasErrors() {
		t.Fatalf("unexpected errors: %v", helper.getError())
	}

	arg := helper.getRemoteArg()
	if arg.BaseAtomosTimeoutInNano != 3*time.Second.Nanoseconds() {
		t.Errorf("timeout = %v, want %v", arg.BaseAtomosTimeoutInNano, 3*time.Second.Nanoseconds())
	}
	if !arg.BaseAtomosAppendToHead {
		t.Error("expected AppendToHead to be true")
	}
	if !arg.BaseAtomosWaitKilled {
		t.Error("expected WaitKilled to be true")
	}
}
