package render

import (
	"math"
	"strings"
)

// sparkLevels are the eight block runes, lowest first. All are single-width, so
// a row keeps a stable rune count.
var sparkLevels = []rune("▁▂▃▄▅▆▇█")

// Sparkline draws a left-to-right sparkline of values scaled to their own range.
//
// Scaling is per window, not absolute: the caller's values carry units this
// pane does not know about, so only the shape inside the window can be shown.
// The window is the last width values, so a scrolling buffer can be passed
// whole. A flat window (max == min) has no shape to show and renders at the
// middle level rather than collapsing to the bottom edge. NaN has no position,
// so it is drawn as 0 and never poisons the range scan.
//
// The result is one rune per value in the window, at most width of them, and ""
// for width <= 0 or no values.
func Sparkline(values []float64, width int) string {
	if width <= 0 || len(values) == 0 {
		return ""
	}
	window := values
	if len(window) > width {
		window = window[len(window)-width:]
	}

	// First pass finds the range; NaN is folded to 0 in both passes so it can
	// neither become the min/max nor produce a NaN scale factor.
	lo, hi := 0.0, 0.0
	for i, v := range window {
		if math.IsNaN(v) {
			v = 0
		}
		if i == 0 || v < lo {
			lo = v
		}
		if i == 0 || v > hi {
			hi = v
		}
	}

	var b strings.Builder
	b.Grow(len(window) * 3)
	if hi == lo {
		mid := sparkLevels[len(sparkLevels)/2]
		for range window {
			b.WriteRune(mid)
		}
		return b.String()
	}

	scale := float64(len(sparkLevels)-1) / (hi - lo)
	for _, v := range window {
		if math.IsNaN(v) {
			v = 0
		}
		idx := clampInt(int(math.Round((v-lo)*scale)), 0, len(sparkLevels)-1)
		b.WriteRune(sparkLevels[idx])
	}
	return b.String()
}
