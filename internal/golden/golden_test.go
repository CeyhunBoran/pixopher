package golden

import (
	"fmt"
	"strings"
	"testing"
)

func TestDiff(t *testing.T) {
	long := strings.Repeat("x", MaxLineLen+5)
	got := Diff([]byte("a\nB\nc\n"+long+"\n"), []byte("a\nb\nc\nshort\n"))
	want := "   2 - b\n   2 + B\n   4 - short\n   4 + " + strings.Repeat("x", MaxLineLen) +
		fmt.Sprintf("... (%d chars)\n", MaxLineLen+5) + "got 5 lines, want 5"
	if got != want {
		t.Fatalf("Diff =\n%s\nwant\n%s", got, want)
	}

	var g, w strings.Builder
	for i := range MaxDiffLines + 3 {
		fmt.Fprintf(&g, "g%d\n", i)
		fmt.Fprintf(&w, "w%d\n", i)
	}
	w.WriteString("extra\n")
	got = Diff([]byte(g.String()), []byte(w.String()))
	if !strings.Contains(got, "... and 5 more differing lines\n") || strings.Count(got, " + ") != MaxDiffLines {
		t.Fatalf("Diff =\n%s", got)
	}
}
