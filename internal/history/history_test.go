package history

import (
	"reflect"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/pxofile"
)

func revs(c Chain) []int {
	var out []int
	for _, e := range c.Entries {
		out = append(out, e.Rev)
	}
	return out
}

func entries(rs ...int) []pxofile.Entry {
	var out []pxofile.Entry
	for _, r := range rs {
		out = append(out, pxofile.Entry{Rev: r, Tx: "t", Author: "cli"})
	}
	return out
}

// TestBuild: the chain follows revs backward from the document's rev,
// skipping entries above it and branches abandoned after a rollback (D49).
func TestBuild(t *testing.T) {
	tests := []struct {
		name     string
		log      []int
		rev      int
		want     []int
		warnings int
	}{
		{"in step", []int{1, 2, 3}, 3, []int{1, 2, 3}, 0},
		{"empty", nil, 0, nil, 0},
		{"log ahead", []int{1, 2, 3, 4}, 2, []int{1, 2}, 1},
		{"log behind", []int{1, 2}, 4, []int{1, 2}, 1},
		{"no log", nil, 3, nil, 1},
		{"rollback then new work", []int{1, 2, 3, 4, 3, 4}, 4, []int{1, 2, 3, 4}, 0},
		{"deep rollback", []int{1, 2, 3, 4, 2}, 2, []int{1, 2}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Build(entries(tt.log...), tt.rev)
			if got := revs(c); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("revs %v, want %v", got, tt.want)
			}
			if len(c.Warnings) != tt.warnings {
				t.Fatalf("warnings %q, want %d", c.Warnings, tt.warnings)
			}
		})
	}
}

// TestUndoneState: undo and redo entries flip the state of their target,
// and an abandoned undo doesn't count.
func TestUndoneState(t *testing.T) {
	log := entries(1, 2)
	log = append(log,
		pxofile.Entry{Rev: 3, Tx: "u", Kind: pxofile.KindUndo, Target: 2},
		pxofile.Entry{Rev: 4, Tx: "u", Kind: pxofile.KindUndo, Target: 1},
		pxofile.Entry{Rev: 5, Tx: "r", Kind: pxofile.KindRedo, Target: 1},
	)
	c := Build(log, 5)
	if c.Undone(1) || !c.Undone(2) {
		t.Fatalf("undone 1=%v 2=%v, want false and true", c.Undone(1), c.Undone(2))
	}
	if c := Build(log, 2); c.Undone(2) {
		t.Fatal("an undo above the document's rev must not count")
	}
}
