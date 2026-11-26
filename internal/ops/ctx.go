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
	// A write that keeps the index is recorded too: when the step changes
	// the palette, the index may name another entry afterwards (Finish).
	if i, ok := c.index[k]; ok {
		c.pixels[i].After = v
	} else {
		c.index[k] = len(c.pixels)
		c.pixels = append(c.pixels, doc.PixelDelta{
			Layer: layer, Frame: frame, X: x, Y: y, Before: before, After: v,
		})
	}
	c.Doc.SetPixel(layer, frame, x, y, v)
}

// Finish returns the step's change: the pixels whose value differs from the
// start of the step, sorted, and a meta delta when the header changed. A
// pixel whose index stays but names another palette entry afterwards, such
// as one palette rm --replace moves onto the entry that slides into its
// index, counts as changed (D60).
func (c *Ctx) Finish() Change {
	var ch Change
	if h := c.Doc.Header(); !reflect.DeepEqual(h, c.header) {
		ch.Meta = &doc.MetaDelta{Before: c.header, After: h}
	}
	ch.Pixels = NetPixels([]Change{{Pixels: c.pixels, Meta: ch.Meta}})
	ch.BBox = bbox(ch.Pixels)
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
// started. A pixel ends where it started when its index is the same and
// names the same palette entry (by key) before its first change and after
// its last; the changes' meta deltas give those palettes (D60). The result
// is sorted.
func NetPixels(chs []Change) []doc.PixelDelta {
	before, after := paletteSpan(chs)
	type net struct {
		doc.PixelDelta
		first, last int // changes the pixel's first and last delta are in
	}
	index := map[pixKey]int{}
	var all []net
	for i, ch := range chs {
		for _, p := range ch.Pixels {
			k := pixKey{p.Layer, p.Frame, p.X, p.Y}
			if j, ok := index[k]; ok {
				all[j].After, all[j].last = p.After, i
				continue
			}
			index[k] = len(all)
			all = append(all, net{p, i, i})
		}
	}
	var out []doc.PixelDelta
	for _, n := range all {
		if n.Before != n.After || !sameKey(before[n.first], after[n.last], n.Before) {
			out = append(out, n.PixelDelta)
		}
	}
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

// paletteSpan returns the palette entries in effect before and after each of
// chs, as far as their meta deltas tell: nil for all of them when no change
// has a meta delta, as the palette then never changes.
func paletteSpan(chs []Change) (before, after [][]doc.Entry) {
	before, after = make([][]doc.Entry, len(chs)), make([][]doc.Entry, len(chs))
	var cur []doc.Entry
	for _, ch := range chs {
		if ch.Meta != nil {
			cur = ch.Meta.Before.Palette.Entries // the palette before the first meta delta
			break
		}
	}
	for i, ch := range chs {
		if ch.Meta != nil {
			cur = ch.Meta.Before.Palette.Entries
		}
		before[i] = cur
		if ch.Meta != nil {
			cur = ch.Meta.After.Palette.Entries
		}
		after[i] = cur
	}
	return before, after
}

// sameKey reports whether index v names entries with the same key in
// palettes a and b. Nil palettes stand for one unchanged palette.
func sameKey(a, b []doc.Entry, v doc.Index) bool {
	if a == nil || b == nil || v == doc.Transparent {
		return true
	}
	if int(v) >= len(a) || int(v) >= len(b) {
		return int(v) >= len(a) && int(v) >= len(b)
	}
	return a[v].Key == b[v].Key
}

// BBox returns the bounding box of deltas, or the zero Rect when there are
// none. Deltas on different cels share one box.
func BBox(deltas []doc.PixelDelta) doc.Rect { return bbox(deltas) }

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
