// Package render turns telemetry values into fixed-size text panes.
//
// Everything here is pure: input structs in, strings out, no I/O, no clock, no
// package state. Callers can re-render every frame without bookkeeping.
package render

import (
	"math"
	"strings"

	"github.com/jcpsimmons/sharkboard/internal/qfeel"
)

// Cell glyphs. Every rune is single-width so callers can count runes, not
// display columns, and a fixed pane never reflows.
const (
	glyphFree    = ' ' // free floor, the confirmed-cleaned area
	glyphFringe  = '·' // inflated costmap fringe around obstacles
	glyphBlocked = '█' // obstacle core
	glyphUnseen  = '░' // no LiDAR return yet
	glyphStray   = '?' // categorical value outside the known set
	glyphPose    = '◉' // robot position
)

// glyphFor maps a class to its rune.
func glyphFor(k qfeel.CellKind) rune {
	switch k {
	case qfeel.CellFree:
		return glyphFree
	case qfeel.CellFringe:
		return glyphFringe
	case qfeel.CellBlocked:
		return glyphBlocked
	case qfeel.CellStray:
		return glyphStray
	}
	return glyphUnseen
}

// Grid downsamples the occupancy grid into width x height text rows.
//
// Output is exactly height strings of exactly width runes each, so a caller can
// drop it straight into a fixed pane; rows that fall in the letterbox are all
// spaces. Nil is returned when there is nothing to draw: nil grid, non-positive
// width/height, or a grid whose own Width/Height are non-positive.
//
// Each output cell box-averages a source block and takes the dominant class.
// Free cells lose ties and obstacles are counted twice, so a block holding a
// wall renders as that wall: downsampling a floor plan mostly averages floor
// away, and walls that vanish with the floor make the plan unreadable. Unseen,
// fringe and stray cells are voted the same way, with blocked > fringe > stray >
// unseen as the tie order, so a coarse blocker is preferred over a fine-grained
// class that would smear.
//
// Terminal cells are about twice as tall as they are wide, so a source block is
// twice as many rows as columns; that is what keeps the plan's real proportions
// instead of squashing it. width and height are a hard budget: the block size is
// expanded to fit both, and when the budget's shape cannot hold the plan's
// Width/Height ratio the map is centred and letterboxed with spaces rather than
// stretched. Upscaling a grid smaller than the budget is allowed, since the
// result still has the right proportions.
//
// A grid with LengthMismatch is rendered anyway, using Width as the row stride.
// Cells past the end read as unseen, which is what a short payload means, so an
// empty or truncated Cells slice renders as an unseen plan rather than an error.
// The output says nothing about the mismatch, so a caller that cares can surface
// it itself.
//
// When HasPose is set the single output cell nearest the pose is overwritten
// with glyphPose, even if it was a wall. The pose is converted from metres with
// Resolution and OriginX/OriginY, then clamped into range; with no resolution
// the pose is read as cell coordinates. Walls are static while the pose is the
// only datum that moves, so the pose has to stay visible over the map rather
// than blending into it.
func Grid(g *qfeel.Grid, width, height int) []string {
	if g == nil || width <= 0 || height <= 0 || g.Width <= 0 || g.Height <= 0 {
		return nil
	}

	// Source cells per output column. The vertical figure is halved because one
	// terminal cell is two source-cells tall; taking the max of the two means
	// the block is always big enough to fit the budget in both axes.
	block := math.Max(
		float64(g.Width)/float64(width),
		float64(g.Height)/(2*float64(height)),
	)
	cols := clampInt(int(math.Ceil(float64(g.Width)/block)), 1, width)
	rows := clampInt(int(math.Ceil(float64(g.Height)/(2*block))), 1, height)

	sx := float64(g.Width) / float64(cols)
	sy := float64(g.Height) / float64(rows)
	padX := (width - cols) / 2
	padY := (height - rows) / 2

	out := make([]string, height)
	line := make([]rune, width)
	for row := 0; row < height; row++ {
		for i := range line {
			line[i] = ' '
		}
		if row >= padY && row < padY+rows {
			y0, y1 := span(row, sy, g.Height)
			for col := 0; col < cols; col++ {
				x0, x1 := span(col, sx, g.Width)
				line[padX+col] = vote(g, x0, x1, y0, y1)
			}
		}
		out[row] = string(line)
	}

	if g.HasPose {
		cx, cy := poseSourceCell(g)
		ox := clampInt(int(float64(cx)*float64(cols)/float64(g.Width)), 0, cols-1)
		oy := clampInt(int(float64(cy)*float64(rows)/float64(g.Height)), 0, rows-1)
		out[padY+oy] = putRune(out[padY+oy], padX+ox, glyphPose)
	}
	return out
}

// vote returns the dominant class in one source block. Free only wins when it
// strictly outnumbers every other class, so ties go to the blocker.
//
// An obstacle counts twice. A one-cell wall is shorter than a display cell, so
// on an unweighted vote a thin horizontal wall loses to the free floor above and
// below it while a vertical wall of the same length survives, and the plan comes
// out with gaps in one direction only. Doubling obstacles keeps both directions
// and still lets a lone speck lose to a block of real floor.
func vote(g *qfeel.Grid, x0, x1, y0, y1 int) rune {
	var n [5]int
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			n[qfeel.Classify(g.CellAt(x, y))]++
		}
	}
	best, bestN := qfeel.CellBlocked, n[qfeel.CellBlocked]*2
	for _, k := range [...]qfeel.CellKind{qfeel.CellFringe, qfeel.CellStray, qfeel.CellUnseen} {
		if n[k] > bestN {
			best, bestN = k, n[k]
		}
	}
	if n[qfeel.CellFree] > bestN {
		return glyphFree
	}
	return glyphFor(best)
}

// span is the source cell range for output index i, at most limit wide. It is
// widened to one cell so an upscaled grid still samples something.
func span(i int, size float64, limit int) (int, int) {
	lo := int(math.Floor(float64(i) * size))
	hi := int(math.Ceil(float64(i+1) * size))
	if hi <= lo {
		hi = lo + 1
	}
	if lo < 0 {
		lo = 0
	}
	if hi > limit {
		hi = limit
	}
	if hi <= lo {
		hi = lo + 1
	}
	return lo, hi
}

// poseSourceCell converts the pose to source cell coordinates and clamps it to
// the grid. A missing resolution means the pose is already in cell units.
func poseSourceCell(g *qfeel.Grid) (int, int) {
	var cx, cy float64
	if g.Resolution > 0 {
		cx = (float64(g.PoseX) - float64(g.OriginX)) / float64(g.Resolution)
		cy = (float64(g.PoseY) - float64(g.OriginY)) / float64(g.Resolution)
	} else {
		cx, cy = float64(g.PoseX), float64(g.PoseY)
	}
	return clampInt(floorInt(cx), 0, g.Width-1), clampInt(floorInt(cy), 0, g.Height-1)
}

// floorInt floors a float, mapping NaN and ±Inf to safe values.
func floorInt(v float64) int {
	switch {
	case math.IsNaN(v):
		return 0
	case v > math.MaxInt32:
		return math.MaxInt32
	case v < math.MinInt32:
		return math.MinInt32
	}
	return int(math.Floor(v))
}

// clampInt keeps v inside [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// putRune overwrites one rune, ignoring an out-of-range index.
func putRune(s string, i int, r rune) string {
	rs := []rune(s)
	if i < 0 || i >= len(rs) {
		return s
	}
	rs[i] = r
	return string(rs)
}

// Legend returns one short line naming the grid glyphs.
//
// It is one line, no newline, and lists every glyph Grid can emit so the pane
// can be read without guessing what a block means.
func Legend() string {
	parts := [...]string{
		"free " + string(glyphFree),
		"fringe " + string(glyphFringe),
		"wall " + string(glyphBlocked),
		"unseen " + string(glyphUnseen),
		"stray " + string(glyphStray),
		"robot " + string(glyphPose),
	}
	return strings.Join(parts[:], "  ")
}
