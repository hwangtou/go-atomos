package atomos

// ElementConfig.Messages schema metadata: filled at element construction
// from the registered decoders (the previously dead proto skeleton,
// atomos.proto ElementConfig.messages = 3), plus the OutDec regression for
// the clone bug the fill surfaced (OutDec cloned the IN prototype).

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestElementConfigMessages_FilledFromDecoders(t *testing.T) {
	testElementLocalAtomSpawnHelper(t, func(p *CosmosProcess) {
		elem, gerr := p.local.getLocalElement(ForTestAtomosName)
		if gerr != nil {
			t.Fatalf("getLocalElement: %v", gerr)
		}
		msgs := elem.elemImpl.Interface.Config.Messages
		if len(msgs) < 2 {
			t.Fatalf("Config.Messages not filled, len=%d", len(msgs))
		}
		cases := []struct{ name, inType, outType string }{
			{"ElementSayHello", "atomos.ForTestHelloI", "atomos.ForTestHelloO"},
			{"Greeting", "atomos.ForTestGreetingI", "atomos.ForTestGreetingO"},
		}
		for _, c := range cases {
			mc, ok := msgs[c.name]
			if !ok {
				t.Fatalf("Config.Messages missing %q; have %v", c.name, mapKeys(msgs))
			}
			if got := mc.GetIn().GetTypeUrl(); !strings.HasSuffix(got, c.inType) {
				t.Errorf("%s.In type url = %q, want suffix %q", c.name, got, c.inType)
			}
			if got := mc.GetOut().GetTypeUrl(); !strings.HasSuffix(got, c.outType) {
				t.Errorf("%s.Out type url = %q, want suffix %q", c.name, got, c.outType)
			}
		}
	})
}

func TestElementConfigMessages_ManualFillPreserved(t *testing.T) {
	iface := NewInterfaceFromDeveloper("manualElem", &testRunnableDev{})
	kept := &AtomMessageConfig{}
	iface.Config.Messages["custom"] = kept
	iface.ElementDecoders = map[string]*IOMessageDecoder{
		"ElementSayHello": forTestElementSayHelloDecoder(),
	}
	fillElementMessages(iface)
	if len(iface.Config.Messages) != 1 || iface.Config.Messages["custom"] != kept {
		t.Fatalf("fillElementMessages clobbered a manual fill: %+v", iface.Config.Messages)
	}
}

func TestElementConfigMessages_EmptyInterfaceSafe(t *testing.T) {
	fillElementMessages(nil)
	fillElementMessages(&ElementInterface{})
	fillElementMessages(&ElementInterface{Config: &ElementConfig{}})
}

// TestIOMessageDecoder_OutDecClonesOutPrototype pins the OutDec fix: it must
// produce instances of the OUT message type, not the IN type (the fill and
// any user-side decode path depends on it).
func TestIOMessageDecoder_OutDecClonesOutPrototype(t *testing.T) {
	dec := forTestElementSayHelloDecoder()
	in, err := dec.InDec(nil, true)
	if err != nil {
		t.Fatalf("InDec: %v", err)
	}
	if _, ok := in.(*ForTestHelloI); !ok {
		t.Fatalf("InDec produced %T, want *ForTestHelloI", in)
	}
	out, err := dec.OutDec(nil, true)
	if err != nil {
		t.Fatalf("OutDec: %v", err)
	}
	if _, ok := out.(*ForTestHelloO); !ok {
		t.Fatalf("OutDec produced %T, want *ForTestHelloO", out)
	}
	// Round-trip through the decoders still works.
	enc, err := proto.Marshal(&ForTestHelloO{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err = dec.OutDec(enc, true); err != nil {
		t.Fatalf("OutDec decode: %v", err)
	}
}

func mapKeys(m map[string]*AtomMessageConfig) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// forTestElementSayHelloDecoder builds the ElementSayHello decoder the way generated code
// does, without depending on the generated fixture's unexported messenger.
func forTestElementSayHelloDecoder() *IOMessageDecoder {
	m := Messenger[*ForTestAtomosElementID, *ForTestAtomosAtomID, ForTestAtomosElement, *ForTestHelloI, *ForTestHelloO]{nil, nil, "ElementSayHello"}
	return m.Decoder(&ForTestHelloI{}, &ForTestHelloO{})
}
