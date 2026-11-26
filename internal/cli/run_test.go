package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/golden"
	"github.com/CeyhunBoran/pixopher/internal/ops"
)

// TestRunTranscript drives `run` end to end: steps, @set, comments and hex
// colors, -m, stdin, undo of a whole run and of one step, line-numbered
// errors that write nothing, --continue-on-error, --dry-run and --pace.
func TestRunTranscript(t *testing.T) {
	testdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	s := &session{t: t, env: map[string]string{}, ops: ops.Default}
	script := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		s.out.WriteString("$ cat " + name + "\n" + body + "\n")
	}

	s.sh("new hero.pxo --size 8x8 --palette none --json")
	script("body.pxs", `# the body, in three steps
palette add o #1a1c2c ink
rect 1,1 6,6 #ef7d57
--- the eyes
px 2,3 ink 5,3 ink   # two dots
---
@set layer=main frame=0
line 1,7 6,7 '#38b764'
`)
	s.run("run body.pxs -m 'block in' --json", "run", "body.pxs", "-m", "block in", "--json")
	s.sh("log")
	s.sh("show --grid")
	s.sh("undo --step --json")
	s.sh("undo --json")
	s.sh("redo --json")

	// A failing line names itself and writes nothing.
	script("typo.pxs", `px 0,0 ink
---
# line 4 has the typo
px 7,0 inc
`)
	s.sh("run typo.pxs --json")
	s.sh("run typo.pxs")
	s.in = "px 0,0 ink\nrect 0,0 2,2 ink --outlin\n"
	s.sh("run hero.pxo")
	s.in = "info\n"
	s.sh("run -")
	s.sh("log --limit 1")

	// Stdin, a dry run and --continue-on-error.
	s.in = "px 0,0 ink\n"
	s.sh("run --dry-run --json")
	s.in = "px 0,0 ink\npx 9,9 ink\n--- more\nbogus 1\npx 7,7 #fff\n"
	s.sh("run --continue-on-error --json")
	s.in = "px 9,9 ink\n"
	s.sh("run --continue-on-error")
	s.sh("log --limit 2")

	// --pace is accepted; direct mode has no viewer, so it changes nothing.
	s.in = "px 1,0 ink\n"
	s.sh("run --pace 40ms --json")
	s.sh("run --pace 40 --json")
	s.sh("run --pace -1s")
	s.sh("run missing.pxs --json")
	s.sh("run a.pxs b.pxs")

	golden.Check(t, filepath.Join(testdata, "run.golden"), []byte(s.out.String()))
	data, err := os.ReadFile("hero.pxo.log")
	if err != nil {
		t.Fatal(err)
	}
	golden.Check(t, filepath.Join(testdata, "run.hero.pxo.log"), data)
}
