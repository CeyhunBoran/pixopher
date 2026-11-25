package ops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/golden"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
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

// call parses a script line for a built-in op.
func call(t testing.TB, line string) (Call, error) {
	t.Helper()
	f := strings.Fields(line)
	op, ok := Default.Lookup(f[0])
	if !ok {
		t.Fatalf("no op %q", f[0])
	}
	args, err := Parse(op.Spec().Args, f[1:])
	if err != nil {
		return Call{}, err
	}
	return Call{Op: op.(Mutator), Args: args, Line: Format(op.Spec(), args)}, nil
}

// TestBuiltinGolden applies each case's lines as one step, compares the
// canonical lines, the change and the resulting document with a golden
// file, and checks that undoing the change, decoded from its history log
// encoding, gives back the original document byte for byte, and that
// redoing it gives the result again.
func TestBuiltinGolden(t *testing.T) {
	tests := []struct {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := fixture(t, tt.fixture)
			before := serialize(t, d)
			ctx := NewCtx(d, 0, 0)
			var out bytes.Buffer
			for _, line := range tt.lines {
				// The test lines use shell quotes around hex colors; the
				// parser gets the unquoted text, as a shell would pass it.
				c, err := call(t, strings.ReplaceAll(line, "'", ""))
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
	if d.W != 4 || d.H != 3 || d.Rev != 0 || len(d.Palette.Entries) != 1 || d.Layers[0].Name != "main" {
		t.Fatalf("got %dx%d rev %d, %d entries", d.W, d.H, d.Rev, len(d.Palette.Entries))
	}
	// ink is not in the empty palette.
	if err := cr.Apply(NewCtx(d, 0, 0), c.Args); errs.CodeOf(err) != errs.UnknownColor {
		t.Fatalf("--bg ink: %v", err)
	}
	for line, code := range map[string]errs.Code{
		"new hero":                errs.Usage,
		"new .pxo":                errs.Usage,
		"new hero.PXO":            errs.Usage,
		"new a.pxo --size 2000x8": errs.TooLarge,
	} {
		c, err := call(t, line)
		if err == nil {
			_, err = c.Op.(Creator).Create(c.Args)
		}
		if errs.CodeOf(err) != code {
			t.Errorf("%s: %v, want %s", line, err, code)
		}
	}
	if c, _ := call(t, "new a.pxo"); c.Line != "new a.pxo" || c.Args.Size("size") != (Size{32, 32}) {
		t.Fatalf("default size: %q %v", c.Line, c.Args.Size("size"))
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
