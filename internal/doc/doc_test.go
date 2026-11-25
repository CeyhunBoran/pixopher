package doc

import (
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/errs"
	_ "github.com/CeyhunBoran/pixopher/internal/golden" // registers -update
)

func TestNew(t *testing.T) {
	d, err := New(32, 16)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("new document is invalid: %v", err)
	}
	if len(d.Layers) != 1 || d.Layers[0] != NewLayer("main") {
		t.Errorf("layers = %+v", d.Layers)
	}
	if len(d.Frames) != 1 || d.Frames[0].DurationMS != 100 {
		t.Errorf("frames = %+v", d.Frames)
	}
	if d.Palette.Len() != 1 || d.Palette.Entries[0].Key != "." || d.KeyWidth != 1 || d.Rev != 0 {
		t.Errorf("palette %+v, key width %d, rev %d", d.Palette, d.KeyWidth, d.Rev)
	}
	if d.Symmetry != (Symmetry{Mode: SymOff, AxisX: 16, AxisY: 8}) {
		t.Errorf("symmetry = %+v", d.Symmetry)
	}
	if !d.Meta.Created.IsZero() {
		t.Error("New must not read the clock")
	}
}

func TestNewSizeLimits(t *testing.T) {
	for _, tt := range []struct {
		w, h int
		code errs.Code
	}{
		{1, 1, ""}, {1024, 1024, ""}, {1025, 1, errs.TooLarge}, {1, 1025, errs.TooLarge}, {0, 5, errs.OutOfBounds}, {5, -1, errs.OutOfBounds},
	} {
		_, err := New(tt.w, tt.h)
		if c := errs.CodeOf(err); c != tt.code {
			t.Errorf("New(%d, %d): code %q, want %q", tt.w, tt.h, c, tt.code)
		}
	}
}

func TestValidKey(t *testing.T) {
	n := 0
	for c := range 128 {
		if ValidKey(string(rune(c)), 1) {
			n++
		}
	}
	// 94 printable non-space ASCII characters minus 12 reserved ones.
	if n != 82 {
		t.Errorf("%d single-character keys, want 82", n)
	}
	for _, k := range []string{" ", `"`, "'", `\`, "#", "@", "%", ":", "+", "-", ",", ".", "_", "é", "ab"} {
		if ValidKey(k, 1) {
			t.Errorf("ValidKey(%q, 1) = true", k)
		}
	}
	for k, want := range map[string]bool{"a1": true, "Zz": true, "a.": false, "..": false, "__": false, "a": false, "abc": false} {
		if ValidKey(k, 2) != want {
			t.Errorf("ValidKey(%q, 2) = %v", k, !want)
		}
	}
}

func TestValidName(t *testing.T) {
	for s, want := range map[string]bool{
		"ink": true, "skin_2": true, "_x": true, "A": true, "auto_3f2a1b": true,
		"": false, "2x": false, "a-b": false, "a b": false, "é": false,
		"transparent": false, "none": false, "key": false, "name": false,
		strings.Repeat("a", 64): true, strings.Repeat("a", 65): false,
	} {
		if ValidName(s) != want {
			t.Errorf("ValidName(%q) = %v", s, !want)
		}
	}
}

func TestAutoEntry(t *testing.T) {
	p := NewPalette(1)
	e, err := p.AutoEntry(RGBA{R: 0x3f, G: 0x2a, B: 0x1b, A: 0xff}, 1)
	if err != nil || e.Key != "a" || e.Name != "auto_3f2a1b" {
		t.Fatalf("got %+v, %v", e, err)
	}
	if p.Len() != 1 {
		t.Fatal("AutoEntry modified the palette")
	}
	p.Entries = append(p.Entries, e, Entry{Key: "b", Name: "auto_3f2a1b80"})
	e, _ = p.AutoEntry(RGBA{R: 0x3f, G: 0x2a, B: 0x1b, A: 0x80}, 1)
	if e.Key != "c" || e.Name != "auto_3f2a1b80_2" {
		t.Fatalf("got %+v", e)
	}

	// Fill every single-character key, then the palette is full.
	full := NewPalette(1)
	for i := 0; i < len(autoKeyOrder); i++ {
		full.Entries = append(full.Entries, Entry{Key: autoKeyOrder[i : i+1]})
	}
	if _, err := full.AutoEntry(RGBA{A: 0xff}, 1); errs.CodeOf(err) != errs.PaletteFull {
		t.Fatalf("got %v, want palette_full", err)
	}

	p2 := NewPalette(2)
	e, _ = p2.AutoEntry(RGBA{A: 0xff}, 2)
	if e.Key != "aa" {
		t.Fatalf("key_width 2 auto key = %q, want aa", e.Key)
	}
}

func TestDefaultBase(t *testing.T) {
	for n, want := range map[int]int{1: 0, 2: 0, 3: 1, 4: 1, 5: 2} {
		if got := DefaultBase(n); got != want {
			t.Errorf("DefaultBase(%d) = %d, want %d", n, got, want)
		}
	}
}

func TestPixelsAndPrune(t *testing.T) {
	d, _ := New(2, 1)
	d.Frames = append(d.Frames, Frame{DurationMS: 100}, Frame{DurationMS: 100})
	d.Palette.Entries = append(d.Palette.Entries, Entry{Key: "a", Name: "a", Color: RGBA{A: 255}})
	d.Cels[CelKey{0, 0}] = &Cel{Pix: []Index{1, 0}}
	d.Cels[CelKey{0, 1}] = &Cel{Linked: true, LinkFrame: 0}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	if p := d.Pixels(0, 1); len(p) != 2 || p[0] != 1 {
		t.Fatalf("linked pixels = %v", p)
	}
	if d.Pixels(0, 2) != nil {
		t.Fatal("missing cel should have nil pixels")
	}
	d.PruneEmptyCels()
	if len(d.Cels) != 2 {
		t.Fatal("prune removed a cel with content")
	}
	d.Cels[CelKey{0, 0}].Pix[0] = 0
	d.PruneEmptyCels()
	if len(d.Cels) != 0 {
		t.Fatalf("prune left %d cels", len(d.Cels))
	}
}

func TestValidateRejects(t *testing.T) {
	tests := []struct {
		name string
		edit func(d *Doc)
		msg  string
	}{
		{"no layers", func(d *Doc) { d.Layers = nil }, "no layers"},
		{"no frames", func(d *Doc) { d.Frames = nil }, "no frames"},
		{"bad transparent", func(d *Doc) { d.Palette.Entries[0].Color.A = 1 }, "entry 0"},
		{"key width", func(d *Doc) { d.KeyWidth = 2 }, "entry 0"},
		{"pixel index", func(d *Doc) { d.Cels[CelKey{0, 0}] = &Cel{Pix: make([]Index, 4)}; d.Cels[CelKey{0, 0}].Pix[0] = 9 }, "index 9"},
		{"pixel count", func(d *Doc) { d.Cels[CelKey{0, 0}] = &Cel{Pix: make([]Index, 3)} }, "3 pixels"},
		{"cel layer", func(d *Doc) { d.Cels[CelKey{1, 0}] = &Cel{Pix: make([]Index, 4)} }, "does not exist"},
		{"linked with pixels", func(d *Doc) {
			d.Frames = append(d.Frames, Frame{DurationMS: 1})
			d.Cels[CelKey{0, 0}] = &Cel{Pix: make([]Index, 4)}
			d.Cels[CelKey{0, 1}] = &Cel{Linked: true, LinkFrame: 0, Pix: make([]Index, 4)}
		}, "has pixels"},
		{"link chain", func(d *Doc) {
			d.Frames = append(d.Frames, Frame{DurationMS: 1}, Frame{DurationMS: 1})
			d.Cels[CelKey{0, 0}] = &Cel{Pix: make([]Index, 4)}
			d.Cels[CelKey{0, 1}] = &Cel{Linked: true, LinkFrame: 0}
			d.Cels[CelKey{0, 2}] = &Cel{Linked: true, LinkFrame: 1}
		}, "no unlinked cel"},
		{"ramp base", func(d *Doc) {
			d.Palette.Entries = append(d.Palette.Entries, Entry{Key: "a", Name: "a"})
			d.Ramps["r"] = Ramp{Steps: []Index{1}, Base: 1}
		}, "base 1"},
		{"variant unknown ramp", func(d *Doc) { d.Variants["v"] = Variant{Ramps: map[string][]RGBA{"r": nil}} }, "unknown ramp"},
		{"part pixels", func(d *Doc) { d.Parts["p"] = Part{W: 2, H: 2, Pix: make([]Index, 3)} }, "3 pixels"},
		{"lint unsorted", func(d *Doc) { d.Lint.Disable = []string{"b", "a"} }, "not sorted"},
		{"negative rev", func(d *Doc) { d.Rev = -1 }, "negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, _ := New(2, 2)
			tt.edit(d)
			err := d.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.msg) {
				t.Fatalf("got %v, want an error containing %q", err, tt.msg)
			}
		})
	}
}
