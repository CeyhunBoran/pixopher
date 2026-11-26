package pxofile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	"github.com/CeyhunBoran/pixopher/internal/golden"
)

func mustParse(t testing.TB, data []byte) *doc.Doc {
	t.Helper()
	d, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v (hint %q)", err, hintOf(err))
	}
	return d
}

func mustSerialize(t testing.TB, d *doc.Doc) []byte {
	t.Helper()
	b, err := Serialize(d)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return b
}

func hintOf(err error) string {
	if e, ok := errors.AsType[*errs.Error](err); ok {
		return e.Hint
	}
	return ""
}

// TestCanonical: every hand-written input serializes to its golden canonical
// form.
func TestCanonical(t *testing.T) {
	ins, _ := filepath.Glob("testdata/canon/*.in.pxo")
	if len(ins) == 0 {
		t.Fatal("no inputs")
	}
	for _, in := range ins {
		t.Run(filepath.Base(in), func(t *testing.T) {
			data, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			got := mustSerialize(t, mustParse(t, data))
			golden.Check(t, strings.TrimSuffix(in, ".in.pxo")+".golden.pxo", got)
		})
	}
}

// canonicalFiles are files that must already be in canonical form: the
// canonical goldens here, every .pxo under the repository's testdata/, and
// the golden documents the engine and CLI tests write. The canon inputs
// (*.in.pxo) are not canonical on purpose, so directories are listed
// rather than the whole repository walked.
func canonicalFiles(t *testing.T) []string {
	t.Helper()
	files, _ := filepath.Glob("testdata/canon/*.golden.pxo")
	for _, dir := range []string{"../../testdata", "../engine/testdata", "../cli/testdata"} {
		err := filepath.WalkDir(dir, func(path string, e os.DirEntry, err error) error {
			if err == nil && !e.IsDir() && strings.HasSuffix(path, ".pxo") {
				files = append(files, path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// TestRoundTrip: for canonical files, parse → serialize is byte-identical and
// parse → serialize → parse gives an equal model.
func TestRoundTrip(t *testing.T) {
	for _, f := range canonicalFiles(t) {
		t.Run(f, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			d := mustParse(t, data)
			out := mustSerialize(t, d)
			if !bytes.Equal(out, data) {
				t.Fatalf("serialize is not byte-identical\n--- got\n%s\n--- want\n%s", out, data)
			}
			if d2 := mustParse(t, out); !reflect.DeepEqual(d, d2) {
				t.Fatalf("model changed after a round trip:\n%+v\n%+v", d, d2)
			}
		})
	}
}

// fullDoc builds a document that uses every field with non-default values.
func fullDoc(t *testing.T, kw int) *doc.Doc {
	t.Helper()
	d, err := doc.New(7, 5)
	if err != nil {
		t.Fatal(err)
	}
	d.KeyWidth = kw
	d.Rev = 42
	d.Palette = doc.NewPalette(kw)
	for _, c := range []doc.RGBA{{R: 1, G: 2, B: 3, A: 255}, {R: 200, G: 100, B: 50, A: 255}, {R: 9, G: 9, B: 9, A: 128}, {R: 0, G: 0, B: 0, A: 255}} {
		e, err := d.Palette.AutoEntry(c, kw)
		if err != nil {
			t.Fatal(err)
		}
		d.Palette.Entries = append(d.Palette.Entries, e)
	}
	d.Meta = doc.Meta{Title: "a \"quoted\" \\ title\twith\ttabs é", Created: time.Date(2026, 11, 23, 18, 0, 0, 500, time.UTC)}
	d.Ramps["r"] = doc.Ramp{Steps: []doc.Index{1, 2, 4}, Base: 2}
	d.Ramps["q"] = doc.Ramp{Steps: []doc.Index{3}, Base: 0}
	d.Symmetry = doc.Symmetry{Mode: doc.SymY, AxisX: 0.5, AxisY: 5}
	d.Layers = []doc.Layer{doc.NewLayer("bg"), {Name: "fg", Visible: false, Locked: true, Opacity: 0, Blend: "add"}}
	d.Anchors["hand"] = doc.Point{X: 6, Y: 4}
	d.Frames = []doc.Frame{{DurationMS: 1}, {DurationMS: 250, Anchors: map[string]doc.Point{"hand": {X: 0, Y: 0}}}}
	d.Tags = []doc.Tag{{Name: "z", From: 1, To: 1, Direction: doc.DirReverse}, {Name: "a", From: 0, To: 1, Direction: doc.DirForward}}
	d.Regions["all"] = doc.Rect{X: 0, Y: 0, W: 7, H: 5}
	d.Variants["v"] = doc.Variant{
		Ramps:   map[string][]doc.RGBA{"r": {{R: 1, G: 1, B: 1, A: 255}, {R: 2, G: 2, B: 2, A: 255}, {R: 3, G: 3, B: 3, A: 3}}},
		Entries: map[string]doc.RGBA{d.Palette.Entries[3].Name: {R: 4, G: 4, B: 4, A: 255}},
	}
	d.Lint = doc.LintConfig{MaxColors: 8, Disable: []string{"a", "b"}}
	d.Parts["p"] = doc.Part{W: 2, H: 1, Pix: []doc.Index{0, 3}}
	pix := make([]doc.Index, 7*5)
	pix[8], pix[9], pix[34] = 1, 2, 4
	d.Cels[doc.CelKey{Layer: 0, Frame: 0}] = &doc.Cel{Pix: pix}
	d.Cels[doc.CelKey{Layer: 0, Frame: 1}] = &doc.Cel{Linked: true, LinkFrame: 0}
	pix2 := make([]doc.Index, 7*5)
	pix2[0] = 3
	d.Cels[doc.CelKey{Layer: 1, Frame: 1}] = &doc.Cel{Pix: pix2}
	return d
}

// TestModelRoundTrip: serialize → parse gives back an equal model for a
// document using every field, at both key widths.
func TestModelRoundTrip(t *testing.T) {
	for _, kw := range []int{1, 2} {
		d := fullDoc(t, kw)
		out := mustSerialize(t, d)
		got := mustParse(t, out)
		if !reflect.DeepEqual(d, got) {
			t.Fatalf("key_width %d: model changed\n%s\nwant %+v\ngot  %+v", kw, out, d, got)
		}
		if again := mustSerialize(t, got); !bytes.Equal(out, again) {
			t.Fatalf("key_width %d: serialize is not stable", kw)
		}
	}
}

func TestSerializeDropsEmptyCels(t *testing.T) {
	d, _ := doc.New(3, 3)
	d.Frames = append(d.Frames, doc.Frame{DurationMS: 100})
	d.Cels[doc.CelKey{Layer: 0, Frame: 0}] = &doc.Cel{Pix: make([]doc.Index, 9)}
	d.Cels[doc.CelKey{Layer: 0, Frame: 1}] = &doc.Cel{Linked: true, LinkFrame: 0}
	out := mustSerialize(t, d)
	if bytes.Contains(out, []byte("[[cels]]")) {
		t.Fatalf("empty cels were written:\n%s", out)
	}
}

func TestEncodingChoice(t *testing.T) {
	keys := []string{".", "a", "b"}
	noisy := func(w, h int) []doc.Index {
		p := make([]doc.Index, w*h)
		for i := range p {
			p[i] = doc.Index(1 + i%2)
		}
		return p
	}
	tests := []struct {
		name string
		pix  []doc.Index
		w, h int
		rle  bool
	}{
		{"small noisy uses grid", noisy(8, 8), 8, 8, false},
		{"uniform uses rle", make([]doc.Index, 64*64), 64, 64, true},
		{"wide noisy still uses rle", noisy(129, 1), 129, 1, true},
		{"128 wide noisy uses grid", noisy(128, 1), 128, 1, false},
		// "4a" is 2 characters per row against 4 raw: not less than half.
		{"rle exactly half uses grid", []doc.Index{1, 1, 1, 1}, 4, 1, false},
		{"rle under half uses rle", []doc.Index{1, 1, 1, 1, 1}, 5, 1, true},
	}
	for _, tt := range tests {
		if got := encode(tt.pix, tt.w, tt.h, keys).RLE; got != tt.rle {
			t.Errorf("%s: rle = %v, want %v", tt.name, got, tt.rle)
		}
	}
}

// TestRLEDigitKeys: the key is the last key_width characters of a token, so
// a digit key after a count is unambiguous.
func TestRLEDigitKeys(t *testing.T) {
	d, _ := doc.New(16, 1)
	d.Palette.Entries = append(d.Palette.Entries, doc.Entry{Key: "1", Name: "one", Color: doc.RGBA{A: 255}})
	g, err := newDecoder(d).decodeRLE("test", []string{"121 4."})
	if err != nil {
		t.Fatal(err)
	}
	if g.W != 16 || g.Pix[11] != 1 || g.Pix[12] != 0 {
		t.Fatalf("got %+v", g)
	}

	d.KeyWidth = 2
	d.Palette = doc.NewPalette(2)
	d.Palette.Entries = append(d.Palette.Entries, doc.Entry{Key: "11", Name: "one", Color: doc.RGBA{A: 255}})
	g, err = newDecoder(d).decodeRLE("test", []string{"1011 6.."})
	if err != nil {
		t.Fatal(err)
	}
	if g.W != 16 || g.Pix[9] != 1 || g.Pix[10] != 0 {
		t.Fatalf("key_width 2: got %+v", g)
	}
}

func TestGridCRLF(t *testing.T) {
	src := "pixopher = 1\r\nsize = [2, 2]\r\n[palette]\r\na = [\"#ffffff\", \"white\"]\r\n" +
		"[[cels]]\r\nlayer = \"main\"\r\nframe = 0\r\ngrid = '''\r\na.\r\n.a\r\n'''\r\n"
	d := mustParse(t, []byte(src))
	if p := d.Pixels(0, 0); !reflect.DeepEqual(p, []doc.Index{1, 0, 0, 1}) {
		t.Fatalf("got %v", p)
	}
}

func TestParseErrors(t *testing.T) {
	const head = "pixopher = 1\nsize = [4, 2]\n[palette]\na = [\"#ffffff\", \"white\"]\n"
	cel := func(body string) string { return head + "[[cels]]\nlayer = \"main\"\nframe = 0\n" + body }
	tests := []struct {
		name, src string
		code      errs.Code
		msg       string
	}{
		{"bad toml", "pixopher = \n", errs.ParseError, "line 1"},
		{"no version", "size = [1, 1]\n", errs.ParseError, "missing `pixopher`"},
		{"future version", "pixopher = 2\nsize = [1, 1]\n", errs.UnsupportedVersion, "version 2"},
		{"future version, new schema", "pixopher = 2\nsize = \"big\"\n", errs.UnsupportedVersion, "version 2"},
		{"unknown key", head + "[meta]\nmood = \"happy\"\n", errs.ParseError, `"meta.mood"`},
		{"unknown top key", "pixopher = 1\nsize = [1, 1]\nzoom = 3\n", errs.ParseError, `"zoom"`},
		{"no size", "pixopher = 1\n", errs.ParseError, "size"},
		{"too large", "pixopher = 1\nsize = [1025, 4]\n", errs.TooLarge, "too large"},
		{"zero size", "pixopher = 1\nsize = [0, 4]\n", errs.OutOfBounds, "empty"},
		{"key width 3", "pixopher = 1\nsize = [1, 1]\nkey_width = 3\n", errs.ParseError, "key_width"},
		{"reserved key", head + "\"#\" = [\"#000000\", \"x\"]\n", errs.ParseError, `invalid palette key "#"`},
		{"long key", head + "ab = [\"#000000\", \"x\"]\n", errs.ParseError, `invalid palette key "ab"`},
		{"dup name", head + "b = [\"#000000\", \"white\"]\n", errs.ParseError, "duplicate palette entry name"},
		{"bad name", head + "b = [\"#000000\", \"2x\"]\n", errs.ParseError, "invalid palette entry name"},
		{"reserved name", head + "b = [\"#000000\", \"none\"]\n", errs.ParseError, "invalid palette entry name"},
		{"bad color", head + "b = [\"#00000\", \"x\"]\n", errs.ParseError, "invalid color"},
		{"no entry name", head + "b = [\"#000000\"]\n", errs.ParseError, "[color, name]"},
		{"colored transparent", head + "\".\" = [\"#ff000000\", \"transparent\"]\n", errs.ParseError, "reserved"},
		{"ragged grid", cel("grid = '''\naaaa\naa\n'''\n"), errs.GridMismatch, "row 1"},
		{"grid outside canvas", cel("origin = [2, 0]\ngrid = '''\naaa\n'''\n"), errs.GridMismatch, "not inside"},
		{"unknown grid key", cel("grid = '''\nab\n'''\n"), errs.ParseError, `unknown palette key "b"`},
		{"erase in grid", cel("grid = '''\na_\n'''\n"), errs.ParseError, "erase marker"},
		{"empty grid", cel("grid = '''\n'''\n"), errs.GridMismatch, "empty"},
		{"rle ragged", cel("encoding = \"rle\"\nrows = [\"4a\", \"3a\"]\n"), errs.GridMismatch, "row 1"},
		{"rle no count", cel("encoding = \"rle\"\nrows = [\"a\"]\n"), errs.ParseError, "count and a key"},
		{"rle bad count", cel("encoding = \"rle\"\nrows = [\"0a 4a\"]\n"), errs.ParseError, "invalid count"},
		{"rle huge count", cel("encoding = \"rle\"\nrows = [\"99999999999999999999a\"]\n"), errs.ParseError, "invalid count"},
		{"rle too wide", cel("encoding = \"rle\"\nrows = [\"2000a\"]\n"), errs.GridMismatch, "wider"},
		{"bad encoding", cel("encoding = \"zip\"\nrows = [\"4a\"]\n"), errs.ParseError, "unknown encoding"},
		{"grid and rows", cel("encoding = \"rle\"\nrows = [\"4a\"]\ngrid = '''\naaaa\n'''\n"), errs.ParseError, "either"},
		{"unknown layer", head + "[[cels]]\nlayer = \"nope\"\nframe = 0\ngrid = 'a'\n", errs.ParseError, "unknown layer"},
		{"bad frame", head + "[[cels]]\nlayer = \"main\"\nframe = 1\ngrid = 'a'\n", errs.ParseError, "frame 1"},
		{"dup cel", cel("grid = 'a'\n") + "[[cels]]\nlayer = \"main\"\nframe = 0\ngrid = 'a'\n", errs.ParseError, "twice"},
		{"self link", head + "[[cels]]\nlayer = \"main\"\nframe = 0\nlink = { frame = 0 }\n", errs.ParseError, "itself"},
		{"dangling link", head + "[[frames]]\n[[frames]]\n[[cels]]\nlayer = \"main\"\nframe = 1\nlink = { frame = 0 }\n", errs.ParseError, "links to frame 0"},
		{"link with grid", head + "[[cels]]\nlayer = \"main\"\nframe = 0\nlink = { frame = 1 }\ngrid = 'a'\n", errs.ParseError, "linked cel"},
		{"ramp unknown key", head + "[ramps]\nr = [\"a\", \"z\"]\n", errs.ParseError, `unknown palette key "z"`},
		{"ramp transparent", head + "[ramps]\nr = [\".\", \"a\"]\n", errs.ParseError, "invalid step"},
		{"ramp dup", head + "[ramps]\nr = [\"a\", \"a\"]\n", errs.ParseError, "twice"},
		{"ramp base not step", head + "b = [\"#000000\", \"black\"]\n[ramps]\nr = { steps = [\"a\"], base = \"b\" }\n", errs.ParseError, "not one of its steps"},
		{"ramp table key", head + "[ramps]\nr = { steps = [\"a\"], top = \"a\" }\n", errs.ParseError, `unknown key "top"`},
		{"ramp empty", head + "[ramps]\nr = []\n", errs.ParseError, "no steps"},
		{"symmetry mode", head + "[symmetry]\nmode = \"radial\"\n", errs.ParseError, "symmetry mode"},
		{"symmetry axis", head + "[symmetry]\nmode = \"x\"\naxis_x = 1.25\n", errs.ParseError, "axis_x"},
		{"dup layer", head + "[[layers]]\nname = \"a\"\n[[layers]]\nname = \"a\"\n", errs.ParseError, "duplicate layer"},
		{"opacity", head + "[[layers]]\nname = \"a\"\nopacity = 300\n", errs.ParseError, "opacity"},
		{"zero duration", head + "[[frames]]\nduration_ms = 0\n", errs.ParseError, "duration_ms"},
		{"tag range", head + "[[tags]]\nname = \"t\"\nfrom = 0\nto = 1\n", errs.ParseError, "range"},
		{"tag direction", head + "[[tags]]\nname = \"t\"\nfrom = 0\nto = 0\ndirection = \"up\"\n", errs.ParseError, "direction"},
		{"region outside", head + "[regions]\nr = [3, 0, 2, 1]\n", errs.ParseError, "region"},
		{"region shape", head + "[regions]\nr = [3, 0]\n", errs.ParseError, "[x, y, w, h]"},
		{"anchor outside", head + "[anchors]\np = [4, 0]\n", errs.ParseError, "anchor"},
		{"frame anchor unknown", head + "[[frames]]\nanchors = { p = [0, 0] }\n", errs.ParseError, "not in [anchors]"},
		{"variant ramp len", head + "[ramps]\nr = [\"a\"]\n[variants.v]\nr = [\"#000000\", \"#111111\"]\n", errs.ParseError, "1 steps"},
		{"variant unknown entry", head + "[variants.v]\nnope = \"#000000\"\n", errs.ParseError, "unknown palette entry"},
		{"lint dup", head + "[lint]\ndisable = [\"x\", \"x\"]\n", errs.ParseError, "duplicate"},
		{"local datetime", head + "[meta]\ncreated = 2026-11-23T18:00:00\n", errs.ParseError, "UTC offset"},
		{"local date", head + "[meta]\ncreated = 2026-11-23\n", errs.ParseError, "UTC offset"},
		{"string datetime", head + "[meta]\ncreated = \"2026-11-23T18:00:00Z\"\n", errs.ParseError, "UTC offset"},
		{"part ragged", head + "[parts.p]\ngrid = '''\naa\na\n'''\n", errs.GridMismatch, "row 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.src))
			if err == nil {
				t.Fatal("parse succeeded")
			}
			if c := errs.CodeOf(err); c != tt.code {
				t.Fatalf("code %q, want %q (%v)", c, tt.code, err)
			}
			if !strings.Contains(err.Error(), tt.msg) {
				t.Fatalf("message %q does not contain %q", err, tt.msg)
			}
		})
	}
}

func TestQuoteKey(t *testing.T) {
	for k, want := range map[string]string{
		"o": "o", "Zz": "Zz", "a-b_1": "a-b_1", ".": `"."`, "!": `"!"`, "a b": `"a b"`, `"`: `"\""`,
	} {
		if got := quoteKey(k); got != want {
			t.Errorf("quoteKey(%q) = %s, want %s", k, got, want)
		}
	}
}

// FuzzParse: parsing never panics, and anything that parses serializes to a
// canonical form that is stable under another round trip.
func FuzzParse(f *testing.F) {
	seeds, _ := filepath.Glob("testdata/canon/*.pxo")
	for _, s := range seeds {
		if b, err := os.ReadFile(s); err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := Parse(data)
		if err != nil {
			if errs.CodeOf(err) == "" {
				t.Fatalf("error without a code: %v", err)
			}
			return
		}
		out, err := Serialize(d)
		if err != nil {
			t.Fatalf("parsed document does not serialize: %v", err)
		}
		d2, err := Parse(out)
		if err != nil {
			t.Fatalf("canonical output does not parse: %v\n%s", err, out)
		}
		out2, err := Serialize(d2)
		if err != nil || !bytes.Equal(out, out2) {
			t.Fatalf("canonical output is not stable: %v\n%s\n%s", err, out, out2)
		}
	})
}

// bench64 is a noisy 64x64 document with 16 colors, a worst case for size.
func bench64(b *testing.B) *doc.Doc {
	b.Helper()
	d, _ := doc.New(64, 64)
	for i := range 16 {
		e, _ := d.Palette.AutoEntry(doc.RGBA{R: uint8(i * 16), A: 255}, 1)
		d.Palette.Entries = append(d.Palette.Entries, e)
	}
	pix := make([]doc.Index, 64*64)
	for i := range pix {
		pix[i] = doc.Index(1 + (i*7+i/64)%16)
	}
	d.Cels[doc.CelKey{}] = &doc.Cel{Pix: pix}
	return d
}

func BenchmarkSerialize64(b *testing.B) {
	d := bench64(b)
	for b.Loop() {
		if _, err := Serialize(d); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParse64(b *testing.B) {
	data := mustSerialize(b, bench64(b))
	for b.Loop() {
		if _, err := Parse(data); err != nil {
			b.Fatal(err)
		}
	}
}
