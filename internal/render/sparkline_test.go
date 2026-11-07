package render

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

// checkSparkShape asserts every rune is a known level and is single-width.
func checkSparkShape(t *testing.T, got string) {
	t.Helper()
	for _, r := range got {
		if !strings.ContainsRune(string(sparkLevels), r) {
			t.Fatalf("rune %q outside the level set", r)
		}
		if utf8.RuneLen(r) != 3 {
			t.Fatalf("rune %q has %d bytes, want 3", r, utf8.RuneLen(r))
		}
	}
}

func TestSparklineEmpty(t *testing.T) {
	cases := []struct {
		name   string
		values []float64
		width  int
	}{
		{"nil values", nil, 10},
		{"empty values", []float64{}, 10},
		{"zero width", []float64{1, 2}, 0},
		{"negative width", []float64{1, 2}, -4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sparkline(tc.values, tc.width); got != "" {
				t.Fatalf("got %q, want empty", got)
			}
		})
	}
}

func TestSparklineScale(t *testing.T) {
	cases := []struct {
		name   string
		values []float64
		width  int
		want   string
	}{
		{"ascending full range", []float64{0, 1, 2, 3, 4, 5, 6, 7}, 8, "▁▂▃▄▅▆▇█"},
		{"descending", []float64{8, 4, 0}, 3, "█▅▁"},
		{"min in the middle", []float64{5, 0, 10}, 3, "▅▁█"},
		{"negatives", []float64{-2, -1, 0}, 3, "▁▅█"},
		{"narrow window keeps ratio", []float64{50, 60, 70}, 3, "▁▅█"},
		{"nan is zero", []float64{math.NaN(), 5, math.NaN()}, 3, "▁█▁"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Sparkline(tc.values, tc.width)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			checkSparkShape(t, got)
			if n := utf8.RuneCountInString(got); n != len(tc.values) {
				t.Fatalf("runes = %d, want %d", n, len(tc.values))
			}
		})
	}
}

func TestSparklineFlat(t *testing.T) {
	got := Sparkline([]float64{42, 42, 42, 42}, 6)
	if n := utf8.RuneCountInString(got); n != 4 {
		t.Fatalf("runes = %d, want 4", n)
	}
	mid := string(sparkLevels[len(sparkLevels)/2])
	if got != strings.Repeat(mid, 4) {
		t.Fatalf("got %q, want four %q", got, mid)
	}
	checkSparkShape(t, got)
}

func TestSparklineTruncates(t *testing.T) {
	values := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	got := Sparkline(values, 3)
	if want := "▁▅█"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	checkSparkShape(t, got)
}
