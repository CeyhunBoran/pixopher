package ops

import (
	"reflect"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	_ "github.com/CeyhunBoran/pixopher/internal/golden" // registers -update
)

type fakeOp struct {
	spec  Spec
	apply func(*Ctx, Args) error
}

func (f fakeOp) Spec() Spec { return f.spec }
func (f fakeOp) Apply(c *Ctx, a Args) error {
	if f.apply == nil {
		return nil
	}
	return f.apply(c, a)
}

func spec(name, group string, args ...ArgSpec) Spec {
	return Spec{Name: name, Group: group, Summary: "test op", Args: args, Mutates: true}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeOp{spec: spec("rect", "draw")})
	r.Register(fakeOp{spec: spec("info", "doc")})
	r.Register(fakeOp{spec: spec("line", "draw")})
	if _, ok := r.Lookup("line"); !ok {
		t.Fatal("lookup line failed")
	}
	if _, ok := r.Lookup("nope"); ok {
		t.Fatal("lookup nope succeeded")
	}
	var names []string
	for _, op := range r.All() {
		names = append(names, op.Spec().Name)
	}
	if got := strings.Join(names, " "); got != "info line rect" {
		t.Fatalf("All() = %s", got)
	}
}

func TestRegisterRejects(t *testing.T) {
	pos := func(name string, req bool) ArgSpec { return ArgSpec{Name: name, Kind: KindPoint, Required: req} }
	for name, s := range map[string]Spec{
		"no summary":         {Name: "x", Group: "g"},
		"unnamed arg":        spec("x", "g", ArgSpec{Kind: KindInt}),
		"duplicate arg":      spec("x", "g", pos("a", true), pos("a", true)),
		"enum without vals":  spec("x", "g", ArgSpec{Name: "e", Kind: KindEnum, Flag: true}),
		"required flag":      spec("x", "g", ArgSpec{Name: "f", Kind: KindInt, Flag: true, Required: true}),
		"short positional":   spec("x", "g", ArgSpec{Name: "p", Kind: KindInt, Short: "p"}),
		"required after opt": spec("x", "g", pos("a", false), pos("b", true)),
		"repeat not last":    spec("x", "g", ArgSpec{Name: "a", Kind: KindPoint, Required: true, Repeat: true}, pos("b", true)),
		"unknown kind":       spec("x", "g", ArgSpec{Name: "a", Kind: ArgKind(99)}),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("Register did not panic")
				}
			}()
			NewRegistry().Register(fakeOp{spec: s})
		})
	}
	t.Run("duplicate op", func(t *testing.T) {
		r := NewRegistry()
		r.Register(fakeOp{spec: spec("x", "g")})
		defer func() {
			if recover() == nil {
				t.Fatal("Register did not panic")
			}
		}()
		r.Register(fakeOp{spec: spec("x", "g")})
	})
}

func TestArgKindString(t *testing.T) {
	if KindPoint.String() != "point" || KindGrid.String() != "grid" || ArgKind(-1).String() != "unknown" {
		t.Fatal("kind names")
	}
}

func TestArgs(t *testing.T) {
	a := Args{"s": "ink", "n": 3, "b": true, "p": doc.Point{X: 1, Y: 2}, "r": doc.Rect{X: 1, Y: 2, W: 3, H: 4}, "z": Size{W: 8, H: 8}}
	if a.String("s") != "ink" || a.Int("n") != 3 || !a.Bool("b") || a.Point("p") != (doc.Point{X: 1, Y: 2}) ||
		a.Rect("r") != (doc.Rect{X: 1, Y: 2, W: 3, H: 4}) || a.Size("z") != (Size{W: 8, H: 8}) {
		t.Fatal("getters")
	}
	if a.Has("missing") || a.Int("s") != 0 || a.String("missing") != "" {
		t.Fatal("missing or mistyped args should give zero values")
	}
}

func newDoc(t *testing.T) *doc.Doc {
	t.Helper()
	d, err := doc.New(4, 4)
	if err != nil {
		t.Fatal(err)
	}
	d.Palette.Entries = append(d.Palette.Entries,
		doc.Entry{Key: "o", Name: "ink", Color: doc.RGBA{A: 255}},
		doc.Entry{Key: "w", Name: "white", Color: doc.RGBA{R: 255, G: 255, B: 255, A: 255}})
	return d
}

func TestCtxRecords(t *testing.T) {
	d := newDoc(t)
	c := NewCtx(d, 0, 0)
	if c.Set(4, 0, 1) || c.Set(-1, 0, 1) {
		t.Fatal("off-canvas Set should be clipped")
	}
	c.Set(3, 2, 1)
	c.Set(0, 1, 1)
	c.Set(0, 1, 2) // overwritten in the same step: one delta, first before, last after
	c.Set(2, 0, 1)
	c.Set(2, 0, 0) // back to where it started: no delta
	c.Set(1, 1, 0) // already transparent: no delta
	if c.Pixel(0, 1) != 2 || c.Pixel(9, 9) != doc.Transparent {
		t.Fatal("Pixel")
	}
	ch := c.Finish()
	want := []doc.PixelDelta{
		{X: 0, Y: 1, Before: 0, After: 2},
		{X: 3, Y: 2, Before: 0, After: 1},
	}
	if !reflect.DeepEqual(ch.Pixels, want) {
		t.Fatalf("pixels = %+v, want %+v", ch.Pixels, want)
	}
	if ch.BBox != (doc.Rect{X: 0, Y: 1, W: 4, H: 2}) {
		t.Fatalf("bbox = %+v", ch.BBox)
	}
	if ch.Meta != nil || ch.Empty() {
		t.Fatal("pixel-only change has meta or is empty")
	}
}

func TestCtxDetectsMeta(t *testing.T) {
	d := newDoc(t)
	c := NewCtx(d, 0, 0)
	if !c.Finish().Empty() {
		t.Fatal("untouched step is not empty")
	}
	d.Palette.Entries[1].Name = "outline"
	ch := c.Finish()
	if ch.Meta == nil || ch.Meta.Before.Palette.Entries[1].Name != "ink" || ch.Meta.After.Palette.Entries[1].Name != "outline" {
		t.Fatal("meta change not detected")
	}
}

// TestRevertReapply: apply then revert is identity, and reapply after revert
// gives the applied document again.
func TestRevertReapply(t *testing.T) {
	d := newDoc(t)
	orig := d.Clone()
	c := NewCtx(d, 0, 0)
	d.Palette.Entries = append(d.Palette.Entries, doc.Entry{Key: "a", Name: "auto_ff0000", Color: doc.RGBA{R: 255, A: 255}})
	for x := range 4 {
		c.Set(x, x, 3)
	}
	c.Set(0, 3, 1)
	ch := c.Finish()
	applied := d.Clone()

	Revert(d, ch)
	d.PruneEmptyCels()
	if !reflect.DeepEqual(d, orig) {
		t.Fatal("revert is not identity")
	}
	Reapply(d, ch)
	if !reflect.DeepEqual(d, applied) {
		t.Fatal("reapply does not restore the change")
	}
}

func TestNetPixels(t *testing.T) {
	a := Change{Pixels: []doc.PixelDelta{{X: 1, Before: 0, After: 1}, {X: 0, Before: 0, After: 1}}}
	b := Change{Pixels: []doc.PixelDelta{{X: 1, Before: 1, After: 0}, {X: 2, Before: 0, After: 2}}}
	got := NetPixels([]Change{a, b})
	want := []doc.PixelDelta{{X: 0, Before: 0, After: 1}, {X: 2, Before: 0, After: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NetPixels = %+v, want %+v", got, want)
	}
	if BBox(nil) != (doc.Rect{}) {
		t.Fatal("empty bbox")
	}
}
