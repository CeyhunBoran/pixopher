package color

import (
	"errors"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/errs"
	_ "github.com/CeyhunBoran/pixopher/internal/golden" // registers -update
)

func TestParseHex(t *testing.T) {
	ok := map[string]doc.RGBA{
		"#f80":      {R: 0xff, G: 0x88, B: 0x00, A: 0xff},
		"#FF8800":   {R: 0xff, G: 0x88, B: 0x00, A: 0xff},
		"#ff880080": {R: 0xff, G: 0x88, B: 0x00, A: 0x80},
		"#00000000": {},
	}
	for s, want := range ok {
		got, err := ParseHex(s)
		if err != nil || got != want {
			t.Errorf("ParseHex(%q) = %v, %v; want %v", s, got, err, want)
		}
	}
	for _, s := range []string{"", "#", "f80", "#f8", "#f808", "#ff88000", "#gg8800", "#ff8800800", " #f80"} {
		if _, err := ParseHex(s); errs.CodeOf(err) != errs.InvalidColor {
			t.Errorf("ParseHex(%q): got %v, want invalid_color", s, err)
		}
	}
}

func TestHex(t *testing.T) {
	for c, want := range map[doc.RGBA]string{
		{R: 0xff, G: 0x88, A: 0xff}:          "#ff8800",
		{R: 0x1a, G: 0x1c, B: 0x2c, A: 0x80}: "#1a1c2c80",
		{}:                                   "#00000000",
	} {
		if got := Hex(c); got != want {
			t.Errorf("Hex(%v) = %q, want %q", c, got, want)
		}
	}
}

// fixture: keys o g G r w, plus a ramp "glass" (o g G) and a ramp "skin"
// whose name is also an entry name, and a digit key "1".
func fixture(t *testing.T) *doc.Doc {
	t.Helper()
	d, err := doc.New(8, 8)
	if err != nil {
		t.Fatal(err)
	}
	add := func(k, name, hex string) {
		c, err := ParseHex(hex)
		if err != nil {
			t.Fatal(err)
		}
		d.Palette.Entries = append(d.Palette.Entries, doc.Entry{Key: k, Name: name, Color: c})
	}
	add("o", "ink", "#1a1c2c")   // 1
	add("g", "glass", "#38b764") // 2
	add("G", "shine", "#a7f070") // 3
	add("s", "skin", "#e0a070")  // 4
	add("S", "skin_lt", "#f0c090")
	add("1", "x", "#010101") // 6: a digit key, and a one-letter name
	d.Ramps["glass"] = doc.Ramp{Steps: []doc.Index{1, 2, 3}, Base: 1}
	d.Ramps["skin"] = doc.Ramp{Steps: []doc.Index{1, 4, 5, 3}, Base: 2}
	d.Ramps["big"] = doc.Ramp{Steps: []doc.Index{1, 2, 3, 4, 5}, Base: 2}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestResolve(t *testing.T) {
	d := fixture(t)
	tests := []struct {
		in    string
		index doc.Index
	}{
		{"o", 1},           // key
		{"ink", 1},         // name
		{"key:G", 3},       // forced key
		{"name:skin", 4},   // forced name
		{"skin", 4},        // a word is a palette name before a ramp name
		{"glass", 2},       // name wins over ramp "glass" (base would also be 2)
		{"big", 3},         // ramp name only: base step
		{"glass:0", 1},     // ramp step from dark
		{"glass:2", 3},     //
		{"glass:-1", 3},    // from light
		{"glass:-3", 1},    //
		{"glass+1", 3},     // relative to base
		{"glass-1", 1},     //
		{"glass+0", 2},     //
		{"skin+1", 3},      // ramp relative, even though "skin" is also an entry
		{"skin:0", 1},      //
		{"1", 6},           // digit key
		{"x", 6},           // one character, not a key: falls back to a name
		{".", 0},           // transparent
		{"transparent", 0}, //
		{"none", 0},        //
		{"#1A1C2C", 1},     // hex matching an entry exactly
		{"#a7f070ff", 3},   //
		{"#12345600", 0},   // any alpha 0 is transparent
		{"#000", 0},        // raw, see below
	}
	for _, tt := range tests {
		r, err := Resolve(d, tt.in)
		if err != nil {
			t.Errorf("Resolve(%q): %v", tt.in, err)
			continue
		}
		if tt.in == "#000" {
			if r.InPalette || r.Color != (doc.RGBA{A: 0xff}) {
				t.Errorf("Resolve(#000) = %+v, want a raw color", r)
			}
			continue
		}
		if !r.InPalette || r.Index != tt.index || r.Color != d.Palette.Entries[tt.index].Color {
			t.Errorf("Resolve(%q) = %+v, want index %d", tt.in, r, tt.index)
		}
	}
}

func TestResolveKeyWidth2(t *testing.T) {
	d, _ := doc.New(4, 4)
	d.KeyWidth = 2
	d.Palette = doc.NewPalette(2)
	d.Palette.Entries = append(d.Palette.Entries,
		doc.Entry{Key: "ab", Name: "cd", Color: doc.RGBA{A: 255}},
		doc.Entry{Key: "cd", Name: "ab", Color: doc.RGBA{R: 1, A: 255}})
	for in, want := range map[string]doc.Index{"ab": 1, "cd": 2, "..": 0, ".": 0, "name:ab": 2, "key:cd": 2} {
		r, err := Resolve(d, in)
		if err != nil || r.Index != want {
			t.Errorf("Resolve(%q) = %+v, %v; want %d", in, r, err, want)
		}
	}
}

func TestResolveErrors(t *testing.T) {
	d := fixture(t)
	tests := []struct {
		in   string
		code errs.Code
		hint string
	}{
		{"", errs.InvalidColor, ""},
		{"inc", errs.UnknownColor, `did you mean "ink"?`},
		{"glas", errs.UnknownColor, `did you mean "glass"?`},
		{"zzzzzz", errs.UnknownColor, ""},
		{"q", errs.UnknownColor, ""},
		{"key:q", errs.UnknownColor, ""},
		{"key:oo", errs.UnknownColor, `did you mean "o"?`},
		{"name:inkk", errs.UnknownColor, `did you mean "ink"?`},
		{"#ggg", errs.InvalidColor, "use #rgb, #rrggbb or #rrggbbaa"},
		{"glass:3", errs.InvalidColor, `ramp "glass" has 3 steps (0..2, base 1)`},
		{"glass:-4", errs.InvalidColor, `ramp "glass" has 3 steps (0..2, base 1)`},
		{"glass+2", errs.InvalidColor, `ramp "glass" has 3 steps (0..2, base 1)`},
		{"glass-2", errs.InvalidColor, `ramp "glass" has 3 steps (0..2, base 1)`},
		{"glass:x", errs.InvalidColor, "use glass:0, glass:-1, glass+1 or glass-1"},
		{"glass:", errs.InvalidColor, "use glass:0, glass:-1, glass+1 or glass-1"},
		{"glass+-1", errs.InvalidColor, "use glass:0, glass:-1, glass+1 or glass-1"},
		{"glass++1", errs.InvalidColor, "use glass:0, glass:-1, glass+1 or glass-1"},
		{"ink:0", errs.UnknownColor, ""},
		{"glss:0", errs.UnknownColor, `did you mean "glass"?`},
		{"a b", errs.InvalidColor, "use a palette key or name, a ramp (skin, skin:0, skin+1), #rrggbb, or transparent"},
		{"-1", errs.InvalidColor, "use a palette key or name, a ramp (skin, skin:0, skin+1), #rrggbb, or transparent"},
	}
	for _, tt := range tests {
		_, err := Resolve(d, tt.in)
		e, ok := errors.AsType[*errs.Error](err)
		if !ok {
			t.Errorf("Resolve(%q): got %v, want %s", tt.in, err, tt.code)
			continue
		}
		if e.Code != tt.code || e.Hint != tt.hint || e.Arg != "color" {
			t.Errorf("Resolve(%q) = {%s %q hint %q arg %q}, want {%s hint %q arg color}", tt.in, e.Code, e.Message, e.Hint, e.Arg, tt.code, tt.hint)
		}
	}
}

func TestResolveDoesNotMutate(t *testing.T) {
	d := fixture(t)
	n := d.Palette.Len()
	if _, err := Resolve(d, "#abcdef"); err != nil {
		t.Fatal(err)
	}
	if d.Palette.Len() != n {
		t.Fatal("Resolve added a palette entry")
	}
}

func TestClosest(t *testing.T) {
	cands := []string{"ink", "inks", "glass"}
	for s, want := range map[string]string{"inc": "ink", "ink": "inks", "gls": "glass", "x": "", "xyzzy": ""} {
		if got := Closest(s, cands); got != want {
			t.Errorf("Closest(%q) = %q, want %q", s, got, want)
		}
	}
}
