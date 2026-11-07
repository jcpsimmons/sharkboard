package render

import (
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jcpsimmons/sharkboard/internal/qfeel"
)

// uniform builds a grid filled with one raw cell value.
func uniform(w, h int, v byte) *qfeel.Grid {
	cells := make([]byte, w*h)
	for i := range cells {
		cells[i] = v
	}
	return &qfeel.Grid{Width: w, Height: h, Cells: cells}
}

// checkShape asserts the pane contract: height rows, each exactly width runes.
func checkShape(t *testing.T, rows []string, width, height int) {
	t.Helper()
	if len(rows) != height {
		t.Fatalf("rows = %d, want %d", len(rows), height)
	}
	for i, row := range rows {
		if n := utf8.RuneCountInString(row); n != width {
			t.Fatalf("row %d has %d runes, want %d: %q", i, n, width, row)
		}
	}
}

// bbox is the bounding box of non-space runes; -1 when the pane is blank.
// Aspect tests use walls because padding and free floor are both spaces.
func bbox(rows []string) (top, bottom, left, right int) {
	top, left = math.MaxInt, math.MaxInt
	bottom, right = -1, -1
	for y, row := range rows {
		for x, r := range []rune(row) {
			if r == ' ' {
				continue
			}
			top = min(top, y)
			bottom = max(bottom, y)
			left = min(left, x)
			right = max(right, x)
		}
	}
	return top, bottom, left, right
}

func TestGridShape(t *testing.T) {
	grid := uniform(40, 20, qfeelFreeValue)
	cases := []struct {
		name string
		grid *qfeel.Grid
		w, h int
	}{
		{"fits budget", grid, 30, 10},
		{"square budget", grid, 20, 20},
		{"upscale small grid", uniform(3, 2, 75), 30, 10},
		{"one cell", uniform(1, 1, 15), 1, 1},
		{"tall budget", grid, 5, 40},
		{"wide budget", grid, 60, 3},
		{"empty cells", &qfeel.Grid{Width: 10, Height: 5}, 6, 4},
		{"stray only", uniform(4, 4, 92), 8, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := Grid(tc.grid, tc.w, tc.h)
			checkShape(t, rows, tc.w, tc.h)
		})
	}
}

func TestGridDegenerate(t *testing.T) {
	cases := []struct {
		name string
		grid *qfeel.Grid
		w, h int
	}{
		{"nil grid", nil, 10, 5},
		{"zero width", uniform(4, 4, 75), 0, 5},
		{"zero height", uniform(4, 4, 75), 10, 0},
		{"negative width", uniform(4, 4, 75), -3, 5},
		{"negative height", uniform(4, 4, 75), 10, -3},
		{"grid zero width", &qfeel.Grid{Height: 5, Cells: []byte{1, 2, 3}}, 10, 5},
		{"grid zero height", &qfeel.Grid{Width: 5, Cells: []byte{1, 2, 3}}, 10, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rows := Grid(tc.grid, tc.w, tc.h); rows != nil {
				t.Fatalf("got %d rows, want nil", len(rows))
			}
		})
	}
}

// TestGridGlyphs pins one rune per class and proves free loses ties, so a wall
// in a shrunk block is never averaged away.
func TestGridGlyphs(t *testing.T) {
	cases := []struct {
		name  string
		cells []byte
		want  rune
	}{
		{"free", []byte{75}, ' '},
		{"blocked", []byte{15}, '█'},
		{"fringe", []byte{100}, '·'},
		{"unseen", []byte{0}, '░'},
		{"stray", []byte{92}, '?'},
		{"three free cells beat one wall", []byte{75, 75, 75, 15}, ' '},
		{"tie goes to wall", []byte{75, 15}, '█'},
		{"two free cells lose to one wall", []byte{75, 75, 15}, '█'},
		{"tie goes to unseen over free", []byte{75, 0}, '░'},
		{"fringe beats unseen", []byte{0, 100}, '·'},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &qfeel.Grid{Width: len(tc.cells), Height: 1, Cells: tc.cells}
			rows := Grid(g, 1, 1)
			got := []rune(rows[0])
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("got %q, want %q", rows[0], string(tc.want))
			}
		})
	}
}

// TestGridRunesSingleWidth checks every emitted rune against the glyph set, so
// no double-width rune can shift a fixed pane.
func TestGridRunesSingleWidth(t *testing.T) {
	allowed := map[rune]bool{' ': true, '·': true, '█': true, '░': true, '?': true, '◉': true}
	g := uniform(50, 50, 75)
	for i := 0; i < 37; i++ {
		g.Cells[i*3] = byte([]int{0, 15, 92, 100}[i%4])
	}
	pose := g
	pose.HasPose, pose.PoseX, pose.PoseY, pose.Resolution = true, 1.2, 1.1, 0.05

	for _, gg := range []*qfeel.Grid{g, pose} {
		for _, row := range Grid(gg, 24, 12) {
			for _, r := range row {
				if !allowed[r] {
					t.Fatalf("rune %q not in glyph set", r)
				}
				if utf8.RuneLen(r) > 3 {
					t.Fatalf("rune %q unexpectedly wide", r)
				}
			}
		}
	}
}

// TestGridAspect checks the drawn area keeps the plan's proportions, filling the
// budget when its shape allows and letterboxing instead of stretching when it
// does not.
func TestGridAspect(t *testing.T) {
	cases := []struct {
		name       string
		gw, gh     int
		w, h       int
		wantTop    int
		wantBottom int
		wantLeft   int
		wantRight  int
	}{
		{"wide plan in matching budget", 200, 100, 80, 20, 0, 19, 0, 79},
		{"square plan letterboxed vertically", 100, 100, 20, 40, 15, 24, 0, 19},
		{"tall plan letterboxed horizontally", 60, 240, 40, 20, 0, 19, 15, 24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := Grid(uniform(tc.gw, tc.gh, 15), tc.w, tc.h)
			checkShape(t, rows, tc.w, tc.h)
			top, bottom, left, right := bbox(rows)
			if top != tc.wantTop || bottom != tc.wantBottom || left != tc.wantLeft || right != tc.wantRight {
				t.Fatalf("bbox = (%d,%d)-(%d,%d), want (%d,%d)-(%d,%d)",
					top, left, bottom, right, tc.wantTop, tc.wantLeft, tc.wantBottom, tc.wantRight)
			}
		})
	}
}

// TestGridAspectRatio measures physical proportions: one terminal cell is about
// two cells tall, so drawn width/(2*drawn height) should track gw/gh.
func TestGridAspectRatio(t *testing.T) {
	cases := []struct{ gw, gh, w, h int }{
		{200, 100, 80, 20},
		{100, 100, 20, 40},
		{300, 50, 40, 20},
		{90, 120, 30, 40},
		{120, 40, 60, 10},
	}
	for _, tc := range cases {
		rows := Grid(uniform(tc.gw, tc.gh, 15), tc.w, tc.h)
		top, bottom, left, right := bbox(rows)
		drawnW := right - left + 1
		drawnH := bottom - top + 1
		got := float64(drawnW) / (2 * float64(drawnH))
		want := float64(tc.gw) / float64(tc.gh)
		// Integer cell counts make the ratio approximate; 25% is well inside
		// "not visibly distorted" and far outside a squashed render.
		if math.Abs(got-want)/want > 0.25 {
			t.Fatalf("%dx%d in %dx%d: ratio %.3f, want %.3f", tc.gw, tc.gh, tc.w, tc.h, got, want)
		}
	}
}

func TestGridPose(t *testing.T) {
	t.Run("clamped bottom right", func(t *testing.T) {
		// Clamping is into the grid, so the pose lands in the drawn area's
		// bottom right corner, not the pane's (letterbox rows are blank).
		g := uniform(20, 20, 15)
		g.HasPose, g.PoseX, g.PoseY, g.Resolution = true, 999, 999, 0.05
		rows := Grid(g, 10, 10)
		checkShape(t, rows, 10, 10)
		if n := countRune(rows, '◉'); n != 1 {
			t.Fatalf("pose count = %d, want 1", n)
		}
		_, bottom, _, right := bbox(rows)
		if got := []rune(rows[bottom])[right]; got != '◉' {
			t.Fatalf("pose at (%d,%d) = %q, want the corner", bottom, right, got)
		}
	})

	t.Run("clamped top left", func(t *testing.T) {
		g := uniform(20, 20, 15)
		g.HasPose, g.PoseX, g.PoseY, g.Resolution = true, -50, -50, 0.05
		rows := Grid(g, 10, 10)
		top, _, left, _ := bbox(rows)
		if got := []rune(rows[top])[left]; got != '◉' {
			t.Fatalf("pose at (%d,%d) = %q, want the corner", top, left, got)
		}
	})

	t.Run("overwrites a wall", func(t *testing.T) {
		g := uniform(20, 20, 15)
		g.HasPose, g.PoseX, g.PoseY, g.Resolution = true, 0.5, 0.5, 0.05
		rows := Grid(g, 10, 10)
		if n := countRune(rows, '◉'); n != 1 {
			t.Fatalf("pose count = %d, want 1", n)
		}
	})

	t.Run("origin offsets the pose", func(t *testing.T) {
		g := uniform(20, 20, 75)
		g.HasPose, g.OriginX, g.OriginY, g.PoseX, g.PoseY, g.Resolution = true, 1.0, 1.0, 1.0, 1.0, 0.05
		rows := Grid(g, 10, 10)
		if got := []rune(rows[2])[0]; got != '◉' {
			t.Fatalf("pose at letterboxed origin = %q, want pose", got)
		}
	})

	t.Run("no resolution reads cell coords", func(t *testing.T) {
		// 10x10 source in a 10x10 budget letterboxes to 10x5 drawn rows, so one
		// output row covers two source rows: source row 6 is drawn row 3.
		g := uniform(10, 10, 15)
		g.HasPose, g.PoseX, g.PoseY = true, 8, 6
		rows := Grid(g, 10, 10)
		top, _, left, _ := bbox(rows)
		if got := []rune(rows[top+3])[left+8]; got != '◉' {
			t.Fatalf("pose = %q, want pose at source row 6", got)
		}
	})

	t.Run("absent pose draws none", func(t *testing.T) {
		rows := Grid(uniform(10, 10, 75), 10, 10)
		if n := countRune(rows, '◉'); n != 0 {
			t.Fatalf("pose count = %d, want 0", n)
		}
	})
}

// A frame can declare a height the buffer cannot back. Before the sampling
// loops were capped, one such frame froze the renderer.
func TestGridAbsurdHeight(t *testing.T) {
	g := &qfeel.Grid{Width: 3, Height: 1 << 40, Cells: []byte{15, 75, 15}, LengthMismatch: true}
	done := make(chan []string, 1)
	go func() { done <- Grid(g, 40, 20) }()
	select {
	case rows := <-done:
		checkShape(t, rows, 40, 20)
		if n := countRune(rows, '█'); n == 0 {
			t.Error("gutter cells were not drawn")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("render did not finish")
	}
}

func TestGridLengthMismatch(t *testing.T) {
	t.Run("short payload", func(t *testing.T) {
		// A 10x40 map in a 10x20 budget is unletterboxed and samples two source
		// rows per output row: rows 0..2 still hold the 55 cells, rows 3..19 are
		// past the end of Cells and read as unseen.
		g := &qfeel.Grid{Width: 10, Height: 40, Cells: repeat(55, 15), LengthMismatch: true}
		rows := Grid(g, 10, 20)
		checkShape(t, rows, 10, 20)
		for _, y := range []int{0, 2} {
			if rows[y] != strings.Repeat("█", 10) {
				t.Fatalf("lost the populated rows at %d: %q", y, rows[y])
			}
		}
		for _, y := range []int{3, 19} {
			if rows[y] != strings.Repeat("░", 10) {
				t.Fatalf("missing tail at row %d = %q, want all unseen", y, rows[y])
			}
		}
	})

	t.Run("long payload", func(t *testing.T) {
		g := &qfeel.Grid{Width: 4, Height: 2, Cells: repeat(40, 15), LengthMismatch: true}
		rows := Grid(g, 4, 2)
		checkShape(t, rows, 4, 2)
		if rows[0] != "████" {
			t.Fatalf("row 0 = %q, want walls", rows[0])
		}
	})
}

func TestLegend(t *testing.T) {
	line := Legend()
	if line == "" {
		t.Fatal("empty legend")
	}
	if strings.Contains(line, "\n") {
		t.Fatalf("legend is not one line: %q", line)
	}
	for _, g := range []rune{' ', '·', '█', '░', '?', '◉'} {
		if !strings.ContainsRune(line, g) {
			t.Fatalf("legend %q misses glyph %q", line, g)
		}
	}
}

const qfeelFreeValue = 75

func countRune(rows []string, want rune) int {
	n := 0
	for _, row := range rows {
		n += strings.Count(row, string(want))
	}
	return n
}

func repeat(n int, v byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = v
	}
	return b
}
