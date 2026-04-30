package atomos

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestUtilProtoMessageIterate(t *testing.T) {
	lm := &LogMail{
		Id: &IDInfo{
			Type:    IDType_Atom,
			Cosmos:  "cosmos",
			Node:    "node",
			Element: "element",
			Atom:    "atom",
			Version: 10,
		},
		Time:    nil,
		Level:   LogLevel_Info,
		Message: "test message",
	}
	m := UtilProtoMessageIterate(lm)
	if len(m) != 8 {
		t.Fatalf("UtilProtoMessageIterate: invalid length. length=(%d)", len(m))
	}
	left, _ := json.Marshal(m)
	right, _ := json.Marshal(map[string]any{
		"id.type":    IDType_Atom,
		"id.cosmos":  "cosmos",
		"id.node":    "node",
		"id.element": "element",
		"id.atom":    "atom",
		"id.version": 10,
		"level":      LogLevel_Info,
		"message":    "test message",
	})
	if bytes.Compare(left, right) != 0 {
		t.Logf("\n%s\n%s", string(left), string(right))
		t.Fatalf("UtilProtoMessageIterate: invalid map. map=(%v)", m)
	}
	t.Logf("UtilProtoMessageIterate: success")
}
