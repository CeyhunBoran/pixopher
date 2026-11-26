package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/history"
	"github.com/CeyhunBoran/pixopher/internal/ops"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
)

// builtinCall parses a script line for an op of ops.Default.
func builtinCall(t testing.TB, line string) ops.Call {
	t.Helper()
	f := strings.Fields(line)
	op, ok := ops.Default.Lookup(f[0])
	if len(f) > 1 {
		if op2, ok2 := ops.Default.Lookup(f[0] + " " + f[1]); ok2 {
			op, ok, f = op2, true, f[1:]
		}
	}
	if !ok {
		t.Fatalf("no op %q", f[0])
	}
	args, err := ops.Parse(op.Spec().Args, f[1:])
	if err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	return ops.Call{Op: op.(ops.Mutator), Args: args, Line: ops.Format(op.Spec(), args)}
}

// potionFile copies testdata/potion.pxo (16x16, palette, ramps, two layers)
// to a temp dir. It has no log, as if its history was cleared.
func potionFile(t testing.TB) string {
	t.Helper()
	data := read(t, filepath.Join("..", "..", "testdata", "potion.pxo"))
	path := filepath.Join(t.TempDir(), "potion.pxo")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// builtinUndoCases run real ops through the engine. A case is one
// transaction; it covers the op of its last line, and earlier lines set it
// up. Cases that change nothing write no history entry (TestNoOpWritesNothing),
// so there is nothing for them to undo and they are left out.
var builtinUndoCases = []struct {
	name, file string // file: "potion", or "ink" for newFile's 8x8
	lines      []string
}{
	{"clear", "potion", []string{"clear"}},
	{"clear-color", "potion", []string{"clear glass_light"}},
	{"clear-hex", "ink", []string{"clear #123456"}},
	{"px", "potion", []string{"px 0,0 ink 6,0 . 0,15 liquid+1"}},
	{"px-hex", "potion", []string{"px 1,1 #123456 2,2 #123456"}},
	{"line", "potion", []string{"line -4,-2 20,10 ink"}},
	{"line-width", "potion", []string{"line 2,12 12,8 liquid --width 2"}},
	{"line-hex", "ink", []string{"line 1,1 6,4 #00ff00 --width 3"}},
	{"rect", "potion", []string{"rect 2,9 5,4 glass"}},
	{"rect-outline", "potion", []string{"rect 1,1,14,14 ink --outline"}},
	{"fill", "potion", []string{"fill 7,3 shine"}},
	{"fill-global", "potion", []string{"fill 0,0 G --contiguous=false"}},
	{"fill-hex", "ink", []string{"px 2,0 #111111 2,1 #111111 1,2 #111111 0,2 #111111", "fill 0,0 #0000ff"}},
	{"erase", "potion", []string{"erase 4,0 8,2"}},
	{"replace", "potion", []string{"replace o liquid"}},
	{"palette-add", "potion", []string{"palette add k #ffcc99 skin"}},
	{"palette-set", "potion", []string{"palette set glass #00ff00"}},
	{"palette-rename", "potion", []string{"palette rename ink outline"}},
	{"palette-rm-unused", "potion", []string{"ramp rm liquid", "palette rm r"}},
	{"palette-rm-replace", "potion", []string{"ramp rm glass", "palette rm g --replace G"}},
	{"palette-rm-transparent", "potion", []string{"ramp rm glass", "palette rm G --replace ."}},
	{"ramp-add", "potion", []string{"ramp add dark o r g"}},
	{"ramp-add-base", "potion", []string{"ramp add hot o r R w --base name:liquid"}},
	{"ramp-rm", "potion", []string{"ramp rm glass"}},
}

// TestBuiltinUndoIdentity: for every mutating op, applying it with the
// engine and then running undo gives the original document back (apart
// from rev), and redo gives the drawn document. Unlike TestBuiltinGolden
// in package ops, this goes through the history log and the palette-aware
// undo of package history (D49, D54), as `pxo undo` does.
func TestBuiltinUndoIdentity(t *testing.T) {
	for _, tt := range builtinUndoCases {
		t.Run(tt.name, func(t *testing.T) {
			path := newFile(t)
			if tt.file == "potion" {
				path = potionFile(t)
			}
			orig := read(t, path)
			s := Step{}
			for _, l := range tt.lines {
				s.Calls = append(s.Calls, builtinCall(t, l))
			}
			apply(t, path, "cli", s)
			drawn := read(t, path)
			if bytes.Equal(drawn, orig) {
				t.Fatal("the case changed nothing")
			}
			if n := logEntries(t, path); n != 1 {
				t.Fatalf("log has %d entries after one transaction, want 1", n)
			}

			undo(t, path, "cli", history.Request{})
			sameDoc(t, read(t, path), orig)
			undo(t, path, "cli", history.Request{Redo: true})
			sameDoc(t, read(t, path), drawn)
		})
	}
}

// TestBuiltinUndoCoverage fails when a mutating op of ops.Default has no
// case in TestBuiltinUndoIdentity. new is left out: it writes no history
// entry, so it has no undo (D48).
func TestBuiltinUndoCoverage(t *testing.T) {
	covered := map[string]bool{}
	for _, tt := range builtinUndoCases {
		c := builtinCall(t, tt.lines[len(tt.lines)-1])
		covered[c.Op.Spec().Name] = true
	}
	for _, op := range ops.Default.All() {
		if _, creator := op.(ops.Creator); !op.Spec().Mutates || creator {
			continue
		}
		if name := op.Spec().Name; !covered[name] {
			t.Errorf("mutating op %q has no undo identity case", name)
		}
	}
}

func logEntries(t testing.TB, path string) int {
	t.Helper()
	return bytes.Count(read(t, pxofile.LogPath(path)), []byte("\n"))
}
