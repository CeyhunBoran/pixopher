package ops

import (
	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/raster"
)

func init() {
	for _, op := range []Op{newOp{}, infoOp{}, clearOp{}, pxOp{}, lineOp{}, rectOp{}, fillOp{}, eraseOp{}, replaceOp{}} {
		Default.Register(op)
	}
}

// MaxLineWidth is the largest brush `line --width` takes.
const MaxLineWidth = 64

type clearOp struct{}

func (clearOp) Spec() Spec {
	return Spec{
		Name: "clear", Group: "document", Summary: "Clear the target cel, or fill it with one color", Mutates: true,
		Args: []ArgSpec{
			{Name: "color", Kind: KindColor, Default: ".", Help: "color to fill with"},
		},
		Examples: []string{"clear", "clear sky"},
	}
}

func (clearOp) Apply(ctx *Ctx, args Args) error {
	if err := writable(ctx); err != nil {
		return err
	}
	b, err := newBrush(ctx, "color", args.String("color"))
	if err != nil {
		return err
	}
	for y := range ctx.Doc.H {
		for x := range ctx.Doc.W {
			b.set(x, y)
		}
	}
	return b.err
}

type pxOp struct{}

func (pxOp) Spec() Spec {
	return Spec{
		Name: "px", Group: "draw", Summary: "Set pixels, each to its own color", Mutates: true,
		Args: []ArgSpec{
			{Name: "at", Kind: KindPoint, Required: true, Repeat: true, Help: "pixel on the canvas"},
			{Name: "color", Kind: KindColor, Required: true, Repeat: true},
		},
		Examples: []string{"px 3,4 ink", "px 3,4 ink 4,4 skin 5,4 ."},
	}
}

func (pxOp) Apply(ctx *Ctx, args Args) error {
	if err := writable(ctx); err != nil {
		return err
	}
	at, _ := args["at"].([]doc.Point)
	colors, _ := args["color"].([]string)
	for i, p := range at {
		if err := inCanvas(ctx.Doc, "at", p); err != nil {
			return err
		}
		// Resolve each color after the previous pixel is painted, so a hex
		// color given twice reuses the auto entry its first use added.
		b, err := newBrush(ctx, "color", colors[i])
		if err != nil {
			return err
		}
		if b.set(p.X, p.Y); b.err != nil {
			return b.err
		}
	}
	return nil
}

type lineOp struct{}

func (lineOp) Spec() Spec {
	return Spec{
		Name: "line", Group: "draw", Summary: "Draw a Bresenham line, clipped to the canvas", Mutates: true,
		Args: []ArgSpec{
			{Name: "from", Kind: KindPoint, Required: true},
			{Name: "to", Kind: KindPoint, Required: true},
			{Name: "color", Kind: KindColor, Required: true},
			{Name: "width", Kind: KindInt, Flag: true, Default: "1", Help: "side of the square brush, 1..64"},
		},
		Examples: []string{"line 0,0 15,7 ink", "line 2,12 13,12 shadow --width 2"},
	}
}

func (lineOp) Apply(ctx *Ctx, args Args) error {
	if err := writable(ctx); err != nil {
		return err
	}
	w := args.Int("width")
	if w < 1 || w > MaxLineWidth {
		return errs.New(errs.Usage, "bad width %d", w).
			WithHint("width is 1..%d", MaxLineWidth).WithArg("width")
	}
	b, err := newBrush(ctx, "color", args.String("color"))
	if err != nil {
		return err
	}
	// The brush is a w x w square; for an even w the extra row and column
	// are on the right and bottom of the line pixel.
	lo := -(w - 1) / 2
	cw, ch := ctx.Doc.W, ctx.Doc.H
	var mark []bool
	if w > 1 {
		mark = make([]bool, cw*ch)
	}
	raster.Line(args.Point("from"), args.Point("to"), func(x, y int) {
		if w == 1 {
			b.set(x, y)
			return
		}
		for yy := max(y+lo, 0); yy < min(y+lo+w, ch); yy++ {
			for xx := max(x+lo, 0); xx < min(x+lo+w, cw); xx++ {
				mark[yy*cw+xx] = true
			}
		}
	})
	for i, ok := range mark {
		if ok {
			b.set(i%cw, i/cw)
		}
	}
	return b.err
}

type rectOp struct{}

func (rectOp) Spec() Spec {
	return Spec{
		Name: "rect", Group: "draw", Summary: "Draw a filled rectangle, or its 1-px border, clipped to the canvas", Mutates: true,
		Args: []ArgSpec{
			{Name: "rect", Kind: KindRect, Required: true},
			{Name: "color", Kind: KindColor, Required: true},
			{Name: "outline", Kind: KindBool, Flag: true, Help: "draw only the 1-px border"},
		},
		Examples: []string{"rect 2,2 12,8 skin", "rect 1,1,14,14 ink --outline"},
	}
}

func (rectOp) Apply(ctx *Ctx, args Args) error {
	if err := writable(ctx); err != nil {
		return err
	}
	b, err := newBrush(ctx, "color", args.String("color"))
	if err != nil {
		return err
	}
	r := args.Rect("rect")
	c, ok := clip(ctx, r)
	if !ok {
		return nil
	}
	outline := args.Bool("outline")
	for y := c.Y; y < c.Y+c.H; y++ {
		for x := c.X; x < c.X+c.W; x++ {
			if !outline || x == r.X || y == r.Y || x == r.X+r.W-1 || y == r.Y+r.H-1 {
				b.set(x, y)
			}
		}
	}
	return b.err
}

type fillOp struct{}

func (fillOp) Spec() Spec {
	return Spec{
		Name: "fill", Group: "draw", Summary: "Flood fill the area around a pixel that has its color", Mutates: true,
		Args: []ArgSpec{
			{Name: "at", Kind: KindPoint, Required: true, Help: "seed pixel on the canvas"},
			{Name: "color", Kind: KindColor, Required: true},
			{Name: "contiguous", Kind: KindBool, Flag: true, Default: "true", Help: "only the connected area; false recolors every pixel of the seed color"},
			{Name: "8", Kind: KindBool, Flag: true, Help: "connect diagonal neighbors too (8-connected)"},
		},
		Examples: []string{"fill 8,8 liquid", "fill 0,0 . --8", "fill 3,3 ink --contiguous=false"},
	}
}

func (fillOp) Apply(ctx *Ctx, args Args) error {
	if err := writable(ctx); err != nil {
		return err
	}
	at := args.Point("at")
	if err := inCanvas(ctx.Doc, "at", at); err != nil {
		return err
	}
	b, err := newBrush(ctx, "color", args.String("color"))
	if err != nil {
		return err
	}
	seed := ctx.Pixel(at.X, at.Y)
	if b.c.InPalette && b.c.Index == seed {
		return nil
	}
	same := func(x, y int) bool { return ctx.Pixel(x, y) == seed }
	var pts []doc.Point
	if args.Bool("contiguous") {
		pts = raster.Flood(ctx.Doc.W, ctx.Doc.H, at, same, args.Bool("8"))
	} else {
		for y := range ctx.Doc.H {
			for x := range ctx.Doc.W {
				if same(x, y) {
					pts = append(pts, doc.Point{X: x, Y: y})
				}
			}
		}
	}
	for _, p := range pts {
		b.set(p.X, p.Y)
	}
	return b.err
}

type eraseOp struct{}

func (eraseOp) Spec() Spec {
	return Spec{
		Name: "erase", Group: "draw", Summary: "Make a rectangle or one pixel transparent, clipped to the canvas", Mutates: true,
		Args: []ArgSpec{
			{Name: "area", Kind: KindArea, Required: true},
		},
		Examples: []string{"erase 4,4", "erase 0,0 8,4"},
	}
}

func (eraseOp) Apply(ctx *Ctx, args Args) error {
	if err := writable(ctx); err != nil {
		return err
	}
	c, ok := clip(ctx, args.Rect("area"))
	if !ok {
		return nil
	}
	for y := c.Y; y < c.Y+c.H; y++ {
		for x := c.X; x < c.X+c.W; x++ {
			ctx.Set(x, y, doc.Transparent)
		}
	}
	return nil
}

type replaceOp struct{}

func (replaceOp) Spec() Spec {
	return Spec{
		Name: "replace", Group: "draw", Summary: "Replace one color with another on the target cel", Mutates: true,
		Args: []ArgSpec{
			{Name: "from", Kind: KindColor, Required: true},
			{Name: "to", Kind: KindColor, Required: true},
		},
		Examples: []string{"replace glass liquid", "replace . sky"},
	}
}

func (replaceOp) Apply(ctx *Ctx, args Args) error {
	if err := writable(ctx); err != nil {
		return err
	}
	from, err := resolveColor(ctx, "from", args.String("from"))
	if err != nil {
		return err
	}
	b, err := newBrush(ctx, "to", args.String("to"))
	if err != nil {
		return err
	}
	// A hex color that is in no entry is on no pixel either.
	if !from.InPalette || b.c.InPalette && b.c.Index == from.Index {
		return nil
	}
	for y := range ctx.Doc.H {
		for x := range ctx.Doc.W {
			if ctx.Pixel(x, y) == from.Index {
				b.set(x, y)
			}
		}
	}
	return b.err
}
