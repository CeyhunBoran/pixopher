package raster

import "github.com/CeyhunBoran/pixopher/internal/doc"

// Line calls visit for every pixel of the Bresenham line from a to b, both
// ends included. Every step advances one pixel along the major axis and at
// most one along the minor axis, so the line is 8-connected and has no
// L-corners. The endpoints are put in a fixed order first (smaller x, then
// smaller y), so Line(a, b) and Line(b, a) visit the same pixels.
func Line(a, b doc.Point, visit func(x, y int)) {
	if b.X < a.X || b.X == a.X && b.Y < a.Y {
		a, b = b, a
	}
	dx, dy := b.X-a.X, abs(b.Y-a.Y)
	sy := 1
	if b.Y < a.Y {
		sy = -1
	}
	x, y := a.X, a.Y
	if dx >= dy {
		// x-major: x always steps right; y steps when the error passes half
		// a pixel. Ties keep y, so they resolve toward a.
		e := 0
		for range dx + 1 {
			visit(x, y)
			x++
			if e += 2 * dy; e > dx {
				y += sy
				e -= 2 * dx
			}
		}
		return
	}
	e := 0
	for range dy + 1 {
		visit(x, y)
		y += sy
		if e += 2 * dx; e > dy {
			x++
			e -= 2 * dy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
