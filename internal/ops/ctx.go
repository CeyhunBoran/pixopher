package ops

import (
	"reflect"
	"slices"

	"github.com/CeyhunBoran/pixopher/internal/doc"
)

// Ctx is what an op works on: the document and the target cel. It records
// every pixel write so the engine can log the change and undo it later. One
// Ctx covers one history step, which may hold several calls.
type Ctx struct {
	Doc          *doc.Doc
	Layer, Frame int // target cel

	header *doc.Doc // header snapshot taken by NewCtx
	pixels []doc.PixelDelta
	index  map[pixKey]int // position of a pixel's delta in pixels
}

type pixKey struct{ layer, frame, x, y int }

// Target returns the cel that commands act on when no --layer or --frame
// is given: drawing ops write it and show, get and stats read it (D55).
// It is the first layer and frame until the active layer and frame land
// (M4).
func Target(*doc.Doc) (layer, frame int) { return 0, 0 }

// NewCtx starts a step on d, targeting the cel (layer, frame).
func NewCtx(d *doc.Doc, layer, frame int) *Ctx {
	return &Ctx{Doc: d, Layer: layer, Frame: frame, header: d.Header(), index: map[pixKey]int{}}
}

// In reports whether (x, y) is on the canvas.
func (c *Ctx) In(x, y int) bool { return x >= 0 && y >= 0 && x < c.Doc.W && y < c.Doc.H }

// Pixel returns the index at (x, y) of the target cel, or transparent off the
// canvas.
func (c *Ctx) Pixel(x, y int) doc.Index {
	if !c.In(x, y) {
		return doc.Transparent
	}
	return c.Doc.Pixel(c.Layer, c.Frame, x, y)
}

// Set writes v at (x, y) of the target cel and records the change. Points
// off the canvas are clipped: Set ignores them and returns false.
func (c *Ctx) Set(x, y int, v doc.Index) bool {
	if !c.In(x, y) {
		return false
	}
	c.SetAt(c.Layer, c.Frame, x, y, v)
	return true
}

// SetAt writes v at (x, y) of the cel (layer, frame) and records the change.
// It is for ops that act on every cel, such as palette rm. The point must be
// on the canvas, and the cel must not be linked: a write through a link
// would be recorded on the wrong cel.
func (c *Ctx) SetAt(layer, frame, x, y int, v doc.Index) {
	before := c.Doc.Pixel(layer, frame, x, y)
	k := pixKey{layer, frame, x, y}
	if i, ok := c.index[k]; ok {
		c.pixels[i].After = v
	} else if before != v {
		c.index[k] = len(c.pixels)
		c.pixels = append(c.pixels, doc.PixelDelta{
			Layer: layer, Frame: frame, X: x, Y: y, Before: before, After: v,
		})
	}
	c.Doc.SetPixel(layer, frame, x, y, v)
}

// Finish returns the step's change: the pixels whose value differs from the
// start of the step, sorted, and a meta delta when the header changed.
func (c *Ctx) Finish() Change {
	ch := Change{Pixels: netPixels(c.pixels)}
	ch.BBox = bbox(ch.Pixels)
	if h := c.Doc.Header(); !reflect.DeepEqual(h, c.header) {
		ch.Meta = &doc.MetaDelta{Before: c.header, After: h}
	}
	return ch
}

// Change is what one step did to a document, with enough information to
// revert it (undo) or apply it again (redo).
type Change struct {
	Pixels []doc.PixelDelta // sorted by layer, frame, y, x; Before != After
	Meta   *doc.MetaDelta   // nil when only pixels changed
	BBox   doc.Rect         // bounding box of Pixels; zero when there are none
}

// Empty reports whether the change does nothing.
func (ch Change) Empty() bool { return len(ch.Pixels) == 0 && ch.Meta == nil }

// Revert undoes ch on d: pixels first, then the header.
func Revert(d *doc.Doc, ch Change) {
	d.ApplyPixels(ch.Pixels, true)
	if ch.Meta != nil {
		d.SetHeader(ch.Meta.Before)
	}
}

// Reapply applies ch to d again: the header first, then pixels.
func Reapply(d *doc.Doc, ch Change) {
	if ch.Meta != nil {
		d.SetHeader(ch.Meta.After)
	}
	d.ApplyPixels(ch.Pixels, false)
}

// NetPixels combines the pixel deltas of consecutive changes: for each pixel
// the first Before and the last After, dropping pixels that end where they
// started. The result is sorted.
func NetPixels(chs []Change) []doc.PixelDelta {
	var all []doc.PixelDelta
	for _, ch := range chs {
		all = append(all, ch.Pixels...)
	}
	return netPixels(all)
}

// BBox returns the bounding box of deltas, or the zero Rect when there are
// none. Deltas on different cels share one box.
func BBox(deltas []doc.PixelDelta) doc.Rect { return bbox(deltas) }

func netPixels(deltas []doc.PixelDelta) []doc.PixelDelta {
	index := make(map[pixKey]int, len(deltas))
	var out []doc.PixelDelta
	for _, p := range deltas {
		k := pixKey{p.Layer, p.Frame, p.X, p.Y}
		if i, ok := index[k]; ok {
			out[i].After = p.After
			continue
		}
		index[k] = len(out)
		out = append(out, p)
	}
	out = slices.DeleteFunc(out, func(p doc.PixelDelta) bool { return p.Before == p.After })
	slices.SortFunc(out, func(a, b doc.PixelDelta) int {
		switch {
		case a.Less(b):
			return -1
		case b.Less(a):
			return 1
		}
		return 0
	})
	return out
}

func bbox(deltas []doc.PixelDelta) doc.Rect {
	if len(deltas) == 0 {
		return doc.Rect{}
	}
	x0, y0, x1, y1 := deltas[0].X, deltas[0].Y, deltas[0].X, deltas[0].Y
	for _, p := range deltas[1:] {
		x0, y0 = min(x0, p.X), min(y0, p.Y)
		x1, y1 = max(x1, p.X), max(y1, p.Y)
	}
	return doc.Rect{X: x0, Y: y0, W: x1 - x0 + 1, H: y1 - y0 + 1}
}
