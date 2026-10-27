package atomos

import (
	"reflect"
	"testing"
	"time"
)

func TestBaseAtomosHelper_ValidArgs(t *testing.T) {
	tests := []struct {
		name            string
		mailType        BaseAtomosMailType
		ext             []ArgsForBaseAtomos
		wantTimeout     time.Duration
		wantAppendHead  bool
		wantWaitKilled  bool
		wantWaitChain   []string
		wantOperationID string
		wantRetryableID string
	}{
		{
			name:     "no args",
			mailType: BaseAtomosMailSync,
		},
		{
			name:            "sync arguments",
			mailType:        BaseAtomosMailSync,
			ext:             []ArgsForBaseAtomos{ArgBaseAtomosTimeout(5 * time.Second), ArgBaseAtomosAppendToHead(), argBaseAtomosWaitChain{chain: []string{"a", "b"}}, ArgBaseAtomosOperationID("operation")},
			wantTimeout:     5 * time.Second,
			wantAppendHead:  true,
			wantWaitChain:   []string{"a", "b"},
			wantOperationID: "operation",
		},
		{
			name:            "async arguments",
			mailType:        BaseAtomosMailAsync,
			ext:             []ArgsForBaseAtomos{ArgBaseAtomosAppendToHead(), ArgBaseAtomosRetryable("message"), ArgBaseAtomosOperationID("operation")},
			wantAppendHead:  true,
			wantOperationID: "operation",
			wantRetryableID: "message",
		},
		{
			name:           "kill arguments",
			mailType:       BaseAtomosMailKill,
			ext:            []ArgsForBaseAtomos{ArgBaseAtomosWaitKilled(), ArgBaseAtomosTimeout(3 * time.Second), ArgBaseAtomosAppendToHead()},
			wantTimeout:    3 * time.Second,
			wantAppendHead: true,
			wantWaitKilled: true,
		},
		{
			name:           "wormhole arguments",
			mailType:       BaseAtomosMailWormhole,
			ext:            []ArgsForBaseAtomos{ArgBaseAtomosTimeout(time.Second), ArgBaseAtomosAppendToHead()},
			wantTimeout:    time.Second,
			wantAppendHead: true,
		},
		{
			name:            "pointer arguments",
			mailType:        BaseAtomosMailSync,
			ext:             []ArgsForBaseAtomos{&argBaseAtomosTimeout{timeout: 2 * time.Second}, &argBaseAtomosWaitChain{chain: []string{"caller"}}, &argBaseAtomosOperationID{operationID: "pointer"}},
			wantTimeout:     2 * time.Second,
			wantWaitChain:   []string{"caller"},
			wantOperationID: "pointer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := createBaseAtomosHelper(tt.mailType, tt.ext)
			if helper.hasErrors() {
				t.Fatalf("unexpected errors: %v", helper.getError())
			}
			if helper.timeout != tt.wantTimeout {
				t.Errorf("timeout = %v, want %v", helper.timeout, tt.wantTimeout)
			}
			if helper.appendToHead != tt.wantAppendHead {
				t.Errorf("appendToHead = %v, want %v", helper.appendToHead, tt.wantAppendHead)
			}
			if helper.waitKilled != tt.wantWaitKilled {
				t.Errorf("waitKilled = %v, want %v", helper.waitKilled, tt.wantWaitKilled)
			}
			if !reflect.DeepEqual(helper.waitChain, tt.wantWaitChain) {
				t.Errorf("waitChain = %v, want %v", helper.waitChain, tt.wantWaitChain)
			}
			if helper.operationID != tt.wantOperationID {
				t.Errorf("operationID = %q, want %q", helper.operationID, tt.wantOperationID)
			}
			if helper.retryableID != tt.wantRetryableID {
				t.Errorf("retryableID = %q, want %q", helper.retryableID, tt.wantRetryableID)
			}
		})
	}
}

func TestBaseAtomosHelper_InvalidArgs(t *testing.T) {
	tests := []struct {
		name          string
		mailType      BaseAtomosMailType
		ext           []ArgsForBaseAtomos
		invalidCount  int
		conflictCount int
	}{
		{name: "nil arg", mailType: BaseAtomosMailSync, ext: []ArgsForBaseAtomos{nil}, invalidCount: 1},
		{name: "unsupported arg", mailType: BaseAtomosMailSync, ext: []ArgsForBaseAtomos{ArgBaseAtomosAny("value")}, invalidCount: 1},
		{name: "timeout on async mail", mailType: BaseAtomosMailAsync, ext: []ArgsForBaseAtomos{ArgBaseAtomosTimeout(time.Second)}, invalidCount: 1},
		{name: "append to head on task mail", mailType: BaseAtomosMailTask, ext: []ArgsForBaseAtomos{ArgBaseAtomosAppendToHead()}, invalidCount: 1},
		{name: "wait killed on sync mail", mailType: BaseAtomosMailSync, ext: []ArgsForBaseAtomos{ArgBaseAtomosWaitKilled()}, invalidCount: 1},
		{name: "wait chain on async mail", mailType: BaseAtomosMailAsync, ext: []ArgsForBaseAtomos{argBaseAtomosWaitChain{chain: []string{"caller"}}}, invalidCount: 1},
		{name: "retryable on sync mail", mailType: BaseAtomosMailSync, ext: []ArgsForBaseAtomos{ArgBaseAtomosRetryable("message")}, invalidCount: 1},
		{name: "empty retryable message ID", mailType: BaseAtomosMailAsync, ext: []ArgsForBaseAtomos{ArgBaseAtomosRetryable("")}, invalidCount: 1},
		{name: "duplicate timeout", mailType: BaseAtomosMailSync, ext: []ArgsForBaseAtomos{ArgBaseAtomosTimeout(time.Second), ArgBaseAtomosTimeout(2 * time.Second)}, conflictCount: 1},
		{name: "duplicate append to head", mailType: BaseAtomosMailSync, ext: []ArgsForBaseAtomos{ArgBaseAtomosAppendToHead(), ArgBaseAtomosAppendToHead()}, conflictCount: 1},
		{name: "duplicate wait killed", mailType: BaseAtomosMailKill, ext: []ArgsForBaseAtomos{ArgBaseAtomosWaitKilled(), ArgBaseAtomosWaitKilled()}, conflictCount: 1},
		{name: "duplicate wait chain", mailType: BaseAtomosMailSync, ext: []ArgsForBaseAtomos{argBaseAtomosWaitChain{chain: []string{"a"}}, argBaseAtomosWaitChain{chain: []string{"b"}}}, conflictCount: 1},
		{name: "duplicate operation ID", mailType: BaseAtomosMailSync, ext: []ArgsForBaseAtomos{ArgBaseAtomosOperationID("a"), ArgBaseAtomosOperationID("b")}, conflictCount: 1},
		{name: "duplicate retryable ID", mailType: BaseAtomosMailAsync, ext: []ArgsForBaseAtomos{ArgBaseAtomosRetryable("a"), ArgBaseAtomosRetryable("b")}, conflictCount: 1},
		{name: "invalid and conflict args", mailType: BaseAtomosMailSync, ext: []ArgsForBaseAtomos{nil, ArgBaseAtomosTimeout(time.Second), ArgBaseAtomosTimeout(2 * time.Second)}, invalidCount: 1, conflictCount: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := createBaseAtomosHelper(tt.mailType, tt.ext)
			if !helper.hasErrors() {
				t.Fatal("expected errors but got none")
			}
			if len(helper.invalidArgs) != tt.invalidCount {
				t.Errorf("invalid args = %d, want %d", len(helper.invalidArgs), tt.invalidCount)
			}
			if len(helper.conflictArgs) != tt.conflictCount {
				t.Errorf("conflict args = %d, want %d", len(helper.conflictArgs), tt.conflictCount)
			}
			if err := helper.getError(); CodeOf(err) != ErrCodeAtomosInvalidArguments {
				t.Errorf("error = %v, want ErrCodeAtomosInvalidArguments", err)
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

	decoded := createBaseAtomosHelper(BaseAtomosMailKill, createExtForCosmosArgs(arg))
	if decoded.hasErrors() {
		t.Fatalf("round trip returned errors: %v", decoded.getError())
	}
	if decoded.timeout != helper.timeout || decoded.appendToHead != helper.appendToHead || decoded.waitKilled != helper.waitKilled {
		t.Errorf("round trip = %+v, want timeout=%v appendToHead=%v waitKilled=%v", decoded, helper.timeout, helper.appendToHead, helper.waitKilled)
	}
	if !reflect.DeepEqual(decoded.fields, helper.fields) {
		t.Errorf("round trip fields = %v, want %v", decoded.fields, helper.fields)
	}
}

func TestBaseAtomosHelper_CreateExtForCosmosArgs(t *testing.T) {
	if ext := createExtForCosmosArgs(nil); len(ext) != 0 {
		t.Fatalf("nil CosmosArgs produced %d arguments, want 0", len(ext))
	}

	arg := &CosmosArgs{
		BaseAtomosTimeoutInNano: int64(2 * time.Second),
		BaseAtomosAppendToHead:  true,
		BaseAtomosWaitKilled:    true,
		WaitChain:               []string{"caller", "target"},
	}
	helper := createBaseAtomosHelper(BaseAtomosMailKill, createExtForCosmosArgs(arg))
	if helper.timeout != 2*time.Second || !helper.appendToHead || !helper.waitKilled {
		t.Errorf("decoded helper = %+v", helper)
	}
	if len(helper.invalidArgs) != 1 || helper.invalidArgs[0].reason != "WaitChain argument is only valid for Sync mails" {
		t.Errorf("WaitChain on Kill should be rejected, got %+v", helper.invalidArgs)
	}

	syncArg := &CosmosArgs{
		BaseAtomosTimeoutInNano: arg.BaseAtomosTimeoutInNano,
		BaseAtomosAppendToHead:  arg.BaseAtomosAppendToHead,
		WaitChain:               arg.WaitChain,
	}
	helper = createBaseAtomosHelper(BaseAtomosMailSync, createExtForCosmosArgs(syncArg))
	if helper.hasErrors() {
		t.Fatalf("unexpected errors: %v", helper.getError())
	}
	if !reflect.DeepEqual(helper.waitChain, syncArg.WaitChain) {
		t.Errorf("waitChain = %v, want %v", helper.waitChain, syncArg.WaitChain)
	}
}
