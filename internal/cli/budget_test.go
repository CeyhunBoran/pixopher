package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CeyhunBoran/pixopher/internal/doc"
	"github.com/CeyhunBoran/pixopher/internal/engine"
	"github.com/CeyhunBoran/pixopher/internal/ops"
	"github.com/CeyhunBoran/pixopher/internal/pxofile"
	"github.com/CeyhunBoran/pixopher/palettes"
)

// Budget is the most a single command may take on a 64x64 document
// (docs/architecture.md, M1 in docs/roadmap.md).
const Budget = 15 * time.Millisecond

// budgetHistory is how many transactions the budget document's log holds
// before timing starts, so undo reads an honest log.
const budgetHistory = 300

// budgetCommands are the commands of the M1 done-when flow, as timed by
// TestBudget64. Drawing alternates between two colors so every run changes
// pixels; undo and redo take back and bring back the last fill.
var budgetCommands = []struct {
	name string
	args func(i int) string
}{
	{"px", func(i int) string { return "px 5,5 " + budgetColor(i) + " 40,9 " + budgetColor(i+1) }},
	{"line", func(i int) string { return "line 0,0 63,40 " + budgetColor(i) + " --width 2" }},
	{"rect", func(i int) string { return "rect 4,4 30,30 " + budgetColor(i) }},
	{"fill", func(i int) string { return "fill 0,63 " + budgetColor(i) + " --contiguous=false" }},
	{"undo", func(int) string { return "undo" }},
	{"redo", func(int) string { return "redo" }},
	{"show", func(int) string { return "show --grid --rulers --legend" }},
	{"stats", func(int) string { return "stats" }},
	{"export", func(int) string { return "export budget.png --scale 8" }},
}

func budgetColor(i int) string { return [...]string{"ink", "red"}[i%2] }

// budgetDoc writes a 64x64 document with the default palette and every
// pixel set, in a pattern that defeats run-length encoding, plus
// budgetHistory transactions of history. It returns its path.
func budgetDoc(tb testing.TB) string {
	tb.Helper()
	d, err := doc.New(64, 64)
	if err != nil {
		tb.Fatal(err)
	}
	pal, _ := palettes.Load(palettes.Default)
	d.Palette, d.Ramps = pal.Palette, pal.Ramps
	n := d.Palette.Len() - 1
	for y := range 64 {
		for x := range 64 {
			d.SetPixel(0, 0, x, y, doc.Index(1+(x*7+y*13+x*y)%n))
		}
	}
	if err := d.Validate(); err != nil {
		tb.Fatal(err)
	}
	data, err := pxofile.Serialize(d)
	if err != nil {
		tb.Fatal(err)
	}
	path := filepath.Join(tb.TempDir(), "budget.pxo")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		tb.Fatal(err)
	}
	for i := range budgetHistory {
		budgetRun(tb, path, fmt.Sprintf("px %d,%d %s", i%64, i/64, budgetColor(i)))
	}
	return path
}

// budgetRun runs one command in-process on path, as the binary would: the
// command tree is built, the file is locked, loaded, changed, written and
// logged. Only process start is left out.
func budgetRun(tb testing.TB, path, line string) {
	tb.Helper()
	cfg := config{
		ops: ops.Default, backend: engine.Direct{},
		getenv: func(string) string { return "" },
		now:    time.Now,
	}
	args := append(strings.Fields(line), "-f", path)
	if code := execute(BuildInfo{}, cfg, args, io.Discard, io.Discard); code != 0 {
		tb.Fatalf("pxo %s: exit %d", line, code)
	}
}

// TestBudget64 checks that each command of the M1 flow takes under Budget
// on a 64x64 document (D61). Timing depends on the machine, so it runs only
// with PIXOPHER_BUDGET=1 and without the race detector; CI runs it in a
// step of its own on Linux.
func TestBudget64(t *testing.T) {
	if os.Getenv("PIXOPHER_BUDGET") != "1" || raceEnabled {
		t.Skip("set PIXOPHER_BUDGET=1 and run without -race")
	}
	path := budgetDoc(t)
	t.Chdir(filepath.Dir(path))
	const runs = 21
	times := make([][]time.Duration, len(budgetCommands))
	for i := range runs {
		for j, c := range budgetCommands {
			start := time.Now()
			budgetRun(t, path, c.args(i))
			times[j] = append(times[j], time.Since(start))
		}
	}
	for j, c := range budgetCommands {
		slices.Sort(times[j])
		med := times[j][runs/2]
		t.Logf("%-7s median %v  max %v", c.name, med.Round(10*time.Microsecond), times[j][runs-1].Round(10*time.Microsecond))
		if med > Budget {
			t.Errorf("%s: median %v is over the %v budget", c.name, med, Budget)
		}
	}
}

// BenchmarkCommand64 times each command of the M1 flow on a 64x64
// document: go test -bench Command64 -run '^$' ./internal/cli
func BenchmarkCommand64(b *testing.B) {
	path := budgetDoc(b)
	b.Chdir(filepath.Dir(path))
	for _, c := range budgetCommands {
		name := c.name
		switch name {
		case "redo":
			continue // timed with undo
		case "undo":
			name = "undo-redo" // alternating, so each has something to do
		}
		b.Run(name, func(b *testing.B) {
			i := 0
			for b.Loop() {
				line := c.args(i)
				if c.name == "undo" && i%2 == 1 {
					line = "redo"
				}
				budgetRun(b, path, line)
				i++
			}
		})
	}
}
