package ops

import (
	"errors"

	"github.com/CeyhunBoran/pixopher/internal/color"
	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// resolveColor resolves the color text of the argument called arg against
// the document. Errors name arg.
func resolveColor(ctx *Ctx, arg, text string) (color.Resolved, error) {
	r, err := color.Resolve(ctx.Doc, text)
	if e, ok := errors.AsType[*errs.Error](err); ok {
		e.Arg = arg
	}
	return r, err
}

// brush paints one color on the target cel. A raw hex color that matches no
// palette entry gets an auto entry, added the first time a pixel on the
// canvas is painted, so a shape that ends up off the canvas leaves the
// palette alone.
type brush struct {
	ctx *Ctx
	c   color.Resolved
	err error
}

func newBrush(ctx *Ctx, arg, text string) (*brush, error) {
	c, err := resolveColor(ctx, arg, text)
	if err != nil {
		return nil, err
	}
	return &brush{ctx: ctx, c: c}, nil
}

// set paints (x, y) and ignores points off the canvas. After a failure
// (a full palette) it does nothing; the op returns b.err.
func (b *brush) set(x, y int) {
	if b.err != nil || !b.ctx.In(x, y) {
		return
	}
	if !b.c.InPalette {
		p := &b.ctx.Doc.Palette
		e, err := p.AutoEntry(b.c.Color, b.ctx.Doc.KeyWidth)
		if err != nil {
			b.err = err
			return
		}
		p.Entries = append(p.Entries, e)
		b.c.Index, b.c.InPalette = doc.Index(len(p.Entries)-1), true
	}
	b.ctx.Set(x, y, b.c.Index)
}

// writable fails with layer_locked when the target layer is locked.
func writable(ctx *Ctx) error {
	l := ctx.Doc.Layers[ctx.Layer]
	if l.Locked {
		return errs.New(errs.LayerLocked, "layer %q is locked", l.Name).
			WithHint("unlock the layer, or draw on another one")
	}
	return nil
}

// inCanvas fails with out_of_bounds, naming arg, when p is off the canvas.
func inCanvas(d *doc.Doc, arg string, p doc.Point) error {
	w, h := d.W, d.H
	switch {
	case p.X < 0 || p.X >= w:
		return errs.New(errs.OutOfBounds, "x=%d is outside the canvas", p.X).
			WithHint("canvas is %dx%d; valid x is 0..%d", w, h, w-1).WithArg(arg)
	case p.Y < 0 || p.Y >= h:
		return errs.New(errs.OutOfBounds, "y=%d is outside the canvas", p.Y).
			WithHint("canvas is %dx%d; valid y is 0..%d", w, h, h-1).WithArg(arg)
	}
	return nil
}

// clip returns r cut to the canvas, and false when nothing is left.
func clip(ctx *Ctx, r doc.Rect) (doc.Rect, bool) {
	x0, y0 := max(r.X, 0), max(r.Y, 0)
	x1, y1 := min(r.X+r.W, ctx.Doc.W), min(r.Y+r.H, ctx.Doc.H)
	if x0 >= x1 || y0 >= y1 {
		return doc.Rect{}, false
	}
	return doc.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}, true
}
