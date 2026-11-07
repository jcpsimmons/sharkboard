package qfeel

import (
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestParseVarintsAndNesting(t *testing.T) {
	// 1: 150, 9 { 8: 100 }, 16: 1
	b := []byte{0x08, 0x96, 0x01, 0x4a, 0x02, 0x40, 0x64, 0x80, 0x01, 0x01}
	m, err := Parse(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Get(1).Uint; got != 150 {
		t.Errorf("field 1 = %d, want 150", got)
	}
	status := m.Get(9)
	if status == nil || status.Msg == nil {
		t.Fatal("field 9 did not recurse into a message")
	}
	if got := status.Msg.Get(8).Uint; got != 100 {
		t.Errorf("field 9.8 = %d, want 100", got)
	}
}

// A field number of zero is not legal protobuf, so it should be rejected.
func TestParseRejectsFieldZero(t *testing.T) {
	if _, err := Parse([]byte{0x00, 0x01}); err == nil {
		t.Error("no error for field number 0")
	}
}

func TestParseTruncated(t *testing.T) {
	cases := map[string][]byte{
		"varint past end":   {0x08},
		"length past end":   {0x0a, 0x05, 'a', 'b'},
		"fixed32 past end":  {0x0d, 0x01, 0x02},
		"fixed64 past end":  {0x09, 0x01},
		"varint too long":   {0x08, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80},
		"unsupported group": {0x0b},
	}
	for name, b := range cases {
		if _, err := Parse(b); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestParseFixed32Float(t *testing.T) {
	// 1: 0.06 as fixed32
	f := Field{Num: 1, Wire: WireFixed32}
	raw, _ := hex.DecodeString("0d8fc2753d") // tag, then 0.06 as fixed32 little-endian
	m, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := m.Get(1)
	if got.Float < 0.059 || got.Float > 0.061 {
		t.Errorf("float = %v, want 0.06 (field %+v)", got.Float, f)
	}
}

func TestParseKeepsNonMessageBytesOpaque(t *testing.T) {
	// 6: bytes that do not parse as a message (tag 0x00 is field zero) stay
	// raw rather than failing the whole frame.
	m, err := Parse([]byte{0x32, 0x02, 0x00, 0x01})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	f := m.Get(6)
	if f == nil {
		t.Fatal("field 6 missing")
	}
	if f.Msg != nil {
		t.Error("unparseable bytes were treated as a message")
	}
	if len(f.Bytes) != 2 {
		t.Errorf("bytes = %d, want 2", len(f.Bytes))
	}
}

func TestMessageNumbersDeduplicates(t *testing.T) {
	m, err := Parse([]byte{0x08, 0x01, 0x10, 0x02, 0x08, 0x03})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	nums := m.Numbers()
	if len(nums) != 2 || nums[0] != 1 || nums[1] != 2 {
		t.Errorf("Numbers() = %v, want [1 2]", nums)
	}
	if len(m.All(1)) != 2 {
		t.Errorf("All(1) = %d fields, want 2", len(m.All(1)))
	}
}

// The three command frames are shipped as base64 text on the wire, so they have
// to round-trip to exactly the captured bytes.
func TestCommandPayloads(t *testing.T) {
	cases := map[string]string{
		CmdPause: "800108",
		CmdPlay:  "3a040a02104b80010a",
		CmdDock:  "3a040a021032800111",
	}
	for b64, wantHex := range cases {
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			t.Fatalf("%s: %v", b64, err)
		}
		if got := hex.EncodeToString(raw); got != wantHex {
			t.Errorf("%s decodes to %s, want %s", b64, got, wantHex)
		}
		m, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: parse: %v", b64, err)
		}
		if b64 != CmdPause {
			mode := m.Get(7)
			if mode == nil || mode.Msg == nil {
				t.Fatalf("%s: no field 7 mode", b64)
			}
			sel := mode.Msg.Get(1)
			if sel == nil || sel.Msg == nil || sel.Msg.Get(2) == nil {
				t.Fatalf("%s: mode selector missing", b64)
			}
		}
		if m.Get(16) == nil {
			t.Errorf("%s: no opcode in field 16", b64)
		}
	}
}

func TestActionLookup(t *testing.T) {
	if len(Actions()) != 5 {
		t.Errorf("Actions() = %d, want 5", len(Actions()))
	}
	a, ok := ActionByKey("dock")
	if !ok {
		t.Fatal("dock not found")
	}
	if a.Payload != CmdDock || !a.Risky {
		t.Errorf("dock action = %+v", a)
	}
	if a, _ := ActionByKey("heartbeat-a"); a.Risky {
		t.Error("heartbeat marked risky")
	}
	if _, ok := ActionByKey("nope"); ok {
		t.Error("unknown action found")
	}
}
