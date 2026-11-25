package doc

import (
	"maps"
	"slices"
)

// PixelDelta is one changed pixel: its cel, position, and the palette index
// before and after the change. Keeping both sides makes a delta reversible in
// either direction (undo and redo).
type PixelDelta struct {
	Layer, Frame  int
	X, Y          int
	Before, After Index
}

// Less orders deltas by layer, frame, y, then x. Sorted deltas give a
// deterministic history log.
func (p PixelDelta) Less(q PixelDelta) bool {
	if p.Layer != q.Layer {
		return p.Layer < q.Layer
	}
	if p.Frame != q.Frame {
		return p.Frame < q.Frame
	}
	if p.Y != q.Y {
		return p.Y < q.Y
	}
	return p.X < q.X
}

// MetaDelta is a change to everything except pixels: size, palette, ramps,
// symmetry, layers, frames, tags, regions, anchors, variants, lint config and
// parts. Before and After are header snapshots (see Doc.Header): documents
// without cels, with Rev 0.
type MetaDelta struct {
	Before, After *Doc
}

// Pixel returns the palette index at (x, y) of (layer, frame), following a
// link. An empty cel reads as transparent. The point must be on the canvas.
func (d *Doc) Pixel(layer, frame, x, y int) Index {
	pix := d.Pixels(layer, frame)
	if pix == nil {
		return Transparent
	}
	return pix[y*d.W+x]
}

// SetPixel sets the palette index at (x, y) of (layer, frame). It creates the
// cel when it is missing and writes through a link to the cel it shares
// pixels with. The point must be on the canvas.
func (d *Doc) SetPixel(layer, frame, x, y int, v Index) {
	k := CelKey{layer, frame}
	c := d.Cels[k]
	if c != nil && c.Linked {
		k = CelKey{layer, c.LinkFrame}
		c = d.Cels[k]
	}
	if c == nil || c.Linked {
		if v == Transparent {
			return
		}
		c = &Cel{Pix: make([]Index, d.W*d.H)}
		d.Cels[k] = c
	}
	c.Pix[y*d.W+x] = v
}

// ApplyPixels sets every delta's After value, or its Before value when
// inverse is true. Inverse deltas are applied in reverse order.
func (d *Doc) ApplyPixels(deltas []PixelDelta, inverse bool) {
	if !inverse {
		for _, p := range deltas {
			d.SetPixel(p.Layer, p.Frame, p.X, p.Y, p.After)
		}
		return
	}
	for _, p := range slices.Backward(deltas) {
		d.SetPixel(p.Layer, p.Frame, p.X, p.Y, p.Before)
	}
}

// Header returns a deep copy of everything except the cels, with Rev 0. Rev
// belongs to the engine, so it is not part of a meta change.
func (d *Doc) Header() *Doc {
	h := *d
	h.Rev = 0
	h.Cels = map[CelKey]*Cel{}
	h.Palette.Entries = slices.Clone(d.Palette.Entries)
	h.Ramps = cloneMap(d.Ramps, func(r Ramp) Ramp { r.Steps = slices.Clone(r.Steps); return r })
	h.Layers = slices.Clone(d.Layers)
	h.Frames = make([]Frame, len(d.Frames))
	for i, f := range d.Frames {
		h.Frames[i] = Frame{DurationMS: f.DurationMS, Anchors: maps.Clone(f.Anchors)}
	}
	h.Tags = slices.Clone(d.Tags)
	h.Regions = maps.Clone(d.Regions)
	h.Anchors = maps.Clone(d.Anchors)
	h.Variants = cloneMap(d.Variants, func(v Variant) Variant {
		return Variant{
			Ramps:   cloneMap(v.Ramps, slices.Clone[[]RGBA]),
			Entries: maps.Clone(v.Entries),
		}
	})
	h.Lint.Disable = slices.Clone(d.Lint.Disable)
	h.Parts = cloneMap(d.Parts, func(p Part) Part { p.Pix = slices.Clone(p.Pix); return p })
	return &h
}

// SetHeader replaces everything except the cels and Rev with a deep copy of
// h's header.
func (d *Doc) SetHeader(h *Doc) {
	n := h.Header()
	n.Rev, n.Cels = d.Rev, d.Cels
	*d = *n
}

// Clone returns a deep copy of d.
func (d *Doc) Clone() *Doc {
	c := d.Header()
	c.Rev = d.Rev
	for k, cel := range d.Cels {
		n := *cel
		n.Pix = slices.Clone(cel.Pix)
		c.Cels[k] = &n
	}
	return c
}

func cloneMap[V any](m map[string]V, f func(V) V) map[string]V {
	if m == nil {
		return nil
	}
	out := make(map[string]V, len(m))
	for k, v := range m {
		out[k] = f(v)
	}
	return out
}
