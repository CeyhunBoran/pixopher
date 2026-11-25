package palettes

import (
	"reflect"
	"slices"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/doc"
)

func TestBuiltins(t *testing.T) {
	if got := Names(); !slices.Equal(got, []string{"default", "sweetie16"}) {
		t.Fatalf("names %v", got)
	}
	// Everything but the keys: colors, names and ramps by index.
	type shape struct {
		Colors []doc.RGBA
		Names  []string
		Ramps  map[string]doc.Ramp
	}
	shapeOf := func(d *doc.Doc) shape {
		s := shape{Ramps: d.Ramps}
		for _, e := range d.Palette.Entries {
			s.Colors = append(s.Colors, e.Color)
			s.Names = append(s.Names, e.Name)
		}
		return s
	}
	var first *shape
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
		// default is sweetie16 with other keys: same colors, names and ramps.
		if s := shapeOf(d); first == nil {
			first = &s
		} else if !reflect.DeepEqual(*first, s) {
			t.Fatalf("%s: differs from %s in more than its keys", name, Names()[0])
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
