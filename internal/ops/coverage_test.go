package ops

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/golden"
)

// TestNewGolden compares the documents new creates, with its creating
// step applied, with golden files. new has no undo (D48), so this and
// TestNewCreate are its tests.
func TestNewGolden(t *testing.T) {
	for _, tt := range []struct{ name, line string }{
		{"new", "new hero.pxo --size 4x3"},
		{"new-bg", "new hero.pxo --size 4x3 --bg ink"},
		{"new-none-hex", "new hero.pxo --size 3x2 --palette none --bg #ff0000"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := call(t, tt.line)
			if err != nil {
				t.Fatal(err)
			}
			cr := c.Op.(Creator)
			d, err := cr.Create(c.Args)
			if err != nil {
				t.Fatal(err)
			}
			ctx := NewCtx(d, 0, 0)
			if err := cr.Apply(ctx, c.Args); err != nil {
				t.Fatal(err)
			}
			ctx.Finish()
			if err := d.Validate(); err != nil {
				t.Fatalf("result is not a valid document: %v", err)
			}
			after := serialize(t, d)
			out := fmt.Sprintf("$ %s\n--- document\n%s", c.Line, after)
			golden.Check(t, filepath.Join("testdata", "builtin", tt.name+".golden"), []byte(out))
			roundTrip(t, after)
		})
	}
}

// TestEveryOpHasGolden fails when a registered op has no golden test: a
// mutator needs a TestBuiltinGolden case whose last line runs it (which
// also checks its undo identity), an observer a golden case of its own.
// Creators (new) have no undo and are covered by TestNewGolden.
func TestEveryOpHasGolden(t *testing.T) {
	covered := map[string]string{
		"new":          "TestNewGolden",
		"info":         "TestInfoGolden",
		"palette list": "TestPaletteListGolden",
	}
	for _, tt := range builtinCases {
		c, err := call(t, tt.lines[len(tt.lines)-1])
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		covered[c.Op.Spec().Name] = "TestBuiltinGolden"
	}
	for _, tt := range observeCases {
		op, ok := Default.Lookup(strings.Fields(tt.line)[0])
		if !ok {
			t.Fatalf("%s: no op", tt.name)
		}
		covered[op.Spec().Name] = "TestObserveGolden"
	}
	for _, op := range Default.All() {
		name := op.Spec().Name
		test, ok := covered[name]
		switch {
		case !ok:
			t.Errorf("op %q has no golden test", name)
		case op.Spec().Mutates && test != "TestBuiltinGolden" && !isCreator(op):
			t.Errorf("mutating op %q needs a TestBuiltinGolden case for its undo identity", name)
		}
	}
}

func isCreator(op Op) bool { _, ok := op.(Creator); return ok }
