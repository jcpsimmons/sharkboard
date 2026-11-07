// Command mapdump prints one captured frame's occupancy grid as text, at a
// fixed sample step. It is the quickest way to look at a frame without a
// terminal: `go run ./cmd/mapdump internal/qfeel/testdata/frame-scan.b64 4`.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jcpsimmons/sharkboard/internal/qfeel"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mapdump <frame.b64> [step]")
		os.Exit(2)
	}
	step := 4
	if len(os.Args) > 2 {
		if v, err := strconv.Atoi(os.Args[2]); err == nil && v > 0 {
			step = v
		}
	}
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	t, err := qfeel.Decode(string(b))
	if err != nil || t.Grid == nil {
		fmt.Fprintln(os.Stderr, "no grid in that frame")
		os.Exit(1)
	}
	g := t.Grid
	fmt.Printf("%dx%d cells, %v m/cell, pose %v %.2f,%.2f\n", g.Width, g.Height, g.Resolution, g.HasPose, g.PoseX, g.PoseY)
	for y := 0; y < g.Height; y += step * 2 {
		var sb strings.Builder
		for x := 0; x < g.Width; x += step {
			switch qfeel.Classify(g.CellAt(x, y)) {
			case qfeel.CellBlocked:
				sb.WriteByte('#')
			case qfeel.CellFringe:
				sb.WriteByte('+')
			case qfeel.CellUnseen:
				sb.WriteByte('.')
			case qfeel.CellStray:
				sb.WriteByte('?')
			default:
				sb.WriteByte(' ')
			}
		}
		fmt.Println(sb.String())
	}
}
