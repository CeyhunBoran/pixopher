package doc

import (
	"reflect"
	"testing"
)

func richDoc(t *testing.T) *Doc {
	t.Helper()
	d, err := New(4, 3)
	if err != nil {
		t.Fatal(err)
	}
	d.Rev = 7
	d.Palette.Entries = append(d.Palette.Entries, Entry{Key: "o", Name: "ink", Color: RGBA{1, 2, 3, 255}})
	d.Ramps["ink"] = Ramp{Steps: []Index{1}}
	d.Frames[0].Anchors = map[string]Point{"cork": {1, 1}}
	d.Frames = append(d.Frames, Frame{DurationMS: 50})
	d.Regions["r"] = Rect{0, 0, 2, 2}
	d.Variants["night"] = Variant{Ramps: map[string][]RGBA{"ink": {{9, 9, 9, 255}}}, Entries: map[string]RGBA{}}
	d.Lint.Disable = []string{"banding"}
	d.Parts["p"] = Part{W: 1, H: 1, Pix: []Index{1}}
	d.SetPixel(0, 0, 1, 1, 1)
	d.Cels[CelKey{0, 1}] = &Cel{Linked: true, LinkFrame: 0}
	return d
}

func TestCloneIsDeep(t *testing.T) {
	d := richDoc(t)
	c := d.Clone()
	if !reflect.DeepEqual(c, d) {
		t.Fatal("clone differs from original")
	}
	c.Palette.Entries[1].Name = "x"
	c.Ramps["ink"].Steps[0] = 0
	c.Frames[0].Anchors["cork"] = Point{}
	c.Regions["r"] = Rect{}
	c.Variants["night"].Ramps["ink"][0] = RGBA{}
	c.Lint.Disable[0] = "x"
	c.Parts["p"].Pix[0] = 0
	c.SetPixel(0, 0, 1, 1, 0)
	if !reflect.DeepEqual(d, richDoc(t)) {
		t.Fatal("changing the clone changed the original")
	}
}

func TestHeader(t *testing.T) {
	d := richDoc(t)
	h := d.Header()
	if h.Rev != 0 || len(h.Cels) != 0 {
		t.Fatalf("header has rev %d and %d cels, want 0 and 0", h.Rev, len(h.Cels))
	}
	n := richDoc(t)
	n.Palette.Entries[1].Name = "outline"
	n.Rev = 99
	d.SetHeader(n)
	if d.Palette.Entries[1].Name != "outline" || d.Rev != 7 || d.Pixel(0, 0, 1, 1) != 1 {
		t.Fatalf("SetHeader: name %q rev %d pixel %d", d.Palette.Entries[1].Name, d.Rev, d.Pixel(0, 0, 1, 1))
	}
	n.Palette.Entries[1].Name = "changed"
	if d.Palette.Entries[1].Name != "outline" {
		t.Fatal("SetHeader did not copy")
	}
}

func TestSetPixel(t *testing.T) {
	d := richDoc(t)
	// Transparent on a missing cel creates nothing.
	d.SetPixel(1, 0, 0, 0, Transparent)
	if _, ok := d.Cels[CelKey{1, 0}]; ok {
		t.Fatal("transparent write created a cel")
	}
	// Writing through a link changes the linked-to cel.
	d.SetPixel(0, 1, 2, 2, 1)
	if d.Pixel(0, 0, 2, 2) != 1 || d.Pixel(0, 1, 2, 2) != 1 {
		t.Fatal("write through link failed")
	}
}

func TestApplyPixels(t *testing.T) {
	d := richDoc(t)
	want := d.Clone()
	deltas := []PixelDelta{
		{Layer: 0, Frame: 0, X: 0, Y: 0, Before: 0, After: 1},
		{Layer: 0, Frame: 0, X: 1, Y: 1, Before: 1, After: 0},
	}
	d.ApplyPixels(deltas, false)
	if d.Pixel(0, 0, 0, 0) != 1 || d.Pixel(0, 0, 1, 1) != 0 {
		t.Fatal("forward apply failed")
	}
	d.ApplyPixels(deltas, true)
	if !reflect.DeepEqual(d, want) {
		t.Fatal("inverse apply is not identity")
	}
}
