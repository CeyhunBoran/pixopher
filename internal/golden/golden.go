// Package golden compares test output with golden files. Run
// `go test ./... -update` to rewrite them. Every package with tests imports
// it, so the -update flag exists in every test binary.
package golden

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Update is set by the -update test flag.
var Update = flag.Bool("update", false, "rewrite golden files")

// Check compares got with the golden file at path, or rewrites the file when
// -update is set.
func Check(t testing.TB, path string, got []byte) {
	t.Helper()
	if *Update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file (run go test -update to accept, then git diff to see it all)\n%s",
			path, Diff(got, want))
	}
}

// Diff limits.
const (
	MaxDiffLines = 8   // differing lines shown
	MaxLineLen   = 160 // characters shown of each line
)

// Diff is a short line-by-line comparison of got with want: the first
// MaxDiffLines lines that differ, as "-" (want) and "+" (got) pairs with
// their line numbers, each cut to MaxLineLen characters, then a count of
// the rest. Lines are compared by position, so an inserted line shows up
// as a run of differences from there on.
func Diff(got, want []byte) string {
	g, w := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	var b strings.Builder
	shown, total := 0, 0
	for i := range max(len(g), len(w)) {
		gl, gok := line(g, i)
		wl, wok := line(w, i)
		if gok == wok && gl == wl {
			continue
		}
		if total++; shown == MaxDiffLines {
			continue
		}
		shown++
		if wok {
			fmt.Fprintf(&b, "%4d - %s\n", i+1, cut(wl))
		}
		if gok {
			fmt.Fprintf(&b, "%4d + %s\n", i+1, cut(gl))
		}
	}
	if total > shown {
		fmt.Fprintf(&b, "... and %d more differing lines\n", total-shown)
	}
	fmt.Fprintf(&b, "got %d lines, want %d", len(g), len(w))
	return b.String()
}

func line(ls []string, i int) (string, bool) {
	if i < len(ls) {
		return ls[i], true
	}
	return "", false
}

func cut(s string) string {
	if r := []rune(s); len(r) > MaxLineLen {
		return string(r[:MaxLineLen]) + fmt.Sprintf("... (%d chars)", len(r))
	}
	return s
}
