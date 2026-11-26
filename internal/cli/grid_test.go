package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CeyhunBoran/pixopher/internal/golden"
	"github.com/CeyhunBoran/pixopher/internal/ops"
)

// TestGridTranscript drives grid paint end to end: the docs example on
// stdin with --legend, . that leaves pixels alone, _ and --opaque that
// erase, a grid given as one token, grid errors with their row, grid
// blocks in a run script, and undo.
func TestGridTranscript(t *testing.T) {
	testdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	s := &session{t: t, env: map[string]string{}, ops: ops.Default}

	s.sh("new head.pxo --size 20x6 --palette none --json")
	s.in = "palette add o #1a1c2c ink\npalette add k #f2c3a0 skin\npalette add K #c08060 skin_dark\n"
	s.sh("run -m palette")

	// The example in docs/agent-workflow.md#grid-paint.
	s.in = "....oooooo....\n..oossssssoo..\n.ossSsssssssso\n"
	s.run(`grid 4,2 --legend "s=skin S=skin_dark"`, "grid", "4,2", "--legend", "s=skin S=skin_dark")
	s.sh("show --rulers --legend")

	// . leaves pixels alone, _ erases; --opaque makes . erase too.
	s.sh("grid 6,2 k_.K --json")
	s.sh("grid 4,4 ..k. --opaque --json")
	s.sh("show")
	s.sh("log --limit 3")

	// Errors name the row (and the cell), and write nothing.
	s.in = "oooo\nooo\n"
	s.sh("grid 0,0 --json")
	s.in = "oo\noq\n"
	s.sh("grid 0,0")
	s.sh("grid 0,0 o+ --json")
	s.in = "\n\n"
	s.sh("grid 0,0")
	s.sh("grid 0,0 o --legend .=ink --json")

	// Grid blocks in a run script.
	script := "--- the eyes\n" +
		"grid 7,3 --legend e=#ffffff <<\n" +
		"  e.e\n" +
		"  o.o\n" +
		">>\n" +
		"--- the chin\n" +
		"grid 6,5 <<\n" +
		"KKKKK\n" +
		">>\n"
	if err := os.WriteFile("face.pxs", []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	s.out.WriteString("$ cat face.pxs\n" + script + "\n")
	s.sh("run face.pxs --json")
	s.sh("show")
	s.sh("log --limit 2")
	s.in = "px 0,0 ink\ngrid 0,0 <<\noo\nob\n>>\n"
	s.sh("run --json")
	s.in = "grid 0,0 <<\noo\n"
	s.sh("run")
	s.sh("undo --json")
	s.sh("show")

	golden.Check(t, filepath.Join(testdata, "grid.golden"), []byte(s.out.String()))
}
