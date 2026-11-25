package palettes

import (
	"slices"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
)

func TestBuiltins(t *testing.T) {
	if got := Names(); !slices.Equal(got, []string{"default"}) {
		t.Fatalf("names %v", got)
	}
	for _, name := range Names() {
		d, ok := Load(name)
		if !ok {
			t.Fatalf("%s: not found", name)
		}
		if err := d.Validate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// A palette file holds a palette and ramps, nothing else.
		empty, _ := doc.New(1, 1)
		empty.Palette, empty.Ramps = d.Palette, d.Ramps
		if d.KeyWidth != 1 || d.Rev != 0 || len(d.Cels) != 0 || !slices.EqualFunc(d.Layers, empty.Layers, func(a, b doc.Layer) bool { return a == b }) ||
			len(d.Regions)+len(d.Anchors)+len(d.Parts)+len(d.Variants)+len(d.Tags) != 0 || d.Meta != empty.Meta {
			t.Fatalf("%s: holds more than a palette and ramps", name)
		}
		if d.Palette.Len() != 17 || len(d.Ramps) != 4 {
			t.Fatalf("%s: %d entries, %d ramps", name, d.Palette.Len(), len(d.Ramps))
		}
	}
	if _, ok := Load("nosuch"); ok {
		t.Fatal("Load(nosuch) found a palette")
	}
	a, _ := Load(Default)
	a.Palette.Entries[1].Name = "changed"
	if b, _ := Load(Default); b.Palette.Entries[1].Name != "ink" {
		t.Fatal("Load does not return a fresh copy")
	}
}
