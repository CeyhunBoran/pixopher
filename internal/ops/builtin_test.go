package ops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/golden"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
	"github.com/CeyhunBoran/pixopher/palettes"
)

// fixture returns a test document: "potion" is testdata/potion.pxo (16x16,
// palette and ramps, drawn on layer "body"), "blank" is an empty 8x8
// document, and "full" is an 8x8 document whose palette has no free key.
func fixture(t testing.TB, name string) *doc.Doc {
	t.Helper()
	switch name {
	case "potion":
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "potion.pxo"))
		if err != nil {
			t.Fatal(err)
		}
		d, err := pxofile.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		return d
	case "default":
		d, _ := doc.New(8, 8)
		pal, _ := palettes.Load(palettes.Default)
		d.Palette, d.Ramps = pal.Palette, pal.Ramps
		return d
	case "linked":
		// potion with a second frame whose body cel links to frame 0, and
		// pixels on the shine layer.
		d := fixture(t, "potion")
		d.Frames = append(d.Frames, doc.Frame{DurationMS: 120})
		d.Cels[doc.CelKey{Layer: 0, Frame: 1}] = &doc.Cel{Linked: true}
		d.SetPixel(1, 0, 5, 5, 6)  // w
		d.SetPixel(1, 0, 6, 5, 3)  // G
		d.SetPixel(1, 1, 0, 15, 5) // R on frame 1
		return d
	case "variants":
		// potion with an entry outside every ramp, and a variant that
		// overrides it and the glass ramp.
		d := fixture(t, "potion")
		d.Palette.Entries = append(d.Palette.Entries, doc.Entry{Key: "x", Name: "extra", Color: doc.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}})
		d.Variants["night"] = doc.Variant{
			Entries: map[string]doc.RGBA{"extra": {A: 0xff}},
			Ramps:   map[string][]doc.RGBA{"glass": {{A: 0xff}, {R: 1, A: 0xff}, {R: 2, A: 0xff}}},
		}
		return d
	case "used":
		// An 8x8 document on a locked layer, with one pixel of entry red.
		d, _ := doc.New(8, 8)
		d.Palette.Entries = append(d.Palette.Entries,
			doc.Entry{Key: "k", Name: "red", Color: doc.RGBA{R: 0xff, A: 0xff}},
			doc.Entry{Key: "b", Name: "blue", Color: doc.RGBA{B: 0xff, A: 0xff}})
		d.SetPixel(0, 0, 0, 0, 1)
		d.SetPixel(0, 0, 1, 0, 2)
		d.Layers[0].Locked = true
		return d
	case "wide":
		// An 8x8 document at key width 2 with two entries, and a pixel
		// of each at (1,1) and (3,1).
		d, _ := doc.New(8, 8)
		d.KeyWidth = 2
		d.Palette.Entries[0].Key = ".."
		d.Palette.Entries = append(d.Palette.Entries,
			doc.Entry{Key: "aa", Name: "red", Color: doc.RGBA{R: 0xff, A: 0xff}},
			doc.Entry{Key: "bb", Name: "blue", Color: doc.RGBA{B: 0xff, A: 0xff}})
		d.SetPixel(0, 0, 1, 1, 2)
		d.SetPixel(0, 0, 3, 1, 1)
		return d
	case "blank", "full", "locked":
		d, err := doc.New(8, 8)
		if err != nil {
			t.Fatal(err)
		}
		if name == "locked" {
			d.Layers[0].Locked = true
		}
		for i := 0; name == "full"; i++ {
			e, err := d.Palette.AutoEntry(doc.RGBA{R: uint8(i), G: uint8(i >> 8), A: 0xff}, d.KeyWidth)
			if err != nil {
				break
			}
			d.Palette.Entries = append(d.Palette.Entries, e)
		}
		return d
	}
	t.Fatalf("no fixture %q", name)
	return nil
}

func serialize(t testing.TB, d *doc.Doc) []byte {
	t.Helper()
	data, err := pxofile.Serialize(d)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// call parses a script line for a built-in op. Single quotes group words
// and are dropped, as a shell does.
func call(t testing.TB, line string) (Call, error) {
	t.Helper()
	f := quoteFields(line)
	op, ok := Default.Lookup(f[0])
	if len(f) > 1 {
		// Two-word ops: palette add, ramp rm, ...
		if op2, ok2 := Default.Lookup(f[0] + " " + f[1]); ok2 {
			op, ok, f = op2, true, f[1:]
		}
	}
	if !ok {
		t.Fatalf("no op %q", f[0])
	}
	args, err := Parse(op.Spec().Args, f[1:])
	if err != nil {
		return Call{}, err
	}
	return Call{Op: op.(Mutator), Args: args, Line: Format(op.Spec(), args)}, nil
}

// quoteFields splits line on spaces outside single quotes and drops the
// quotes.
func quoteFields(line string) []string {
	var (
		out []string
		tok strings.Builder
		in  bool // inside a token
		q   bool // inside quotes
	)
	for _, c := range line {
		switch {
		case c == '\'':
			q, in = !q, true
		case c == ' ' && !q:
			if in {
				out = append(out, tok.String())
				tok.Reset()
				in = false
			}
		default:
			tok.WriteRune(c)
			in = true
		}
	}
	if in {
		out = append(out, tok.String())
	}
	return out
}

// builtinCases are TestBuiltinGolden's cases. A case covers the op of its
// last line; earlier lines set it up (TestEveryOpHasGolden).
var builtinCases = []struct {
	name, fixture string
	lines         []string
}{
	{"clear", "potion", []string{"clear"}},
	{"clear-color", "potion", []string{"clear glass_light"}},
	{"clear-hex", "blank", []string{"clear '#123456'"}},
	{"px", "potion", []string{"px 0,0 ink"}},
	{"px-group", "potion", []string{"px 0,15 ink 1,15 glass:-1 6,0 . 0,15 liquid+1"}},
	{"px-hex-twice", "blank", []string{"px 1,1 #ff0000 2,2 #ff0000 3,3 #ff000080"}},
	{"line", "potion", []string{"line 0,15 15,9 ink"}},
	{"line-steep", "potion", []string{"line 3,15 5,0 glass"}},
	{"line-clipped", "potion", []string{"line -4,-2 20,10 ink"}},
	{"line-width2", "potion", []string{"line 2,12 12,8 liquid --width 2"}},
	{"line-width3-hex", "blank", []string{"line 1,1 6,4 #00ff00 --width 3"}},
	{"line-off-canvas-hex", "blank", []string{"line -5,-5 -1,-9 #ff0000"}},
	{"rect", "potion", []string{"rect 2,9 5,4 glass"}},
	{"rect-outline", "potion", []string{"rect 1,1,14,14 ink --outline"}},
	{"rect-clipped-outline", "potion", []string{"rect -2,-2 6,6 liquid --outline"}},
	{"rect-fill-false", "potion", []string{"rect 0,0 16,16 shine --outline=false"}},
	{"fill", "potion", []string{"fill 7,3 shine"}},
	{"fill-8", "potion", []string{"fill 7,3 shine --8"}},
	{"fill-ring", "potion", []string{"fill 6,1 liquid"}},
	{"fill-same", "potion", []string{"fill 6,1 o"}},
	{"fill-global", "potion", []string{"fill 0,0 G --contiguous=false"}},
	{"fill-hex", "blank", []string{"px 2,0 #111111 2,1 #111111 1,2 #111111 0,2 #111111", "fill 0,0 '#0000ff'"}},
	{"erase", "potion", []string{"erase 6,1"}},
	{"erase-rect", "potion", []string{"erase 4,0 8,2"}},
	{"erase-clipped", "potion", []string{"erase -3,-3,10,5"}},
	{"replace", "potion", []string{"replace o liquid"}},
	{"replace-transparent", "potion", []string{"replace . '#202020'"}},
	{"replace-unknown-hex", "potion", []string{"replace #abcdef ink"}},
	{"replace-same", "potion", []string{"replace ink o"}},
	{"default-draw", "default", []string{"px 0,0 ink 1,0 warm 2,0 green+1 3,0 gray:0 4,0 #ffcd75"}},
	{"palette-add", "potion", []string{"palette add k #ffcc99 skin"}},
	{"palette-add-alpha", "blank", []string{"palette add k #ff000080 glow"}},
	{"palette-set", "potion", []string{"palette set glass #00ff00"}},
	{"palette-set-key", "potion", []string{"palette set key:w #ffffff"}},
	{"palette-set-same", "potion", []string{"palette set ink #1a1c2c"}},
	{"palette-rename", "potion", []string{"palette rename ink outline"}},
	{"palette-rename-same", "potion", []string{"palette rename o ink"}},
	{"palette-rename-variant", "variants", []string{"palette rename extra spare"}},
	{"palette-rm-unused", "potion", []string{"ramp rm liquid", "palette rm r"}},
	{"palette-rm-replace", "potion", []string{"ramp rm glass", "palette rm g --replace G"}},
	{"palette-rm-transparent", "potion", []string{"ramp rm glass", "palette rm G --replace ."}},
	{"palette-rm-linked", "linked", []string{"ramp rm glass", "palette rm o --replace R"}},
	{"palette-rm-locked", "used", []string{"palette rm red --replace blue"}},
	{"ramp-add", "potion", []string{"ramp add dark o r g"}},
	{"ramp-add-base", "potion", []string{"ramp add hot o r R w --base name:liquid"}},
	{"ramp-rm", "potion", []string{"ramp rm glass"}},
	{"grid", "potion", []string{"grid 5,1 oG.g,_RRo,.ww."}},
	{"grid-opaque", "potion", []string{"grid 5,1 oG.g,_RRo,.ww. --opaque"}},
	{"grid-clipped", "potion", []string{"grid -1,14 oooo,gggg,GGGG"}},
	{"grid-legend", "potion", []string{"grid 0,0 aab.,a_bo --legend 'a=glass_light  b=liquid+1 o=liquid:0'"}},
	{"grid-legend-transparent", "potion", []string{"grid 5,1 tot --legend t=."}},
	{"grid-legend-hex", "blank", []string{"grid 0,0 ab.c,cba. --legend 'a=#ff0000 b=#00ff00 c=#ff0000'"}},
	{"grid-legend-hex-clipped", "blank", []string{"grid 7,7 ab,ba --legend 'a=#ff0000 b=#0000ff'"}},
	{"grid-wide", "wide", []string{"grid 1,1 aabb..,__bbaa --opaque"}},
}

// TestBuiltinGolden applies each case's lines as one step, compares the
// canonical lines, the change and the resulting document with a golden
// file, and checks that undoing the change, decoded from its history log
// encoding, gives back the original document byte for byte, and that
// redoing it gives the result again. The result also round-trips:
// parse then serialize gives the same bytes and an equal model.
func TestBuiltinGolden(t *testing.T) {
	for _, tt := range builtinCases {
		t.Run(tt.name, func(t *testing.T) {
			d := fixture(t, tt.fixture)
			before := serialize(t, d)
			ctx := NewCtx(d, 0, 0)
			var out bytes.Buffer
			for _, line := range tt.lines {
				// The test lines use shell quotes; the parser gets the
				// unquoted words, as a shell would pass them.
				c, err := call(t, line)
				if err != nil {
					t.Fatal(err)
				}
				if err := c.Op.Apply(ctx, c.Args); err != nil {
					t.Fatal(err)
				}
				fmt.Fprintf(&out, "$ %s\n", c.Line)
			}
			ch := ctx.Finish()
			after := serialize(t, d)
			if err := d.Validate(); err != nil {
				t.Fatalf("result is not a valid document: %v", err)
			}
			fmt.Fprintf(&out, "changed %d  bbox %d,%d %d,%d  meta %t\n--- document\n%s",
				len(ch.Pixels), ch.BBox.X, ch.BBox.Y, ch.BBox.W, ch.BBox.H, ch.Meta != nil, after)
			golden.Check(t, filepath.Join("testdata", "builtin", tt.name+".golden"), out.Bytes())
			roundTrip(t, after)

			undo := decode(t, ch, d.W)
			Revert(d, undo)
			if got := serialize(t, d); !bytes.Equal(got, before) {
				t.Fatalf("undo is not the identity\n--- got\n%s\n--- want\n%s", got, before)
			}
			Reapply(d, undo)
			if got := serialize(t, d); !bytes.Equal(got, after) {
				t.Fatalf("redo differs\n--- got\n%s\n--- want\n%s", got, after)
			}
		})
	}
}

// roundTrip checks that data, a serialized document, parses and serializes
// back to the same bytes, and that parsing those again gives an equal model.
func roundTrip(t testing.TB, data []byte) {
	t.Helper()
	d, err := pxofile.Parse(data)
	if err != nil {
		t.Fatalf("result does not parse: %v", err)
	}
	again := serialize(t, d)
	if !bytes.Equal(again, data) {
		t.Fatalf("serialize is not stable\n%s", golden.Diff(again, data))
	}
	d2, err := pxofile.Parse(again)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, d2) {
		t.Fatalf("model changed after a round trip:\n%+v\n%+v", d, d2)
	}
}

// decode round-trips ch through the history log's undo encoding (D31), as
// undo will read it.
func decode(t testing.TB, ch Change, width int) Change {
	t.Helper()
	meta, err := pxofile.EncodeMeta(ch.Meta)
	if err != nil {
		t.Fatal(err)
	}
	pix, err := pxofile.DecodePixels(pxofile.EncodePixels(ch.Pixels, width))
	if err != nil {
		t.Fatal(err)
	}
	m, err := pxofile.DecodeMeta(meta)
	if err != nil {
		t.Fatal(err)
	}
	return Change{Pixels: pix, Meta: m}
}

func TestBuiltinErrors(t *testing.T) {
	tests := []struct {
		fixture, line string
		code          errs.Code
		arg, msg      string
	}{
		{"potion", "px 16,0 ink", errs.OutOfBounds, "at", "x=16 is outside the canvas"},
		{"potion", "px 0,0 ink 3,-1 ink", errs.OutOfBounds, "at", "y=-1 is outside the canvas"},
		{"potion", "px 0,0 inc", errs.UnknownColor, "color", `unknown color "inc"`},
		{"potion", "px 0,0 glass:9", errs.InvalidColor, "color", `ramp step "glass:9" is out of range`},
		{"potion", "line 0,0 1,1 ink --width 0", errs.Usage, "width", "bad width 0"},
		{"potion", "line 0,0 1,1 ink --width 65", errs.Usage, "width", "bad width 65"},
		{"potion", "rect 0,0 2,2 #ggg", errs.InvalidColor, "color", `invalid hex color`},
		{"potion", "fill -1,0 ink", errs.OutOfBounds, "at", "x=-1 is outside the canvas"},
		{"potion", "fill 0,0 nosuch", errs.UnknownColor, "color", `unknown color "nosuch"`},
		{"potion", "replace nosuch ink", errs.UnknownColor, "from", `unknown color "nosuch"`},
		{"potion", "replace ink nosuch", errs.UnknownColor, "to", `unknown color "nosuch"`},
		{"potion", "clear nosuch", errs.UnknownColor, "color", `unknown color "nosuch"`},
		{"full", "px 0,0 #fedcba", errs.PaletteFull, "", "palette is full"},
		{"locked", "px 0,0 .", errs.LayerLocked, "", `layer "main" is locked`},
		{"locked", "erase 0,0", errs.LayerLocked, "", `layer "main" is locked`},
		{"potion", "palette add o #000000 black", errs.AlreadyExists, "key", `palette key "o" is taken by "ink"`},
		{"potion", "palette add k #000000 ink", errs.AlreadyExists, "name", `palette entry "ink" already exists`},
		{"potion", "palette add # #000000 black", errs.InvalidColor, "key", `invalid palette key "#"`},
		{"potion", "palette add kk #000000 black", errs.InvalidColor, "key", `invalid palette key "kk"`},
		{"potion", "palette add k ink black", errs.InvalidColor, "color", `palette colors are #hex colors`},
		{"potion", "palette add k #12 black", errs.InvalidColor, "color", `invalid hex color`},
		{"potion", "palette add k #000000 9lives", errs.Usage, "name", `invalid name "9lives"`},
		{"potion", "palette add k #000000 none", errs.Usage, "name", `invalid name "none"`},
		{"potion", "palette set nosuch #000000", errs.UnknownColor, "entry", `no palette entry "nosuch"`},
		{"potion", "palette set . #000000", errs.InvalidColor, "entry", "the transparent entry is reserved"},
		{"potion", "palette set transparent #000000", errs.InvalidColor, "entry", "the transparent entry is reserved"},
		{"potion", "palette set #1a1c2c #000000", errs.InvalidColor, "entry", `"#1a1c2c" is a color`},
		{"potion", "palette set glass:0 #000000", errs.UnknownColor, "entry", `no palette entry "glass:0"`},
		{"potion", "palette set key:ink #000000", errs.UnknownColor, "entry", `no palette entry "key:ink"`},
		{"potion", "palette rename ink glass", errs.AlreadyExists, "name", `palette entry "glass" already exists`},
		{"potion", "palette rename ink 1x", errs.Usage, "name", `invalid name "1x"`},
		{"potion", "palette rm ink", errs.InUse, "entry", `palette entry "ink" is a step of ramp "glass"`},
		{"variants", "palette rm extra", errs.InUse, "entry", `palette entry "extra" is overridden by variant "night"`},
		{"used", "palette rm red", errs.InUse, "entry", `palette entry "red" is used by 1 pixel`},
		{"used", "palette rm red --replace #00ff00", errs.UnknownColor, "replace", "#00ff00 is not in the palette"},
		{"used", "palette rm red --replace red", errs.InvalidColor, "replace", "--replace is the entry being removed"},
		{"used", "palette rm red --replace nosuch", errs.UnknownColor, "replace", `unknown color "nosuch"`},
		{"potion", "ramp add glass o g", errs.AlreadyExists, "name", `ramp "glass" already exists`},
		{"potion", "ramp add hot o ink", errs.InvalidColor, "step", `palette entry "ink" is given twice`},
		{"potion", "ramp add hot o .", errs.InvalidColor, "step", "the transparent entry is reserved"},
		{"potion", "ramp add hot o glass:1", errs.UnknownColor, "step", `no palette entry "glass:1"`},
		{"potion", "ramp add hot o g --base w", errs.InvalidColor, "base", `base "w" is not a step of the ramp`},
		{"potion", "ramp add 1hot o", errs.Usage, "name", `invalid name "1hot"`},
		{"potion", "ramp rm glas", errs.UnknownColor, "name", `no ramp named "glas"`},
		{"variants", "ramp rm glass", errs.InUse, "name", `ramp "glass" is overridden by variant "night"`},
		{"potion", "ramp add hot", errs.Usage, "step", ""},
		{"potion", "grid 0,0 oo,ooo", errs.GridMismatch, "grid", "row 2 has 3 cells, but row 1 has 2"},
		{"potion", "grid 0,0 oo,,oo", errs.GridMismatch, "grid", "row 2 has 0 cells, but row 1 has 2"},
		{"wide", "grid 0,0 aab,aab", errs.GridMismatch, "grid", "row 1 is 3 characters, not a whole number of 2-character cells"},
		{"potion", "grid 0,0 oo,oq", errs.UnknownColor, "grid", `unknown key "q"`},
		{"potion", "grid 0,0 o+,oo", errs.InvalidColor, "grid", `"+" can't be a palette key`},
		{"potion", "grid 0,0 o#", errs.InvalidColor, "grid", `"#" can't be a palette key`},
		{"wide", "grid 0,0 aa_.", errs.InvalidColor, "grid", `"_." can't be a palette key`},
		{"potion", "grid 0,0 , ", errs.Usage, "grid", "empty grid"},
		{"potion", "grid 0,0 s --legend s=nosuch", errs.UnknownColor, "legend", `unknown color "nosuch"`},
		{"potion", "grid 0,0 o --legend s=nosuch", errs.UnknownColor, "legend", `unknown color "nosuch"`},
		{"potion", "grid 0,0 o --legend s", errs.Usage, "legend", `bad legend entry "s"`},
		{"potion", "grid 0,0 o --legend s=", errs.Usage, "legend", `bad legend entry "s="`},
		{"potion", "grid 0,0 o --legend .=ink", errs.Usage, "legend", `bad legend letter "."`},
		{"potion", "grid 0,0 o --legend _=ink", errs.Usage, "legend", `bad legend letter "_"`},
		{"potion", "grid 0,0 o --legend ss=ink", errs.Usage, "legend", `bad legend letter "ss"`},
		{"potion", "grid 0,0 o --legend ,=ink", errs.Usage, "legend", `bad legend letter ","`},
		{"potion", "grid 0,0 o --legend 's=ink s=glass'", errs.Usage, "legend", `legend letter "s" is given twice`},
		{"full", "grid 0,0 a --legend a=#fedcba", errs.PaletteFull, "", "palette is full"},
		{"locked", "grid 0,0 .", errs.LayerLocked, "", `layer "main" is locked`},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			c, err := call(t, tt.line)
			if err == nil {
				err = c.Op.Apply(NewCtx(fixture(t, tt.fixture), 0, 0), c.Args)
			}
			e, ok := errors.AsType[*errs.Error](err)
			if !ok || e.Code != tt.code || e.Arg != tt.arg || !strings.HasPrefix(e.Message, tt.msg) {
				t.Fatalf("got %v (%#v), want %s %q on %q", err, e, tt.code, tt.msg, tt.arg)
			}
		})
	}
}

func TestNewCreate(t *testing.T) {
	c, err := call(t, "new hero.pxo --size 4x3 --bg ink")
	if err != nil {
		t.Fatal(err)
	}
	if c.Line != "new hero.pxo --size 4,3 --bg ink" {
		t.Fatalf("line %q", c.Line)
	}
	cr := c.Op.(Creator)
	d, err := cr.Create(c.Args)
	if err != nil {
		t.Fatal(err)
	}
	// The default palette: 16 colors and ramps, ink among them.
	if d.W != 4 || d.H != 3 || d.Rev != 0 || len(d.Palette.Entries) != 17 || len(d.Ramps) != 4 || d.Layers[0].Name != "main" {
		t.Fatalf("got %dx%d rev %d, %d entries", d.W, d.H, d.Rev, len(d.Palette.Entries))
	}
	ctx := NewCtx(d, 0, 0)
	if err := cr.Apply(ctx, c.Args); err != nil || ctx.Finish().BBox != (doc.Rect{W: 4, H: 3}) {
		t.Fatalf("--bg ink: %v", err)
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	// --palette none: ink is not in the empty palette.
	c, _ = call(t, "new hero.pxo --palette none --bg ink")
	if c.Line != "new hero.pxo --bg ink --palette none" {
		t.Fatalf("line %q", c.Line)
	}
	d, err = cr.Create(c.Args)
	if err != nil || len(d.Palette.Entries) != 1 || len(d.Ramps) != 0 {
		t.Fatalf("--palette none: %v", err)
	}
	if err := cr.Apply(NewCtx(d, 0, 0), c.Args); errs.CodeOf(err) != errs.UnknownColor {
		t.Fatalf("--bg ink: %v", err)
	}
	for line, code := range map[string]errs.Code{
		"new hero":                   errs.Usage,
		"new .pxo":                   errs.Usage,
		"new hero.PXO":               errs.Usage,
		"new a.pxo --size 2000x8":    errs.TooLarge,
		"new a.pxo --palette nosuch": errs.Usage,
		"new a.pxo --palette":        errs.Usage,
	} {
		c, err := call(t, line)
		if err == nil {
			_, err = c.Op.(Creator).Create(c.Args)
		}
		if errs.CodeOf(err) != code {
			t.Errorf("%s: %v, want %s", line, err, code)
		}
	}
	if c, _ := call(t, "new a.pxo --palette default"); c.Line != "new a.pxo" || c.Args.Size("size") != (Size{32, 32}) {
		t.Fatalf("defaults: %q %v", c.Line, c.Args.Size("size"))
	}
}

// TestInfoGolden compares info's JSON and human forms with a golden file,
// and checks that observing leaves the document unchanged.
func TestInfoGolden(t *testing.T) {
	for _, name := range []string{"potion", "blank", "layers"} {
		t.Run(name, func(t *testing.T) {
			fix := name
			if name == "layers" {
				fix = "blank"
			}
			d := fixture(t, fix)
			if name == "layers" {
				// Non-default layer settings, two frames and one color.
				d.Layers = append(d.Layers, doc.Layer{Name: "shine", Locked: true, Opacity: 128, Blend: "multiply"})
				d.Frames = append(d.Frames, doc.Frame{DurationMS: 80})
				d.Palette.Entries = append(d.Palette.Entries, doc.Entry{Key: "o", Name: "ink", Color: doc.RGBA{A: 0xff}})
				d.Anchors["eye"] = doc.Point{X: 2, Y: 3}
			}
			before := serialize(t, d)
			op, _ := Default.Lookup("info")
			args, err := Parse(op.Spec().Args, nil)
			if err != nil {
				t.Fatal(err)
			}
			rep, err := op.(Observer).Observe(d, args)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(rep); err != nil {
				t.Fatal(err)
			}
			buf.WriteString("---\n" + rep.Text())
			golden.Check(t, filepath.Join("testdata", "info", name+".golden"), buf.Bytes())
			if !bytes.Equal(serialize(t, d), before) {
				t.Fatal("info changed the document")
			}
		})
	}
}

// TestPaletteListGolden compares palette list's JSON and human forms with
// a golden file, and checks that listing leaves the document unchanged.
func TestPaletteListGolden(t *testing.T) {
	for _, name := range []string{"potion", "blank", "default"} {
		t.Run(name, func(t *testing.T) {
			d := fixture(t, name)
			before := serialize(t, d)
			op, _ := Default.Lookup("palette list")
			rep, err := op.(Observer).Observe(d, Args{})
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(rep); err != nil {
				t.Fatal(err)
			}
			buf.WriteString("---\n" + rep.Text())
			golden.Check(t, filepath.Join("testdata", "palette-list", name+".golden"), buf.Bytes())
			if !bytes.Equal(serialize(t, d), before) {
				t.Fatal("palette list changed the document")
			}
		})
	}
}

// observeFixture returns the documents of TestObserveGolden that fixture
// doesn't have: "wide" (150x80, content from 13,71 to 147,75, so show
// crops it and prints two tiles), "wide-empty" (100x2, nothing drawn) and
// "kw2" (12x3 with key width 2).
func observeFixture(t *testing.T, name string) *doc.Doc {
	t.Helper()
	ink := doc.Entry{Key: "o", Name: "ink", Color: doc.RGBA{R: 0x1a, G: 0x1c, B: 0x2c, A: 0xff}}
	switch name {
	case "wide", "wide-empty":
		w, h := 150, 80
		if name == "wide-empty" {
			w, h = 100, 2
		}
		d, err := doc.New(w, h)
		if err != nil {
			t.Fatal(err)
		}
		d.Palette.Entries = append(d.Palette.Entries, ink)
		if name == "wide" {
			for x := 13; x <= 147; x += 2 {
				d.SetPixel(0, 0, x, 70+x%6, 1)
			}
		}
		return d
	case "kw2":
		d, err := doc.New(12, 3)
		if err != nil {
			t.Fatal(err)
		}
		d.KeyWidth, d.Palette = 2, doc.NewPalette(2)
		ink.Key = "oo"
		d.Palette.Entries = append(d.Palette.Entries, ink,
			doc.Entry{Key: "g1", Name: "glass", Color: doc.RGBA{R: 0x38, G: 0xb7, B: 0x64, A: 0x80}})
		for x := 2; x < 10; x++ {
			d.SetPixel(0, 0, x, 1, doc.Index(1+x%2))
		}
		return d
	}
	return fixture(t, name)
}

// observeCases are TestObserveGolden's cases.
var observeCases = []struct{ name, fixture, line string }{
	{"show", "potion", "show"},
	{"show-rulers-legend", "potion", "show --grid --rulers --legend"},
	{"show-linked", "linked", "show --legend"},
	{"show-blank", "blank", "show --rulers --legend"},
	{"show-crop", "wide", "show --rulers --legend"},
	{"show-full", "wide", "show --full --legend"},
	{"show-empty-large", "wide-empty", "show --rulers"},
	{"show-kw2", "kw2", "show --rulers --legend"},
	{"get", "potion", "get 6,0"},
	{"get-transparent", "potion", "get 0,0"},
	{"get-kw2", "kw2", "get 3,1"},
	{"stats", "potion", "stats"},
	{"stats-linked", "linked", "stats"},
	{"stats-blank", "blank", "stats"},
	{"stats-kw2", "kw2", "stats"},
}

// TestObserveGolden compares the JSON and human forms of show, get and
// stats with golden files, and checks that observing leaves the document
// unchanged. The fixture "linked" has pixels on a second layer and frame
// that these ops must not read (they read the target cel, D55).
func TestObserveGolden(t *testing.T) {
	for _, tt := range observeCases {
		t.Run(tt.name, func(t *testing.T) {
			d := observeFixture(t, tt.fixture)
			before := serialize(t, d)
			f := strings.Fields(tt.line)
			op, _ := Default.Lookup(f[0])
			args, err := Parse(op.Spec().Args, f[1:])
			if err != nil {
				t.Fatal(err)
			}
			rep, err := op.(Observer).Observe(d, args)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			buf.WriteString("$ " + tt.line + "\n")
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(rep); err != nil {
				t.Fatal(err)
			}
			buf.WriteString("---\n" + rep.Text())
			golden.Check(t, filepath.Join("testdata", "observe", tt.name+".golden"), buf.Bytes())
			if !bytes.Equal(serialize(t, d), before) {
				t.Fatalf("%s changed the document", f[0])
			}
		})
	}
}

// TestGetErrors checks that get fails like px for a point off the canvas.
func TestGetErrors(t *testing.T) {
	op, _ := Default.Lookup("get")
	for _, tc := range []struct{ at, msg string }{
		{"16,0", "x=16 is outside the canvas"},
		{"0,-1", "y=-1 is outside the canvas"},
	} {
		args, err := Parse(op.Spec().Args, []string{tc.at})
		if err != nil {
			t.Fatal(err)
		}
		_, err = op.(Observer).Observe(fixture(t, "potion"), args)
		e, ok := errors.AsType[*errs.Error](err)
		if !ok || e.Code != errs.OutOfBounds || e.Message != tc.msg || e.Arg != "at" ||
			e.Hint != "canvas is 16x16; valid "+tc.msg[:1]+" is 0..15" {
			t.Errorf("get %s: %#v", tc.at, err)
		}
	}
}
