package raster

import (
	"slices"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	_ "github.com/CeyhunBoran/pixopher/internal/golden" // registers -update
)

func linePoints(a, b doc.Point) []doc.Point {
	var out []doc.Point
	Line(a, b, func(x, y int) { out = append(out, doc.Point{X: x, Y: y}) })
	return out
}

func sorted(pts []doc.Point) []doc.Point {
	out := slices.Clone(pts)
	slices.SortFunc(out, func(p, q doc.Point) int {
		if p.Y != q.Y {
			return p.Y - q.Y
		}
		return p.X - q.X
	})
	return out
}

// lCorners returns the pixels that have both a horizontal and a vertical
// neighbor in the set: the "L" doubles that make a 1-px line look thick.
func lCorners(pts []doc.Point) []doc.Point {
	set := map[doc.Point]bool{}
	for _, p := range pts {
		set[p] = true
	}
	var out []doc.Point
	for _, p := range pts {
		h := set[doc.Point{X: p.X - 1, Y: p.Y}] || set[doc.Point{X: p.X + 1, Y: p.Y}]
		v := set[doc.Point{X: p.X, Y: p.Y - 1}] || set[doc.Point{X: p.X, Y: p.Y + 1}]
		if h && v {
			out = append(out, p)
		}
	}
	return out
}

// TestLineProperties checks every line between points of a 9x9 grid: both
// ends are drawn, each step moves one pixel on the major axis and at most
// one on the minor axis, there are no L-corners, and swapping the ends
// gives the same pixels.
func TestLineProperties(t *testing.T) {
	var grid []doc.Point
	for y := -4; y <= 4; y++ {
		for x := -4; x <= 4; x++ {
			grid = append(grid, doc.Point{X: x, Y: y})
		}
	}
	for _, a := range grid {
		for _, b := range grid {
			pts := linePoints(a, b)
			dx, dy := abs(b.X-a.X), abs(b.Y-a.Y)
			if len(pts) != max(dx, dy)+1 {
				t.Fatalf("%v-%v: %d pixels, want %d", a, b, len(pts), max(dx, dy)+1)
			}
			if !slices.Contains(pts, a) || !slices.Contains(pts, b) {
				t.Fatalf("%v-%v: an end is missing: %v", a, b, pts)
			}
			for i := 1; i < len(pts); i++ {
				sx, sy := abs(pts[i].X-pts[i-1].X), abs(pts[i].Y-pts[i-1].Y)
				major, minor := sx, sy
				if dy > dx {
					major, minor = sy, sx
				}
				if major != 1 || minor > 1 {
					t.Fatalf("%v-%v: step %v to %v", a, b, pts[i-1], pts[i])
				}
			}
			if c := lCorners(pts); len(c) > 0 {
				t.Fatalf("%v-%v: L-corners at %v in %v", a, b, c, pts)
			}
			if back := linePoints(b, a); !slices.Equal(sorted(pts), sorted(back)) {
				t.Fatalf("%v-%v: %v, reversed %v", a, b, sorted(pts), sorted(back))
			}
		}
	}
}

func TestLineShapes(t *testing.T) {
	tests := []struct {
		a, b doc.Point
		want []doc.Point
	}{
		{doc.Point{X: 2, Y: 2}, doc.Point{X: 2, Y: 2}, []doc.Point{{X: 2, Y: 2}}},
		// A tie at the middle resolves toward the end with the smaller x,
		// whichever end comes first.
		{doc.Point{}, doc.Point{X: 4, Y: 1}, []doc.Point{{}, {X: 1}, {X: 2}, {X: 3, Y: 1}, {X: 4, Y: 1}}},
		{doc.Point{X: 4, Y: 1}, doc.Point{}, []doc.Point{{}, {X: 1}, {X: 2}, {X: 3, Y: 1}, {X: 4, Y: 1}}},
		{doc.Point{X: 0, Y: 3}, doc.Point{X: 1, Y: 0}, []doc.Point{{X: 0, Y: 3}, {X: 0, Y: 2}, {X: 1, Y: 1}, {X: 1, Y: 0}}},
	}
	for _, tt := range tests {
		if got := linePoints(tt.a, tt.b); !slices.Equal(got, tt.want) {
			t.Errorf("Line(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestFlood(t *testing.T) {
	// Two blobs that only touch diagonally, inside a ring.
	rows := []string{
		"#####",
		"#..##",
		"#..##",
		"###.#",
		"#####",
	}
	match := func(x, y int) bool { return rows[y][x] == '.' }
	four := Flood(5, 5, doc.Point{X: 1, Y: 1}, match, false)
	want4 := []doc.Point{{X: 1, Y: 1}, {X: 2, Y: 1}, {X: 1, Y: 2}, {X: 2, Y: 2}}
	if !slices.Equal(four, want4) {
		t.Errorf("4-connected: %v, want %v", four, want4)
	}
	eight := Flood(5, 5, doc.Point{X: 1, Y: 1}, match, true)
	want8 := append(slices.Clone(want4), doc.Point{X: 3, Y: 3})
	if !slices.Equal(eight, want8) {
		t.Errorf("8-connected: %v, want %v", eight, want8)
	}
}
