package errs

import (
	"fmt"
	"testing"

	_ "github.com/CeyhunBoran/pixopher/internal/golden" // registers -update
)

func TestExitCode(t *testing.T) {
	for c, want := range map[Code]int{
		Usage: 1, ParseError: 2, InvalidColor: 2, UnknownColor: 2, OutOfBounds: 2, TooLarge: 2,
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
