package qfeel

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func readFrame(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return string(b)
}

// The docked frame is the baseline: no scan bundle at all, because a parked
// robot sends none.
func TestDecodeDockedFrame(t *testing.T) {
	tel, err := Decode(readFrame(t, "frame-docked.b64"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if tel.State != StateIdleDock {
		t.Errorf("state = %d, want %d (idle on dock)", tel.State, StateIdleDock)
	}
	if tel.Model != 74 {
		t.Errorf("model = %d, want the constant 74", tel.Model)
	}
	if !tel.HasBattery || tel.Battery != 100 {
		t.Errorf("battery = %d (present %v), want 100", tel.Battery, tel.HasBattery)
	}
	if tel.Grid != nil {
		t.Error("docked frame carried a grid, want none")
	}
	if tel.Tick == 0 {
		t.Error("tick was not decoded")
	}
}

// The driven frame carries a full 169x234 floor plan at 6 cm/cell.
func TestDecodeScanFrame(t *testing.T) {
	tel, err := Decode(readFrame(t, "frame-scan.b64"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	g := tel.Grid
	if g == nil {
		t.Fatal("no grid decoded from a frame whose field 7 carries one")
	}
	if g.Width != 169 || g.Height != 234 {
		t.Errorf("grid = %dx%d, want 169x234", g.Width, g.Height)
	}
	if len(g.Cells) != g.Width*g.Height {
		t.Errorf("cells = %d, want %d", len(g.Cells), g.Width*g.Height)
	}
	if g.LengthMismatch {
		t.Error("LengthMismatch set on an intact grid")
	}
	if g.Resolution < 0.059 || g.Resolution > 0.061 {
		t.Errorf("resolution = %v, want 0.06", g.Resolution)
	}
	if !g.HasPose {
		t.Error("pose missing")
	}
	if w, h := g.ExtentMetres(); w < 10 || h < 14 {
		t.Errorf("extent = %vx%v, want about 10.1x14.0", w, h)
	}
	// The robot was mid-run, which is the whole reason field 7 is on the wire.
	if tel.State != StateRunning && tel.State != StateDocking && tel.State != 6 {
		t.Logf("state = %d (%s)", tel.State, tel.State.Label())
	}
	if len(tel.Ranges) == 0 {
		t.Error("no range arrays decoded from the scan bundle")
	}
}

// Both scan captures are the same run a second apart, and both have to come
// back the same shape. A regression that keys off one frame's quirks shows up
// here.
func TestScanFramesAgree(t *testing.T) {
	a, err := Decode(readFrame(t, "frame-scan.b64"))
	if err != nil {
		t.Fatalf("first frame: %v", err)
	}
	b, err := Decode(readFrame(t, "frame-scan2.b64"))
	if err != nil {
		t.Fatalf("second frame: %v", err)
	}
	if a.Grid == nil || b.Grid == nil {
		t.Fatal("a scan frame lost its grid")
	}
	if a.Grid.Width != b.Grid.Width || a.Grid.Height != b.Grid.Height {
		t.Errorf("grid shapes differ: %dx%d and %dx%d", a.Grid.Width, a.Grid.Height, b.Grid.Width, b.Grid.Height)
	}
	if a.Grid.Resolution != b.Grid.Resolution {
		t.Errorf("resolution differs: %v and %v", a.Grid.Resolution, b.Grid.Resolution)
	}
	if b.Tick <= a.Tick {
		t.Errorf("second tick %d is not after first %d", b.Tick, a.Tick)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	for _, tc := range []struct{ name, payload string }{
		{"not base64", "!!!!"},
		{"empty", ""},
		{"base64 of nonsense", base64.StdEncoding.EncodeToString([]byte{0xff, 0xff, 0xff})},
	} {
		if _, err := Decode(tc.payload); err == nil {
			t.Errorf("%s: no error", tc.name)
		}
	}
}

func TestSnapshotLabelsEveryField(t *testing.T) {
	tel, err := Decode(readFrame(t, "frame-docked.b64"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	snap := tel.Snapshot()
	if len(snap.Lines) != len(tel.Raw.Fields) {
		t.Fatalf("snapshot has %d lines for %d fields", len(snap.Lines), len(tel.Raw.Fields))
	}
	seen := map[int]bool{}
	for _, l := range snap.Lines {
		seen[l.Field] = true
		if l.Name == "" || l.Value == "" {
			t.Errorf("field %d has empty name or value", l.Field)
		}
		if l.Mapped && l.Note == "" {
			t.Errorf("mapped field %d has no evidence note", l.Field)
		}
	}
	// 4 is the state field, and its line has to read as prose, not a bare int.
	stateLine := false
	for _, l := range snap.Lines {
		if l.Field == 4 {
			stateLine = l.Name == "state" && l.Mapped
		}
	}
	if !stateLine {
		t.Error("state field is not labelled")
	}
	if !seen[9] {
		t.Error("status field missing from snapshot")
	}
}

func TestPickRunIgnoresGridPadding(t *testing.T) {
	// A run of one repeated byte is grid padding (free cells read as 'K'), not
	// an identifier.
	if got := pickRun(nil); got != "" {
		t.Errorf("empty: %q", got)
	}
	if got := pickRun([]byte("KKKKKKKKKKKKKKKKKKKKKK")); got != "" {
		t.Errorf("repeated run: %q, want empty", got)
	}
	if got := pickRun([]byte("..unnamed..79F28BD6..")); got != "79F28BD6" {
		t.Errorf("id = %q, want the longest identifier-shaped run", got)
	}
}

// The command frames and the telemetry body must not be mistaken for labels.
func TestFindLabelSkipsLongArraysAndGrid(t *testing.T) {
	sum := &Message{}
	// field 7 -> 5 -> 6, the grid: long, printable, must be skipped
	grid := &Message{Fields: []Field{{Num: 6, Wire: WireBytes, Bytes: []byte(strings.Repeat("K", 400))}}}
	scan := &Message{Fields: []Field{{Num: 5, Wire: WireBytes, Bytes: []byte{1}, Msg: grid}}}
	sum.Fields = append(sum.Fields, Field{Num: 7, Wire: WireBytes, Msg: scan})
	if got := findLabel(sum); got != "" {
		t.Errorf("label = %q, want empty for a grid-only frame", got)
	}
	// a short byte field with a label in it is taken
	withLabel := &Message{Fields: []Field{{Num: 9, Wire: WireBytes, Bytes: []byte("xx.79F28BD6.xx")}}}
	if got := findLabel(withLabel); got != "79F28BD6" {
		t.Errorf("label = %q", got)
	}
}

func TestStateLabels(t *testing.T) {
	cases := map[State]string{
		StateStopped:  "stopped off dock",
		StateDocking:  "returning to dock",
		StateIdleDock: "idle on dock",
		StateRunning:  "cleaning run",
	}
	for s, want := range cases {
		if got := s.Label(); got != want {
			t.Errorf("State(%d).Label() = %q, want %q", uint32(s), got, want)
		}
	}
	for _, s := range []State{0, 6, 13, 99} {
		if got := s.Label(); got == "" || got == "unmapped" {
			t.Errorf("State(%d).Label() = %q, want an unmapped phrase", uint32(s), got)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := map[byte]CellKind{
		0:   CellUnseen,
		15:  CellBlocked,
		100: CellFringe,
		75:  CellFree,
		92:  CellStray,
		7:   CellStray,
	}
	for v, want := range cases {
		if got := Classify(v); got != want {
			t.Errorf("Classify(%d) = %v, want %v", v, got, want)
		}
	}
}

func TestGridCellAt(t *testing.T) {
	g := &Grid{Width: 2, Height: 2, Cells: []byte{1, 2, 3, 4}}
	if got := g.CellAt(1, 1); got != 4 {
		t.Errorf("CellAt(1,1) = %d, want 4", got)
	}
	if got := g.CellAt(-1, 0); got != 0 {
		t.Errorf("CellAt(-1,0) = %d, want 0", got)
	}
	if got := g.CellAt(2, 0); got != 0 {
		t.Errorf("CellAt(2,0) = %d, want 0", got)
	}
	var nilGrid *Grid
	if got := nilGrid.CellAt(0, 0); got != 0 {
		t.Errorf("nil CellAt = %d, want 0", got)
	}
}
