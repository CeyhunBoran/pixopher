package raster

import "github.com/CeyhunBoran/pixopher/internal/doc"

// Flood returns the region of a w x h grid that is connected to start and
// matches: 4-connected, or 8-connected when diagonal is set. The result is in
// row-major order. start must be on the grid and match.
func Flood(w, h int, start doc.Point, match func(x, y int) bool, diagonal bool) []doc.Point {
	seen := make([]bool, w*h)
	steps := []doc.Point{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}}
	if diagonal {
		steps = append(steps, doc.Point{X: 1, Y: 1}, doc.Point{X: 1, Y: -1}, doc.Point{X: -1, Y: 1}, doc.Point{X: -1, Y: -1})
	}
	seen[start.Y*w+start.X] = true
	stack := []doc.Point{start}
	n := 1
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, s := range steps {
			x, y := p.X+s.X, p.Y+s.Y
			if x < 0 || y < 0 || x >= w || y >= h || seen[y*w+x] || !match(x, y) {
				continue
			}
			seen[y*w+x] = true
			n++
			stack = append(stack, doc.Point{X: x, Y: y})
		}
	}
	out := make([]doc.Point, 0, n)
	for i, ok := range seen {
		if ok {
			out = append(out, doc.Point{X: i % w, Y: i / w})
		}
	}
	return out
}
