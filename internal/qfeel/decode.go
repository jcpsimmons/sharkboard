package qfeel

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Topic names, as observed on the wire. The topic set is fixed, there is no
// device identifier in any frame, and two robots on one network answer to the
// same topics.
const (
	TopicOutput = "/qfeel/PbOutput"
	TopicInput  = "/qfeel/PbInput"
	TopicWiFi   = "/viper/wifiIn"
)

// State is field 4 of a telemetry frame: what the robot says it is doing.
type State uint32

// Activity states, all confirmed by watching the robot move or stop.
const (
	StateStopped  State = 4  // stopped, off the dock, not cleaning
	StateDocking  State = 7  // returning to dock / docking in progress
	StateIdleDock State = 14 // idle, sitting on the dock
	StateRunning  State = 24 // active cleaning run
)

// Label returns a human phrase for the state.
func (s State) Label() string {
	switch s {
	case StateStopped:
		return "stopped off dock"
	case StateDocking:
		return "returning to dock"
	case StateIdleDock:
		return "idle on dock"
	case StateRunning:
		return "cleaning run"
	case 0:
		return "unknown (0, not in the November 4 enum)"
	case 6:
		return "unmapped (6, seen once leaving the dock on Nov 7)"
	case 13:
		return "unmapped (13, seen for ~1s during a Nov 7 dock)"
	}
	return fmt.Sprintf("unmapped (%d)", uint32(s))
}

// ShortLabel is the compact form used where a line has to stay one row.
func (s State) ShortLabel() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateDocking:
		return "returning"
	case StateIdleDock:
		return "idle/dock"
	case StateRunning:
		return "running"
	}
	return fmt.Sprintf("unmapped %d", uint32(s))
}

// Grid is the floor plan the robot pushes while it is moving.
type Grid struct {
	Width      int
	Height     int
	Resolution float32 // metres per cell
	OriginX    float32
	OriginY    float32
	PoseX      float32
	PoseY      float32
	HasPose    bool
	Cells      []byte
	// LengthMismatch is set when len(Cells) != Width*Height. A mismatched grid
	// renders as diagonal shear, which is the classic bad offset bug.
	LengthMismatch bool
}

// Telemetry is one decoded /qfeel/PbOutput frame.
type Telemetry struct {
	At    time.Time
	Tick  uint64
	Model uint32
	State State

	HasBattery  bool
	Battery     uint32 // percent
	HasCharging bool
	Charging    uint32 // 3 observed while docked, never confirmed as "charging"

	Progress uint32 // climbed 48 -> 51 in a run; probably run progress

	EchoedCmd string // field 16, echoes the last command the robot accepted

	Scan   *Message // field 7 container, only while moving
	Grid   *Grid    // field 7 -> 5
	Ranges [][]byte // field 7 -> 8/10, paired LiDAR byte arrays
	MapID  string   // ASCII map identifier if the frame carries one

	Raw *Message // every field, including unmapped ones
}

// Decode parses one base64 protobuf frame from /qfeel/PbOutput.
func Decode(b64 string) (*Telemetry, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("qfeel: payload is not base64 text: %w", err)
	}
	if len(raw) == 0 {
		return nil, ErrNoFrame
	}
	m, err := Parse(raw)
	if err != nil && len(m.Fields) == 0 {
		return nil, err
	}
	t := &Telemetry{At: time.Now(), Raw: m}
	t.Tick = m.Get(1).UintValue()
	t.Model = uint32(m.Get(2).UintValue())
	t.State = State(m.Get(4).UintValue())
	t.Progress = uint32(m.Get(13).UintValue())
	if st := m.Get(9); st != nil && st.Msg != nil {
		if b := st.Msg.Get(8); b != nil {
			t.Battery, t.HasBattery = uint32(b.Uint), true
		}
		if c := st.Msg.Get(1); c != nil {
			t.Charging, t.HasCharging = uint32(c.Uint), true
		}
	}
	if ex := m.Get(16); ex != nil && ex.Msg != nil {
		t.EchoedCmd = fmt.Sprintf("16 {%s}", ex.Msg.compact())
	}
	if scan := m.Get(7); scan != nil && scan.Msg != nil {
		t.Scan = scan.Msg
		if g := scan.Msg.Get(5); g != nil && g.Msg != nil {
			t.Grid = parseGrid(g.Msg)
		}
		for _, n := range []int{8, 10} {
			for _, f := range scan.Msg.All(n) {
				if len(f.Bytes) > 8 {
					t.Ranges = append(t.Ranges, f.Bytes)
				}
			}
		}
	}
	if id := findLabel(m); id != "" {
		t.MapID = id
	}
	return t, nil
}

// UintValue is a nil-safe accessor for varint fields.
func (f *Field) UintValue() uint64 {
	if f == nil {
		return 0
	}
	return f.Uint
}

func parseGrid(m *Message) *Grid {
	g := &Grid{}
	if f := m.Get(1); f != nil && f.Wire == WireFixed32 {
		g.Resolution = f.Float
	}
	if o := m.Get(2); o != nil && o.Msg != nil {
		g.OriginX = float32f(o.Msg.Get(1))
		g.OriginY = float32f(o.Msg.Get(2))
	}
	if c := m.Get(6); c != nil {
		g.Cells = c.Bytes
	}
	if p := m.Get(9); p != nil && p.Msg != nil && p.Msg.Get(1) != nil && p.Msg.Get(2) != nil {
		g.PoseX, g.PoseY, g.HasPose = float32f(p.Msg.Get(1)), float32f(p.Msg.Get(2)), true
	}
	if len(g.Cells) == 0 {
		return nil
	}

	// Width and height are raw varints, so a hostile or corrupt frame can
	// declare 2^40 cells. Nothing may loop on the declaration alone: clamp both
	// dimensions to the buffer that actually arrived, then let the row stride
	// come from Width and the rows from what is left. Without this, one frame
	// freezes the renderer for 10^13 iterations.
	w := m.Get(3).UintValue()
	h := m.Get(4).UintValue()
	n := uint64(len(g.Cells))
	if w == 0 || w > n {
		w = n
	}
	if h == 0 || h > n {
		h = n
	}
	g.Width, g.Height = int(w), int(h)
	if g.Width*int(g.Height) != len(g.Cells) {
		g.LengthMismatch = true
		g.Height = len(g.Cells) / g.Width
		if g.Height == 0 {
			g.Height = 1
		}
	}
	return g
}

func float32f(f *Field) float32 {
	if f == nil || f.Wire != WireFixed32 {
		return 0
	}
	return f.Float
}

// ExtentMetres reports the covered floor area, assuming Resolution is metres
// per cell. Returns 0,0 when the frame carries no usable resolution.
func (g *Grid) ExtentMetres() (float64, float64) {
	if g == nil || g.Resolution <= 0 {
		return 0, 0
	}
	return float64(g.Width) * float64(g.Resolution), float64(g.Height) * float64(g.Resolution)
}

// CellAt reads a cell, returning 0 for out-of-range coordinates.
func (g *Grid) CellAt(x, y int) byte {
	if g == nil || x < 0 || y < 0 || x >= g.Width || y >= g.Height {
		return 0
	}
	i := y*g.Width + x
	if i >= len(g.Cells) {
		return 0
	}
	return g.Cells[i]
}

// CellKind names a raw cell value. The values are categorical, not
// probabilistic: the robot paints free floor, obstacle cores, and an inflated
// costmap fringe around them.
type CellKind int

// Cell classes, in the order the maps legend lists them.
const (
	CellUnseen  CellKind = iota // 0
	CellBlocked                 // 15
	CellFringe                  // 100
	CellFree                    // 75
	CellStray                   // anything else (92 seen once)
)

// Classify buckets a raw cell value.
func Classify(v byte) CellKind {
	switch v {
	case 0:
		return CellUnseen
	case 15:
		return CellBlocked
	case 100:
		return CellFringe
	case 75:
		return CellFree
	}
	return CellStray
}

// compact renders nested fields for display: bounded width, bounded depth, and
// one builder pass so a large frame stays cheap.
func (m *Message) compact() string { return strings.TrimSpace(m.compactDepth(0)) }

const (
	compactMaxDepth = 4
	compactMaxLen   = 360
)

func (m *Message) compactDepth(depth int) string {
	if m == nil {
		return ""
	}
	var b strings.Builder
	for i, f := range m.Fields {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%d:", f.Num)
		switch {
		case f.Msg != nil && depth < compactMaxDepth:
			b.WriteByte('{')
			b.WriteString(f.Msg.compactDepth(depth + 1))
			b.WriteByte('}')
		case f.Msg != nil:
			fmt.Fprintf(&b, "{%d fields}", len(f.Msg.Fields))
		case f.Wire == WireBytes:
			fmt.Fprintf(&b, "%q", truncate(f.Bytes, 16))
		default:
			fmt.Fprintf(&b, "%d", f.Uint)
		}
		if b.Len() >= compactMaxLen {
			b.WriteString(" …")
			break
		}
	}
	return b.String()
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// findLabel looks for the map label and map id, which ride in a short byte
// field of the big map record ("unnamed", "79F28BD6").
//
// The grid bytes themselves are mostly printable - free floor is 75, the ASCII
// letter K - so only short byte fields are considered at all, and inside them
// only identifier-shaped runs that are not one repeated character.
func findLabel(m *Message) string {
	best := ""
	var walk func(*Message)
	walk = func(mm *Message) {
		for _, f := range mm.Fields {
			if f.Msg != nil {
				walk(f.Msg)
				continue
			}
			if f.Wire != WireBytes || len(f.Bytes) < 4 || len(f.Bytes) > 32 {
				continue
			}
			if s := pickRun(f.Bytes); len(s) > len(best) {
				best = s
			}
		}
	}
	walk(m)
	return best
}

// pickRun returns the longest identifier-shaped run in b, or "".
func pickRun(b []byte) string {
	var best, cur []byte
	flush := func() {
		if len(cur) >= 4 && !allSame(cur) && hasLetterOrDigit(cur) && len(cur) > len(best) {
			best = append(best[:0], cur...)
		}
		cur = cur[:0]
	}
	for _, c := range b {
		if isIdent(c) {
			cur = append(cur, c)
			continue
		}
		flush()
	}
	flush()
	return string(best)
}

func hasLetterOrDigit(b []byte) bool {
	for _, c := range b {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return true
		}
	}
	return false
}

// allSame reports a run of one repeated byte, which is grid padding rather than
// an identifier.
func allSame(b []byte) bool {
	for _, c := range b[1:] {
		if c != b[0] {
			return false
		}
	}
	return true
}

func isIdent(c byte) bool {
	switch {
	case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		return true
	case c == '_' || c == '-':
		return true
	}
	return false
}

// ErrNoFrame reports a payload that carried no telemetry at all.
var ErrNoFrame = errors.New("qfeel: empty frame")

// Snapshot describes a frame in labelled lines, one per field present.
type Snapshot struct {
	Lines []SnapshotLine
}

// SnapshotLine is one labelled field: the raw number, the inferred name, the
// value, and the evidence for the name.
type SnapshotLine struct {
	Field  int
	Name   string
	Value  string
	Note   string
	Mapped bool
}

// Snapshot labels every field in a frame. Unmapped fields keep their number as
// the name so nothing is silently dropped.
func (t *Telemetry) Snapshot() Snapshot {
	var s Snapshot
	for _, f := range t.Raw.Fields {
		line := SnapshotLine{Field: f.Num, Value: valueString(f)}
		if l, ok := labels[f.Num]; ok {
			line.Name, line.Note, line.Mapped = l.name, l.note, true
		} else {
			line.Name = fmt.Sprintf("x%d", f.Num)
		}
		switch {
		case f.Num == 7 && t.Grid != nil && t.Grid.LengthMismatch:
			line.Value = fmt.Sprintf("scan bundle, grid %dx%d but %d cells", t.Grid.Width, t.Grid.Height, len(t.Grid.Cells))
		case f.Num == 7 && t.Grid != nil:
			w, h := t.Grid.Width, t.Grid.Height
			line.Value = fmt.Sprintf("scan bundle, grid %dx%d (%d cells)", w, h, len(t.Grid.Cells))
		case f.Num == 9:
			batt, flag := "n/a", "n/a"
			if t.HasBattery {
				batt = fmt.Sprintf("%d%%", t.Battery)
			}
			if t.HasCharging {
				flag = fmt.Sprintf("%d", t.Charging)
			}
			line.Value = "battery " + batt + ", charging flag " + flag
		case f.Num == 4:
			line.Value = fmt.Sprintf("%d (%s)", uint32(t.State), t.State.ShortLabel())
		}
		s.Lines = append(s.Lines, line)
	}
	return s
}

// ChargingEcho returns the observed charging flag. Check HasCharging first;
// absent flags are not 0.
func (t *Telemetry) ChargingEcho() uint32 { return t.Charging }

func valueString(f Field) string {
	switch {
	case f.Msg != nil:
		return "{" + f.Msg.compact() + "}"
	case f.Wire == WireBytes:
		if len(f.Bytes) > 24 {
			return fmt.Sprintf("bytes[%d]", len(f.Bytes))
		}
		return fmt.Sprintf("%q", f.Bytes)
	case f.Wire == WireFixed32:
		return fmt.Sprintf("%d (fixed32)", f.Uint)
	}
	return fmt.Sprintf("%d", f.Uint)
}

type label struct{ name, note string }

// labels is the schema, with the evidence attached. Anything absent from this
// table is genuinely unmapped.
var labels = map[int]label{
	1:  {"tick_ms", "ms counter, steps by exactly 201 per frame (~5 Hz)"},
	2:  {"model", "constant 74; model or firmware id, NOT the state"},
	4:  {"state", "14 idle-on-dock / 24 running / 7 docking / 4 stopped off dock"},
	7:  {"scan", "container, only while the robot moves; 20-170 KB frames"},
	9:  {"status", "battery and charge flags"},
	10: {"x10", "constant 1"},
	13: {"progress", "climbed 48 -> 51 across one run; probable run progress"},
	16: {"extra", "also echoes the last accepted command"},
	18: {"x18", "seen as {5: 5} during a driven run"},
	35: {"x35", "constant 2"},
	36: {"x36", "constant 2"},
	38: {"x38", "only present while active"},
	40: {"x40", "only present while active"},
	41: {"x41", "only present while active"},
	45: {"x45", "0 docked, 2 while active"},
}
