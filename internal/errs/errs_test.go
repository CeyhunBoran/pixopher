package errs

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	_ "github.com/CeyhunBoran/pixopher/internal/golden" // registers -update
)

func TestExitCode(t *testing.T) {
	for c, want := range map[Code]int{
		Usage: 1, ParseError: 2, InvalidColor: 2, UnknownColor: 2, OutOfBounds: 2, TooLarge: 2, AlreadyExists: 2, InUse: 2,
		RevConflict: 3, Locked: 3, UndoConflict: 3, IO: 4, NoFile: 4, ServerUnavailable: 4,
	} {
		if got := ExitCode(c); got != want {
			t.Errorf("ExitCode(%s) = %d, want %d", c, got, want)
		}
	}
}

func TestCodeOf(t *testing.T) {
	e := New(InvalidColor, "bad %q", "x").WithHint("try %s", "y").WithArg("color")
	if e.Error() != `bad "x"` || e.Hint != "try y" || e.Arg != "color" {
		t.Fatalf("got %+v", e)
	}
	if CodeOf(fmt.Errorf("wrapped: %w", e)) != InvalidColor {
		t.Fatal("CodeOf does not unwrap")
	}
	if CodeOf(fmt.Errorf("plain")) != "" {
		t.Fatal("plain errors have no code")
	}
}

func TestJSON(t *testing.T) {
	full := New(OutOfBounds, "x=40 is outside the canvas").WithHint("canvas is 32x32; valid x is 0..31").WithArg("x")
	b, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"code":"out_of_bounds","message":"x=40 is outside the canvas","hint":"canvas is 32x32; valid x is 0..31","arg":"x"}`
	if string(b) != want {
		t.Fatalf("got %s, want %s", b, want)
	}
	b, _ = json.Marshal(New(Locked, "busy"))
	if string(b) != `{"code":"locked","message":"busy"}` {
		t.Fatalf("empty hint and arg are not omitted: %s", b)
	}
}

// section returns the body of the "### <title>" section of docs/cli.md.
func section(t *testing.T, title string) string {
	t.Helper()
	data, err := os.ReadFile("../../docs/cli.md")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(data), "\n### "+title+"\n")
	if !ok {
		t.Fatalf("docs/cli.md has no %q section", title)
	}
	body, _, _ = strings.Cut(body, "\n#")
	return body
}

var codeRe = regexp.MustCompile("`([a-z_]+)`")

// TestCodesMatchDocs: the error code list in docs/cli.md is exactly All(),
// in the same order.
func TestCodesMatchDocs(t *testing.T) {
	list, _, _ := strings.Cut(section(t, "Error codes [done]"), "\n\n")
	// Parentheses explain a code and may quote other words, such as line.
	list = regexp.MustCompile(`\([^)]*\)`).ReplaceAllString(list, "")
	var doc []Code
	for _, m := range codeRe.FindAllStringSubmatch(list, -1) {
		doc = append(doc, Code(m[1]))
	}
	if !slices.Equal(doc, All()) {
		t.Fatalf("docs/cli.md lists %v\nerrs.All() is %v", doc, All())
	}
}

// TestExitCodesMatchDocs: every code sits on the row of the exit table in
// docs/cli.md that ExitCode maps it to; a code on no row is a domain error.
func TestExitCodesMatchDocs(t *testing.T) {
	want := map[Code]int{}
	for line := range strings.Lines(section(t, "Exit codes [done]")) {
		cells := strings.Split(line, "|")
		if len(cells) < 3 {
			continue
		}
		exit, err := strconv.Atoi(strings.TrimSpace(cells[1]))
		if err != nil {
			continue
		}
		for _, m := range codeRe.FindAllStringSubmatch(cells[2], -1) {
			if c := Code(m[1]); slices.Contains(All(), c) {
				want[c] = exit
			}
		}
	}
	if len(want) == 0 {
		t.Fatal("no codes found in the exit table")
	}
	for _, c := range All() {
		w, ok := want[c]
		if !ok {
			w = ExitDomain
		}
		if got := ExitCode(c); got != w {
			t.Errorf("ExitCode(%s) = %d, docs/cli.md says %d", c, got, w)
		}
	}
}
