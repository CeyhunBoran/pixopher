package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/golden"
	"github.com/CeyhunBoran/pixopher/internal/ops"
)

// TestHistoryTranscript is the M1 done-when flow: new, draw with the
// built-in ops, then undo back to the empty canvas, with log, redo and the
// history errors along the way.
func TestHistoryTranscript(t *testing.T) {
	testdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	s := &session{t: t, env: map[string]string{}, ops: ops.Default}

	s.sh("new hero.pxo --size 8x8 --palette none --json")
	fresh, err := os.ReadFile("hero.pxo")
	if err != nil {
		t.Fatal(err)
	}
	s.sh("undo --json")
	s.sh("rect 1,1 4,4 #ef7d57 --json")
	s.run("px 2,2 #1a1c2c 3,2 #1a1c2c --json -m 'the eyes'", "px", "2,2", "#1a1c2c", "3,2", "#1a1c2c", "--json", "-m", "the eyes")
	s.sh("line 0,7 7,7 #38b764 --as bot --json")
	s.sh("fill 6,0 #29366f --json")
	s.sh("show --grid --json")
	s.sh("export hero.png --scale 8 --json")
	s.sh("log")
	s.sh("log --limit 2 --json")

	// cli's undo skips bot's line. The line added a palette entry after the
	// px step, but undo only checks the entries a step changed (D54), so
	// the px step can be undone. Once bot recolors the px step's own entry,
	// it conflicts.
	s.sh("undo --json")
	s.sh("undo 0")
	s.sh("undo 2 --dry-run")
	s.sh("palette set b #5d275d --as bot --json")
	s.sh("undo 2")
	s.sh("undo --json")
	s.sh("undo --any --as bot --json")
	s.sh("log")
	s.sh("redo --as bot")
	s.sh("redo 3 --json")
	s.sh("redo --json")
	s.sh("undo --any --step --expect-rev 1")
	s.sh("undo 6 --any")
	s.sh("log --limit -1")
	s.sh("undo --help")

	// Undo everything: the document is the fresh one again, apart from rev.
	s.sh("undo 5 --any --json")
	s.sh("show --grid")
	s.sh("log --limit 3")

	golden.Check(t, filepath.Join(testdata, "history.golden"), []byte(s.out.String()))
	data, err := os.ReadFile("hero.pxo.log")
	if err != nil {
		t.Fatal(err)
	}
	golden.Check(t, filepath.Join(testdata, "history.hero.pxo.log"), data)
	got, err := os.ReadFile("hero.pxo")
	if err != nil {
		t.Fatal(err)
	}
	strip := func(b []byte) string {
		var out []string
		for l := range strings.Lines(string(b)) {
			if !strings.HasPrefix(l, "rev = ") {
				out = append(out, l)
			}
		}
		return strings.Join(out, "")
	}
	if strip(got) != strip(fresh) {
		t.Fatalf("after undoing everything:\n%s\nwant:\n%s", got, fresh)
	}
}
