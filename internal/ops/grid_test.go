package ops

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/errs"
)

// TestGridErrorDetails: grid errors name the 1-based row, and the column
// in cells when one cell is at fault (D64).
func TestGridErrorDetails(t *testing.T) {
	for _, tt := range []struct {
		fixture, line string
		want          gridPos
	}{
		{"potion", "grid 0,0 oo,oo,o", gridPos{Row: 3}},
		{"potion", "grid 0,0 oo,oq", gridPos{Row: 2, Col: 2}},
		{"potion", "grid 0,0 +o,oo", gridPos{Row: 1, Col: 1}},
		{"wide", "grid 0,0 aaaa,aabb,aax.", gridPos{Row: 3, Col: 2}},
		{"wide", "grid 0,0 aaaa,aab", gridPos{Row: 2}},
	} {
		t.Run(tt.line, func(t *testing.T) {
			c, err := call(t, tt.line)
			if err != nil {
				t.Fatal(err)
			}
			err = c.Op.Apply(NewCtx(fixture(t, tt.fixture), 0, 0), c.Args)
			if e, _ := errors.AsType[*errs.Error](err); e == nil || e.Details != tt.want {
				t.Fatalf("got %#v, want details %+v", err, tt.want)
			}
		})
	}
}

// TestGridPastesShow: the rows `show` prints, painted with --opaque on a
// cleared copy, give back the same document, so a grid an agent reads can
// be pasted back.
func TestGridPastesShow(t *testing.T) {
	d := fixture(t, "potion")
	want := serialize(t, d)
	showOp, _ := Default.Lookup("show")
	rep, err := showOp.(Observer).Observe(d, Args{})
	if err != nil {
		t.Fatal(err)
	}
	rows := rep.(*ShowReport).Rows
	ctx := NewCtx(d, 0, 0)
	for _, line := range []string{"clear", "grid 0,0 " + strings.Join(rows, GridSep) + " --opaque"} {
		c, err := call(t, line)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Op.Apply(ctx, c.Args); err != nil {
			t.Fatal(err)
		}
	}
	if got := serialize(t, d); !bytes.Equal(got, want) {
		t.Fatalf("pasted grid differs\n--- got\n%s\n--- want\n%s", got, want)
	}
}
